package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"common/viewer"
)

// 原文片段：原始记录（速记、粘贴的聊天、截图说明、对话）切成小段算向量，作为记忆检索的兜底。
// 整理成记忆时漏掉的话、只在原文里提过一句的细节（「表妹 11 月 22 号在杭州结婚」），也能搜到。
//
// 原始记录归 intake 管，这里只读 pa_source_note（只增不改，读不会和它打架）。
// 片段向量和记忆向量放在同一个向量存储里，存储键是「记忆的键 + -notes」（embo-01@t2-notes）。

const (
	passageRunes   = 300 // 一段最多多少字
	passageOverlap = 60  // 相邻两段重叠多少字，免得一句话被切在两段中间都搜不到
	passageBatch   = 16
)

// Passage 是一段原文。Score 是和问题的向量相似度。
type Passage struct {
	NoteKey    string     `json:"noteKey"`
	Index      int        `json:"index"`
	Channel    string     `json:"channel"`
	Text       string     `json:"text"`
	OccurredAt *time.Time `json:"occurredAt"`
	Score      float64    `json:"score"`
}

type noteSource struct {
	NoteKey    string
	Channel    string
	RawText    string
	OccurredAt *time.Time
	CreatedAt  time.Time
}

func (s *Service) passageVectorKey(ctx context.Context) string {
	key := s.vectorKey(ctx)
	if key == "" {
		return ""
	}
	return key + "-notes"
}

func passageKey(noteKey string, index int) string { return fmt.Sprintf("%s#%d", noteKey, index) }

// notePassages 把原文切段。对话整理出来的原始记录开头是一段给抽取模型看的说明，去掉。
func notePassages(channel, text string) []string {
	text = strings.TrimSpace(text)
	if channel == "chat" {
		if index := strings.Index(text, "\n\n"); index >= 0 && strings.HasPrefix(text, "以下是") {
			text = strings.TrimSpace(text[index+2:])
		}
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= passageRunes {
		return []string{text}
	}
	var passages []string
	for start := 0; start < len(runes); start += passageRunes - passageOverlap {
		end := min(start+passageRunes, len(runes))
		passages = append(passages, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return passages
}

func (note noteSource) when() time.Time {
	if note.OccurredAt != nil {
		return *note.OccurredAt
	}
	return note.CreatedAt
}

func (s *Service) loadNotes(ctx context.Context, familyKey string, noteKeys []string) ([]noteSource, error) {
	// 可见范围由 viewer 插件自动加
	query := s.db.WithContext(ctx).Table("pa_source_note").
		Select("note_key", "channel", "raw_text", "occurred_at", "created_at").
		Where("family_key = ? AND raw_text <> ''", familyKey)
	if noteKeys != nil {
		query = query.Where("note_key IN ?", noteKeys)
	}
	var notes []noteSource
	err := query.Order("id").Find(&notes).Error
	return notes, err
}

// ReindexPassages 给还没有向量的原文片段补算，返回补算段数。启动时后台跑一次，新记录整理完时调 IndexNote。
func (s *Service) ReindexPassages(ctx context.Context, familyKey string) (int, error) {
	return s.indexNotes(ctx, familyKey, nil)
}

// IndexNote 给一条原始记录算片段向量（已经算过的跳过）。
func (s *Service) IndexNote(ctx context.Context, familyKey, noteKey string) error {
	_, err := s.indexNotes(ctx, familyKey, []string{noteKey})
	return err
}

func (s *Service) indexNotes(ctx context.Context, familyKey string, noteKeys []string) (int, error) {
	// 算向量不看是谁的：每个人的记录都要能被自己搜到，检索时再按可见范围过滤
	ctx = viewer.Unscoped(ctx)
	model := s.passageVectorKey(ctx)
	if model == "" {
		return 0, nil
	}
	if err := s.passages.ensure(ctx, s.vectors, familyKey, model, true); err != nil {
		return 0, err
	}
	has := s.passages.has(familyKey, model)
	notes, err := s.loadNotes(ctx, familyKey, noteKeys)
	if err != nil {
		return 0, err
	}
	type pending struct{ key, text string }
	var missing []pending
	for _, note := range notes {
		for index, text := range notePassages(note.Channel, note.RawText) {
			if key := passageKey(note.NoteKey, index); !has[key] {
				// 带上来源和日期：问「上个月说的……」时也能沾上边
				missing = append(missing, pending{key, fmt.Sprintf("[%s %s] %s", channelText(note.Channel), note.when().Format("2006年1月2日"), text)})
			}
		}
	}
	total := 0
	for start := 0; start < len(missing); start += passageBatch {
		batch := missing[start:min(start+passageBatch, len(missing))]
		texts := make([]string, len(batch))
		for i, item := range batch {
			texts[i] = item.text
		}
		vectors, _, err := s.embedder.Embed(ctx, texts)
		if err != nil {
			return total, err
		}
		write := make(map[string][]float32, len(batch))
		for i, item := range batch {
			write[item.key] = vectors[i]
		}
		if err := s.vectors.Write(ctx, familyKey, model, write); err != nil {
			return total, err
		}
		for key, vector := range write {
			s.passages.upsert(familyKey, model, key, vector)
		}
		total += len(batch)
	}
	return total, nil
}

// SearchPassages 按向量相似度找原文片段，只返回不低于 minScore 的，同一条记录只取最相关的一段。
func (s *Service) SearchPassages(ctx context.Context, familyKey, text string, limit int, minScore float64) ([]Passage, error) {
	model := s.passageVectorKey(ctx)
	if model == "" || strings.TrimSpace(text) == "" {
		return nil, nil
	}
	if err := s.passages.ensure(ctx, s.vectors, familyKey, model, false); err != nil {
		return nil, err
	}
	vector, err := s.queryVector(ctx, text)
	if err != nil {
		return nil, err
	}
	bestByNote := map[string]vectorHit{}
	var noteKeys []string
	allow, err := s.visibleNotes(ctx, familyKey)
	if err != nil {
		return nil, err
	}
	for _, hit := range s.passages.search(familyKey, vector, limit*4, allow) {
		if hit.Score < minScore {
			break
		}
		noteKey, _, _ := strings.Cut(hit.MemoryKey, "#")
		if _, ok := bestByNote[noteKey]; !ok {
			bestByNote[noteKey] = hit
			noteKeys = append(noteKeys, noteKey)
		}
		if len(noteKeys) >= limit {
			break
		}
	}
	if len(noteKeys) == 0 {
		return nil, nil
	}
	notes, err := s.loadNotes(ctx, familyKey, noteKeys)
	if err != nil {
		return nil, err
	}
	var result []Passage
	for _, note := range notes {
		hit := bestByNote[note.NoteKey]
		var index int
		fmt.Sscanf(hit.MemoryKey[len(note.NoteKey)+1:], "%d", &index)
		passages := notePassages(note.Channel, note.RawText)
		if index >= len(passages) {
			continue // 原文和向量对不上（不该发生：原始记录只增不改）
		}
		when := note.when()
		result = append(result, Passage{NoteKey: note.NoteKey, Index: index, Channel: note.Channel, Text: passages[index], OccurredAt: &when, Score: hit.Score})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Score > result[j].Score })
	return result, nil
}

func channelText(channel string) string {
	switch channel {
	case "quick_note":
		return "速记"
	case "paste":
		return "粘贴的聊天"
	case "screenshot":
		return "截图说明"
	case "capture":
		return "随手录"
	case "chat":
		return "对话"
	case "import":
		return "导入的聊天记录"
	}
	return "记录"
}

// ChannelText 是原始记录来源的中文名。
func ChannelText(channel string) string { return channelText(channel) }

// queryVector 给检索的问题算向量。同一句话在一轮里会被记忆检索和原文检索各用一次，缓存一分钟省一次调用。
func (s *Service) queryVector(ctx context.Context, text string) ([]float32, error) {
	key := s.vectorKey(ctx) + "\x00" + text
	s.queryMu.Lock()
	if cached, ok := s.queryCache[key]; ok && time.Since(cached.at) < time.Minute {
		s.queryMu.Unlock()
		return cached.vector, nil
	}
	s.queryMu.Unlock()
	vector, _, err := s.embedder.EmbedQuery(ctx, text)
	if err != nil {
		return nil, err
	}
	s.queryMu.Lock()
	defer s.queryMu.Unlock()
	if len(s.queryCache) > 256 {
		s.queryCache = map[string]cachedQuery{}
	}
	s.queryCache[key] = cachedQuery{vector: vector, at: time.Now()}
	return vector, nil
}

type cachedQuery struct {
	vector []float32
	at     time.Time
}

// visibleNotes 返回只放行当前账号看得到的原始记录片段（键是 note_xxx#序号）；不过滤时返回 nil。
func (s *Service) visibleNotes(ctx context.Context, familyKey string) (func(string) bool, error) {
	if _, unscoped, _ := viewer.Accounts(ctx); unscoped {
		return nil, nil
	}
	var keys []string
	if err := s.db.WithContext(ctx).Table("pa_source_note").Where("family_key = ?", familyKey).
		Pluck("note_key", &keys).Error; err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	return func(key string) bool {
		noteKey, _, _ := strings.Cut(key, "#")
		return allowed[noteKey]
	}, nil
}

package memory

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"fmt"
)

// vectorIndex 是进程内的暴力余弦检索。
//
// 一个家庭的记忆是千到万条量级：1 万条 × 1024 维 ≈ 40MB，全量点积 < 10ms，
// 而且结果是精确的。等哪天量级上去了，再把这一层换成 Qdrant / pgvector，
// Search 的调用方不用改。向量入库前已归一化，余弦相似度 = 点积。
type vectorIndex struct {
	mu       sync.RWMutex
	families map[string]*familyVectors
}

type familyVectors struct {
	model   string
	vectors map[string][]float32
	// cursor 是向量存储给的读取进度；syncedAt 是上次同步的时间。两个 API 进程各有一份内存索引，
	// 对方新算的向量（或换模型后后台补算的）靠定期增量读取才能看到。
	cursor   any
	syncedAt time.Time
}

// vectorSyncInterval：同一份索引最多每隔这么久去存储里增量读一次。
const vectorSyncInterval = 30 * time.Second

type vectorHit struct {
	MemoryKey string
	Score     float64
}

func newVectorIndex() *vectorIndex {
	return &vectorIndex{families: map[string]*familyVectors{}}
}

// ensure 保证某个家庭在指定模型下的向量已加载：模型变了就整体重载，
// 否则每隔 vectorSyncInterval 增量读一次别的进程新写的向量。force 跳过时间间隔（补算前用）。
func (index *vectorIndex) ensure(ctx context.Context, store VectorStore, familyKey, model string, force bool) error {
	now := time.Now()
	index.mu.RLock()
	current := index.families[familyKey]
	sameModel := current != nil && current.model == model
	fresh := sameModel && !force && now.Sub(current.syncedAt) < vectorSyncInterval
	var cursor any
	if sameModel {
		cursor = current.cursor
	}
	index.mu.RUnlock()
	if fresh {
		return nil
	}
	vectors, next, full, err := store.Read(ctx, familyKey, model, cursor)
	if err != nil {
		return err
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	target := index.families[familyKey]
	if full || target == nil || target.model != model {
		target = &familyVectors{model: model, vectors: make(map[string][]float32, len(vectors))}
		index.families[familyKey] = target
	}
	for memoryKey, vector := range vectors {
		target.vectors[memoryKey] = vector
	}
	target.cursor, target.syncedAt = next, now
	return nil
}

// has 返回已有向量的记忆编号（补算时用来找缺的）。
func (index *vectorIndex) has(familyKey, model string) map[string]bool {
	index.mu.RLock()
	defer index.mu.RUnlock()
	result := map[string]bool{}
	if current := index.families[familyKey]; current != nil && current.model == model {
		for memoryKey := range current.vectors {
			result[memoryKey] = true
		}
	}
	return result
}

func (index *vectorIndex) upsert(familyKey, model, memoryKey string, vector []float32) {
	index.mu.Lock()
	defer index.mu.Unlock()
	current := index.families[familyKey]
	if current == nil || current.model != model {
		return // 还没加载过，下次 ensure 会从库里读到
	}
	current.vectors[memoryKey] = vector
}

func (index *vectorIndex) search(familyKey string, query []float32, topK int) []vectorHit {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current := index.families[familyKey]
	if current == nil {
		return nil
	}
	hits := make([]vectorHit, 0, len(current.vectors))
	for memoryKey, vector := range current.vectors {
		if len(vector) != len(query) {
			continue
		}
		var dot float64
		for i := range vector {
			dot += float64(vector[i]) * float64(query[i])
		}
		hits = append(hits, vectorHit{MemoryKey: memoryKey, Score: dot})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}

// embeddingTextVersion 是「算向量的文字」的格式版本，和向量模型一起作为向量的存储键（embo-01@t2）：
// 改了 embeddingText 的格式就加一，启动时会自动按新格式全量重算，新旧向量不会混用。
const embeddingTextVersion = "t2"

// vectorKey 是当前向量的存储键：向量模型 + 文字格式版本；没配向量化时为空。
func (s *Service) vectorKey(ctx context.Context) string {
	if s.embedder == nil {
		return ""
	}
	model := s.embedder.EmbeddingModel(ctx)
	if model == "" {
		return ""
	}
	return model + "@" + embeddingTextVersion
}

// embeddingText 是拿去算向量的文字：标题和正文之外，带上类型、关于谁、说话人、位置、用途、时间。
// 问「妈说过……」「放在书房的……」「九月份修的……」时，这些信息也能参与语义匹配。
func embeddingText(row memoryRow, labels map[string]string) string {
	label := func(key string) string {
		if name := labels[key]; name != "" {
			return name
		}
		return key
	}
	var builder strings.Builder
	builder.WriteString("[" + TypeLabel(row.Type) + "] " + row.Title)
	if row.Content != row.Title {
		builder.WriteString("\n" + row.Content)
	}
	if row.SubjectPerson != nil && *row.SubjectPerson != "" {
		builder.WriteString("\n关于：" + label(*row.SubjectPerson))
	}
	var attrs struct {
		Location   string `json:"location"`
		Usage      string `json:"usage"`
		Speaker    string `json:"speaker"`
		PromisedTo string `json:"promisedTo"`
	}
	_ = json.Unmarshal(row.Attrs, &attrs)
	if attrs.Speaker != "" {
		builder.WriteString("\n说话人：" + label(attrs.Speaker))
	}
	if attrs.PromisedTo != "" {
		builder.WriteString("\n答应了：" + label(attrs.PromisedTo))
	}
	if attrs.Location != "" {
		builder.WriteString("\n存放位置：" + attrs.Location)
	}
	if attrs.Usage != "" {
		builder.WriteString("\n用途：" + attrs.Usage)
	}
	if row.OccurredAt != nil {
		builder.WriteString("\n时间：" + row.OccurredAt.Format("2006年1月2日"))
	}
	return builder.String()
}

// embedAsync 在后台给一条记忆生成向量。失败不影响写入，ReindexMissing 会补。
func (s *Service) embedAsync(familyKey string, row memoryRow) {
	if s.embedder == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.embedRows(ctx, familyKey, []memoryRow{row}); err != nil {
			log.Printf("记忆 %s 向量化失败（稍后补算）: %v", row.MemoryKey, err)
		}
	}()
}

func (s *Service) embedRows(ctx context.Context, familyKey string, rows []memoryRow) error {
	var labels map[string]string
	if s.labels != nil {
		labels = s.labels(ctx, familyKey)
	}
	texts := make([]string, len(rows))
	for i, row := range rows {
		texts[i] = embeddingText(row, labels)
	}
	model := s.vectorKey(ctx)
	vectors, _, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	batch := make(map[string][]float32, len(rows))
	for i, row := range rows {
		batch[row.MemoryKey] = vectors[i]
	}
	if err := s.vectors.Write(ctx, familyKey, model, batch); err != nil {
		return err
	}
	for memoryKey, vector := range batch {
		s.index.upsert(familyKey, model, memoryKey, vector)
	}
	return nil
}

// ReindexMissing 给当前 embedding 模型下还没有向量的记忆补算，返回补算条数。
// 进程启动时后台跑一次；管理端换了 embedding 路由后自动再跑。
func (s *Service) ReindexMissing(ctx context.Context, familyKey string) (int, error) {
	if s.embedder == nil {
		return 0, nil
	}
	model := s.vectorKey(ctx)
	if model == "" {
		return 0, nil
	}
	if err := s.index.ensure(ctx, s.vectors, familyKey, model, true); err != nil {
		return 0, err
	}
	has := s.index.has(familyKey, model)
	var rows []memoryRow
	if err := s.db.WithContext(ctx).
		Where("family_key = ? AND status IN ?", familyKey, []string{StatusActive, StatusPending, StatusSuperseded}).
		Order("id").Find(&rows).Error; err != nil {
		return 0, err
	}
	var missing []memoryRow
	for _, row := range rows {
		if !has[row.MemoryKey] {
			missing = append(missing, row)
		}
	}
	total := 0
	for start := 0; start < len(missing); start += 16 {
		batch := missing[start:min(start+16, len(missing))]
		if err := s.embedRows(ctx, familyKey, batch); err != nil {
			return total, err
		}
		total += len(batch)
	}
	return total, nil
}

// PrepareVectors 在启动时整理向量存储：本地文件截掉崩溃留下的半条、压缩重复记录；
// 第一次从 MySQL 切到本地文件时，把 pa_memory_embedding 里已有的向量导进来，不用重新调向量化接口。
func (s *Service) PrepareVectors(ctx context.Context, familyKey string) error {
	if s.embedder == nil {
		return nil
	}
	model := s.vectorKey(ctx)
	if model == "" {
		return nil
	}
	file, ok := s.vectors.(FileVectorStore)
	if !ok {
		return nil
	}
	if err := file.Maintain(familyKey, model); err != nil {
		return fmt.Errorf("整理向量文件失败: %w", err)
	}
	empty, err := file.Empty(ctx, familyKey, model)
	if err != nil || !empty {
		return err
	}
	legacy, _, _, err := MySQLVectorStore{DB: s.db}.Read(ctx, familyKey, model, nil)
	if err != nil || len(legacy) == 0 {
		return err
	}
	if err := file.Write(ctx, familyKey, model, legacy); err != nil {
		return err
	}
	log.Printf("已把 MySQL 里 %d 条 %s 向量导入本地文件 %s", len(legacy), model, file.path(familyKey, model))
	return nil
}

func encodeVector(vector []float32) []byte {
	data := make([]byte, 4*len(vector))
	for i, value := range vector {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	return data
}

func decodeVector(data []byte) []float32 {
	vector := make([]float32, len(data)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vector
}

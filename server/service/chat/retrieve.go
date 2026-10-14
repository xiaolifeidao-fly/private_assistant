package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"service/bot"
	"service/llm"
	"service/memory"
)

// taskQuery：追问时把这句话改写成能独立检索的说法（「那户口本呢」→「户口本放在哪」）。
// taskRerank：从宽一点捞出的候选里挑出真正相关的并排序。
const (
	taskQuery  = "chat.query"
	taskRerank = "chat.rerank"
)

const (
	recentLineLimit = 6
	rewriteTimeout  = 20 * time.Second
	maxQueries      = 3

	// 原文片段兜底：每轮最多带几段、向量相似度至少多少（没有重排时用）
	maxPassages     = 3
	passageMinScore = 0.35

	// 重排：先不套门槛捞这么多候选（记忆、原文），交给模型挑
	rerankCandidates = 20
	rerankPassages   = 5
	rerankPassageMin = 0.25
)

// retrieval 是一轮自动检索的结果：记忆，以及原始记录里的原文片段（整理时可能漏掉的细节）。
type retrieval struct {
	hits     []memory.Hit
	passages []memory.Passage
}

// retrieve 是每轮自动检索（L2）：在模型开口前，按这句话找出可能相关的记忆放进 <turn_context>。
// 模型自己调 search_memory（L3）走的是同一个 memory.Search，这里额外负责理解这一轮的问题：
// 有上文时先改写成独立的检索词（可能拆成几个说法），分别检索再合并。
func (s *Service) retrieve(ctx context.Context, familyKey string, item bot.Bot, dir directory, text string, history []string, limit int) (retrieval, error) {
	queries := []string{text}
	if rewritten := s.rewriteQueries(ctx, text, history); len(rewritten) > 0 {
		queries = rewritten
	}
	// 配了重排就先宽一点捞候选（不套门槛），交给模型挑；没配或重排失败就用分数门槛
	merged, err := s.searchAll(ctx, familyKey, item, dir, text, queries, rerankCandidates, false)
	if err != nil {
		return retrieval{}, err
	}
	var passages []memory.Passage
	// 原文里可能有和分身本人无关的人和事，分身只看整理过、按人隔离的记忆
	if item.MemoryScope == bot.ScopeAll {
		passages = s.searchPassages(ctx, familyKey, queries, rerankPassageMin, rerankPassages)
	}
	if picked, ok := s.rerank(ctx, dir, text, history, merged, passages, limit); ok {
		return picked, nil
	}
	hits := memory.Focus(merged)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	var focused []memory.Passage
	for _, passage := range passages {
		if passage.Score >= passageMinScore && len(focused) < maxPassages {
			focused = append(focused, passage)
		}
	}
	return retrieval{hits: hits, passages: focused}, nil
}

// searchAll 几个检索词并行查，合并去重，按分数排。
func (s *Service) searchAll(ctx context.Context, familyKey string, item bot.Bot, dir directory, text string, queries []string, limit int, focused bool) ([]memory.Hit, error) {
	results := make([][]memory.Hit, len(queries))
	errs := make([]error, len(queries))
	var wait sync.WaitGroup
	for index, queryText := range queries {
		wait.Add(1)
		go func(index int, queryText string) {
			defer wait.Done()
			query := memory.SearchQuery{Text: queryText, Persons: dir.mentioned(queryText + " " + text), Limit: limit, Focused: focused}
			if item.MemoryScope == bot.ScopePerson && item.PersonKey != "" {
				query.VisibleTo = item.PersonKey
			}
			results[index], errs[index] = s.memory.Search(ctx, familyKey, query)
		}(index, queryText)
	}
	wait.Wait()
	best := map[string]memory.Hit{}
	var firstErr error
	for index, hits := range results {
		if errs[index] != nil && firstErr == nil {
			firstErr = errs[index]
		}
		for _, hit := range hits {
			if old, ok := best[hit.Memory.MemoryKey]; !ok || hit.Score > old.Score {
				best[hit.Memory.MemoryKey] = hit
			}
		}
	}
	if len(best) == 0 && firstErr != nil {
		return nil, firstErr
	}
	merged := make([]memory.Hit, 0, len(best))
	for _, hit := range best {
		merged = append(merged, hit)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// rerank 让模型从候选里挑出真正相关的排在前面（路由 chat.rerank），再接上分数门槛放行的。
// 没配路由、调用失败时返回 false，只用分数门槛。
func (s *Service) rerank(ctx context.Context, dir directory, text string, history []string, hits []memory.Hit, passages []memory.Passage, limit int) (retrieval, bool) {
	if len(hits) == 0 && len(passages) == 0 {
		return retrieval{}, true
	}
	var builder strings.Builder
	if len(history) > 0 {
		builder.WriteString("最近的对话：\n" + strings.Join(history, "\n") + "\n\n")
	}
	builder.WriteString("我现在问：" + text + "\n\n候选：\n")
	for index, hit := range hits {
		fmt.Fprintf(&builder, "%d. %s\n", index+1, oneLine(formatMemory(dir, hit.Memory), 220))
	}
	for index, passage := range passages {
		fmt.Fprintf(&builder, "%d. [原文 | %s | %s] %s\n", len(hits)+index+1, memory.ChannelText(passage.Channel), passage.OccurredAt.Format("2006-01-02"), oneLine(passage.Text, 220))
	}
	ctx, cancel := context.WithTimeout(ctx, rewriteTimeout)
	defer cancel()
	response, err := s.llm.Chat(ctx, llm.Request{
		Task: taskRerank,
		System: []llm.SystemPart{{Text: `你在帮一个家庭记忆库挑检索结果。给你我的问题和一批候选记忆（编号开头），挑出能回答这个问题、或者回答时确实用得上的，按有用程度从高到低输出编号。
- 叫法不同但说的是同一样东西、同一件事，要挑（比如 洗衣液 = 洗涤剂，插线板 = 排插，退烧贴 = 冰宝贴）。
- 只挑真正相关的：同一类东西但不是问的那个（问吸尘器，候选是电钻），不要挑。
- 问「放哪了」优先挑那件东西本身；问「说过什么」优先挑原话；问「什么时候/多少钱」优先挑那件事。
- 都不相关就输出空数组。不要编号以外的任何内容。`, Cache: true}},
		Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: builder.String()}}}},
		JSONSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"relevant": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}},
			"required":   []string{"relevant"},
		},
	}, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "没有配置任务") {
			log.Printf("重排检索结果失败，改用分数门槛: %v", err)
		}
		return retrieval{}, false
	}
	var parsed struct {
		Relevant []int `json:"relevant"`
	}
	if json.Unmarshal([]byte(llm.JSONText(response.Message.Text())), &parsed) != nil {
		return retrieval{}, false
	}
	var result retrieval
	seen := map[int]bool{}
	for _, number := range parsed.Relevant {
		index := number - 1
		if seen[index] || index < 0 {
			continue
		}
		seen[index] = true
		switch {
		case index < len(hits) && len(result.hits) < limit:
			result.hits = append(result.hits, hits[index])
		case index >= len(hits) && index < len(hits)+len(passages) && len(result.passages) < maxPassages:
			result.passages = append(result.passages, passages[index-len(hits)])
		}
	}
	// 重排只负责往前排、往回捞，不负责删：它偶尔会把明摆着的答案漏掉（同样的输入，有时返回空列表），
	// 所以分数门槛放行的照样补在后面。最差也和只用门槛一样。
	picked := map[string]bool{}
	for _, hit := range result.hits {
		picked[hit.Memory.MemoryKey] = true
	}
	for _, hit := range memory.Focus(hits) {
		if !picked[hit.Memory.MemoryKey] && len(result.hits) < limit {
			result.hits = append(result.hits, hit)
		}
	}
	pickedNotes := map[string]bool{}
	for _, passage := range result.passages {
		pickedNotes[passage.NoteKey] = true
	}
	for _, passage := range passages {
		if !pickedNotes[passage.NoteKey] && passage.Score >= passageMinScore && len(result.passages) < maxPassages {
			result.passages = append(result.passages, passage)
		}
	}
	return result, true
}

// searchPassages 每个检索词各找几段原文，同一条记录只留最相关的一段。
func (s *Service) searchPassages(ctx context.Context, familyKey string, queries []string, minScore float64, limit int) []memory.Passage {
	best := map[string]memory.Passage{}
	for _, queryText := range queries {
		passages, err := s.memory.SearchPassages(ctx, familyKey, queryText, limit, minScore)
		if err != nil {
			log.Printf("原文检索失败: %v", err)
			continue
		}
		for _, passage := range passages {
			if old, ok := best[passage.NoteKey]; !ok || passage.Score > old.Score {
				best[passage.NoteKey] = passage
			}
		}
	}
	passages := make([]memory.Passage, 0, len(best))
	for _, passage := range best {
		passages = append(passages, passage)
	}
	sort.Slice(passages, func(i, j int) bool { return passages[i].Score > passages[j].Score })
	if len(passages) > limit {
		passages = passages[:limit]
	}
	return passages
}

// rewriteQueries 有上文时把这句话改写成独立的检索词；没有上文、没配路由或调用失败时返回 nil（就用原话检索）。
func (s *Service) rewriteQueries(ctx context.Context, text string, history []string) []string {
	if len(history) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, rewriteTimeout)
	defer cancel()
	response, err := s.llm.Chat(ctx, llm.Request{
		Task: taskQuery,
		System: []llm.SystemPart{{Text: `你在帮一个家庭记忆库做检索。给你最近几句对话和我最新说的一句话，把最新这句改写成 1 到 3 个可以独立检索的说法：
- 补全指代和省略：「那户口本呢」在问护照放哪之后 → 「户口本放在哪」；「她吃的药」→ 写出「她」是谁。
- 保留原话里的关键词，可以再给一个换了说法的版本（同义词、用途、类别），但不要编造对话里没有的事实。
- 如果最新这句和上文无关，就照原样输出一个。`, Cache: true}},
		Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: "最近的对话：\n" + strings.Join(history, "\n") + "\n\n我最新说的：" + text}}}},
		JSONSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			"required":   []string{"queries"},
		},
	}, nil)
	if err != nil {
		log.Printf("改写检索词失败，用原话检索: %v", err)
		return nil
	}
	var parsed struct {
		Queries []string `json:"queries"`
	}
	if json.Unmarshal([]byte(llm.JSONText(response.Message.Text())), &parsed) != nil {
		return nil
	}
	var queries []string
	for _, query := range parsed.Queries {
		if query = strings.TrimSpace(query); query != "" && len(queries) < maxQueries {
			queries = append(queries, query)
		}
	}
	return queries
}

// recentLines 取这一轮之前最近几句对话（只要正文，不要工具调用），给改写检索词用。
func (s *Service) recentLines(ctx context.Context, conversationKey string, segment int) []string {
	var rows []messageRow
	s.db.WithContext(ctx).Select("role", "content").
		Where("conversation_key = ? AND segment = ? AND status = 'done' AND role IN ? AND content <> ''", conversationKey, segment, []string{llm.RoleUser, llm.RoleAssistant}).
		Order("id DESC").Limit(recentLineLimit).Find(&rows)
	lines := make([]string, 0, len(rows))
	for index := len(rows) - 1; index >= 0; index-- {
		speaker := "我："
		if rows[index].Role == llm.RoleAssistant {
			speaker = "助理："
		}
		lines = append(lines, speaker+oneLine(rows[index].Content, 200))
	}
	return lines
}

// Retrieved 是检索评测看到的一条结果：记忆（mem_xxx）或原始记录片段（note_xxx）。
type Retrieved struct {
	Key    string  `json:"key"`
	Title  string  `json:"title"`
	Score  float64 `json:"score"`
	Vector float64 `json:"vector"` // 向量相似度
	Text   float64 `json:"text"`   // 全文匹配（归一化）
}

// Retrieve 跑一遍和对话里完全相同的自动检索，给检索评测用。history 是这句话之前的对话（「我：…」「助理：…」）。
func (s *Service) Retrieve(ctx context.Context, familyKey, botKey string, history []string, text string) ([]Retrieved, error) {
	item, err := s.bots.Get(ctx, familyKey, botKey)
	if err != nil {
		return nil, err
	}
	dir, err := s.loadDirectory(ctx, familyKey)
	if err != nil {
		return nil, err
	}
	found, err := s.retrieve(ctx, familyKey, item, dir, text, history, s.settings.Budget(ctx).TurnContextHits)
	if err != nil {
		return nil, err
	}
	result := make([]Retrieved, 0, len(found.hits)+len(found.passages))
	for _, hit := range found.hits {
		result = append(result, Retrieved{Key: hit.Memory.MemoryKey, Title: hit.Memory.Title, Score: hit.Score, Vector: hit.Vector, Text: hit.Text})
	}
	for _, passage := range found.passages {
		result = append(result, Retrieved{Key: passage.NoteKey, Title: oneLine(passage.Text, 30), Score: passage.Score, Vector: passage.Score})
	}
	return result, nil
}

package memory

import (
	"context"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"common/viewer"

	"gorm.io/gorm"
)

// SearchQuery 描述一次检索。
type SearchQuery struct {
	Text    string
	Persons []string // 问题里涉及的人，命中加分
	Types   []string
	Limit   int
	// IncludeHistory 为 true 时也返回已被替代的旧记忆（问「她以前怎么想」时用）。
	IncludeHistory bool
	// VisibleTo 非空时只返回这个人能「看到」的记忆：关于她本人的 + 公共的。
	// 分身用它保证人设不串。
	VisibleTo string
	// Focused 为 true 时只留和问题真正相关的（相关度门槛，见 relevant）：每轮自动检索用，
	// 宁可少放也不往上下文里塞无关记忆。模型主动查（search_memory）时不设，由它自己判断。
	Focused bool
}

// 相关度门槛（Focused）。向量分是余弦相似度；全文分是相对本次最高分的比例，单看不可靠，要配合向量分。
const (
	focusMinVector  = 0.40 // 向量分至少这么高
	focusWindow     = 0.20 // 且不比本次最好的低太多
	focusTextVector = 0.30 // 全文高度匹配，或问题点名的人和记忆对得上时，向量分放宽到这里
	focusText       = 0.60
)

// relevant 判断一条候选算不算和问题相关。vectorUsed=false（没配向量化或向量化失败）时只看全文。
// personMatch：问题里提到的人正是这条记忆关于的人（「女儿最近想去哪玩」和女儿的原话）。
func relevant(vector, text, topVector float64, vectorUsed, personMatch bool) bool {
	if !vectorUsed {
		return text > 0 || personMatch
	}
	if vector >= math.Max(focusMinVector, topVector-focusWindow) {
		return true
	}
	return vector >= focusTextVector && (text >= focusText || personMatch)
}

type Hit struct {
	Memory Memory  `json:"memory"`
	Score  float64 `json:"score"`
	Vector float64 `json:"vector"`
	Text   float64 `json:"text"`

	personMatch bool
}

// 打分权重（设计文档 3.5，07 · 9 按检索评测调过）。向量分最能反映语义相关；全文分是相对本次最高分的比例，
// 字面上沾一点就能拿满分，只作辅助，否则「她说过的事」这类字面接近的会压过真正的答案。
const (
	weightVector     = 0.55
	weightText       = 0.08
	weightImportance = 0.10
	weightRecency    = 0.10
	weightPerson     = 0.10
	halfLifeDays     = 90.0
)

// Search 混合检索：向量召回 ∪ 全文召回 ∪ 相关人物的近期记忆，统一打分后取 Top N。
func (s *Service) Search(ctx context.Context, familyKey string, query SearchQuery) ([]Hit, error) {
	limit := query.Limit
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	text := strings.TrimSpace(query.Text)
	vectorScores := map[string]float64{}
	textScores := map[string]float64{}
	candidates := map[string]bool{}

	if text != "" && s.embedder != nil {
		if model := s.vectorKey(ctx); model != "" {
			if err := s.index.ensure(ctx, s.vectors, familyKey, model, false); err == nil {
				vector, err := s.queryVector(ctx, text)
				if err == nil && len(vector) > 0 {
					allowed, err := s.visibleKeys(ctx, familyKey, query.IncludeHistory)
					if err != nil {
						return nil, err
					}
					for _, hit := range s.index.search(familyKey, vector, 60, allowed) {
						vectorScores[hit.MemoryKey] = hit.Score
						candidates[hit.MemoryKey] = true
					}
				} else if err != nil {
					log.Printf("检索向量化失败，退化为全文检索: %v", err)
				}
			}
		}
	}

	if text != "" {
		type textHit struct {
			MemoryKey string
			Score     float64
		}
		var hits []textHit
		clause, clauseArgs := viewer.Clause(ctx, "")
		args := append([]any{text, familyKey}, clauseArgs...)
		err := s.db.WithContext(ctx).Raw(
			`SELECT memory_key, MATCH(title, content) AGAINST (? IN NATURAL LANGUAGE MODE) AS score
			   FROM pa_memory
			  WHERE family_key = ? AND `+clause+` AND MATCH(title, content) AGAINST (? IN NATURAL LANGUAGE MODE)
			  ORDER BY score DESC LIMIT 60`, append(args, text)...).Scan(&hits).Error
		if err != nil {
			log.Printf("全文检索失败: %v", err)
		}
		maxScore := 0.0
		for _, hit := range hits {
			maxScore = math.Max(maxScore, hit.Score)
		}
		for _, hit := range hits {
			if maxScore > 0 {
				textScores[hit.MemoryKey] = hit.Score / maxScore
			}
			candidates[hit.MemoryKey] = true
		}
	}

	if len(query.Persons) > 0 {
		var recent []string
		clause, clauseArgs := viewer.Clause(ctx, "m.")
		s.db.WithContext(ctx).Model(&linkRow{}).
			Joins("JOIN pa_memory m ON m.memory_key = pa_memory_link.memory_key").
			Where(clause, clauseArgs...).
			Where("m.family_key = ? AND m.status = ? AND pa_memory_link.target_type = 'person' AND pa_memory_link.target_key IN ?", familyKey, StatusActive, query.Persons).
			Order("m.importance DESC, COALESCE(m.occurred_at, m.created_at) DESC").Limit(40).
			Pluck("pa_memory_link.memory_key", &recent)
		for _, key := range recent {
			candidates[key] = true
		}
	}

	if len(candidates) == 0 {
		return []Hit{}, nil
	}
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	dbQuery := s.db.WithContext(ctx).Where("family_key = ? AND memory_key IN ?", familyKey, keys)
	statuses := []string{StatusActive}
	if query.IncludeHistory {
		statuses = append(statuses, StatusSuperseded)
	}
	dbQuery = dbQuery.Where("status IN ?", statuses)
	if !query.IncludeHistory {
		dbQuery = dbQuery.Where("(valid_to IS NULL OR valid_to > ?)", time.Now())
	}
	if len(query.Types) > 0 {
		dbQuery = dbQuery.Where("type IN ?", query.Types)
	}
	if query.VisibleTo != "" {
		dbQuery = dbQuery.Where("(subject_person = ? OR subject_person IS NULL OR scope IN ('shared','place'))", query.VisibleTo)
	}
	var rows []memoryRow
	if err := dbQuery.Find(&rows).Error; err != nil {
		return nil, err
	}
	memories, err := s.toDTOs(ctx, rows)
	if err != nil {
		return nil, err
	}

	personSet := map[string]bool{}
	for _, person := range query.Persons {
		personSet[person] = true
	}
	now := time.Now()
	hits := make([]Hit, 0, len(memories))
	for _, memory := range memories {
		vector := vectorScores[memory.MemoryKey]
		textScore := textScores[memory.MemoryKey]
		score := weightVector*math.Max(vector, 0) + weightText*textScore + weightImportance*float64(memory.Importance)/5
		score += weightRecency * recency(memory, now)
		personMatch := false
		for _, person := range memory.Persons {
			if personSet[person] {
				personMatch = true
				score += weightPerson
				break
			}
		}
		hits = append(hits, Hit{Memory: memory, Score: score, Vector: vector, Text: textScore, personMatch: personMatch})
	}
	if query.Focused {
		hits = focus(hits, len(vectorScores) > 0)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func focus(hits []Hit, vectorUsed bool) []Hit {
	topVector := 0.0
	for _, hit := range hits {
		topVector = math.Max(topVector, hit.Vector)
	}
	var kept []Hit
	for _, hit := range hits {
		if relevant(hit.Vector, hit.Text, topVector, vectorUsed, hit.personMatch) {
			kept = append(kept, hit)
		}
	}
	return kept
}

// Focus 对一组不带门槛检索出的结果套用相关度门槛（和 SearchQuery.Focused 相同）。
func Focus(hits []Hit) []Hit {
	vectorUsed := false
	for _, hit := range hits {
		if hit.Vector != 0 {
			vectorUsed = true
			break
		}
	}
	return focus(hits, vectorUsed)
}

func recency(memory Memory, now time.Time) float64 {
	info, ok := typeInfo(memory.Type)
	if !ok || !info.Decays {
		return 1
	}
	at := memory.CreatedAt
	if memory.OccurredAt != nil {
		at = *memory.OccurredAt
	}
	days := now.Sub(at).Hours() / 24
	if days < 0 {
		days = 0
	}
	return math.Exp(-days * math.Ln2 / halfLifeDays)
}

// CoreFilter 选常驻上下文（L1）的记忆。
type CoreFilter struct {
	Persons       []string // 只取关于这些人的；空表示全家
	MinImportance int
	Limit         int
	VisibleTo     string
}

// Core 返回常驻上下文：高重要度的稳定记忆 + 未完成的承诺/诉求 + 近 7 天的事件。
// 结果按类型、重要度排序且顺序稳定，便于命中提示词缓存。
func (s *Service) Core(ctx context.Context, familyKey string, filter CoreFilter) ([]Memory, error) {
	if filter.MinImportance <= 0 {
		filter.MinImportance = 4
	}
	if filter.Limit <= 0 {
		filter.Limit = 40
	}
	base := func() *gorm.DB {
		query := s.db.WithContext(ctx).Model(&memoryRow{}).
			Where("family_key = ? AND status = ? AND (valid_to IS NULL OR valid_to > ?)", familyKey, StatusActive, time.Now())
		if len(filter.Persons) > 0 {
			query = query.Where("(subject_person IN ? OR memory_key IN (SELECT memory_key FROM pa_memory_link WHERE target_type='person' AND target_key IN ?))", filter.Persons, filter.Persons)
		}
		if filter.VisibleTo != "" {
			query = query.Where("(subject_person = ? OR subject_person IS NULL OR scope IN ('shared','place'))", filter.VisibleTo)
		}
		return query
	}
	var stable []memoryRow
	if err := base().Where("importance >= ? AND type IN ?", filter.MinImportance,
		[]string{"trait", "habit", "preference", "taboo", "conflict", "health", "custom", "routine"}).
		Order("type, importance DESC, id").Limit(filter.Limit).Find(&stable).Error; err != nil {
		return nil, err
	}
	var open []memoryRow
	if err := base().Where("type IN ? AND (attrs IS NULL OR JSON_UNQUOTE(JSON_EXTRACT(attrs, '$.status')) IS NULL OR JSON_UNQUOTE(JSON_EXTRACT(attrs, '$.status')) IN ('open',''))",
		[]string{"commitment", "need"}).
		Order("importance DESC, id").Limit(20).Find(&open).Error; err != nil {
		return nil, err
	}
	var recent []memoryRow
	if err := base().Where("type IN ? AND COALESCE(occurred_at, created_at) > ?", []string{"event", "quote"}, time.Now().AddDate(0, 0, -7)).
		Order("COALESCE(occurred_at, created_at) DESC").Limit(15).Find(&recent).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var rows []memoryRow
	for _, group := range [][]memoryRow{stable, open, recent} {
		for _, row := range group {
			if !seen[row.MemoryKey] {
				seen[row.MemoryKey] = true
				rows = append(rows, row)
			}
		}
	}
	return s.toDTOs(ctx, rows)
}

// visibleKeys 是当前账号看得到的记忆编号，向量召回时先按它过滤，
// 免得 Top K 被别人的私有记忆占满（奶奶的记忆少，不先过滤可能一条也召回不到）。
// 不过滤（Unscoped）时返回 nil。
func (s *Service) visibleKeys(ctx context.Context, familyKey string, includeHistory bool) (func(string) bool, error) {
	if _, unscoped, _ := viewer.Accounts(ctx); unscoped {
		return nil, nil
	}
	statuses := []string{StatusActive}
	if includeHistory {
		statuses = append(statuses, StatusSuperseded)
	}
	var keys []string
	if err := s.db.WithContext(ctx).Model(&memoryRow{}).Where("family_key = ? AND status IN ?", familyKey, statuses).
		Pluck("memory_key", &keys).Error; err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	return func(key string) bool { return allowed[key] }, nil
}

package memory

import (
	"context"
	"errors"
	"sort"
	"time"
)

// SimilarPair 是一对疑似重复的记忆（同类型、同一个主体，向量相似度超过阈值）。
type SimilarPair struct {
	A          string  `json:"a"`
	B          string  `json:"b"`
	Similarity float64 `json:"similarity"`
}

// SimilarPairs 找出疑似重复的生效记忆，相似度从高到低。
// 只在同类型、同主体的记忆之间比较：一个家庭千到万条记忆，分组后两两比较很便宜。
func (s *Service) SimilarPairs(ctx context.Context, familyKey string, threshold float64, limit int) ([]SimilarPair, error) {
	if s.embedder == nil {
		return nil, errors.New("没有配置向量化，无法找重复")
	}
	model := s.vectorKey(ctx)
	if model == "" {
		return nil, errors.New("没有配置向量化，无法找重复")
	}
	if err := s.index.ensure(ctx, s.vectors, familyKey, model, false); err != nil {
		return nil, err
	}
	var rows []memoryRow
	if err := s.db.WithContext(ctx).Select("memory_key", "type", "subject_person", "owner_account").
		Where("family_key = ? AND status = ? AND type <> 'insight'", familyKey, StatusActive).Find(&rows).Error; err != nil {
		return nil, err
	}
	groups := map[string][]string{}
	for _, row := range rows {
		subject := ""
		if row.SubjectPerson != nil {
			subject = *row.SubjectPerson
		}
		// 不同人名下的记忆不算重复，免得合并时把私有的并进别人的
		groups[row.Type+"|"+subject+"|"+row.OwnerAccount] = append(groups[row.Type+"|"+subject], row.MemoryKey)
	}

	s.index.mu.RLock()
	vectors := s.index.families[familyKey]
	var pairs []SimilarPair
	if vectors != nil {
		for _, keys := range groups {
			for i := 0; i < len(keys); i++ {
				a := vectors.vectors[keys[i]]
				if a == nil {
					continue
				}
				for j := i + 1; j < len(keys); j++ {
					b := vectors.vectors[keys[j]]
					if len(b) != len(a) {
						continue
					}
					var dot float64
					for k := range a {
						dot += float64(a[k]) * float64(b[k])
					}
					if dot >= threshold {
						first, second := keys[i], keys[j]
						if second < first {
							first, second = second, first
						}
						pairs = append(pairs, SimilarPair{A: first, B: second, Similarity: dot})
					}
				}
			}
		}
	}
	s.index.mu.RUnlock()
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Similarity > pairs[j].Similarity })
	if limit > 0 && len(pairs) > limit {
		pairs = pairs[:limit]
	}
	return pairs, nil
}

// Since 返回某段时间以来新增或发生的生效记忆（周洞察用）。
func (s *Service) Since(ctx context.Context, familyKey string, since time.Time, limit int) ([]Memory, error) {
	var rows []memoryRow
	if err := s.db.WithContext(ctx).
		Where("family_key = ? AND status = ? AND type <> 'insight' AND (created_at >= ? OR occurred_at >= ?)", familyKey, StatusActive, since, since).
		Order("COALESCE(occurred_at, created_at)").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.toDTOs(ctx, rows)
}

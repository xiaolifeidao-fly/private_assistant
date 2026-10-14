package memory

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// VectorStore 是向量的持久化。检索永远在进程内存里算（vectorIndex），这里只管存和读：
//   - FileVectorStore：服务器本地文件（默认），见 vectorfile.go；
//   - MySQLVectorStore：pa_memory_embedding 表（早期做法，配置 vector.store=mysql 可切回）。
//
// 向量都能用记忆原文重新算出来，丢了只是多调一次向量化接口，不丢真实数据。
type VectorStore interface {
	// Read 读某个家庭、某个模型下 cursor 之后新写入的向量。cursor 为 nil 或已失效（文件被压缩重写过）
	// 时返回全量，full=true，调用方要整体替换。
	Read(ctx context.Context, familyKey, model string, cursor any) (vectors map[string][]float32, next any, full bool, err error)
	Write(ctx context.Context, familyKey, model string, vectors map[string][]float32) error
	// Empty 表示这个模型下还一条向量都没有（用来决定要不要从旧存储导入）。
	Empty(ctx context.Context, familyKey, model string) (bool, error)
}

// MySQLVectorStore 把向量存在 pa_memory_embedding（一条记忆一行，换模型时覆盖）。
type MySQLVectorStore struct{ DB *gorm.DB }

func (store MySQLVectorStore) Read(ctx context.Context, familyKey, model string, cursor any) (map[string][]float32, any, bool, error) {
	now := time.Now()
	query := store.DB.WithContext(ctx).Where("family_key = ? AND model = ?", familyKey, model)
	since, incremental := cursor.(time.Time)
	if incremental {
		// 往前多拉一点，避开两边时钟和事务提交的先后
		query = query.Where("updated_at >= ?", since.Add(-time.Minute))
	}
	var rows []embeddingRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, cursor, false, err
	}
	vectors := make(map[string][]float32, len(rows))
	for _, row := range rows {
		vectors[row.MemoryKey] = decodeVector(row.Vector)
	}
	return vectors, now, !incremental, nil
}

func (store MySQLVectorStore) Write(ctx context.Context, familyKey, model string, vectors map[string][]float32) error {
	now := time.Now()
	for memoryKey, vector := range vectors {
		record := embeddingRow{MemoryKey: memoryKey, FamilyKey: familyKey, Model: model, Dim: len(vector), Vector: encodeVector(vector), UpdatedAt: now}
		if err := store.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}

func (store MySQLVectorStore) Empty(ctx context.Context, familyKey, model string) (bool, error) {
	var count int64
	err := store.DB.WithContext(ctx).Model(&embeddingRow{}).Where("family_key = ? AND model = ?", familyKey, model).Count(&count).Error
	return count == 0, err
}

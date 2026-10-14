package persona

import (
	"context"
	"time"

	"service/family"
	"service/llm"
	"service/memory"
	"service/setting"
)

// 人设卡领域只声明生成卡片所需的最小能力，由 wire 注入实现。
type FamilyPort interface {
	GetPerson(ctx context.Context, familyKey, personKey string) (family.Person, error)
	ListPersons(ctx context.Context, familyKey string) ([]family.Person, error)
	ListRelations(ctx context.Context, familyKey, personKey string) ([]family.Relation, error)
}

type MemoryPort interface {
	LatestChange(ctx context.Context, familyKey, personKey string) (int64, *time.Time, error)
	List(ctx context.Context, familyKey string, filter memory.ListFilter) ([]memory.Memory, int64, error)
}

type LLMPort interface {
	Chat(ctx context.Context, request llm.Request, onDelta func(llm.Delta)) (llm.Response, error)
}

type SettingPort interface {
	Learning(ctx context.Context) setting.Learning
}

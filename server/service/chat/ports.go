package chat

import (
	"context"

	"service/bot"
	"service/family"
	"service/intake"
	"service/llm"
	"service/memory"
	"service/persona"
	"service/reminder"
	"service/setting"
	"service/skill"
)

// 编排层只依赖下游领域的最小能力。具体实现由 wire 注入，
// 避免对话服务把家庭、记忆、模型和人设卡服务的其他方法也变成隐式依赖。
type FamilyPort interface {
	ListPersons(ctx context.Context, familyKey string) ([]family.Person, error)
	ListPlaces(ctx context.Context, familyKey string) ([]family.Place, error)
	ListRelations(ctx context.Context, familyKey, personKey string) ([]family.Relation, error)
	ResolvePerson(ctx context.Context, familyKey, mention string) (family.Person, error)
}

type MemoryPort interface {
	Search(ctx context.Context, familyKey string, query memory.SearchQuery) ([]memory.Hit, error)
	Core(ctx context.Context, familyKey string, filter memory.CoreFilter) ([]memory.Memory, error)
	List(ctx context.Context, familyKey string, filter memory.ListFilter) ([]memory.Memory, int64, error)
	Create(ctx context.Context, familyKey string, input memory.Input) (memory.Memory, error)
	Get(ctx context.Context, familyKey, memoryKey string) (memory.Memory, error)
	Supersede(ctx context.Context, familyKey, oldKey string, input memory.Input) (memory.Memory, error)
	SearchPassages(ctx context.Context, familyKey, text string, limit int, minScore float64) ([]memory.Passage, error)
	MarkUsed(ctx context.Context, memoryKeys []string)
}

// NotePort 是对话里上传照片时用的原始记录能力：先存记录，再当场整理成记忆。
type NotePort interface {
	ProcessNow(ctx context.Context, familyKey, noteKey string) (intake.Note, error)
	GetNote(ctx context.Context, familyKey, noteKey string) (intake.Note, error)
}

type BotPort interface {
	Get(ctx context.Context, familyKey, botKey string) (bot.Bot, error)
}

type LLMPort interface {
	Chat(ctx context.Context, request llm.Request, onDelta func(llm.Delta)) (llm.Response, error)
}

type PersonaPort interface {
	Active(ctx context.Context, familyKey, personKey string) (persona.Card, bool, error)
}

type ReminderPort interface {
	Upcoming(ctx context.Context, familyKey string, days int) ([]reminder.Reminder, error)
	CreateManual(ctx context.Context, familyKey string, input reminder.ManualInput) (reminder.Reminder, error)
	Dismiss(ctx context.Context, familyKey, reminderKey string) error
}

type SkillPort interface {
	ForBot(ctx context.Context, familyKey, botKey string) ([]skill.Skill, error)
	Get(ctx context.Context, familyKey, skillKey string) (skill.Skill, error)
	Render(ctx context.Context, familyKey string, item skill.Skill) string
	DraftFromFeedback(ctx context.Context, familyKey string, draft skill.FeedbackDraft) (skill.Example, error)
}

type SettingPort interface {
	Safety(ctx context.Context) setting.Safety
	Budget(ctx context.Context) setting.Budget
	Learning(ctx context.Context) setting.Learning
}

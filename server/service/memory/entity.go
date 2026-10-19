package memory

import (
	"encoding/json"
	"time"

	"common/middleware/db"
)

type memoryRow struct {
	ID            int64
	MemoryKey     string
	FamilyKey     string
	Type          string
	Scope         string
	SubjectPerson *string
	PlaceKey      *string
	Title         string
	Content       string
	Attrs         db.JSON
	Importance    int
	Confidence    float64
	Status        string
	OccurredAt    *time.Time
	ValidFrom     *time.Time
	ValidTo       *time.Time
	SupersedesKey *string
	SourceNoteKey *string
	CreatedBy     string
	OwnerAccount  string
	Visibility    string
	UseCount      int
	LastUsedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (memoryRow) TableName() string { return "pa_memory" }

func (row memoryRow) toDTO() Memory {
	return Memory{
		MemoryKey: row.MemoryKey, Type: row.Type, TypeLabel: TypeLabel(row.Type), Scope: row.Scope,
		SubjectPerson: deref(row.SubjectPerson), PlaceKey: deref(row.PlaceKey), Title: row.Title, Content: row.Content,
		Attrs: json.RawMessage(row.Attrs), Importance: row.Importance, Confidence: row.Confidence, Status: row.Status,
		OccurredAt: row.OccurredAt, ValidFrom: row.ValidFrom, ValidTo: row.ValidTo, SupersedesKey: deref(row.SupersedesKey),
		SourceNoteKey: deref(row.SourceNoteKey), CreatedBy: row.CreatedBy, OwnerAccount: row.OwnerAccount, Visibility: row.Visibility, UseCount: row.UseCount, LastUsedAt: row.LastUsedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

type linkRow struct {
	ID         int64
	MemoryKey  string
	TargetType string
	TargetKey  string
}

func (linkRow) TableName() string { return "pa_memory_link" }

type embeddingRow struct {
	ID        int64
	MemoryKey string
	FamilyKey string
	Model     string
	Dim       int
	Vector    []byte
	UpdatedAt time.Time
}

func (embeddingRow) TableName() string { return "pa_memory_embedding" }

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

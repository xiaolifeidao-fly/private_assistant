package persona

import "time"

type cardRow struct {
	ID           int64
	CardKey      string
	FamilyKey    string
	PersonKey    string
	Version      int
	Content      string
	Changes      string
	MemoryCount  int
	MemoryCutoff *time.Time
	Status       string
	CreatedBy    string
	CreatedAt    time.Time
}

func (cardRow) TableName() string { return "pa_persona_card" }

func (row cardRow) toDTO() Card {
	return Card{
		CardKey: row.CardKey, PersonKey: row.PersonKey, Version: row.Version, Content: row.Content, Changes: row.Changes,
		MemoryCount: row.MemoryCount, MemoryCutoff: row.MemoryCutoff, Status: row.Status, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}
}

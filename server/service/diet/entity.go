package diet

import (
	"encoding/json"
	"time"

	"common/middleware/db"
)

type planRow struct {
	ID        int64
	PlanKey   string
	FamilyKey string
	ChildKey  string
	PlanDate  time.Time
	Meal      string
	Foods     string
	Note      string
	Status    string
	CreatedBy string
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (planRow) TableName() string { return "pa_diet_plan" }

func (row planRow) toDTO() Plan {
	return Plan{PlanKey: row.PlanKey, ChildKey: row.ChildKey, Date: row.PlanDate.Format(dateLayout), Meal: row.Meal,
		Foods: row.Foods, Note: row.Note, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt}
}

type recordRow struct {
	ID         int64
	RecordKey  string
	FamilyKey  string
	ChildKey   string
	EatenAt    time.Time
	Meal       string
	Foods      string
	Amount     string
	Note       string
	Source     string
	RawText    string
	Photos     db.JSON
	CaptureKey string
	Status     string
	CreatedBy  string
	UpdatedBy  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (recordRow) TableName() string { return "pa_diet_record" }

func (row recordRow) photos() []Photo {
	var photos []Photo
	if len(row.Photos) > 0 {
		_ = json.Unmarshal(row.Photos, &photos)
	}
	return photos
}

func (row recordRow) toDTO() Record {
	return Record{RecordKey: row.RecordKey, ChildKey: row.ChildKey, EatenAt: row.EatenAt, Meal: row.Meal, Foods: row.Foods,
		Amount: row.Amount, Note: row.Note, Source: row.Source, RawText: row.RawText, PhotoCount: len(row.photos()),
		CaptureKey: row.CaptureKey, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

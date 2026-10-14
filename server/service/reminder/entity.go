package reminder

import (
	"encoding/json"
	"time"

	"common/middleware/db"
)

type reminderRow struct {
	ID          int64
	ReminderKey string
	FamilyKey   string
	Kind        string
	Title       string
	Body        string
	EventAt     *time.Time
	RemindAt    time.Time
	Status      string
	SourceType  string
	SourceKey   string
	DedupeKey   string
	TargetURL   string
	Payload     db.JSON
	Attempts    int
	LastError   string
	SentAt      *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (reminderRow) TableName() string { return "pa_reminder" }

func (row reminderRow) toDTO() Reminder {
	return Reminder{
		ReminderKey: row.ReminderKey, Kind: row.Kind, Title: row.Title, Body: row.Body,
		EventAt: row.EventAt, RemindAt: row.RemindAt, Status: row.Status,
		SourceType: row.SourceType, SourceKey: row.SourceKey, TargetURL: row.TargetURL,
		Payload: json.RawMessage(row.Payload), Attempts: row.Attempts, LastError: row.LastError,
		SentAt: row.SentAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

type tripRow struct {
	ID        int64
	TripKey   string
	FamilyKey string
	PlaceKey  string
	Title     string
	StartAt   time.Time
	EndAt     *time.Time
	People    db.StringList
	Notes     string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (tripRow) TableName() string { return "pa_trip" }

func (row tripRow) toDTO() Trip {
	people := []string(row.People)
	if people == nil {
		people = []string{}
	}
	return Trip{TripKey: row.TripKey, PlaceKey: row.PlaceKey, Title: row.Title, StartAt: row.StartAt,
		EndAt: row.EndAt, People: people, Notes: row.Notes, Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

type subscriptionRow struct {
	ID              int64
	SubscriptionKey string
	FamilyKey       string
	AccountKey      string
	EndpointHash    string
	Endpoint        string
	P256dh          string
	Auth            string
	UserAgent       string
	Enabled         int
	LastSuccessAt   *time.Time
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (subscriptionRow) TableName() string { return "pa_push_subscription" }

type deliveryRow struct {
	ID              int64
	DeliveryKey     string
	FamilyKey       string
	ReminderKey     string
	SubscriptionKey string
	Status          string
	HTTPStatus      int
	Error           string
	CreatedAt       time.Time
}

func (deliveryRow) TableName() string { return "pa_push_delivery" }

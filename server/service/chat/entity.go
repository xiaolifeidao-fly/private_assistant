package chat

import (
	"time"

	"common/middleware/db"
)

type conversationRow struct {
	ID              int64
	ConversationKey string
	FamilyKey       string
	AccountKey      string
	BotKey          string
	Title           string
	Status          string
	MessageCount    int
	SystemSnapshot  string
	ToolsSnapshot   db.StringList
	BotVersion      int
	Segment            int
	ExtractedMessageID int64
	PersonaVersions    db.JSON
	LastMessageAt   *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (conversationRow) TableName() string { return "pa_conversation" }

func (row conversationRow) toDTO() Conversation {
	return Conversation{
		ConversationKey: row.ConversationKey, AccountKey: row.AccountKey, BotKey: row.BotKey, Title: row.Title,
		Status: row.Status, MessageCount: row.MessageCount, LastMessageAt: row.LastMessageAt, CreatedAt: row.CreatedAt,
	}
}

type messageRow struct {
	ID              int64
	MessageKey      string
	ConversationKey string
	TurnKey         string
	Segment         int
	Role            string
	Content         string
	Blocks          db.JSON
	Provider        string
	Model           string
	Raw             db.JSON
	Usage           db.JSON
	Meta            db.JSON
	Status          string
	Error           string
	Feedback        string
	FeedbackNote    string
	CreatedAt       time.Time
}

func (messageRow) TableName() string { return "pa_message" }

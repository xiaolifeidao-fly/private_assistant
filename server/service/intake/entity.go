package intake

import (
	"encoding/json"
	"time"

	"common/middleware/db"
)

type noteRow struct {
	ID              int64
	NoteKey         string
	FamilyKey       string
	Channel         string
	RawText         string
	Attachments     db.JSON
	AboutPersons    db.StringList
	OccurredAt      *time.Time
	ExtractStatus   string
	ExtractAttempts int
	ClaimedAt       *time.Time
	ExtractedAt     *time.Time
	ExtractResult   db.JSON
	ExtractError    *string
	CreatedBy       string
	OwnerAccount    string
	Visibility      string
	CreatedAt       time.Time
}

func (noteRow) TableName() string { return "pa_source_note" }

func (row noteRow) attachments() []Attachment {
	var attachments []Attachment
	_ = json.Unmarshal(row.Attachments, &attachments)
	return attachments
}

func (row noteRow) toDTO() Note {
	note := Note{
		NoteKey: row.NoteKey, Channel: row.Channel, RawText: row.RawText, Attachments: row.attachments(),
		AboutPersons: []string(row.AboutPersons), OccurredAt: row.OccurredAt, ExtractStatus: row.ExtractStatus,
		ExtractAttempts: row.ExtractAttempts, ExtractedAt: row.ExtractedAt, CreatedBy: row.CreatedBy, OwnerAccount: row.OwnerAccount, Visibility: row.Visibility, CreatedAt: row.CreatedAt,
	}
	if note.Attachments == nil {
		note.Attachments = []Attachment{}
	}
	if note.AboutPersons == nil {
		note.AboutPersons = []string{}
	}
	if row.ExtractError != nil {
		note.ExtractError = *row.ExtractError
	}
	if len(row.ExtractResult) > 0 {
		var result Result
		if json.Unmarshal(row.ExtractResult, &result) == nil {
			note.Result = &result
		}
	}
	return note
}

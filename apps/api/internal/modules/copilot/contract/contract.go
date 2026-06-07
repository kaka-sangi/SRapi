// Package contract defines the persistence boundary for admin-copilot
// conversation history. Every method is scoped by adminUserID so one admin can
// never read or mutate another admin's transcripts.
package contract

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when a conversation does not exist for the admin.
var ErrNotFound = errors.New("copilot conversation not found")

// Conversation is one persisted copilot chat transcript.
type Conversation struct {
	ID          int
	AdminUserID int
	Title       string
	Messages    json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ConversationSummary is a sidebar list row (no message body).
type ConversationSummary struct {
	ID        int
	Title     string
	UpdatedAt time.Time
}

// ConversationStore persists copilot conversations per admin.
type ConversationStore interface {
	ListByAdmin(ctx context.Context, adminUserID, limit int) ([]ConversationSummary, error)
	Get(ctx context.Context, adminUserID, id int) (Conversation, error)
	Create(ctx context.Context, adminUserID int, title string, messages json.RawMessage) (Conversation, error)
	Update(ctx context.Context, adminUserID, id int, title string, messages json.RawMessage) (Conversation, error)
	Rename(ctx context.Context, adminUserID, id int, title string) (Conversation, error)
	Delete(ctx context.Context, adminUserID, id int) error
}

// Package copilotconv is the Ent-backed store for admin-copilot conversation
// history. Every query is scoped by admin_user_id for per-admin isolation.
package copilotconv

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entconv "github.com/srapi/srapi/apps/api/ent/copilotconversation"
	"github.com/srapi/srapi/apps/api/internal/modules/copilot/contract"
)

// ErrInvalidStore is returned for nil clients or invalid arguments.
var ErrInvalidStore = errors.New("invalid copilot conversation store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) ListByAdmin(ctx context.Context, adminUserID, limit int) ([]contract.ConversationSummary, error) {
	if adminUserID <= 0 {
		return nil, ErrInvalidStore
	}
	q := s.client.CopilotConversation.Query().
		Where(entconv.AdminUserIDEQ(adminUserID)).
		Order(ent.Desc(entconv.FieldUpdatedAt))
	if limit > 0 {
		q = q.Limit(limit)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ConversationSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, contract.ConversationSummary{ID: row.ID, Title: row.Title, UpdatedAt: row.UpdatedAt})
	}
	return out, nil
}

func (s *Store) Get(ctx context.Context, adminUserID, id int) (contract.Conversation, error) {
	if adminUserID <= 0 || id <= 0 {
		return contract.Conversation{}, ErrInvalidStore
	}
	row, err := s.client.CopilotConversation.Query().
		Where(entconv.IDEQ(id), entconv.AdminUserIDEQ(adminUserID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Conversation{}, contract.ErrNotFound
		}
		return contract.Conversation{}, err
	}
	return toConversation(row), nil
}

func (s *Store) Create(ctx context.Context, adminUserID int, title string, messages json.RawMessage) (contract.Conversation, error) {
	if adminUserID <= 0 {
		return contract.Conversation{}, ErrInvalidStore
	}
	row, err := s.client.CopilotConversation.Create().
		SetAdminUserID(adminUserID).
		SetTitle(title).
		SetMessagesJSON(messagesString(messages)).
		Save(ctx)
	if err != nil {
		return contract.Conversation{}, err
	}
	return toConversation(row), nil
}

func (s *Store) Update(ctx context.Context, adminUserID, id int, title string, messages json.RawMessage) (contract.Conversation, error) {
	if adminUserID <= 0 || id <= 0 {
		return contract.Conversation{}, ErrInvalidStore
	}
	affected, err := s.client.CopilotConversation.Update().
		Where(entconv.IDEQ(id), entconv.AdminUserIDEQ(adminUserID)).
		SetTitle(title).
		SetMessagesJSON(messagesString(messages)).
		Save(ctx)
	if err != nil {
		return contract.Conversation{}, err
	}
	if affected == 0 {
		return contract.Conversation{}, contract.ErrNotFound
	}
	return s.Get(ctx, adminUserID, id)
}

func (s *Store) Rename(ctx context.Context, adminUserID, id int, title string) (contract.Conversation, error) {
	if adminUserID <= 0 || id <= 0 {
		return contract.Conversation{}, ErrInvalidStore
	}
	affected, err := s.client.CopilotConversation.Update().
		Where(entconv.IDEQ(id), entconv.AdminUserIDEQ(adminUserID)).
		SetTitle(title).
		Save(ctx)
	if err != nil {
		return contract.Conversation{}, err
	}
	if affected == 0 {
		return contract.Conversation{}, contract.ErrNotFound
	}
	return s.Get(ctx, adminUserID, id)
}

func (s *Store) Delete(ctx context.Context, adminUserID, id int) error {
	if adminUserID <= 0 || id <= 0 {
		return ErrInvalidStore
	}
	affected, err := s.client.CopilotConversation.Delete().
		Where(entconv.IDEQ(id), entconv.AdminUserIDEQ(adminUserID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrNotFound
	}
	return nil
}

func messagesString(messages json.RawMessage) string {
	if len(messages) == 0 {
		return "[]"
	}
	return string(messages)
}

func toConversation(row *ent.CopilotConversation) contract.Conversation {
	msgs := row.MessagesJSON
	if msgs == "" {
		msgs = "[]"
	}
	return contract.Conversation{
		ID:          row.ID,
		AdminUserID: row.AdminUserID,
		Title:       row.Title,
		Messages:    json.RawMessage(msgs),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

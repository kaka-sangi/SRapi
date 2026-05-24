package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entauthsession "github.com/srapi/srapi/apps/api/ent/authsession"
	"github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
)

var (
	ErrInvalidStore    = errors.New("invalid auth ent store")
	ErrSessionNotFound = errors.New("auth session not found")
)

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateSession) (contract.Session, error) {
	sessionID := strings.TrimSpace(input.ID)
	csrfToken := strings.TrimSpace(input.CSRFToken)
	if sessionID == "" || csrfToken == "" || input.UserID <= 0 || input.ExpiresAt.IsZero() {
		return contract.Session{}, ErrInvalidStore
	}

	create := s.client.AuthSession.Create().
		SetSessionIDHash(tokenHash(sessionID)).
		SetCsrfTokenHash(tokenHash(csrfToken)).
		SetUserID(input.UserID).
		SetExpiresAt(input.ExpiresAt).
		SetStatus("active")
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.Session{}, err
	}
	return contract.Session{
		ID:        sessionID,
		UserID:    created.UserID,
		CSRFToken: csrfToken,
		ExpiresAt: created.ExpiresAt,
		CreatedAt: created.CreatedAt,
	}, nil
}

func (s *Store) FindByID(ctx context.Context, id string) (contract.Session, error) {
	sessionID := strings.TrimSpace(id)
	if sessionID == "" {
		return contract.Session{}, ErrInvalidStore
	}
	found, err := s.client.AuthSession.Query().
		Where(
			entauthsession.SessionIDHashEQ(tokenHash(sessionID)),
			entauthsession.StatusEQ("active"),
			entauthsession.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		return contract.Session{}, err
	}
	return toSession(sessionID, found), nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	sessionID := strings.TrimSpace(id)
	if sessionID == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := s.client.AuthSession.Update().
		Where(
			entauthsession.SessionIDHashEQ(tokenHash(sessionID)),
			entauthsession.DeletedAtIsNil(),
		).
		SetStatus("revoked").
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (s *Store) Touch(ctx context.Context, id string, at time.Time) error {
	sessionID := strings.TrimSpace(id)
	if sessionID == "" || at.IsZero() {
		return ErrInvalidStore
	}
	affected, err := s.client.AuthSession.Update().
		Where(
			entauthsession.SessionIDHashEQ(tokenHash(sessionID)),
			entauthsession.StatusEQ("active"),
			entauthsession.DeletedAtIsNil(),
		).
		SetLastActiveAt(at).
		SetUpdatedAt(at).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSessionNotFound
	}
	return err
}

func toSession(rawSessionID string, row *ent.AuthSession) contract.Session {
	return contract.Session{
		ID:         rawSessionID,
		UserID:     row.UserID,
		CSRFToken:  hashedTokenMarker(row.CsrfTokenHash),
		ExpiresAt:  row.ExpiresAt,
		CreatedAt:  row.CreatedAt,
		LastSeenAt: row.LastActiveAt,
	}
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hashedTokenMarker(hash string) string {
	return "sha256:" + strings.TrimSpace(hash)
}

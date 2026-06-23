package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
)

type Store struct {
	mu                  sync.Mutex
	sessions            map[string]contract.Session
	passwordResetTokens map[string]contract.PasswordResetToken
	emailVerifyTokens   map[string]contract.EmailVerificationToken
	pendingOAuth        map[string]contract.PendingOAuthSession
	nextResetTokenID    int
	nextEmailVerifyID   int
	nextPendingOAuthID  int
}

func New() *Store {
	return &Store{
		sessions:            map[string]contract.Session{},
		passwordResetTokens: map[string]contract.PasswordResetToken{},
		emailVerifyTokens:   map[string]contract.EmailVerificationToken{},
		pendingOAuth:        map[string]contract.PendingOAuthSession{},
		nextResetTokenID:    1,
		nextEmailVerifyID:   1,
		nextPendingOAuthID:  1,
	}
}

func (s *Store) Create(_ context.Context, input contract.CreateSession) (contract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := contract.Session{
		ID:        input.ID,
		UserID:    input.UserID,
		CSRFToken: input.CSRFToken,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: input.CreatedAt,
	}
	s.sessions[session.ID] = session
	return session, nil
}

func (s *Store) FindByID(_ context.Context, id string) (contract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return contract.Session{}, errors.New("session not found")
	}
	return session, nil
}

func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *Store) DeleteByUserID(_ context.Context, userID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, id)
		}
	}
	return nil
}

func (s *Store) Touch(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return errors.New("session not found")
	}
	session.LastSeenAt = &at
	s.sessions[id] = session
	return nil
}

func (s *Store) UpdateCSRFToken(_ context.Context, sessionID string, newToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return errors.New("session not found")
	}
	session.CSRFToken = newToken
	s.sessions[sessionID] = session
	return nil
}

func (s *Store) CreatePasswordResetToken(_ context.Context, input contract.CreatePasswordResetToken) (contract.PasswordResetToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.UserID <= 0 || input.TokenHash == "" || input.ExpiresAt.IsZero() {
		return contract.PasswordResetToken{}, errors.New("invalid password reset token")
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	token := contract.PasswordResetToken{
		ID:        s.nextResetTokenID,
		UserID:    input.UserID,
		TokenHash: input.TokenHash,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: createdAt,
	}
	s.nextResetTokenID++
	s.passwordResetTokens[token.TokenHash] = token
	return token, nil
}

func (s *Store) ConsumePasswordResetToken(_ context.Context, tokenHash string, now time.Time) (contract.PasswordResetToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	token, ok := s.passwordResetTokens[tokenHash]
	if !ok || token.UsedAt != nil || !token.ExpiresAt.After(now) {
		return contract.PasswordResetToken{}, errors.New("password reset token not found")
	}
	usedAt := now
	token.UsedAt = &usedAt
	s.passwordResetTokens[tokenHash] = token
	return token, nil
}

func (s *Store) CreateEmailVerificationToken(_ context.Context, input contract.CreateEmailVerificationToken) (contract.EmailVerificationToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.UserID <= 0 || input.TokenHash == "" || input.ExpiresAt.IsZero() {
		return contract.EmailVerificationToken{}, errors.New("invalid email verification token")
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	token := contract.EmailVerificationToken{
		ID:        s.nextEmailVerifyID,
		UserID:    input.UserID,
		TokenHash: input.TokenHash,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: createdAt,
	}
	s.nextEmailVerifyID++
	s.emailVerifyTokens[token.TokenHash] = token
	return token, nil
}

func (s *Store) ConsumeEmailVerificationToken(_ context.Context, tokenHash string, now time.Time) (contract.EmailVerificationToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	token, ok := s.emailVerifyTokens[tokenHash]
	if !ok || token.UsedAt != nil || !token.ExpiresAt.After(now) {
		return contract.EmailVerificationToken{}, errors.New("email verification token not found")
	}
	usedAt := now
	token.UsedAt = &usedAt
	s.emailVerifyTokens[tokenHash] = token
	return token, nil
}

func (s *Store) CreatePendingOAuthSession(_ context.Context, input contract.CreatePendingOAuthSession) (contract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.SessionTokenHash == "" || input.Intent == "" || input.Provider == "" || input.ProviderKey == "" || input.ProviderSubjectHash == "" || input.ExpiresAt.IsZero() {
		return contract.PendingOAuthSession{}, errors.New("invalid pending oauth session")
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	session := contract.PendingOAuthSession{
		ID:                  s.nextPendingOAuthID,
		SessionTokenHash:    input.SessionTokenHash,
		Intent:              input.Intent,
		Provider:            input.Provider,
		ProviderKey:         input.ProviderKey,
		ProviderSubjectHash: input.ProviderSubjectHash,
		SubjectHint:         input.SubjectHint,
		TargetUserID:        cloneInt(input.TargetUserID),
		RedirectTo:          input.RedirectTo,
		ResolvedEmail:       input.ResolvedEmail,
		DisplayName:         input.DisplayName,
		EmailVerified:       input.EmailVerified,
		AvatarURL:           input.AvatarURL,
		ExpiresAt:           input.ExpiresAt,
		CreatedAt:           createdAt,
		UpdatedAt:           createdAt,
	}
	s.nextPendingOAuthID++
	s.pendingOAuth[session.SessionTokenHash] = session
	return clonePendingOAuthSession(session), nil
}

func (s *Store) ConsumePendingOAuthSession(_ context.Context, sessionTokenHash string, now time.Time) (contract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	session, ok := s.pendingOAuth[sessionTokenHash]
	if !ok || session.ConsumedAt != nil || !session.ExpiresAt.After(now) {
		return contract.PendingOAuthSession{}, errors.New("pending oauth session not found")
	}
	consumedAt := now
	session.ConsumedAt = &consumedAt
	session.UpdatedAt = now
	s.pendingOAuth[sessionTokenHash] = session
	return clonePendingOAuthSession(session), nil
}

func (s *Store) FindPendingOAuthSession(_ context.Context, sessionTokenHash string, now time.Time) (contract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	session, ok := s.pendingOAuth[sessionTokenHash]
	if !ok || session.ConsumedAt != nil || !session.ExpiresAt.After(now) {
		return contract.PendingOAuthSession{}, errors.New("pending oauth session not found")
	}
	return clonePendingOAuthSession(session), nil
}

func (s *Store) CompletePendingOAuthEmail(_ context.Context, sessionTokenHash string, email string, now time.Time) (contract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	session, ok := s.pendingOAuth[sessionTokenHash]
	if !ok || session.ConsumedAt != nil || !session.ExpiresAt.After(now) || session.ResolvedEmail != "" {
		return contract.PendingOAuthSession{}, errors.New("pending oauth session not found")
	}
	session.ResolvedEmail = strings.ToLower(strings.TrimSpace(email))
	session.EmailVerified = true
	session.UpdatedAt = now
	s.pendingOAuth[sessionTokenHash] = session
	return clonePendingOAuthSession(session), nil
}

func (s *Store) CleanupExpiredSessions(_ context.Context, now time.Time) (contract.CleanupExpiredSessionsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := contract.CleanupExpiredSessionsResult{}
	for id, session := range s.sessions {
		if session.ExpiresAt.IsZero() || session.ExpiresAt.After(now) {
			continue
		}
		result.Selected++
		delete(s.sessions, id)
		result.Expired++
	}
	return result, nil
}

func clonePendingOAuthSession(session contract.PendingOAuthSession) contract.PendingOAuthSession {
	session.TargetUserID = cloneInt(session.TargetUserID)
	if session.ConsumedAt != nil {
		consumedAt := *session.ConsumedAt
		session.ConsumedAt = &consumedAt
	}
	return session
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

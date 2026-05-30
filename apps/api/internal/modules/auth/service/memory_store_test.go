package service

import (
	"context"
	"sync"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
)

type memoryStore struct {
	mu                  sync.Mutex
	sessions            map[string]authcontract.Session
	passwordResetTokens map[string]authcontract.PasswordResetToken
	emailVerifyTokens   map[string]authcontract.EmailVerificationToken
	pendingOAuth        map[string]authcontract.PendingOAuthSession
	nextResetTokenID    int
	nextEmailVerifyID   int
	nextPendingOAuthID  int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		sessions:            map[string]authcontract.Session{},
		passwordResetTokens: map[string]authcontract.PasswordResetToken{},
		emailVerifyTokens:   map[string]authcontract.EmailVerificationToken{},
		pendingOAuth:        map[string]authcontract.PendingOAuthSession{},
		nextResetTokenID:    1,
		nextEmailVerifyID:   1,
		nextPendingOAuthID:  1,
	}
}

func (s *memoryStore) Create(_ context.Context, input authcontract.CreateSession) (authcontract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := authcontract.Session{
		ID:        input.ID,
		UserID:    input.UserID,
		CSRFToken: input.CSRFToken,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: input.CreatedAt,
	}
	s.sessions[session.ID] = session
	return session, nil
}

func (s *memoryStore) FindByID(_ context.Context, id string) (authcontract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return authcontract.Session{}, ErrSessionNotFound
	}
	return session, nil
}

func (s *memoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *memoryStore) DeleteByUserID(_ context.Context, userID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, id)
		}
	}
	return nil
}

func (s *memoryStore) Touch(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	session.LastSeenAt = &at
	s.sessions[id] = session
	return nil
}

func (s *memoryStore) CreatePasswordResetToken(_ context.Context, input authcontract.CreatePasswordResetToken) (authcontract.PasswordResetToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := authcontract.PasswordResetToken{
		ID:        s.nextResetTokenID,
		UserID:    input.UserID,
		TokenHash: input.TokenHash,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: input.CreatedAt,
	}
	s.nextResetTokenID++
	s.passwordResetTokens[input.TokenHash] = token
	return token, nil
}

func (s *memoryStore) ConsumePasswordResetToken(_ context.Context, tokenHash string, now time.Time) (authcontract.PasswordResetToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.passwordResetTokens[tokenHash]
	if !ok || token.UsedAt != nil || !token.ExpiresAt.After(now) {
		return authcontract.PasswordResetToken{}, ErrPasswordResetInvalid
	}
	usedAt := now
	token.UsedAt = &usedAt
	s.passwordResetTokens[tokenHash] = token
	return token, nil
}

func (s *memoryStore) CreateEmailVerificationToken(_ context.Context, input authcontract.CreateEmailVerificationToken) (authcontract.EmailVerificationToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := authcontract.EmailVerificationToken{
		ID:        s.nextEmailVerifyID,
		UserID:    input.UserID,
		TokenHash: input.TokenHash,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: input.CreatedAt,
	}
	s.nextEmailVerifyID++
	s.emailVerifyTokens[input.TokenHash] = token
	return token, nil
}

func (s *memoryStore) ConsumeEmailVerificationToken(_ context.Context, tokenHash string, now time.Time) (authcontract.EmailVerificationToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.emailVerifyTokens[tokenHash]
	if !ok || token.UsedAt != nil || !token.ExpiresAt.After(now) {
		return authcontract.EmailVerificationToken{}, ErrEmailVerificationInvalid
	}
	usedAt := now
	token.UsedAt = &usedAt
	s.emailVerifyTokens[tokenHash] = token
	return token, nil
}

func (s *memoryStore) CreatePendingOAuthSession(_ context.Context, input authcontract.CreatePendingOAuthSession) (authcontract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := authcontract.PendingOAuthSession{
		ID:                  s.nextPendingOAuthID,
		SessionTokenHash:    input.SessionTokenHash,
		Intent:              input.Intent,
		Provider:            input.Provider,
		ProviderKey:         input.ProviderKey,
		ProviderSubjectHash: input.ProviderSubjectHash,
		SubjectHint:         input.SubjectHint,
		TargetUserID:        cloneTestInt(input.TargetUserID),
		RedirectTo:          input.RedirectTo,
		ResolvedEmail:       input.ResolvedEmail,
		DisplayName:         input.DisplayName,
		EmailVerified:       input.EmailVerified,
		AvatarURL:           input.AvatarURL,
		ExpiresAt:           input.ExpiresAt,
		CreatedAt:           input.CreatedAt,
		UpdatedAt:           input.CreatedAt,
	}
	s.nextPendingOAuthID++
	s.pendingOAuth[session.SessionTokenHash] = session
	return clonePendingOAuthSession(session), nil
}

func (s *memoryStore) ConsumePendingOAuthSession(_ context.Context, sessionTokenHash string, now time.Time) (authcontract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.pendingOAuth[sessionTokenHash]
	if !ok || session.ConsumedAt != nil || !session.ExpiresAt.After(now) {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthInvalid
	}
	consumedAt := now
	session.ConsumedAt = &consumedAt
	session.UpdatedAt = now
	s.pendingOAuth[sessionTokenHash] = session
	return clonePendingOAuthSession(session), nil
}

func (s *memoryStore) FindPendingOAuthSession(_ context.Context, sessionTokenHash string, now time.Time) (authcontract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.pendingOAuth[sessionTokenHash]
	if !ok || session.ConsumedAt != nil || !session.ExpiresAt.After(now) {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthInvalid
	}
	return clonePendingOAuthSession(session), nil
}

func (s *memoryStore) CompletePendingOAuthEmail(_ context.Context, sessionTokenHash string, email string, now time.Time) (authcontract.PendingOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.pendingOAuth[sessionTokenHash]
	if !ok || session.ConsumedAt != nil || !session.ExpiresAt.After(now) || session.ResolvedEmail != "" {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthInvalid
	}
	session.ResolvedEmail = email
	session.EmailVerified = true
	session.UpdatedAt = now
	s.pendingOAuth[sessionTokenHash] = session
	return clonePendingOAuthSession(session), nil
}

func clonePendingOAuthSession(session authcontract.PendingOAuthSession) authcontract.PendingOAuthSession {
	session.TargetUserID = cloneTestInt(session.TargetUserID)
	if session.ConsumedAt != nil {
		consumedAt := *session.ConsumedAt
		session.ConsumedAt = &consumedAt
	}
	return session
}

func cloneTestInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

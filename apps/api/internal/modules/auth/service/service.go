package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const (
	defaultSessionTTL = 12 * time.Hour
	sessionIDBytes    = 32
	csrfTokenBytes    = 32
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type UserService interface {
	AuthenticatePassword(ctx context.Context, email, password string) (userscontract.StoredUser, error)
	FindByID(ctx context.Context, id int) (userscontract.StoredUser, error)
	TouchLastLogin(ctx context.Context, id int) error
}

type Service struct {
	users    UserService
	sessions authcontract.Store
	clock    Clock
	ttl      time.Duration
}

func New(users UserService, sessions authcontract.Store, ttl time.Duration, clock Clock) (*Service, error) {
	if users == nil || sessions == nil {
		return nil, ErrInvalidInput
	}
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{users: users, sessions: sessions, clock: clock, ttl: ttl}, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (authcontract.LoginResult, error) {
	storedUser, err := s.users.AuthenticatePassword(ctx, email, password)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	now := s.clock.Now()
	sessionID, err := randomToken("sess", sessionIDBytes)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	csrfToken, err := randomToken("csrf", csrfTokenBytes)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	session, err := s.sessions.Create(ctx, authcontract.CreateSession{
		ID:        sessionID,
		UserID:    storedUser.ID,
		CSRFToken: csrfToken,
		ExpiresAt: now.Add(s.ttl),
		CreatedAt: now,
	})
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	if err := s.users.TouchLastLogin(ctx, storedUser.ID); err != nil {
		_ = s.sessions.Delete(ctx, session.ID)
		return authcontract.LoginResult{}, err
	}
	storedUser.LastLoginAt = &now
	return authcontract.LoginResult{
		User:    storedUser.User,
		Session: session,
	}, nil
}

func (s *Service) AuthenticateSession(ctx context.Context, sessionID string) (authcontract.LoginResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return authcontract.LoginResult{}, ErrSessionNotFound
	}
	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return authcontract.LoginResult{}, ErrSessionNotFound
	}
	now := s.clock.Now()
	if !session.ExpiresAt.After(now) {
		_ = s.sessions.Delete(ctx, session.ID)
		return authcontract.LoginResult{}, ErrSessionExpired
	}
	storedUser, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return authcontract.LoginResult{}, ErrSessionUserUnavailable
	}
	if storedUser.Status != userscontract.StatusActive {
		return authcontract.LoginResult{}, ErrSessionUserUnavailable
	}
	if err := s.sessions.Touch(ctx, session.ID, now); err != nil {
		return authcontract.LoginResult{}, err
	}
	session.LastSeenAt = &now
	return authcontract.LoginResult{
		User:    storedUser.User,
		Session: session,
	}, nil
}

func (s *Service) CurrentUser(ctx context.Context, sessionID string) (userscontract.User, error) {
	result, err := s.AuthenticateSession(ctx, sessionID)
	if err != nil {
		return userscontract.User{}, err
	}
	return result.User, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrSessionNotFound
	}
	return s.sessions.Delete(ctx, sessionID)
}

func ValidateCSRF(session authcontract.Session, token string) error {
	token = strings.TrimSpace(token)
	if token == "" || !hmac.Equal([]byte(session.CSRFToken), []byte(token)) {
		return ErrCSRFTokenInvalid
	}
	return nil
}

func randomToken(prefix string, size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(bytes), nil
}

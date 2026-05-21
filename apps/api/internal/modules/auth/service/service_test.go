package service

import (
	"context"
	"errors"
	"testing"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

func TestLoginCreatesSessionAndTouchesUser(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:        1,
				Email:     "admin@srapi.local",
				Name:      "Admin",
				Status:    userscontract.StatusActive,
				Roles:     []userscontract.Role{userscontract.RoleAdmin},
				CreatedAt: now.Add(-time.Hour),
			},
		},
		password: "password123",
	}
	sessions := newMemoryStore()
	svc, err := New(users, sessions, time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.Login(context.Background(), "admin@srapi.local", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Session.ID == "" || result.Session.CSRFToken == "" {
		t.Fatal("expected generated session id and csrf token")
	}
	if !result.Session.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected expiry: %s", result.Session.ExpiresAt)
	}
	if users.lastLoginUserID != 1 {
		t.Fatalf("expected last login touch for user 1, got %d", users.lastLoginUserID)
	}
}

func TestAuthenticateSessionReturnsCurrentUserAndTouchesSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:        7,
				Email:     "user@srapi.local",
				Name:      "User",
				Status:    userscontract.StatusActive,
				Roles:     []userscontract.Role{userscontract.RoleUser},
				CreatedAt: now.Add(-time.Hour),
			},
		},
		password: "password123",
	}
	sessions := newMemoryStore()
	_, err := sessions.Create(context.Background(), authcontract.CreateSession{
		ID:        "sess_existing",
		UserID:    7,
		CSRFToken: "csrf_existing",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc, err := New(users, sessions, time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.AuthenticateSession(context.Background(), "sess_existing")
	if err != nil {
		t.Fatalf("authenticate session: %v", err)
	}
	if result.User.ID != 7 {
		t.Fatalf("expected user 7, got %d", result.User.ID)
	}
	if result.Session.LastSeenAt == nil || !result.Session.LastSeenAt.Equal(now) {
		t.Fatalf("expected last seen %s, got %v", now, result.Session.LastSeenAt)
	}
}

func TestAuthenticateSessionRejectsExpiredSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	sessions := newMemoryStore()
	_, err := sessions.Create(context.Background(), authcontract.CreateSession{
		ID:        "sess_expired",
		UserID:    7,
		CSRFToken: "csrf_existing",
		ExpiresAt: now.Add(-time.Second),
		CreatedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc, err := New(&fakeUserService{}, sessions, time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.AuthenticateSession(context.Background(), "sess_expired")
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
	if _, findErr := sessions.FindByID(context.Background(), "sess_expired"); !errors.Is(findErr, ErrSessionNotFound) {
		t.Fatalf("expected expired session to be deleted, got %v", findErr)
	}
}

func TestValidateCSRFRejectsMismatch(t *testing.T) {
	err := ValidateCSRF(authcontract.Session{CSRFToken: "csrf_expected"}, "csrf_wrong")
	if !errors.Is(err, ErrCSRFTokenInvalid) {
		t.Fatalf("expected ErrCSRFTokenInvalid, got %v", err)
	}
}

type fakeUserService struct {
	user            userscontract.StoredUser
	password        string
	lastLoginUserID int
}

func (s *fakeUserService) AuthenticatePassword(_ context.Context, email, password string) (userscontract.StoredUser, error) {
	if s.user.Email == email && s.password == password {
		return s.user, nil
	}
	return userscontract.StoredUser{}, errors.New("invalid credentials")
}

func (s *fakeUserService) FindByID(_ context.Context, id int) (userscontract.StoredUser, error) {
	if s.user.ID == id {
		return s.user, nil
	}
	return userscontract.StoredUser{}, errors.New("user not found")
}

func (s *fakeUserService) TouchLastLogin(_ context.Context, id int) error {
	s.lastLoginUserID = id
	return nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

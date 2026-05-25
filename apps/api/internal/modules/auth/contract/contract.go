package contract

import (
	"context"
	"time"

	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

type Session struct {
	ID         string
	UserID     int
	CSRFToken  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt *time.Time
}

type CreateSession struct {
	ID        string
	UserID    int
	CSRFToken string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type LoginResult struct {
	User    userscontract.User
	Session Session
}

// CleanupExpiredSessionsResult reports how many expired sessions were selected and expired.
type CleanupExpiredSessionsResult struct {
	Selected int
	Expired  int
}

type Store interface {
	Create(ctx context.Context, input CreateSession) (Session, error)
	FindByID(ctx context.Context, id string) (Session, error)
	Delete(ctx context.Context, id string) error
	Touch(ctx context.Context, id string, at time.Time) error
}

// CleanupStore removes active sessions that have passed their expiry time.
type CleanupStore interface {
	CleanupExpiredSessions(ctx context.Context, now time.Time) (CleanupExpiredSessionsResult, error)
}

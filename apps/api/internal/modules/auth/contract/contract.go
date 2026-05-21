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

type Store interface {
	Create(ctx context.Context, input CreateSession) (Session, error)
	FindByID(ctx context.Context, id string) (Session, error)
	Delete(ctx context.Context, id string) error
	Touch(ctx context.Context, id string, at time.Time) error
}

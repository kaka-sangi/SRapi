package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when no quota exists for a (user, platform) pair.
var ErrNotFound = errors.New("user platform quota not found")

// Quota is an operator-managed spend ceiling for one user on one upstream
// platform (provider family). Limit fields are decimal USD strings; a nil
// window limit means that window is uncapped.
type Quota struct {
	ID           int
	UserID       int
	Platform     string
	DailyLimit   *string
	WeeklyLimit  *string
	MonthlyLimit *string
	Currency     string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UpsertQuota is the create-or-update input keyed by (UserID, Platform).
type UpsertQuota struct {
	UserID       int
	Platform     string
	DailyLimit   *string
	WeeklyLimit  *string
	MonthlyLimit *string
	Currency     string
	Enabled      bool
}

type Store interface {
	UpsertQuota(ctx context.Context, input UpsertQuota) (Quota, error)
	DeleteByUserPlatform(ctx context.Context, userID int, platform string) error
	FindByUserPlatform(ctx context.Context, userID int, platform string) (Quota, error)
	ListByUser(ctx context.Context, userID int) ([]Quota, error)
}

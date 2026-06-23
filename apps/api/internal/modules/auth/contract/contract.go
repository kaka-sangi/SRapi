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
	User                       userscontract.User
	Session                    Session
	RequiresSecondFactor       bool
	SecondFactorChallengeID    string
	SecondFactorChallengeUntil *time.Time
}

// CleanupExpiredSessionsResult reports how many expired sessions were selected and expired.
type CleanupExpiredSessionsResult struct {
	Selected int
	Expired  int
}

type PasswordResetToken struct {
	ID        int
	UserID    int
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type CreatePasswordResetToken struct {
	UserID    int
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type EmailVerificationToken struct {
	ID        int
	UserID    int
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type CreateEmailVerificationToken struct {
	UserID    int
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type PendingOAuthIntent string

const (
	PendingOAuthIntentLogin           PendingOAuthIntent = "login"
	PendingOAuthIntentBindCurrentUser PendingOAuthIntent = "bind_current_user"
)

type PendingOAuthSession struct {
	ID                  int
	SessionTokenHash    string
	Intent              PendingOAuthIntent
	Provider            userscontract.AuthIdentityProvider
	ProviderKey         string
	ProviderSubjectHash string
	SubjectHint         string
	TargetUserID        *int
	RedirectTo          string
	ResolvedEmail       string
	DisplayName         string
	EmailVerified       bool
	AvatarURL           string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type CreatePendingOAuthSession struct {
	SessionTokenHash    string
	Intent              PendingOAuthIntent
	Provider            userscontract.AuthIdentityProvider
	ProviderKey         string
	ProviderSubjectHash string
	SubjectHint         string
	TargetUserID        *int
	RedirectTo          string
	ResolvedEmail       string
	DisplayName         string
	EmailVerified       bool
	AvatarURL           string
	ExpiresAt           time.Time
	CreatedAt           time.Time
}

type Store interface {
	Create(ctx context.Context, input CreateSession) (Session, error)
	FindByID(ctx context.Context, id string) (Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID int) error
	Touch(ctx context.Context, id string, at time.Time) error
}

// CSRFRotator is an optional Store capability for rotating CSRF tokens
// after state-changing operations. Stores that don't implement it cause
// CSRF rotation to be silently skipped (the existing token stays valid).
type CSRFRotator interface {
	UpdateCSRFToken(ctx context.Context, sessionID string, newToken string) error
}

// CleanupStore removes active sessions that have passed their expiry time.
type CleanupStore interface {
	CleanupExpiredSessions(ctx context.Context, now time.Time) (CleanupExpiredSessionsResult, error)
}

// PasswordResetStore persists hash-only password reset tokens.
type PasswordResetStore interface {
	CreatePasswordResetToken(ctx context.Context, input CreatePasswordResetToken) (PasswordResetToken, error)
	ConsumePasswordResetToken(ctx context.Context, tokenHash string, now time.Time) (PasswordResetToken, error)
}

// EmailVerificationStore persists hash-only email verification tokens.
type EmailVerificationStore interface {
	CreateEmailVerificationToken(ctx context.Context, input CreateEmailVerificationToken) (EmailVerificationToken, error)
	ConsumeEmailVerificationToken(ctx context.Context, tokenHash string, now time.Time) (EmailVerificationToken, error)
}

// PendingOAuthStore persists hash-only short-lived OAuth/OIDC decision sessions.
type PendingOAuthStore interface {
	CreatePendingOAuthSession(ctx context.Context, input CreatePendingOAuthSession) (PendingOAuthSession, error)
	FindPendingOAuthSession(ctx context.Context, sessionTokenHash string, now time.Time) (PendingOAuthSession, error)
	CompletePendingOAuthEmail(ctx context.Context, sessionTokenHash string, email string, now time.Time) (PendingOAuthSession, error)
	ConsumePendingOAuthSession(ctx context.Context, sessionTokenHash string, now time.Time) (PendingOAuthSession, error)
}

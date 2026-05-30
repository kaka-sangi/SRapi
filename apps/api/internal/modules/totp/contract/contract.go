package contract

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput         = errors.New("invalid totp input")
	ErrSecretNotFound       = errors.New("totp secret not found")
	ErrSecretDisabled       = errors.New("totp secret disabled")
	ErrSecretAlreadyEnabled = errors.New("totp secret already enabled")
	ErrInvalidCode          = errors.New("invalid totp code")
	ErrSecretDecrypt        = errors.New("totp secret decrypt failed")
	ErrRecoveryCodeRandom   = errors.New("totp recovery code random failed")
)

type Secret struct {
	ID                 int
	UserID             int
	SecretCiphertext   string
	SecretVersion      string
	Enabled            bool
	RecoveryCodeHashes []string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastUsedAt         *time.Time
}

type UpsertSecretInput struct {
	UserID             int
	SecretCiphertext   string
	SecretVersion      string
	Enabled            bool
	RecoveryCodeHashes []string
	Now                time.Time
}

type EnableSecretInput struct {
	UserID             int
	RecoveryCodeHashes []string
	Now                time.Time
}

type DisableSecretInput struct {
	UserID int
	Now    time.Time
}

type MarkUsedInput struct {
	UserID             int
	RecoveryCodeHashes []string
	LastUsedAt         time.Time
}

type Store interface {
	FindByUserID(ctx context.Context, userID int) (Secret, error)
	UpsertSetup(ctx context.Context, input UpsertSecretInput) (Secret, error)
	Enable(ctx context.Context, input EnableSecretInput) (Secret, error)
	Disable(ctx context.Context, input DisableSecretInput) error
	MarkUsed(ctx context.Context, input MarkUsedInput) (Secret, error)
}

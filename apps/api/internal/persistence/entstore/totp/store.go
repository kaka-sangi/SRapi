package totp

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entusertotpsecret "github.com/srapi/srapi/apps/api/ent/usertotpsecret"
	"github.com/srapi/srapi/apps/api/internal/modules/totp/contract"
)

var ErrInvalidStore = errors.New("invalid totp ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) FindByUserID(ctx context.Context, userID int) (contract.Secret, error) {
	if userID <= 0 {
		return contract.Secret{}, ErrInvalidStore
	}
	row, err := s.client.UserTOTPSecret.Query().
		Where(entusertotpsecret.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Secret{}, contract.ErrSecretNotFound
		}
		return contract.Secret{}, err
	}
	return toSecret(row), nil
}

func (s *Store) UpsertSetup(ctx context.Context, input contract.UpsertSecretInput) (contract.Secret, error) {
	if input.UserID <= 0 || input.SecretCiphertext == "" || input.SecretVersion == "" {
		return contract.Secret{}, ErrInvalidStore
	}
	now := normalizedNow(input.Now)
	affected, err := s.client.UserTOTPSecret.Update().
		Where(entusertotpsecret.UserIDEQ(input.UserID)).
		SetSecretCiphertext([]byte(input.SecretCiphertext)).
		SetSecretVersion(input.SecretVersion).
		SetEnabled(input.Enabled).
		SetRecoveryCodeHashesJSON(cloneStrings(input.RecoveryCodeHashes)).
		ClearLastUsedAt().
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Secret{}, err
	}
	if affected == 0 {
		row, err := s.client.UserTOTPSecret.Create().
			SetUserID(input.UserID).
			SetSecretCiphertext([]byte(input.SecretCiphertext)).
			SetSecretVersion(input.SecretVersion).
			SetEnabled(input.Enabled).
			SetRecoveryCodeHashesJSON(cloneStrings(input.RecoveryCodeHashes)).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx)
		if err != nil && ent.IsConstraintError(err) {
			return s.UpsertSetup(ctx, input)
		}
		if err != nil {
			return contract.Secret{}, err
		}
		return toSecret(row), nil
	}
	return s.FindByUserID(ctx, input.UserID)
}

func (s *Store) Enable(ctx context.Context, input contract.EnableSecretInput) (contract.Secret, error) {
	if input.UserID <= 0 {
		return contract.Secret{}, ErrInvalidStore
	}
	now := normalizedNow(input.Now)
	affected, err := s.client.UserTOTPSecret.Update().
		Where(entusertotpsecret.UserIDEQ(input.UserID)).
		SetEnabled(true).
		SetRecoveryCodeHashesJSON(cloneStrings(input.RecoveryCodeHashes)).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Secret{}, err
	}
	if affected == 0 {
		return contract.Secret{}, contract.ErrSecretNotFound
	}
	return s.FindByUserID(ctx, input.UserID)
}

func (s *Store) Disable(ctx context.Context, input contract.DisableSecretInput) error {
	if input.UserID <= 0 {
		return ErrInvalidStore
	}
	affected, err := s.client.UserTOTPSecret.Delete().
		Where(entusertotpsecret.UserIDEQ(input.UserID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrSecretNotFound
	}
	return nil
}

func (s *Store) MarkUsed(ctx context.Context, input contract.MarkUsedInput) (contract.Secret, error) {
	if input.UserID <= 0 || input.LastUsedAt.IsZero() {
		return contract.Secret{}, ErrInvalidStore
	}
	lastUsedAt := input.LastUsedAt.UTC()
	affected, err := s.client.UserTOTPSecret.Update().
		Where(entusertotpsecret.UserIDEQ(input.UserID)).
		SetRecoveryCodeHashesJSON(cloneStrings(input.RecoveryCodeHashes)).
		SetLastUsedAt(lastUsedAt).
		SetUpdatedAt(lastUsedAt).
		Save(ctx)
	if err != nil {
		return contract.Secret{}, err
	}
	if affected == 0 {
		return contract.Secret{}, contract.ErrSecretNotFound
	}
	return s.FindByUserID(ctx, input.UserID)
}

func toSecret(row *ent.UserTOTPSecret) contract.Secret {
	return contract.Secret{
		ID:                 row.ID,
		UserID:             row.UserID,
		SecretCiphertext:   string(row.SecretCiphertext),
		SecretVersion:      row.SecretVersion,
		Enabled:            row.Enabled,
		RecoveryCodeHashes: cloneStrings(row.RecoveryCodeHashesJSON),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
		LastUsedAt:         row.LastUsedAt,
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func normalizedNow(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now().UTC()
	}
	return at.UTC()
}

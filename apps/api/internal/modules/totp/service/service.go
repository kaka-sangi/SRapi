package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	totpcontract "github.com/srapi/srapi/apps/api/internal/modules/totp/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

const (
	secretVersionV1   = "v1"
	defaultIssuer     = "SRapi"
	recoveryCodeCount = 10
	recoveryCodeBytes = 10
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store     totpcontract.Store
	cryptoKey []byte
	issuer    string
	clock     Clock
}

type SetupResult struct {
	Enabled    bool
	Secret     string
	OTPAuthURL string
}

type StatusResult struct {
	Enabled      bool
	PendingSetup bool
}

type EnableResult struct {
	Enabled       bool
	RecoveryCodes []string
}

func New(store totpcontract.Store, encryptionKey string, issuer string, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	derivedKey, err := platformcrypto.DeriveAESKey(encryptionKey)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if issuer = strings.TrimSpace(issuer); issuer == "" {
		issuer = defaultIssuer
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, cryptoKey: derivedKey, issuer: issuer, clock: clock}, nil
}

func (s *Service) IsEnabled(ctx context.Context, userID int) (bool, error) {
	secret, err := s.store.FindByUserID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return secret.Enabled, nil
}

func (s *Service) Status(ctx context.Context, userID int) (StatusResult, error) {
	if userID <= 0 {
		return StatusResult{}, ErrInvalidInput
	}
	secret, err := s.store.FindByUserID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return StatusResult{}, nil
		}
		return StatusResult{}, err
	}
	return StatusResult{Enabled: secret.Enabled, PendingSetup: !secret.Enabled}, nil
}

func (s *Service) BeginSetup(ctx context.Context, user userscontract.User) (SetupResult, error) {
	if user.ID <= 0 || strings.TrimSpace(user.Email) == "" {
		return SetupResult{}, ErrInvalidInput
	}
	existing, err := s.store.FindByUserID(ctx, user.ID)
	if err != nil && !isNotFound(err) {
		return SetupResult{}, err
	}
	if err == nil && existing.Enabled {
		return SetupResult{}, ErrSecretAlreadyEnabled
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer,
		AccountName: strings.TrimSpace(user.Email),
		Period:      30,
		SecretSize:  20,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return SetupResult{}, err
	}
	secret := key.Secret()
	ciphertext, err := s.encryptSecret(secret)
	if err != nil {
		return SetupResult{}, err
	}
	now := s.clock.Now()
	stored, err := s.store.UpsertSetup(ctx, totpcontract.UpsertSecretInput{
		UserID:           user.ID,
		SecretCiphertext: ciphertext,
		SecretVersion:    secretVersionV1,
		Enabled:          false,
		Now:              now,
	})
	if err != nil {
		return SetupResult{}, err
	}
	return SetupResult{
		Enabled:    stored.Enabled,
		Secret:     secret,
		OTPAuthURL: key.URL(),
	}, nil
}

func (s *Service) Enable(ctx context.Context, userID int, code string) (EnableResult, error) {
	secret, rawSecret, err := s.enabledCandidateSecret(ctx, userID, code)
	if err != nil {
		return EnableResult{}, err
	}
	ok, err := validateTOTP(code, rawSecret, s.clock.Now())
	if err != nil || !ok {
		return EnableResult{}, ErrInvalidCode
	}
	recoveryCodes, hashes, err := s.newRecoveryCodes()
	if err != nil {
		return EnableResult{}, err
	}
	enabled, err := s.store.Enable(ctx, totpcontract.EnableSecretInput{
		UserID:             secret.UserID,
		RecoveryCodeHashes: hashes,
		Now:                s.clock.Now(),
	})
	if err != nil {
		return EnableResult{}, err
	}
	return EnableResult{Enabled: enabled.Enabled, RecoveryCodes: recoveryCodes}, nil
}

func (s *Service) Disable(ctx context.Context, userID int, code string) error {
	if err := s.VerifyLogin(ctx, userID, code); err != nil {
		return err
	}
	return s.store.Disable(ctx, totpcontract.DisableSecretInput{UserID: userID, Now: s.clock.Now()})
}

func (s *Service) VerifyLogin(ctx context.Context, userID int, code string) error {
	if userID <= 0 {
		return ErrInvalidInput
	}
	code = normalizeCode(code)
	if code == "" {
		return ErrInvalidCode
	}
	secret, err := s.store.FindByUserID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return ErrSecretNotFound
		}
		return err
	}
	if !secret.Enabled {
		return ErrSecretDisabled
	}
	rawSecret, err := s.decryptSecret(secret.SecretCiphertext)
	if err != nil {
		return ErrSecretDecrypt
	}
	ok, err := validateTOTP(code, rawSecret, s.clock.Now())
	if err == nil && ok {
		_, err = s.store.MarkUsed(ctx, totpcontract.MarkUsedInput{
			UserID:             userID,
			RecoveryCodeHashes: append([]string(nil), secret.RecoveryCodeHashes...),
			LastUsedAt:         s.clock.Now(),
		})
		return err
	}
	if err := s.consumeRecoveryCode(ctx, secret, code); err != nil {
		return ErrInvalidCode
	}
	return nil
}

func (s *Service) enabledCandidateSecret(ctx context.Context, userID int, code string) (totpcontract.Secret, string, error) {
	if userID <= 0 || normalizeCode(code) == "" {
		return totpcontract.Secret{}, "", ErrInvalidInput
	}
	secret, err := s.store.FindByUserID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return totpcontract.Secret{}, "", ErrSecretNotFound
		}
		return totpcontract.Secret{}, "", err
	}
	if secret.SecretCiphertext == "" {
		return totpcontract.Secret{}, "", ErrSecretNotFound
	}
	rawSecret, err := s.decryptSecret(secret.SecretCiphertext)
	if err != nil {
		return totpcontract.Secret{}, "", ErrSecretDecrypt
	}
	return secret, rawSecret, nil
}

func (s *Service) consumeRecoveryCode(ctx context.Context, secret totpcontract.Secret, code string) error {
	want := recoveryCodeHash(code, s.cryptoKey)
	hashes := append([]string(nil), secret.RecoveryCodeHashes...)
	for index, hash := range hashes {
		if !hmac.Equal([]byte(hash), []byte(want)) {
			continue
		}
		hashes = append(hashes[:index], hashes[index+1:]...)
		_, err := s.store.MarkUsed(ctx, totpcontract.MarkUsedInput{
			UserID:             secret.UserID,
			RecoveryCodeHashes: hashes,
			LastUsedAt:         s.clock.Now(),
		})
		return err
	}
	return ErrInvalidCode
}

func (s *Service) newRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for i := 0; i < recoveryCodeCount; i++ {
		raw := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, ErrRecoveryCodeRandom
		}
		code := strings.ToUpper(hex.EncodeToString(raw[:4])) + "-" + strings.ToUpper(hex.EncodeToString(raw[4:7])) + "-" + strings.ToUpper(hex.EncodeToString(raw[7:]))
		codes = append(codes, code)
		hashes = append(hashes, recoveryCodeHash(code, s.cryptoKey))
	}
	return codes, hashes, nil
}

func (s *Service) encryptSecret(secret string) (string, error) {
	block, err := aes.NewCipher(s.cryptoKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(secret), []byte(secretVersionV1))
	return fmt.Sprintf("%s:%s:%s", secretVersionV1, base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(ciphertext)), nil
}

func (s *Service) decryptSecret(ciphertext string) (string, error) {
	parts := strings.Split(ciphertext, ":")
	if len(parts) != 3 || parts[0] != secretVersionV1 {
		return "", ErrInvalidInput
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.cryptoKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, []byte(secretVersionV1))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validateTOTP(code string, secret string, at time.Time) (bool, error) {
	return totp.ValidateCustom(normalizeCode(code), secret, at, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
}

func recoveryCodeHash(code string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(normalizeCode(code)))
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
}

func isNotFound(err error) bool {
	return err == ErrSecretNotFound || strings.Contains(strings.ToLower(err.Error()), "not found")
}

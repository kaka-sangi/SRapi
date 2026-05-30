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
	"strconv"
	"strings"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const (
	passwordResetTokenBytes = 32
	passwordResetTTL        = 30 * time.Minute
	passwordResetTokenV1    = "v1"
)

type PasswordResetRequestResult struct {
	Accepted  bool
	ExpiresAt *time.Time
	UserID    *int
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) (PasswordResetRequestResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return PasswordResetRequestResult{}, ErrInvalidInput
	}
	result := PasswordResetRequestResult{Accepted: true}
	users, ok := s.users.(PasswordResetUserService)
	if !ok {
		return PasswordResetRequestResult{}, ErrPasswordResetUnavailable
	}
	resetStore, ok := s.sessions.(authcontract.PasswordResetStore)
	if !ok {
		return PasswordResetRequestResult{}, ErrPasswordResetUnavailable
	}
	user, err := users.FindByEmail(ctx, email)
	if err != nil {
		return result, nil
	}
	if user.ID <= 0 || user.Status != userscontract.StatusActive {
		return result, nil
	}
	if len(s.resetTokenKey) == 0 || s.events == nil {
		return PasswordResetRequestResult{}, ErrPasswordResetUnavailable
	}
	rawToken, err := randomToken("pwreset", passwordResetTokenBytes)
	if err != nil {
		return PasswordResetRequestResult{}, err
	}
	tokenHash := s.passwordResetTokenHash(rawToken)
	now := s.clock.Now()
	expiresAt := now.Add(passwordResetTTL)
	if _, err := resetStore.CreatePasswordResetToken(ctx, authcontract.CreatePasswordResetToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}); err != nil {
		return PasswordResetRequestResult{}, err
	}
	tokenCiphertext, err := s.encryptPasswordResetToken(rawToken)
	if err != nil {
		return PasswordResetRequestResult{}, err
	}
	if _, err := s.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "AuthPasswordResetRequested",
		EventVersion:   "v1",
		ProducerModule: "auth",
		AggregateType:  "user",
		AggregateID:    strconv.Itoa(user.ID),
		IdempotencyKey: "auth.password_reset:" + strconv.Itoa(user.ID) + ":" + tokenHash[:16],
		Payload: map[string]any{
			"template":               "auth.password_reset",
			"recipient_user_id":      user.ID,
			"recipient_email_hash":   emailHash(email),
			"reset_token_ciphertext": tokenCiphertext,
			"reset_token_version":    passwordResetTokenV1,
			"reset_url_path":         "/reset-password",
			"expires_at":             expiresAt.Format(time.RFC3339Nano),
		},
		Metadata: map[string]any{
			"token_delivery": "encrypted_outbox",
		},
	}); err != nil {
		return PasswordResetRequestResult{}, err
	}
	result.ExpiresAt = &expiresAt
	result.UserID = &user.ID
	return result, nil
}

func (s *Service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	token = strings.TrimSpace(token)
	if token == "" || strings.TrimSpace(newPassword) == "" || len(newPassword) < 8 {
		return ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return ErrPasswordResetUnavailable
	}
	users, ok := s.users.(PasswordResetUserService)
	if !ok {
		return ErrPasswordResetUnavailable
	}
	resetStore, ok := s.sessions.(authcontract.PasswordResetStore)
	if !ok {
		return ErrPasswordResetUnavailable
	}
	now := s.clock.Now()
	resetToken, err := resetStore.ConsumePasswordResetToken(ctx, s.passwordResetTokenHash(token), now)
	if err != nil {
		return ErrPasswordResetInvalid
	}
	if _, err := users.ResetPassword(ctx, resetToken.UserID, newPassword); err != nil {
		return ErrPasswordResetInvalid
	}
	return s.sessions.DeleteByUserID(ctx, resetToken.UserID)
}

func (s *Service) passwordResetTokenHash(token string) string {
	mac := hmac.New(sha256.New, s.resetTokenKey)
	mac.Write([]byte("password_reset:"))
	mac.Write([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) encryptPasswordResetToken(token string) (string, error) {
	block, err := aes.NewCipher(s.resetTokenKey)
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
	ciphertext := gcm.Seal(nil, nonce, []byte(token), []byte("auth.password_reset:"+passwordResetTokenV1))
	return strings.Join([]string{
		passwordResetTokenV1,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, ":"), nil
}

func emailHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

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
	emailVerificationTokenBytes = 32
	emailVerificationTTL        = 24 * time.Hour
	emailVerificationTokenV1    = "v1"
)

type EmailVerificationRequestResult struct {
	Accepted  bool
	ExpiresAt *time.Time
	UserID    *int
}

func (s *Service) RequestEmailVerification(ctx context.Context, email string) (EmailVerificationRequestResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return EmailVerificationRequestResult{}, ErrInvalidInput
	}
	result := EmailVerificationRequestResult{Accepted: true}
	users, ok := s.users.(EmailVerificationUserService)
	if !ok {
		return EmailVerificationRequestResult{}, ErrEmailVerificationUnavailable
	}
	verifyStore, ok := s.sessions.(authcontract.EmailVerificationStore)
	if !ok {
		return EmailVerificationRequestResult{}, ErrEmailVerificationUnavailable
	}
	user, err := users.FindByEmail(ctx, email)
	if err != nil {
		return result, nil
	}
	if user.ID <= 0 || user.Status != userscontract.StatusActive || user.EmailVerifiedAt != nil {
		return result, nil
	}
	if len(s.resetTokenKey) == 0 || s.events == nil {
		return EmailVerificationRequestResult{}, ErrEmailVerificationUnavailable
	}
	rawToken, err := randomToken("emailverify", emailVerificationTokenBytes)
	if err != nil {
		return EmailVerificationRequestResult{}, err
	}
	tokenHash := s.emailVerificationTokenHash(rawToken)
	now := s.clock.Now()
	expiresAt := now.Add(emailVerificationTTL)
	if _, err := verifyStore.CreateEmailVerificationToken(ctx, authcontract.CreateEmailVerificationToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}); err != nil {
		return EmailVerificationRequestResult{}, err
	}
	tokenCiphertext, err := s.encryptEmailVerificationToken(rawToken)
	if err != nil {
		return EmailVerificationRequestResult{}, err
	}
	if _, err := s.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "AuthEmailVerificationRequested",
		EventVersion:   emailVerificationTokenV1,
		ProducerModule: "auth",
		AggregateType:  "user",
		AggregateID:    strconv.Itoa(user.ID),
		IdempotencyKey: "auth.email_verification:" + strconv.Itoa(user.ID) + ":" + tokenHash[:16],
		Payload: map[string]any{
			"template":                      "auth.email_verification",
			"recipient_user_id":             user.ID,
			"recipient_email_hash":          emailHash(email),
			"verification_token_ciphertext": tokenCiphertext,
			"verification_token_version":    emailVerificationTokenV1,
			"verification_url_path":         "/verify-email",
			"expires_at":                    expiresAt.Format(time.RFC3339Nano),
		},
		Metadata: map[string]any{
			"token_delivery": "encrypted_outbox",
		},
	}); err != nil {
		return EmailVerificationRequestResult{}, err
	}
	result.ExpiresAt = &expiresAt
	result.UserID = &user.ID
	return result, nil
}

func (s *Service) ConfirmEmailVerification(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return ErrEmailVerificationUnavailable
	}
	users, ok := s.users.(EmailVerificationUserService)
	if !ok {
		return ErrEmailVerificationUnavailable
	}
	verifyStore, ok := s.sessions.(authcontract.EmailVerificationStore)
	if !ok {
		return ErrEmailVerificationUnavailable
	}
	now := s.clock.Now()
	verifyToken, err := verifyStore.ConsumeEmailVerificationToken(ctx, s.emailVerificationTokenHash(token), now)
	if err != nil {
		return ErrEmailVerificationInvalid
	}
	if _, err := users.VerifyEmail(ctx, verifyToken.UserID, now); err != nil {
		return ErrEmailVerificationInvalid
	}
	return nil
}

func (s *Service) emailVerificationTokenHash(token string) string {
	mac := hmac.New(sha256.New, s.resetTokenKey)
	mac.Write([]byte("email_verification:"))
	mac.Write([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) encryptEmailVerificationToken(token string) (string, error) {
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
	ciphertext := gcm.Seal(nil, nonce, []byte(token), []byte("auth.email_verification:"+emailVerificationTokenV1))
	return strings.Join([]string{
		emailVerificationTokenV1,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, ":"), nil
}

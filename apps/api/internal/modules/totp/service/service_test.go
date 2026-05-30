package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	totpmemory "github.com/srapi/srapi/apps/api/internal/modules/totp/store/memory"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

func TestSetupEnableAndVerifyTOTP(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := totpmemory.New()
	svc, err := New(store, "totp_test_encryption_key_32_bytes_min", "SRapi Test", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	setup, err := svc.BeginSetup(context.Background(), userscontract.User{ID: 7, Email: "user@srapi.local"})
	if err != nil {
		t.Fatalf("begin setup: %v", err)
	}
	if setup.Secret == "" || !strings.HasPrefix(setup.OTPAuthURL, "otpauth://totp/") {
		t.Fatalf("unexpected setup result: %+v", setup)
	}
	stored, err := store.FindByUserID(context.Background(), 7)
	if err != nil {
		t.Fatalf("find secret: %v", err)
	}
	if strings.Contains(stored.SecretCiphertext, setup.Secret) {
		t.Fatalf("stored secret must be encrypted: %+v", stored)
	}

	code, err := testTOTPCode(setup.Secret, now)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	enabled, err := svc.Enable(context.Background(), 7, code)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !enabled.Enabled || len(enabled.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("unexpected enable result: %+v", enabled)
	}
	if err := svc.VerifyLogin(context.Background(), 7, code); err != nil {
		t.Fatalf("verify totp login: %v", err)
	}
}

func TestVerifyLoginConsumesRecoveryCode(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := totpmemory.New()
	svc, err := New(store, "totp_test_encryption_key_32_bytes_min", "SRapi Test", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	setup, err := svc.BeginSetup(context.Background(), userscontract.User{ID: 7, Email: "user@srapi.local"})
	if err != nil {
		t.Fatalf("begin setup: %v", err)
	}
	code, err := testTOTPCode(setup.Secret, now)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	enabled, err := svc.Enable(context.Background(), 7, code)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	recoveryCode := enabled.RecoveryCodes[0]
	if err := svc.VerifyLogin(context.Background(), 7, recoveryCode); err != nil {
		t.Fatalf("verify recovery code: %v", err)
	}
	if err := svc.VerifyLogin(context.Background(), 7, recoveryCode); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected used recovery code rejection, got %v", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func testTOTPCode(secret string, at time.Time) (string, error) {
	return totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
}

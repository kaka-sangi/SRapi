package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestCurrentUserTOTPSetupEnableAndTwoFactorLogin(t *testing.T) {
	cfg := config.Load()
	cfg.Security.TOTPEncryptionKey = "totp_http_encryption_key_32_bytes_min"
	handler := New(cfg, nil)

	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/totp/status", nil)
	statusReq.AddCookie(sessionCookie)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	var statusResp apiopenapi.TOTPStatusResponse
	if err := json.NewDecoder(statusRec.Body).Decode(&statusResp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if statusResp.Data.Enabled || statusResp.Data.PendingSetup {
		t.Fatalf("expected fresh totp status, got %+v", statusResp.Data)
	}

	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/totp/setup", nil)
	setupReq.AddCookie(sessionCookie)
	setupReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	setupRec := httptest.NewRecorder()
	handler.ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("expected setup 200, got %d body=%s", setupRec.Code, setupRec.Body.String())
	}
	var setupResp apiopenapi.TOTPSetupResponse
	if err := json.NewDecoder(setupRec.Body).Decode(&setupResp); err != nil {
		t.Fatalf("decode setup: %v", err)
	}
	if setupResp.Data.Secret == "" || !strings.HasPrefix(setupResp.Data.OtpAuthUrl, "otpauth://totp/") {
		t.Fatalf("unexpected setup response: %+v", setupResp.Data)
	}

	code, err := testTOTPCode(setupResp.Data.Secret)
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	enableReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/totp/enable", strings.NewReader(`{"code":"`+code+`"}`))
	enableReq.Header.Set("Content-Type", "application/json")
	enableReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	enableReq.AddCookie(sessionCookie)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("expected enable 200, got %d body=%s", enableRec.Code, enableRec.Body.String())
	}
	var enableResp apiopenapi.TOTPEnableResponse
	if err := json.NewDecoder(enableRec.Body).Decode(&enableResp); err != nil {
		t.Fatalf("decode enable: %v", err)
	}
	if !enableResp.Data.Enabled || len(enableResp.Data.RecoveryCodes) == 0 {
		t.Fatalf("unexpected enable response: %+v", enableResp.Data)
	}

	secondLoginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@srapi.local","password":"password123"}`))
	secondLoginReq.Header.Set("Content-Type", "application/json")
	secondLoginRec := httptest.NewRecorder()
	handler.ServeHTTP(secondLoginRec, secondLoginReq)
	if secondLoginRec.Code != http.StatusAccepted {
		t.Fatalf("expected password login 202, got %d body=%s", secondLoginRec.Code, secondLoginRec.Body.String())
	}
	if len(secondLoginRec.Result().Cookies()) != 0 {
		t.Fatalf("expected no session cookie before second factor")
	}
	var challengeResp apiopenapi.LoginTwoFactorRequiredResponse
	if err := json.NewDecoder(secondLoginRec.Body).Decode(&challengeResp); err != nil {
		t.Fatalf("decode challenge response: %v", err)
	}
	if !bool(challengeResp.Data.Required) || challengeResp.Data.ChallengeId == "" {
		t.Fatalf("unexpected challenge response: %+v", challengeResp.Data)
	}

	code, err = testTOTPCode(setupResp.Data.Secret)
	if err != nil {
		t.Fatalf("generate second totp code: %v", err)
	}
	secondFactorReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/2fa", strings.NewReader(`{"challenge_id":"`+challengeResp.Data.ChallengeId+`","code":"`+code+`"}`))
	secondFactorReq.Header.Set("Content-Type", "application/json")
	secondFactorRec := httptest.NewRecorder()
	handler.ServeHTTP(secondFactorRec, secondFactorReq)
	if secondFactorRec.Code != http.StatusOK {
		t.Fatalf("expected second factor login 200, got %d body=%s", secondFactorRec.Code, secondFactorRec.Body.String())
	}
	if len(secondFactorRec.Result().Cookies()) == 0 {
		t.Fatalf("expected session cookie after second factor")
	}
	var finalLogin apiopenapi.LoginResponse
	if err := json.NewDecoder(secondFactorRec.Body).Decode(&finalLogin); err != nil {
		t.Fatalf("decode final login: %v", err)
	}
	if finalLogin.Data.CsrfToken == "" || finalLogin.Data.User.Email != "admin@srapi.local" {
		t.Fatalf("unexpected final login: %+v", finalLogin.Data)
	}
}

func testTOTPCode(secret string) (string, error) {
	return totp.GenerateCodeCustom(secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
}

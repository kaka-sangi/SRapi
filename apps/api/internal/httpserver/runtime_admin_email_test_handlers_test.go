package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestSendAdminTestEmailRequiresAdminSession(t *testing.T) {
	handler := New(config.Load(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/send-test-email", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without session, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSendAdminTestEmailRequiresCSRF(t *testing.T) {
	handler := New(config.Load(), nil)
	_, sessionCookie := mustLoginAdmin(t, handler)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/send-test-email", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSendAdminTestEmailReportsNotConfigured(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	result := mustSendAdminTestEmail(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{}`)
	if result.Ok {
		t.Fatalf("expected ok=false when SMTP not configured, got %+v", result)
	}
	if result.Status != apiopenapi.AdminTestResultStatus("failed") {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.Checks == nil {
		t.Fatalf("expected per-step checks, got nil")
	}
	checks := map[string]any(*result.Checks)
	if present, _ := checks["smtp_host_present"].(bool); present {
		t.Fatalf("expected smtp_host_present=false, got %+v", checks)
	}
	if sent, _ := checks["sent"].(bool); sent {
		t.Fatalf("expected sent=false when not configured, got %+v", checks)
	}
	if errCode, _ := checks["error"].(string); errCode != "smtp_not_configured" {
		t.Fatalf("expected error=smtp_not_configured, got %+v", checks)
	}
}

func TestSendAdminTestEmailFailsWhenSMTPHostUnreachable(t *testing.T) {
	cfg := config.Load()
	// A configured-but-unroutable host exercises the real send path: the probe
	// proceeds past the config gate and fails at dial/auth instead.
	cfg.Email.SMTPHost = "127.0.0.1"
	cfg.Email.SMTPPort = 1 // reserved/unbindable port -> connection refused
	cfg.Email.SMTPFrom = "noreply@srapi.local"
	cfg.Email.SMTPPassword = "secret"
	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	result := mustSendAdminTestEmail(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"recipient":"probe@srapi.local"}`)
	if result.Ok {
		t.Fatalf("expected ok=false when SMTP host unreachable, got %+v", result)
	}
	checks := map[string]any(*result.Checks)
	if present, _ := checks["smtp_host_present"].(bool); !present {
		t.Fatalf("expected smtp_host_present=true, got %+v", checks)
	}
	if pwd, _ := checks["smtp_password_present"].(bool); !pwd {
		t.Fatalf("expected smtp_password_present=true (sourced from static cfg), got %+v", checks)
	}
	if errCode, _ := checks["error"].(string); errCode != "send_failed" {
		t.Fatalf("expected error=send_failed, got %+v", checks)
	}
	if sent, _ := checks["sent"].(bool); sent {
		t.Fatalf("expected sent=false on send failure, got %+v", checks)
	}
}

func mustSendAdminTestEmail(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.AdminTestResult {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/send-test-email", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from send-test-email, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode test email response: %v", err)
	}
	return resp.Data
}

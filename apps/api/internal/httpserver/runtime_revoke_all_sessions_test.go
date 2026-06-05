package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestRevokeAllSessionsRequiresAuthAndCSRFThenInvalidatesSession(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	const path = "/api/v1/me/sessions/revoke-all"

	unauthReq := httptest.NewRequest(http.MethodPost, path, nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated revoke 401, got %d body=%s", unauthRec.Code, unauthRec.Body.String())
	}

	missingCSRFReq := httptest.NewRequest(http.MethodPost, path, nil)
	missingCSRFReq.AddCookie(sessionCookie)
	missingCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRec, missingCSRFReq)
	if missingCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing csrf 403, got %d body=%s", missingCSRFRec.Code, missingCSRFRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected revoke 204, got %d body=%s", rec.Code, rec.Body.String())
	}

	// The revoked cookie must no longer authenticate against a protected route.
	afterReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/auth-identities", nil)
	afterReq.AddCookie(sessionCookie)
	afterRec := httptest.NewRecorder()
	handler.ServeHTTP(afterRec, afterReq)
	if afterRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked session 401, got %d body=%s", afterRec.Code, afterRec.Body.String())
	}
}

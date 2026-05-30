package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestAdminNotificationEmailTemplateLifecycle(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/notifications/email-templates", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected template list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	list := decodeObjectResponse(t, listRec)
	templates, ok := list["templates"].([]any)
	if !ok || len(templates) < 4 {
		t.Fatalf("expected editable templates in list, got %+v", list)
	}

	noCSRFReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/notifications/email-templates/auth.password_reset", strings.NewReader(`{"subject":"Reset","html":"<p>Reset</p>"}`))
	noCSRFReq.Header.Set("Content-Type", "application/json")
	noCSRFReq.AddCookie(sessionCookie)
	noCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(noCSRFRec, noCSRFReq)
	if noCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing csrf 403, got %d body=%s", noCSRFRec.Code, noCSRFRec.Body.String())
	}

	updateBody := `{"subject":"Reset {{ recipient_name }}","html":"<p>{{recipient_name}}</p><a href=\"{{action_url}}\">Reset</a>"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/notifications/email-templates/auth.password_reset", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected template update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	updated := decodeObjectResponse(t, updateRec)
	if updated["is_custom"] != true || updated["subject"] != "Reset {{ recipient_name }}" {
		t.Fatalf("unexpected updated template: %+v", updated)
	}

	previewBody := `{"event":"auth.password_reset","subject":"Reset {{recipient_name}}","html":"<p>{{recipient_name}}</p><a href=\"{{action_url}}\">Reset</a>","variables":{"recipient_name":"<Root>","action_url":"javascript:alert(1)"}}`
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notifications/email-template-preview", strings.NewReader(previewBody))
	previewReq.Header.Set("Content-Type", "application/json")
	previewReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	previewReq.AddCookie(sessionCookie)
	previewRec := httptest.NewRecorder()
	handler.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected template preview 200, got %d body=%s", previewRec.Code, previewRec.Body.String())
	}
	preview := decodeObjectResponse(t, previewRec)
	if preview["subject"] != "Reset &lt;Root&gt;" {
		t.Fatalf("expected escaped preview subject, got %+v", preview)
	}
	html, _ := preview["html"].(string)
	if !strings.Contains(html, "&lt;Root&gt;") || strings.Contains(html, "javascript:") || !strings.Contains(html, `href=""`) {
		t.Fatalf("expected escaped preview HTML with unsafe URL blanked, got %s", html)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notifications/email-templates/auth.password_reset/restore", nil)
	restoreReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	restoreReq.AddCookie(sessionCookie)
	restoreRec := httptest.NewRecorder()
	handler.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected template restore 200, got %d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	restored := decodeObjectResponse(t, restoreRec)
	if restored["is_custom"] != false || restored["subject"] != "Reset your SRapi password" {
		t.Fatalf("unexpected restored template: %+v", restored)
	}
}

func TestAdminNotificationEmailTemplateRejectsUnsupportedPlaceholder(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/notifications/email-templates/auth.password_reset", strings.NewReader(`{"subject":"Reset {{evil}}","html":"<p>Reset</p>"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported placeholder 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func decodeObjectResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data response, got %+v", envelope)
	}
	return data
}

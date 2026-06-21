package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func mustSetMaintenance(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, enabled bool, message string, recoveryAt *time.Time) {
	t.Helper()
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Maintenance.Enabled = enabled
	settingsResp.Data.Maintenance.Message = message
	settingsResp.Data.Maintenance.ExpectedRecoveryAt = recoveryAt
	body, err := json.Marshal(settingsResp.Data)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected maintenance settings update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMaintenanceGateBlocksGatewayAndSurfacesSiteConfig(t *testing.T) {
	handler, _ := newWithServer(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken

	// Sanity-check the baseline: maintenance is off so site-config reports
	// disabled and the gateway falls through to its own auth handling (401)
	// rather than tripping the maintenance gate.
	siteResp := fetchSiteConfig(t, handler)
	if maintenance := siteConfigMaintenance(t, siteResp); maintenance["enabled"] != false {
		t.Fatalf("expected baseline maintenance disabled, got %v", maintenance)
	}
	chatRec := doGatewayPost(t, handler, "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	if chatRec.Code == http.StatusServiceUnavailable {
		t.Fatalf("baseline gateway must not 503 with maintenance disabled, body=%s", chatRec.Body.String())
	}

	// Enable maintenance with a recovery window in the near future so we can
	// assert both the structured 503 body and the Retry-After header.
	recovery := time.Now().Add(5 * time.Minute)
	mustSetMaintenance(t, handler, sessionCookie, csrfToken, true, "scheduled upgrade", &recovery)

	// site-config exposes the public summary so the web frontend can render a banner.
	siteAfter := fetchSiteConfig(t, handler)
	maintenance := siteConfigMaintenance(t, siteAfter)
	if maintenance["enabled"] != true {
		t.Fatalf("expected enabled=true after toggle, got %v", maintenance)
	}
	if maintenance["message"] != "scheduled upgrade" {
		t.Fatalf("expected message to surface to site-config, got %v", maintenance["message"])
	}
	if _, ok := maintenance["expected_recovery_at"]; !ok {
		t.Fatalf("expected recovery timestamp in site-config maintenance summary")
	}

	// OpenAI-style gateway path returns the standard gateway error envelope.
	chatRec = doGatewayPost(t, handler, "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	if chatRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /v1/chat/completions 503, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	retry := chatRec.Header().Get("Retry-After")
	if seconds, err := strconv.Atoi(strings.TrimSpace(retry)); err != nil || seconds <= 0 {
		t.Fatalf("expected positive Retry-After header for maintenance with recovery time, got %q", retry)
	}
	var gatewayErr apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&gatewayErr); err != nil {
		t.Fatalf("decode gateway error: %v", err)
	}
	if gatewayErr.Error.Type != apiopenapi.ServiceUnavailableError ||
		gatewayErr.Error.Message != "scheduled upgrade" ||
		gatewayErr.Error.Code == nil ||
		*gatewayErr.Error.Code != "maintenance" {
		t.Fatalf("unexpected gateway error during maintenance: %+v", gatewayErr.Error)
	}

	// Gemini-style gateway path returns the RPC-shaped envelope.
	geminiRec := doGatewayPost(t, handler, "/v1beta/models/gemini-2.5-pro:generateContent", `{"contents":[]}`)
	if geminiRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /v1beta/models/... 503, got %d body=%s", geminiRec.Code, geminiRec.Body.String())
	}
	var geminiErr apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(geminiRec.Body).Decode(&geminiErr); err != nil {
		t.Fatalf("decode gemini error: %v", err)
	}
	if geminiErr.Error.Status != "UNAVAILABLE" || geminiErr.Error.Message != "scheduled upgrade" {
		t.Fatalf("unexpected gemini error during maintenance: %+v", geminiErr.Error)
	}

	// Admin surface remains reachable so the operator can flip the flag back off.
	adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	adminReq.AddCookie(sessionCookie)
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin settings must remain reachable during maintenance, got %d body=%s", adminRec.Code, adminRec.Body.String())
	}

	// Disabling maintenance restores the gateway pipeline.
	mustSetMaintenance(t, handler, sessionCookie, csrfToken, false, "", nil)
	afterDisable := doGatewayPost(t, handler, "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	if afterDisable.Code == http.StatusServiceUnavailable {
		var maybe apiopenapi.GatewayErrorResponse
		_ = json.NewDecoder(afterDisable.Body).Decode(&maybe)
		if maybe.Error.Code != nil && *maybe.Error.Code == "maintenance" {
			t.Fatalf("gateway still reporting maintenance after disable: %+v", maybe.Error)
		}
	}
}

func TestMaintenanceNormalizationDropsStalePromisedRecovery(t *testing.T) {
	handler, _ := newWithServer(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken

	// Persist a recovery time that has already passed; the normalizer must
	// drop it so the public banner does not display a stale promise.
	past := time.Now().Add(-1 * time.Hour)
	mustSetMaintenance(t, handler, sessionCookie, csrfToken, true, "ongoing", &past)

	resp := mustGetAdminSettings(t, handler, sessionCookie)
	if resp.Data.Maintenance.ExpectedRecoveryAt != nil {
		t.Fatalf("expected past recovery timestamp to be cleared, got %v", resp.Data.Maintenance.ExpectedRecoveryAt)
	}

	site := fetchSiteConfig(t, handler)
	maintenance := siteConfigMaintenance(t, site)
	if _, ok := maintenance["expected_recovery_at"]; ok {
		t.Fatalf("site-config must not surface stale recovery timestamp, got %v", maintenance)
	}
}

func fetchSiteConfig(t *testing.T, handler http.Handler) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/site-config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected site-config 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode site-config: %v", err)
	}
	return payload
}

func siteConfigMaintenance(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("site-config missing data: %v", payload)
	}
	maintenance, ok := data["maintenance"].(map[string]any)
	if !ok {
		t.Fatalf("site-config missing maintenance summary: %v", data)
	}
	return maintenance
}

func doGatewayPost(t *testing.T, handler http.Handler, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

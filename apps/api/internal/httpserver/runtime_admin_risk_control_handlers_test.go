package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminRiskControlConfigReturnsEmptyArrays(t *testing.T) {
	handler := New(config.Load(), nil)
	_, sessionCookie := mustLoginAdmin(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/risk-control/config", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected risk config 200, got %d body=%s", response.Code, response.Body.String())
	}

	var body apiopenapi.RiskControlConfigResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode risk config response: %v", err)
	}
	if body.Data.BlockedCountries == nil {
		t.Fatal("expected blocked_countries to be an empty array, got nil")
	}
	if body.Data.BlockedIps == nil {
		t.Fatal("expected blocked_ips to be an empty array, got nil")
	}
}

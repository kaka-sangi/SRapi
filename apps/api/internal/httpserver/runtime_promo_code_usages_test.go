package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminPromoCodeUsagesListsRedemptionsAndIsAdminGated(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promo-codes",
		strings.NewReader(`{"code":"USAGE10","discount_type":"amount","discount_value":"10.00","currency":"USD"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(sessionCookie)
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create promo code, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp apiopenapi.PromoCodeResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode promo code response: %v", err)
	}
	promoID := string(createResp.Data.Id)

	// Unauthenticated callers cannot read redemptions.
	anonReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promo-codes/"+promoID+"/usages", nil)
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusForbidden {
		t.Fatalf("expected unauthenticated usages 403, got %d", anonRec.Code)
	}

	// Admin sees an (empty, freshly-created) redemption list with the right shape.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promo-codes/"+promoID+"/usages", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected usages 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.PromoCodeUsageListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode usages response: %v", err)
	}
	if listResp.Data == nil {
		t.Fatal("expected a (possibly empty) data array, got nil")
	}
}

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

func TestCurrentUserRedeemCodeCreditsBalanceOnce(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateAdminRedeemCode(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"code":"WELCOME10","type":"balance","value":"10","currency":"USD","max_redemptions":1}`)

	missingCSRFReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/redeem-codes/redeem", strings.NewReader(`{"code":"WELCOME10"}`))
	missingCSRFReq.Header.Set("Content-Type", "application/json")
	missingCSRFReq.AddCookie(sessionCookie)
	missingCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRec, missingCSRFReq)
	if missingCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing csrf 403, got %d body=%s", missingCSRFRec.Code, missingCSRFRec.Body.String())
	}

	redeemResp := mustRedeemCurrentUserCode(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"code":" welcome10 "}`)
	if redeemResp.Data.AlreadyRedeemed || redeemResp.Data.Redemption.Amount != "10.00000000" || redeemResp.Data.Redemption.BalanceAfter != "10.00000000" {
		t.Fatalf("unexpected redemption response: %+v", redeemResp.Data)
	}

	repeated := mustRedeemCurrentUserCode(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"code":"WELCOME10"}`)
	if !repeated.Data.AlreadyRedeemed || repeated.Data.Redemption.Id != redeemResp.Data.Redemption.Id {
		t.Fatalf("expected repeat redemption to be idempotent, got %+v", repeated.Data)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/balance", nil)
	balanceReq.AddCookie(sessionCookie)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("expected balance 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp apiopenapi.UserBalanceResponse
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if balanceResp.Data.Balance != "10.00000000" {
		t.Fatalf("balance = %s, want 10.00000000", balanceResp.Data.Balance)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1/balance-history", nil)
	historyReq.AddCookie(sessionCookie)
	historyRec := httptest.NewRecorder()
	handler.ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("expected history 200, got %d body=%s", historyRec.Code, historyRec.Body.String())
	}
	var historyResp apiopenapi.BillingLedgerListResponse
	if err := json.NewDecoder(historyRec.Body).Decode(&historyResp); err != nil {
		t.Fatalf("decode balance history: %v", err)
	}
	if len(historyResp.Data) != 1 || historyResp.Data[0].Type != apiopenapi.RedeemCodeCredit {
		t.Fatalf("unexpected balance history: %+v", historyResp.Data)
	}
}

func mustCreateAdminRedeemCode(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.RedeemCodeResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/redeem-codes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected redeem code create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.RedeemCodeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode redeem code response: %v", err)
	}
	return resp
}

func mustRedeemCurrentUserCode(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.RedeemCodeRedemptionResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/redeem-codes/redeem", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected redeem 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.RedeemCodeRedemptionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode redeem response: %v", err)
	}
	return resp
}

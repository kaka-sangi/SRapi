package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestBatchGetAdminAccountsUsageTodayParsing mirrors the iter-24
// users-spending-today batch test. The happy path (real usage rows
// roll up into per-account totals) is exercised by the single-account
// usage-today flow; this test guards iter-23's batch endpoint's input
// handling so first-paint behaviour (empty list, unknown ids) and
// bad-input refusal don't silently regress.
func TestBatchGetAdminAccountsUsageTodayParsing(t *testing.T) {
	handler := New(config.Load(), nil)
	_, sessionCookie := mustLoginAdmin(t, handler)

	// Empty ids returns 200 with no rows — admin tables paint empty first.
	emptyRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/usage-today/batch?account_ids=")
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("empty ids: want 200, got %d body=%s", emptyRec.Code, emptyRec.Body.String())
	}
	var empty apiopenapi.BatchAccountUsageTodayResponse
	if err := json.NewDecoder(emptyRec.Body).Decode(&empty); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if len(empty.Data) != 0 {
		t.Fatalf("empty ids: want zero rows, got %d", len(empty.Data))
	}

	// Unknown id should round-trip as a zero-traffic row, not drop out — the
	// frontend keys rows by id so a missing row would break the join.
	unknownRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/usage-today/batch?account_ids=99999")
	if unknownRec.Code != http.StatusOK {
		t.Fatalf("unknown id: want 200, got %d", unknownRec.Code)
	}
	var unknown apiopenapi.BatchAccountUsageTodayResponse
	if err := json.NewDecoder(unknownRec.Body).Decode(&unknown); err != nil {
		t.Fatalf("decode unknown: %v", err)
	}
	if len(unknown.Data) != 1 {
		t.Fatalf("unknown id: want 1 zero-row, got %d", len(unknown.Data))
	}
	row := unknown.Data[0]
	if row.AccountId != "99999" || row.Requests != 0 || row.SuccessRate != 0 {
		t.Fatalf("unknown id zero-row mismatch: %+v", row)
	}

	// Bad input is a 400.
	badRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/usage-today/batch?account_ids=foo")
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad ids: want 400, got %d", badRec.Code)
	}

	// Auth gate.
	anonReq, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/accounts/usage-today/batch?account_ids=1", nil)
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusForbidden {
		t.Fatalf("anon: want 403, got %d", anonRec.Code)
	}
}

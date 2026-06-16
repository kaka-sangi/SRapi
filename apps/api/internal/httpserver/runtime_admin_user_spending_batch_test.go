package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestBatchGetAdminUsersSpendingTodayParsing mirrors iter 24's parsing-focused
// test for the user-attributes batch endpoint. The happy path (real usage rows
// roll up into per-user totals) is exercised by the iter-23 single-account
// usage-today flow; this test guards the batch endpoint's input handling.
func TestBatchGetAdminUsersSpendingTodayParsing(t *testing.T) {
	handler := New(config.Load(), nil)
	_, sessionCookie := mustLoginAdmin(t, handler)

	// Empty user_ids returns 200 with no rows — first paint of a list view
	// passes an empty list and we shouldn't 400 on it.
	emptyRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/users/spending-today/batch?user_ids=")
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("empty user_ids: want 200, got %d body=%s", emptyRec.Code, emptyRec.Body.String())
	}
	var empty apiopenapi.BatchUserSpendingTodayResponse
	if err := json.NewDecoder(emptyRec.Body).Decode(&empty); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if len(empty.Data) != 0 {
		t.Fatalf("empty user_ids: want zero rows, got %d", len(empty.Data))
	}

	// Unknown id should round-trip as a zero-traffic row, not drop out.
	unknownRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/users/spending-today/batch?user_ids=99999")
	if unknownRec.Code != http.StatusOK {
		t.Fatalf("unknown user_id: want 200, got %d", unknownRec.Code)
	}
	var unknown apiopenapi.BatchUserSpendingTodayResponse
	if err := json.NewDecoder(unknownRec.Body).Decode(&unknown); err != nil {
		t.Fatalf("decode unknown: %v", err)
	}
	if len(unknown.Data) != 1 {
		t.Fatalf("unknown user_id: want 1 zero-row, got %d", len(unknown.Data))
	}
	row := unknown.Data[0]
	if row.UserId != "99999" || row.Requests != 0 || row.SuccessRate != 0 {
		t.Fatalf("unknown user_id zero-row mismatch: %+v", row)
	}

	// Bad input is a 400 — a typo'd id parameter shouldn't masquerade as success.
	badRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/users/spending-today/batch?user_ids=foo")
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad user_ids: want 400, got %d", badRec.Code)
	}

	// Auth gate.
	anonReq, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/users/spending-today/batch?user_ids=1", nil)
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusForbidden {
		t.Fatalf("anon: want 403, got %d", anonRec.Code)
	}
}

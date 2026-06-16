package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestBatchListAdminUserAttributeValuesParsing covers the request parsing for
// the iter-24 batch endpoint: empty user_ids returns 200 with no rows, an
// unknown id silently drops, and a non-numeric piece returns 400. The full
// per-user happy path is already covered by the per-user endpoint that the
// iter-11 dialog drives.
func TestBatchListAdminUserAttributeValuesParsing(t *testing.T) {
	handler := New(config.Load(), nil)
	_, sessionCookie := mustLoginAdmin(t, handler)

	// Empty user_ids must return 200 with an empty data array — list views
	// pass an empty list on first paint and we shouldn't 400 on it.
	emptyRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/users/attributes/batch?user_ids=")
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("empty user_ids: expected 200, got %d body=%s", emptyRec.Code, emptyRec.Body.String())
	}
	var empty apiopenapi.BatchUserAttributeValuesResponse
	if err := json.NewDecoder(emptyRec.Body).Decode(&empty); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if len(empty.Data) != 0 {
		t.Fatalf("empty user_ids: expected zero rows, got %d", len(empty.Data))
	}

	// Unknown ids silently drop — handler shouldn't 404 on stale list rows.
	unknownRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/users/attributes/batch?user_ids=99999")
	if unknownRec.Code != http.StatusOK {
		t.Fatalf("unknown user_ids: expected 200, got %d", unknownRec.Code)
	}

	// Bad input is a 400 — a typo'd id parameter shouldn't masquerade as success.
	badRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/users/attributes/batch?user_ids=foo")
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad user_ids: expected 400, got %d", badRec.Code)
	}

	// Auth gate.
	anonReq, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/users/attributes/batch?user_ids=1", nil)
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusForbidden {
		t.Fatalf("anon: expected 403, got %d", anonRec.Code)
	}
}

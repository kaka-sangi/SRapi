package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestAdminScheduledTestPlanProbeModelRoundTrip(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scheduled-test-plans", strings.NewReader(`{
		"name":"probe model plan",
		"enabled":true,
		"scope_type":"all",
		"interval_seconds":3600,
		"cron_expression":"0 */6 * * *",
		"probe_model":"gpt-probe"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create scheduled test 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data scheduledTestPlanPayload `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Data.ProbeModel != "gpt-probe" || created.Data.CronExpression != "0 */6 * * *" {
		t.Fatalf("expected probe model and cron round trip, got %+v", created.Data)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/scheduled-test-plans/"+strconv.Itoa(created.Data.ID), strings.NewReader(`{"probe_model":"gpt-updated"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update scheduled test 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	var updated struct {
		Data scheduledTestPlanPayload `json:"data"`
	}
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Data.ProbeModel != "gpt-updated" {
		t.Fatalf("expected updated probe model, got %+v", updated.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduled-test-plans", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list scheduled tests 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Data []scheduledTestPlanPayload `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	for _, plan := range listed.Data {
		if plan.ID == created.Data.ID {
			if plan.ProbeModel != "gpt-updated" {
				t.Fatalf("expected listed probe model to match update, got %+v", plan)
			}
			return
		}
	}
	t.Fatalf("created scheduled test plan missing from list: %+v", listed.Data)
}

package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminPayloadRulesUseOpenAPIWireTypes(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken

	createResp := createAdminPayloadRule(t, handler, sessionCookie, csrfToken, `{"name":"openapi-payload-rule","enabled":true,"priority":-2,"action":"override","match_model":"gpt-*","match_protocol":"openai-compatible","params":{"temperature":0.2,"metadata.trace":"enabled"}}`)
	if createResp.Data.Id == 0 ||
		createResp.Data.Name != "openapi-payload-rule" ||
		!createResp.Data.Enabled ||
		createResp.Data.Priority != -2 ||
		createResp.Data.Action != apiopenapi.PayloadRuleActionOverride ||
		createResp.Data.MatchModel != "gpt-*" ||
		createResp.Data.MatchProtocol != "openai-compatible" ||
		createResp.Data.Params["metadata.trace"] != "enabled" {
		t.Fatalf("unexpected payload rule create response: %+v", createResp.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payload-rules?page=1&page_size=10", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected payload rule list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.PayloadRuleListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode payload rule list response: %v", err)
	}
	if listResp.Pagination.Total != 1 || !payloadRuleListHasName(listResp.Data, "openapi-payload-rule") {
		t.Fatalf("unexpected payload rule list: %+v pagination=%+v", listResp.Data, listResp.Pagination)
	}

	updateResp := updateAdminPayloadRule(t, handler, sessionCookie, csrfToken, createResp.Data.Id, `{"enabled":false,"priority":8,"action":"filter","params":{"metadata.trace":true}}`)
	if updateResp.Data.Enabled ||
		updateResp.Data.Priority != 8 ||
		updateResp.Data.Action != apiopenapi.PayloadRuleActionFilter ||
		updateResp.Data.Params["metadata.trace"] != true {
		t.Fatalf("unexpected payload rule update response: %+v", updateResp.Data)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/payload-rules/"+strconv.FormatInt(createResp.Data.Id, 10), nil)
	deleteReq.Header.Set("X-CSRF-Token", csrfToken)
	deleteReq.AddCookie(sessionCookie)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected payload rule delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	var deleteResp apiopenapi.DeleteResponse
	if err := json.NewDecoder(deleteRec.Body).Decode(&deleteResp); err != nil {
		t.Fatalf("decode payload rule delete response: %v", err)
	}
	if !deleteResp.Data.Deleted {
		t.Fatalf("unexpected payload rule delete response: %+v", deleteResp.Data)
	}
}

func createAdminPayloadRule(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.PayloadRuleResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/payload-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected payload rule create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.PayloadRuleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode payload rule create response: %v", err)
	}
	return resp
}

func updateAdminPayloadRule(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, ruleID int64, body string) apiopenapi.PayloadRuleResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/payload-rules/"+strconv.FormatInt(ruleID, 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected payload rule update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.PayloadRuleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode payload rule update response: %v", err)
	}
	return resp
}

func payloadRuleListHasName(items []apiopenapi.PayloadRule, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

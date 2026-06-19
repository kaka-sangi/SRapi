package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminErrorPassthroughRulesUseOpenAPIWireTypesAndFeedGateway(t *testing.T) {
	handler, srv := newWithServer(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken

	createResp := createAdminErrorPassthroughRule(t, handler, sessionCookie, csrfToken, `{"name":"openapi-error-rule","enabled":true,"priority":3,"action":"mask","status_codes":[400],"classes":["invalid_request"],"keywords":["temperature"],"response_code":422,"custom_message":" upstream rejected temperature "}`)
	if createResp.Data.Id == 0 ||
		createResp.Data.Name != "openapi-error-rule" ||
		!createResp.Data.Enabled ||
		createResp.Data.Priority != 3 ||
		createResp.Data.Action != apiopenapi.ErrorPassthroughRuleActionMask ||
		len(createResp.Data.StatusCodes) != 1 ||
		createResp.Data.StatusCodes[0] != http.StatusBadRequest ||
		createResp.Data.ResponseStatus == nil ||
		*createResp.Data.ResponseStatus != http.StatusUnprocessableEntity ||
		createResp.Data.ResponseCode == nil ||
		*createResp.Data.ResponseCode != http.StatusUnprocessableEntity ||
		createResp.Data.CustomMessage == nil ||
		*createResp.Data.CustomMessage != "upstream rejected temperature" {
		t.Fatalf("unexpected error passthrough create response: %+v", createResp.Data)
	}

	providerErr := provideradaptercontract.ProviderError{
		Class:      "invalid_request",
		StatusCode: http.StatusBadRequest,
		Message:    `{"error":{"message":"temperature is unsupported"}}`,
	}
	gatewayResp := srv.gatewayPublicErrorResponse(providerErr, "invalid_request", http.StatusBadRequest, nil)
	if gatewayResp.Status != http.StatusUnprocessableEntity ||
		gatewayResp.Message != "upstream rejected temperature" {
		t.Fatalf("expected admin rule to drive gateway response, got %+v", gatewayResp)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/error-passthrough-rules?page=1&page_size=10", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected error passthrough list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.ErrorPassthroughRuleListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode error passthrough list response: %v", err)
	}
	if listResp.Pagination.Total != 1 || !errorPassthroughListHasName(listResp.Data, "openapi-error-rule") {
		t.Fatalf("unexpected error passthrough list: %+v pagination=%+v", listResp.Data, listResp.Pagination)
	}

	updateResp := updateAdminErrorPassthroughRule(t, handler, sessionCookie, csrfToken, createResp.Data.Id, `{"enabled":false,"response_code":0,"custom_message":""}`)
	if updateResp.Data.Enabled ||
		len(updateResp.Data.StatusCodes) != 1 ||
		updateResp.Data.StatusCodes[0] != http.StatusBadRequest ||
		len(updateResp.Data.Classes) != 1 ||
		updateResp.Data.Classes[0] != "invalid_request" ||
		len(updateResp.Data.Keywords) != 1 ||
		updateResp.Data.Keywords[0] != "temperature" ||
		updateResp.Data.ResponseStatus != nil ||
		updateResp.Data.ResponseCode != nil ||
		updateResp.Data.CustomMessage != nil {
		t.Fatalf("unexpected error passthrough update response: %+v", updateResp.Data)
	}

	afterDisable := srv.gatewayPublicErrorResponse(providerErr, "invalid_request", http.StatusBadRequest, nil)
	if afterDisable.Status != http.StatusBadRequest ||
		afterDisable.Message != "provider rejected request" {
		t.Fatalf("expected disabled admin rule to stop affecting gateway response, got %+v", afterDisable)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/error-passthrough-rules/"+strconv.FormatInt(createResp.Data.Id, 10), nil)
	deleteReq.Header.Set("X-CSRF-Token", csrfToken)
	deleteReq.AddCookie(sessionCookie)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected error passthrough delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	var deleteResp apiopenapi.DeleteResponse
	if err := json.NewDecoder(deleteRec.Body).Decode(&deleteResp); err != nil {
		t.Fatalf("decode error passthrough delete response: %v", err)
	}
	if !deleteResp.Data.Deleted {
		t.Fatalf("unexpected error passthrough delete response: %+v", deleteResp.Data)
	}
}

func createAdminErrorPassthroughRule(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ErrorPassthroughRuleResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/error-passthrough-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected error passthrough create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ErrorPassthroughRuleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error passthrough create response: %v", err)
	}
	return resp
}

func updateAdminErrorPassthroughRule(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, ruleID int64, body string) apiopenapi.ErrorPassthroughRuleResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/error-passthrough-rules/"+strconv.FormatInt(ruleID, 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected error passthrough update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ErrorPassthroughRuleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error passthrough update response: %v", err)
	}
	return resp
}

func errorPassthroughListHasName(items []apiopenapi.ErrorPassthroughRule, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

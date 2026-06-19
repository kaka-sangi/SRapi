package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	admincontrolmemory "github.com/srapi/srapi/apps/api/internal/modules/admin_control/store/memory"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type failingRiskConfigStore struct {
	*admincontrolmemory.Store
}

func (s *failingRiskConfigStore) Get(ctx context.Context, key string) (map[string]any, bool, error) {
	if key == "admin_control.risk_config" {
		return nil, false, errors.New("risk config unavailable")
	}
	return s.Store.Get(ctx, key)
}

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

func TestGatewayRiskControlSkipsGateWhenConfigUnavailable(t *testing.T) {
	adminSvc, err := admincontrolservice.New(&failingRiskConfigStore{Store: admincontrolmemory.New()}, nil)
	if err != nil {
		t.Fatalf("new admin control service: %v", err)
	}
	rt := &runtimeState{adminControl: adminSvc}
	err = rt.enforceGatewayRiskControl(t.Context(), apikeycontract.AuthResult{
		UserID: 1,
		Key:    apikeycontract.APIKey{ID: 2},
	}, "203.0.113.44")
	if err != nil {
		t.Fatalf("risk control should fail open when config is unavailable: %v", err)
	}
}

func TestGatewayRiskControlEnforceBlocksConfiguredIP(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	configReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/risk-control/config", strings.NewReader(`{"enabled":true,"mode":"enforce","max_failed_requests_per_minute":0,"max_cost_per_day":"0","cooldown_seconds":60,"blocked_countries":[],"blocked_ips":["203.0.113.0/24"]}`))
	configReq.Header.Set("Content-Type", "application/json")
	configReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	configReq.AddCookie(sessionCookie)
	configRec := httptest.NewRecorder()
	handler.ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("expected risk config update 200, got %d body=%s", configRec.Code, configRec.Body.String())
	}

	gatewayReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"unused","messages":[{"role":"user","content":"blocked"}]}`))
	gatewayReq.Header.Set("Content-Type", "application/json")
	gatewayReq.Header.Set("Authorization", "Bearer "+apiKey)
	gatewayReq.Header.Set("X-Forwarded-For", "203.0.113.44")
	gatewayRec := httptest.NewRecorder()
	handler.ServeHTTP(gatewayRec, gatewayReq)
	if gatewayRec.Code != http.StatusForbidden {
		t.Fatalf("expected gateway risk block 403, got %d body=%s", gatewayRec.Code, gatewayRec.Body.String())
	}

	logReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/risk-control/logs?level=block", nil)
	logReq.AddCookie(sessionCookie)
	logRec := httptest.NewRecorder()
	handler.ServeHTTP(logRec, logReq)
	if logRec.Code != http.StatusOK {
		t.Fatalf("expected risk logs 200, got %d body=%s", logRec.Code, logRec.Body.String())
	}
	var logs apiopenapi.RiskControlLogListResponse
	if err := json.NewDecoder(logRec.Body).Decode(&logs); err != nil {
		t.Fatalf("decode risk logs: %v", err)
	}
	if len(logs.Data) != 1 || logs.Data[0].Reason != "blocked_ip" || logs.Data[0].Action != "gateway.block" {
		t.Fatalf("expected blocked_ip log, got %+v", logs.Data)
	}
}

func TestGatewayRiskControlMonitorLogsConfiguredIPWithoutBlocking(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"risk monitor ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"risk-monitor-provider","display_name":"Risk Monitor Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"risk-monitor-model","display_name":"Risk Monitor Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"risk-monitor-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"risk-monitor-account","runtime_class":"api_key","credential":{"api_key":"risk-monitor-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	configReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/risk-control/config", strings.NewReader(`{"enabled":true,"mode":"monitor","max_failed_requests_per_minute":0,"max_cost_per_day":"0","cooldown_seconds":60,"blocked_countries":[],"blocked_ips":["198.51.100.7"]}`))
	configReq.Header.Set("Content-Type", "application/json")
	configReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	configReq.AddCookie(sessionCookie)
	configRec := httptest.NewRecorder()
	handler.ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("expected risk config update 200, got %d body=%s", configRec.Code, configRec.Body.String())
	}

	gatewayReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"risk-monitor-model","messages":[{"role":"user","content":"monitor"}]}`))
	gatewayReq.Header.Set("Content-Type", "application/json")
	gatewayReq.Header.Set("Authorization", "Bearer "+apiKey)
	gatewayReq.Header.Set("X-Forwarded-For", "198.51.100.7")
	gatewayRec := httptest.NewRecorder()
	handler.ServeHTTP(gatewayRec, gatewayReq)
	if gatewayRec.Code != http.StatusOK {
		t.Fatalf("expected monitor mode gateway 200, got %d body=%s", gatewayRec.Code, gatewayRec.Body.String())
	}

	logReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/risk-control/logs?level=warn", nil)
	logReq.AddCookie(sessionCookie)
	logRec := httptest.NewRecorder()
	handler.ServeHTTP(logRec, logReq)
	if logRec.Code != http.StatusOK {
		t.Fatalf("expected risk logs 200, got %d body=%s", logRec.Code, logRec.Body.String())
	}
	var logs apiopenapi.RiskControlLogListResponse
	if err := json.NewDecoder(logRec.Body).Decode(&logs); err != nil {
		t.Fatalf("decode risk logs: %v", err)
	}
	if len(logs.Data) != 1 || logs.Data[0].Reason != "blocked_ip" || logs.Data[0].Action != "gateway.risk_detected" {
		t.Fatalf("expected monitor warning log, got %+v", logs.Data)
	}
}

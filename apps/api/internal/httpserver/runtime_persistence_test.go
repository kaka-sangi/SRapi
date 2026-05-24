package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/config"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/persistence/entstore"

	_ "github.com/mattn/go-sqlite3"
)

func TestInjectedPersistentStoresSurviveRuntimeRebuild(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, persistenceSQLiteDSN(t))
	defer client.Close()

	stores, err := entstore.New(client)
	if err != nil {
		t.Fatalf("new ent stores: %v", err)
	}

	cfg := config.Load()
	handler := New(cfg, nil,
		WithUserStore(stores.Users),
		WithAPIKeyStore(stores.APIKeys),
		WithProviderStore(stores.Providers),
		WithModelStore(stores.Models),
		WithAccountStore(stores.Accounts),
		WithAuditStore(stores.Audit),
		WithAuthSessionStore(stores.AuthSessions),
		WithBillingStore(stores.Billing),
		WithEventStore(stores.Events),
		WithSchedulerStore(stores.Scheduler),
		WithUsageStore(stores.Usage),
		WithDatabasePinger(stubDependencyPinger{}),
		WithRedisPinger(stubDependencyPinger{}),
	)

	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, plaintextKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"persist-provider","display_name":"Persist Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"persist-model","display_name":"Persist Model","status":"active"}`)
	mappingBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"persist-upstream","status":"active"}`
	_ = mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), mappingBody)
	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"persist-account","runtime_class":"api_key","credential":{"api_key":"persist-secret"},"status":"active"}`
	_ = mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"persist-model","messages":[{"role":"user","content":"persist usage"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+plaintextKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected persisted runtime chat 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	restarted := New(cfg, nil,
		WithUserStore(stores.Users),
		WithAPIKeyStore(stores.APIKeys),
		WithProviderStore(stores.Providers),
		WithModelStore(stores.Models),
		WithAccountStore(stores.Accounts),
		WithAuditStore(stores.Audit),
		WithAuthSessionStore(stores.AuthSessions),
		WithBillingStore(stores.Billing),
		WithEventStore(stores.Events),
		WithSchedulerStore(stores.Scheduler),
		WithUsageStore(stores.Usage),
		WithDatabasePinger(stubDependencyPinger{}),
		WithRedisPinger(stubDependencyPinger{}),
	)

	restartedLoginResp, restartedSessionCookie := mustLoginAdmin(t, restarted)
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	listReq.AddCookie(restartedSessionCookie)
	listRec := httptest.NewRecorder()
	restarted.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected persisted api key list 200, got %d", listRec.Code)
	}
	var listResp apiopenapi.ApiKeyListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode api key list: %v", err)
	}
	if len(listResp.Data) != 1 {
		t.Fatalf("expected one persisted api key, got %d", len(listResp.Data))
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	currentReq.AddCookie(sessionCookie)
	currentRec := httptest.NewRecorder()
	restarted.ServeHTTP(currentRec, currentReq)
	if currentRec.Code != http.StatusOK {
		t.Fatalf("expected persisted session cookie to authenticate after restart, got %d body=%s", currentRec.Code, currentRec.Body.String())
	}
	persistedSessionKey := mustCreateAPIKey(t, restarted, sessionCookie, loginResp.Data.CsrfToken, `{"name":"persisted-session","scopes":["gateway:invoke"]}`)
	if persistedSessionKey.Data.PlaintextKey == "" {
		t.Fatal("expected persisted session csrf token to authorize writes after restart")
	}

	modelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+plaintextKey)
	modelsRec := httptest.NewRecorder()
	restarted.ServeHTTP(modelsRec, modelsReq)
	if modelsRec.Code != http.StatusOK {
		t.Fatalf("expected persisted api key to authenticate after restart, got %d", modelsRec.Code)
	}

	prefix, ok := apikeyservice.PrefixFromPlaintext(plaintextKey)
	if !ok {
		t.Fatalf("expected generated plaintext key to have prefix")
	}
	stored, err := stores.APIKeys.FindByPrefix(t.Context(), prefix)
	if err != nil {
		t.Fatalf("find persisted api key: %v", err)
	}
	if stored.Hash == "" {
		t.Fatal("expected database store to retain only key hash")
	}
	if stored.LastUsedAt == nil {
		t.Fatal("expected gateway authentication to touch last_used_at")
	}

	assertListCount(t, restarted, restartedSessionCookie, "/api/v1/admin/providers", func(payload []byte) int {
		var resp apiopenapi.ProviderListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode providers list: %v", err)
		}
		return len(resp.Data)
	}, 2)
	assertListCount(t, restarted, restartedSessionCookie, "/api/v1/admin/models", func(payload []byte) int {
		var resp apiopenapi.ModelListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode models list: %v", err)
		}
		return len(resp.Data)
	}, 2)
	assertListCount(t, restarted, restartedSessionCookie, "/api/v1/admin/accounts", func(payload []byte) int {
		var resp apiopenapi.ProviderAccountListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode accounts list: %v", err)
		}
		return len(resp.Data)
	}, 2)
	assertListCount(t, restarted, restartedSessionCookie, "/api/v1/admin/usage-logs?model=persist-model", func(payload []byte) int {
		var resp apiopenapi.UsageLogListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode usage logs: %v", err)
		}
		if len(resp.Data) == 1 && resp.Data[0].RequestId == "" {
			t.Fatal("expected persisted usage log request id")
		}
		return len(resp.Data)
	}, 1)

	schedulerReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=persist-model", nil)
	schedulerReq.AddCookie(restartedSessionCookie)
	schedulerRec := httptest.NewRecorder()
	restarted.ServeHTTP(schedulerRec, schedulerReq)
	if schedulerRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decision list 200, got %d body=%s", schedulerRec.Code, schedulerRec.Body.String())
	}
	var schedulerResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(schedulerRec.Body).Decode(&schedulerResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(schedulerResp.Data) != 1 {
		t.Fatalf("expected one persisted scheduler decision, got %d", len(schedulerResp.Data))
	}
	if !strings.HasPrefix(schedulerResp.Data[0].StrategyConfigHash, "sha256:") {
		t.Fatalf("expected persisted strategy config hash, got %q", schedulerResp.Data[0].StrategyConfigHash)
	}

	assertListCount(t, restarted, restartedSessionCookie, "/api/v1/admin/billing-ledger?reference_type=usage_log", func(payload []byte) int {
		var resp apiopenapi.BillingLedgerListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode billing ledger: %v", err)
		}
		return len(resp.Data)
	}, 0)
	assertListCount(t, restarted, restartedSessionCookie, "/api/v1/admin/ops/events/outbox?event_type=GatewayRequestCompleted", func(payload []byte) int {
		var resp apiopenapi.DomainEventOutboxListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode outbox events: %v", err)
		}
		return len(resp.Data)
	}, 1)
	assertListCount(t, restarted, restartedSessionCookie, "/api/v1/admin/audit-logs?action=provider.create", func(payload []byte) int {
		var resp apiopenapi.AuditLogListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode audit logs: %v", err)
		}
		return len(resp.Data)
	}, 1)

	invalidReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	invalidReq.Header.Set("Authorization", "Bearer sk_000000000000_0000000000000000000000000000000000000000000000000000000000000000")
	invalidRec := httptest.NewRecorder()
	restarted.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unknown persisted api key prefix to return 401, got %d", invalidRec.Code)
	}

	_ = restartedLoginResp
}

func TestGatewayProviderAliasHonorsAPIKeyGroupBindings(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, persistenceSQLiteDSN(t))
	defer client.Close()

	stores, err := entstore.New(client)
	if err != nil {
		t.Fatalf("new ent stores: %v", err)
	}

	handler := New(config.Load(), nil,
		WithUserStore(stores.Users),
		WithAPIKeyStore(stores.APIKeys),
		WithProviderStore(stores.Providers),
		WithModelStore(stores.Models),
		WithAccountStore(stores.Accounts),
		WithAuditStore(stores.Audit),
		WithBillingStore(stores.Billing),
		WithEventStore(stores.Events),
		WithSchedulerStore(stores.Scheduler),
		WithUsageStore(stores.Usage),
		WithDatabasePinger(stubDependencyPinger{}),
		WithRedisPinger(stubDependencyPinger{}),
	)

	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"alias-group-model","display_name":"Alias Group Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-group-upstream","status":"active"}`)

	blockedAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(openaiProvider.Id)+`","name":"alias-blocked-account","runtime_class":"api_key","credential":{"api_key":"blocked-secret"},"metadata":{"health_score":0.99,"remaining_ratio":1.0},"status":"active"}`)
	allowedAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(openaiProvider.Id)+`","name":"alias-allowed-account","runtime_class":"api_key","credential":{"api_key":"allowed-secret"},"metadata":{"health_score":0.2,"remaining_ratio":0.2},"status":"active"}`)

	allowedAccountID, err := strconv.Atoi(string(allowedAccount.Data.Id))
	if err != nil {
		t.Fatalf("parse allowed account id: %v", err)
	}
	blockedAccountID, err := strconv.Atoi(string(blockedAccount.Data.Id))
	if err != nil {
		t.Fatalf("parse blocked account id: %v", err)
	}
	group, err := client.AccountGroup.Create().SetName("alias-allowed-group").Save(t.Context())
	if err != nil {
		t.Fatalf("create account group: %v", err)
	}
	if _, err := client.AccountGroupMember.Create().SetAccountID(allowedAccountID).SetAccountGroupID(group.ID).Save(t.Context()); err != nil {
		t.Fatalf("create allowed account membership: %v", err)
	}
	if got, err := stores.Accounts.ListGroupIDsByAccount(t.Context(), blockedAccountID); err != nil {
		t.Fatalf("list blocked account groups: %v", err)
	} else if len(got) != 0 {
		t.Fatalf("expected blocked account to have no groups, got %v", got)
	}

	keyReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"alias-group-key","scopes":["gateway:invoke"],"group_ids":["`+strconv.Itoa(group.ID)+`"]}`))
	keyReq.Header.Set("Content-Type", "application/json")
	keyReq.AddCookie(sessionCookie)
	keyReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	keyRec := httptest.NewRecorder()
	handler.ServeHTTP(keyRec, keyReq)
	if keyRec.Code != http.StatusCreated {
		t.Fatalf("expected api key create 201, got %d body=%s", keyRec.Code, keyRec.Body.String())
	}
	var keyResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(keyRec.Body).Decode(&keyResp); err != nil {
		t.Fatalf("decode api key response: %v", err)
	}

	chatReq := httptest.NewRequest(http.MethodPost, "/api/provider/openai-compatible/v1/chat/completions", strings.NewReader(`{"model":"alias-group-model","messages":[{"role":"user","content":"group route"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected provider alias chat 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=alias-group-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias-group decision, got %d", len(decisionsResp.Data))
	}
	decision := decisionsResp.Data[0]
	if decision.CandidateCount != 1 {
		t.Fatalf("expected API key group filter to leave one candidate, got %+v", decision)
	}
	if decision.SelectedAccountId == nil || *decision.SelectedAccountId != strconv.Itoa(allowedAccountID) {
		t.Fatalf("expected selected account %d, got %+v", allowedAccountID, decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/chat/completions" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func assertListCount(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, path string, countFn func([]byte) int, expected int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s 200, got %d body=%s", path, rec.Code, rec.Body.String())
	}
	if got := countFn(rec.Body.Bytes()); got != expected {
		t.Fatalf("expected %s count %d, got %d", path, expected, got)
	}
}

func persistenceSQLiteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "runtime.db") + "?_fk=1"
}

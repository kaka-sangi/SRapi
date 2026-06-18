package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

// TestCredentialRefreshSingleflight proves concurrent serve-time refreshes of
// the same OAuth account coalesce into ONE upstream token call (providers
// rotate refresh tokens — a second parallel refresh would burn the same token
// twice and invalidate the session), and that a caller arriving after the
// rotation reuses the stored credential instead of replaying its stale token.
func TestCredentialRefreshSingleflight(t *testing.T) {
	var tokenCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("unexpected upstream path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		tokenCalls.Add(1)
		// Hold the flight open long enough for every concurrent caller to pile
		// onto the in-flight refresh instead of racing past it.
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"rotated-access","refresh_token":"rotated-refresh","expires_in":3600}`)
	}))
	defer upstream.Close()

	rt, err := newRuntimeState(config.Load(), slog.Default(), runtimeOptions{})
	if err != nil {
		t.Fatalf("newRuntimeState: %v", err)
	}
	ctx := context.Background()
	providerStatus := providercontract.StatusActive
	provider, err := rt.providers.Create(ctx, providercontract.CreateRequest{
		Name:        "singleflight-provider",
		DisplayName: "Singleflight Provider",
		AdapterType: "reverse-proxy-codex-cli",
		Protocol:    "openai-compatible",
		Status:      &providerStatus,
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	staleCredential := map[string]any{
		"access_token":  "stale-access",
		"refresh_token": "stale-refresh",
		"expires_at":    time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	}
	upstreamClient := "codex_cli"
	account, err := rt.accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:     provider.ID,
		Name:           "singleflight-account",
		RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
		UpstreamClient: &upstreamClient,
		Credential:     staleCredential,
		Metadata: map[string]any{
			"base_url":        upstream.URL + "/backend-api/codex",
			"oauth_token_url": upstream.URL + "/oauth/token",
			"user_agent":      "codex-cli/test",
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	const callers = 8
	var wg sync.WaitGroup
	results := make([]map[string]any, callers)
	oks := make([]bool, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], oks[idx], errs[idx] = rt.refreshReverseProxyCredential(ctx, account, staleCredential)
		}(i)
	}
	wg.Wait()

	if got := tokenCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 coalesced upstream refresh, got %d", got)
	}
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: unexpected error: %v", i, errs[i])
		}
		if !oks[i] || mapString(results[i], "access_token") != "rotated-access" {
			t.Fatalf("caller %d: expected shared rotated credential, got ok=%v credential=%v", i, oks[i], results[i])
		}
	}

	// A late force-refresh (e.g. post-401 retry decided before the rotation
	// landed) must reuse the stored rotated credential, not burn another call.
	late, ok, err := rt.forceRefreshReverseProxyCredential(ctx, account, staleCredential)
	if err != nil {
		t.Fatalf("late force refresh: %v", err)
	}
	if !ok || mapString(late, "access_token") != "rotated-access" {
		t.Fatalf("late force refresh should reuse stored credential, got ok=%v credential=%v", ok, late)
	}
	if got := tokenCalls.Load(); got != 1 {
		t.Fatalf("late force refresh must not hit upstream again, total calls=%d", got)
	}
}

// TestCredentialRefreshUsesLatestAccountMetadata guards the stale-account
// race covered by sub2api's token version checks: a caller can enter the
// refresh path with an old account snapshot whose metadata still has
// force_refresh, while another path already persisted a fresh token and
// cleared that hint. In that case the gateway must reuse the stored
// credential instead of refreshing again.
func TestCredentialRefreshUsesLatestAccountMetadata(t *testing.T) {
	var tokenCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"should-not-be-called","refresh_token":"unused","expires_in":3600}`)
	}))
	defer upstream.Close()

	rt, err := newRuntimeState(config.Load(), slog.Default(), runtimeOptions{})
	if err != nil {
		t.Fatalf("newRuntimeState: %v", err)
	}
	ctx := context.Background()
	providerStatus := providercontract.StatusActive
	provider, err := rt.providers.Create(ctx, providercontract.CreateRequest{
		Name:        "metadata-refresh-provider",
		DisplayName: "Metadata Refresh Provider",
		AdapterType: "reverse-proxy-codex-cli",
		Protocol:    "openai-compatible",
		Status:      &providerStatus,
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	upstreamClient := "codex_cli"
	account, err := rt.accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:     provider.ID,
		Name:           "metadata-refresh-account",
		RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
		UpstreamClient: &upstreamClient,
		Credential: map[string]any{
			"access_token":       "fresh-access",
			"refresh_token":      "fresh-refresh",
			"expires_at":         time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			"_token_version":     int64(200),
			"chatgpt_account_id": "chatgpt-acc",
		},
		Metadata: map[string]any{
			"base_url":        upstream.URL + "/backend-api/codex",
			"oauth_token_url": upstream.URL + "/oauth/token",
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	staleAccount := account
	staleAccount.Metadata = map[string]any{
		"base_url":        upstream.URL + "/backend-api/codex",
		"oauth_token_url": upstream.URL + "/oauth/token",
		"force_refresh":   true,
	}
	staleCredential := map[string]any{
		"access_token":  "stale-access",
		"refresh_token": "stale-refresh",
		"expires_at":    time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	}

	refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, staleAccount, staleCredential)
	if err != nil {
		t.Fatalf("refreshReverseProxyCredential: %v", err)
	}
	if !ok || mapString(refreshed, "access_token") != "fresh-access" {
		t.Fatalf("expected stored fresh credential, got ok=%v credential=%v", ok, refreshed)
	}
	if got := tokenCalls.Load(); got != 0 {
		t.Fatalf("expected no upstream refresh, got %d calls", got)
	}
}

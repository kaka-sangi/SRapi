package accountstokenrefresh

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

// stubRefresher always returns the configured response (or error) when
// asked. Lets us drive both the success and failure branches without
// touching a real upstream.
type stubRefresher struct {
	credential map[string]any
	err        error
	proxyID    *string
}

func (s *stubRefresher) Refresh(_ context.Context, req reverseproxycontract.RefreshRequest) (reverseproxycontract.RefreshResponse, error) {
	if s.err != nil {
		return reverseproxycontract.RefreshResponse{}, s.err
	}
	s.proxyID = cloneString(req.Account.ProxyID)
	cred := s.credential
	if cred == nil {
		cred = map[string]any{}
	}
	cred["access_token"] = "refreshed"
	cred["refresh_token"] = "refreshed"
	if _, ok := cred["expires_at"]; !ok {
		cred["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	}
	return reverseproxycontract.RefreshResponse{Credential: cred}, nil
}

// TestWorkerMetricsTrackRefreshOutcomes seeds an oauth account whose token
// is about to expire, runs a couple of RunOnce passes, and asserts the
// counter snapshot matches the outcomes — first a success, then a
// permanent error.
func TestWorkerMetricsTrackRefreshOutcomes(t *testing.T) {
	store := accountmemory.New()
	ctx := context.Background()
	svc, err := accountservice.New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	expires := time.Now().UTC().Add(time.Minute) // inside the refresh window
	account, err := svc.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   1,
		Name:         "oauth",
		RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
		Credential:   map[string]any{"access_token": "old", "refresh_token": "old", "expires_at": expires.Format(time.RFC3339)},
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	// Bring the token into the refresh window (TokenExpiresAt must be set; the
	// initial Create doesn't populate it, so we patch it directly).
	account.TokenExpiresAt = &expires
	if _, err := store.Update(ctx, account); err != nil {
		t.Fatalf("seed token_expires_at: %v", err)
	}

	stub := &stubRefresher{credential: map[string]any{}}
	w, err := New(store, nil, Config{
		MasterKey:        "0123456789abcdef0123456789abcdef",
		Interval:         time.Hour,
		RefreshThreshold: 5 * time.Minute,
		MaxConcurrent:    2,
		Timeout:          5 * time.Second,
		Refresher:        stub,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatalf("first runonce: %v", err)
	}
	snap := w.Metrics()
	if snap.RefreshAttempted != 1 || snap.RefreshSucceeded != 1 {
		t.Fatalf("expected 1 attempted/1 succeeded, got %+v", snap)
	}

	// Re-arm the account so the next pass picks it up again, then make the
	// refresher fail with a permanent (invalid_grant) error.
	account, err = svc.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	soon := time.Now().UTC().Add(time.Minute)
	account.TokenExpiresAt = &soon
	if _, err := store.Update(ctx, account); err != nil {
		t.Fatalf("re-arm token_expires_at: %v", err)
	}
	stub.err = errors.New("invalid_grant: token revoked")
	if _, err := w.RunOnce(ctx); err == nil {
		t.Fatalf("expected refresh error to bubble up")
	}
	snap = w.Metrics()
	if snap.RefreshAttempted != 2 {
		t.Fatalf("expected 2 attempts after second pass, got %+v", snap)
	}
	if snap.RefreshFailedPermanent != 1 {
		t.Fatalf("expected 1 permanent failure, got %+v", snap)
	}
}

func TestWorkerRefreshMaterializesProxyDefinitionID(t *testing.T) {
	store := accountmemory.New()
	ctx := context.Background()
	svc, err := accountservice.New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	proxyURL := "http://proxy.example.invalid:8080"
	proxy, err := svc.CreateProxy(ctx, accountcontract.CreateProxyRequest{
		Name: "refresh-egress",
		Type: accountcontract.ProxyTypeHTTP,
		URL:  proxyURL,
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	proxyID := strconv.Itoa(proxy.ID)
	expires := time.Now().UTC().Add(time.Minute)
	account, err := svc.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   1,
		Name:         "oauth-proxy",
		RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
		Credential:   map[string]any{"access_token": "old", "refresh_token": "old", "expires_at": expires.Format(time.RFC3339)},
		ProxyID:      &proxyID,
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	account.TokenExpiresAt = &expires
	if _, err := store.Update(ctx, account); err != nil {
		t.Fatalf("seed token_expires_at: %v", err)
	}

	stub := &stubRefresher{credential: map[string]any{}}
	w, err := New(store, nil, Config{
		MasterKey:        "0123456789abcdef0123456789abcdef",
		Interval:         time.Hour,
		RefreshThreshold: 5 * time.Minute,
		MaxConcurrent:    1,
		Timeout:          5 * time.Second,
		Refresher:        stub,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatalf("runonce: %v", err)
	}
	if stub.proxyID == nil || *stub.proxyID != proxyURL {
		t.Fatalf("expected runtime proxy url %q, got %v", proxyURL, stub.proxyID)
	}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

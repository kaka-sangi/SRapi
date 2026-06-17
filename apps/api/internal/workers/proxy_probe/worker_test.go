package proxyprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
)

// TestWorkerMetricsTrackProbeOutcomes drives a couple of RunOnce passes
// against a tiny fake upstream and asserts the counter snapshot the
// runtime metrics collector will read back reflects the outcomes.
func TestWorkerMetricsTrackProbeOutcomes(t *testing.T) {
	store := accountmemory.New()
	ctx := context.Background()

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(okServer.Close)

	// Seed two active proxies that will be probed each pass.
	if _, err := seedProxy(t, store, "proxy-a", okServer.URL); err != nil {
		t.Fatalf("seed proxy a: %v", err)
	}
	if _, err := seedProxy(t, store, "proxy-b", okServer.URL); err != nil {
		t.Fatalf("seed proxy b: %v", err)
	}

	w, err := New(store, nil, Config{
		Enabled:       true,
		MasterKey:     "0123456789abcdef0123456789abcdef",
		Interval:      time.Hour,
		Timeout:       2 * time.Second,
		MaxConcurrent: 2,
		ProbeURL:      okServer.URL,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatalf("first runonce: %v", err)
	}
	snap := w.Metrics()
	if snap.ProbeAttempted != 2 {
		t.Fatalf("expected 2 probes attempted, got %+v", snap)
	}
	if snap.ProbeSucceeded+snap.ProbeFailed != snap.ProbeAttempted {
		t.Fatalf("succeeded+failed must equal attempted, got %+v", snap)
	}

	// Second pass — counters cumulate, not reset.
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatalf("second runonce: %v", err)
	}
	if got := w.Metrics().ProbeAttempted; got != 4 {
		t.Fatalf("expected 4 attempts after 2 passes, got %d", got)
	}
}

// seedProxy creates an http proxy row pointed at the loopback so
// ResolveProxyURL succeeds inside probeOne. We use the real accounts service
// so the credential is correctly encrypted.
func seedProxy(t *testing.T, store accountcontract.Store, name string, urlStr string) (accountcontract.ProxyDefinition, error) {
	t.Helper()
	svc, err := accountservice.New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		return accountcontract.ProxyDefinition{}, err
	}
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return accountcontract.ProxyDefinition{}, err
	}
	return svc.CreateProxy(context.Background(), accountcontract.CreateProxyRequest{
		Name: name,
		Type: accountcontract.ProxyTypeHTTP,
		URL:  "http://" + parsed.Host,
	})
}

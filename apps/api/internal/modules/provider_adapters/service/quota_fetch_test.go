package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

func TestFetchAccountQuotaMapsAnthropicUsageAndCachesSuccess(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/oauth/usage" {
			t.Fatalf("unexpected quota path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer anthropic-access-token" {
			t.Fatalf("unexpected auth header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"usage": {
				"five_hour": {"utilization": 0.25, "resets_at": "2026-06-09T05:00:00Z"},
				"seven_day": {"used": 40, "limit": 100, "resets_at": "2026-06-10T00:00:00Z"},
				"seven_day_sonnet": {"usage_ratio": "0.75", "resets_at": "2026-06-11T00:00:00Z"}
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	req := anthropicQuotaProbeRequest(upstream.URL + "/api/oauth/usage")
	report, err := svc.FetchAccountQuota(context.Background(), req)
	if err != nil {
		t.Fatalf("fetch account quota: %v", err)
	}
	if !report.Supported || report.Source != "endpoint" {
		t.Fatalf("unexpected quota report support/source: %+v", report)
	}
	if len(report.QuotaSignals) != 3 {
		t.Fatalf("expected three anthropic quota signals, got %+v", report.QuotaSignals)
	}
	assertQuotaSignal(t, report.QuotaSignals, "anthropic_5h", "25", "75", "100", 0.75)
	assertQuotaSignal(t, report.QuotaSignals, "anthropic_7d", "40", "60", "100", 0.6)
	assertQuotaSignal(t, report.QuotaSignals, "anthropic_7d_sonnet", "75", "25", "100", 0.25)

	cached, err := svc.FetchAccountQuota(context.Background(), req)
	if err != nil {
		t.Fatalf("fetch cached account quota: %v", err)
	}
	if len(cached.QuotaSignals) != 3 {
		t.Fatalf("expected cached anthropic quota signals, got %+v", cached.QuotaSignals)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected successful quota cache to reuse upstream response, got %d calls", calls.Load())
	}
}

func TestFetchAccountQuotaSingleflightsConcurrentRequests(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"five_hour":{"utilization":0.2,"resets_at":"2026-06-09T05:00:00Z"}}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	req := anthropicQuotaProbeRequest(upstream.URL + "/api/oauth/usage")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			report, err := svc.FetchAccountQuota(context.Background(), req)
			if err != nil {
				errs <- err
				return
			}
			if len(report.QuotaSignals) != 1 {
				errs <- errors.New("missing anthropic quota signal")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent fetch failed: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected singleflight quota fetch, got %d upstream calls", calls.Load())
	}
}

func TestFetchAccountQuotaClassifiesForbiddenAndCachesFailure(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
			"error": {
				"type": "violation",
				"message": "policy violation; validate at https://console.anthropic.com/validate",
				"validation_url": "https://console.anthropic.com/validate"
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	req := anthropicQuotaProbeRequest(upstream.URL + "/api/oauth/usage")
	for i := 0; i < 2; i++ {
		_, err = svc.FetchAccountQuota(context.Background(), req)
		var providerErr contract.ProviderError
		if !errors.As(err, &providerErr) {
			t.Fatalf("expected provider error, got %v", err)
		}
		if providerErr.Class != "policy_violation" || providerErr.StatusCode != http.StatusForbidden {
			t.Fatalf("unexpected provider error: %+v", providerErr)
		}
		if providerErr.Metadata["validation_url"] != "https://console.anthropic.com/validate" {
			t.Fatalf("expected validation URL metadata, got %+v", providerErr.Metadata)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected failed quota cache to suppress duplicate upstream calls, got %d", calls.Load())
	}
}

func anthropicQuotaProbeRequest(quotaURL string) contract.ProbeRequest {
	return contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          1,
			Name:        "anthropic",
			AdapterType: "anthropic-compatible",
			Protocol:    "anthropic-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url": quotaURL,
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           10,
			ProviderID:   1,
			Name:         "anthropic-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{},
		},
		Credential: map[string]any{"access_token": "anthropic-access-token"},
	}
}

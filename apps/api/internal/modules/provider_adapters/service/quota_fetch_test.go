package service_test

import (
	"context"
	"errors"
	"math"
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

func assertQuotaSignalWithoutReset(t *testing.T, signals []contract.QuotaSignal, quotaType string, used string, remaining string, limit string, remainingRatio float32) {
	t.Helper()
	for _, signal := range signals {
		if signal.QuotaType != quotaType {
			continue
		}
		if signal.Used != used ||
			signal.Remaining != remaining ||
			signal.QuotaLimit != limit ||
			math.Abs(float64(signal.RemainingRatio-remainingRatio)) > 0.000001 ||
			signal.SnapshotAt.IsZero() {
			t.Fatalf("unexpected quota signal for %s: %+v", quotaType, signal)
		}
		return
	}
	t.Fatalf("missing quota signal %q in %+v", quotaType, signals)
}

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

func TestFetchAccountQuotaMapsCodexAccountPlanCredits(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer codex-access-token" {
			t.Fatalf("unexpected auth header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"account_plan": {
				"account_plan_id": "plus",
				"subscription_plan": {
					"allowance": "900",
					"usage": "100",
					"limit": "1000",
					"currency": "credits"
				}
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          2,
			Name:        "codex-cli",
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url":                    upstream.URL + "/backend-api/accounts/check/v4-2023-04-27",
				"quota_plan_path":              "account_plan.account_plan_id",
				"quota_credits_remaining_path": "account_plan.subscription_plan.allowance",
				"quota_credits_used_path":      "account_plan.subscription_plan.usage",
				"quota_credits_limit_path":     "account_plan.subscription_plan.limit",
				"quota_currency_path":          "account_plan.subscription_plan.currency",
				"auth_mode":                    "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           20,
			ProviderID:   2,
			Name:         "codex-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{},
		},
		Credential: map[string]any{"access_token": "codex-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch codex quota: %v", err)
	}
	if !report.Supported || report.Plan != "plus" || report.CreditsRemaining != "900" || report.CreditsUsed != "100" || report.CreditsLimit != "1000" || report.Currency != "credits" {
		t.Fatalf("unexpected codex quota report: %+v", report)
	}
}

func TestFetchAccountQuotaFallsBackAccountPlanCreditsWithoutPaths(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer codex-access-token" {
			t.Fatalf("unexpected auth header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"account_plan": {
				"account_plan_id": "plus",
				"subscription_plan": {
					"allowance": "900",
					"usage": "100",
					"limit": "1000",
					"currency": "credits"
				}
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          21,
			Name:        "codex-cli",
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url": upstream.URL + "/backend-api/accounts/check/v4-2023-04-27",
				"auth_mode": "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           21,
			ProviderID:   21,
			Name:         "codex-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{},
		},
		Credential: map[string]any{"access_token": "codex-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch codex quota: %v", err)
	}
	if !report.Supported || report.Plan != "plus" || report.CreditsRemaining != "900" || report.CreditsUsed != "100" || report.CreditsLimit != "1000" || report.Currency != "credits" {
		t.Fatalf("unexpected fallback quota report: %+v", report)
	}
}

func TestFetchAccountQuotaExplicitPathsWinOverAccountPlanFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"custom": {
				"plan": "configured",
				"remaining": "9",
				"used": "1",
				"limit": "10",
				"currency": "configured-credits"
			},
			"account_plan": {
				"account_plan_id": "fallback",
				"subscription_plan": {
					"allowance": "900",
					"usage": "100",
					"limit": "1000",
					"currency": "credits"
				}
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          23,
			Name:        "codex-cli",
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url":                    upstream.URL + "/quota",
				"quota_plan_path":              "custom.plan",
				"quota_credits_remaining_path": "custom.remaining",
				"quota_credits_used_path":      "custom.used",
				"quota_credits_limit_path":     "custom.limit",
				"quota_currency_path":          "custom.currency",
				"auth_mode":                    "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           23,
			ProviderID:   23,
			Name:         "codex-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{},
		},
		Credential: map[string]any{"access_token": "codex-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch codex quota: %v", err)
	}
	if report.Plan != "configured" || report.CreditsRemaining != "9" || report.CreditsUsed != "1" || report.CreditsLimit != "10" || report.Currency != "configured-credits" {
		t.Fatalf("explicit quota paths should win over fallback, got %+v", report)
	}
}

func TestFetchAccountQuotaMapsCodexAccountsCheckPlan(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var gotAccountID string
	var gotOrigin string
	var gotReferer string
	var gotUserAgent string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		gotAccountID = r.Header.Get("ChatGPT-Account-ID")
		gotOrigin = r.Header.Get("Origin")
		gotReferer = r.Header.Get("Referer")
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"accounts": {
				"personal": {
					"account": {"id": "personal", "plan_type": "free", "is_default": true},
					"entitlement": {"subscription_plan": "free"}
				},
				"team-account": {
					"account": {"id": "team-account", "plan_type": "team", "is_default": false},
					"entitlement": {"subscription_plan": "team", "expires_at": "2026-05-02T20:32:12Z"},
					"account_plan": {
						"subscription_plan": {
							"allowance": 1200,
							"usage": 300,
							"limit": 1500,
							"currency": "credits"
						}
					}
				}
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          22,
			Name:        "codex-cli",
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"auth_mode": "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           220,
			ProviderID:   22,
			Name:         "codex-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata: map[string]any{
				"base_url":           upstream.URL + "/backend-api/codex",
				"chatgpt_account_id": "team-account",
				"user_agent":         "codex-cli/test",
			},
		},
		Credential: map[string]any{"access_token": "codex-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch codex quota: %v", err)
	}
	if gotPath != "/backend-api/accounts/check/v4-2023-04-27" {
		t.Fatalf("unexpected quota path %s", gotPath)
	}
	if gotAuthorization != "Bearer codex-access-token" || gotAccountID != "team-account" || gotOrigin != "https://chatgpt.com" || gotReferer != "https://chatgpt.com/" || gotUserAgent != "codex-cli/test" {
		t.Fatalf("unexpected Codex quota headers auth=%q account=%q origin=%q referer=%q ua=%q", gotAuthorization, gotAccountID, gotOrigin, gotReferer, gotUserAgent)
	}
	if !report.Supported ||
		report.Plan != "team" ||
		report.CreditsRemaining != "1200" ||
		report.CreditsUsed != "300" ||
		report.CreditsLimit != "1500" ||
		report.Currency != "credits" {
		t.Fatalf("unexpected codex accounts/check quota report: %+v", report)
	}
}

func TestFetchAccountQuotaAntigravityUsesProjectHeaderAndSupportsQuota(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer antigravity-access-token" {
			t.Fatalf("unexpected auth header %q", got)
		}
		if got := r.Header.Get("x-goog-user-project"); got != "project-1" {
			t.Fatalf("unexpected project header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"account_plan": {
				"account_plan_id": "antigravity-pro",
				"subscription_plan": {
					"allowance": 42,
					"usage": 8,
					"limit": 50,
					"currency": "credits"
				}
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          3,
			Name:        "antigravity",
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "gemini-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url":                    upstream.URL + "/v1internal:quota",
				"quota_plan_path":              "account_plan.account_plan_id",
				"quota_credits_remaining_path": "account_plan.subscription_plan.allowance",
				"quota_credits_used_path":      "account_plan.subscription_plan.usage",
				"quota_credits_limit_path":     "account_plan.subscription_plan.limit",
				"quota_currency_path":          "account_plan.subscription_plan.currency",
				"quota_headers":                map[string]any{"x-goog-user-project": "{{project_id}}"},
				"auth_mode":                    "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           30,
			ProviderID:   3,
			Name:         "antigravity-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{"project_id": "project-1"},
		},
		Credential: map[string]any{"access_token": "antigravity-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch antigravity quota: %v", err)
	}
	if !report.Supported || report.Source != "endpoint" || report.Plan != "antigravity-pro" || report.CreditsRemaining != "42" || report.CreditsUsed != "8" || report.CreditsLimit != "50" || report.Currency != "credits" {
		t.Fatalf("unexpected antigravity quota report: %+v", report)
	}
}

func TestFetchAccountQuotaAntigravityMapsPaidTierCreditsFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"currentTier": {"id": "free-tier", "name": "Free"},
			"paidTier": {
				"id": "g1-pro-tier",
				"name": "Google One Pro",
				"availableCredits": [
					{"creditType": "OTHER", "creditAmount": "10", "minimumCreditAmountForUsage": "1"},
					{"creditType": "GOOGLE_ONE_AI", "creditAmount": "25000", "minimumCreditAmountForUsage": "50"}
				]
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          3,
			Name:        "antigravity",
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "gemini-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url": upstream.URL + "/v1internal:loadCodeAssist",
				"auth_mode": "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           31,
			ProviderID:   3,
			Name:         "antigravity-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{},
		},
		Credential: map[string]any{"access_token": "antigravity-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch antigravity quota: %v", err)
	}
	if !report.Supported ||
		report.Plan != "g1-pro-tier" ||
		report.CreditsRemaining != "25000" ||
		report.CreditsUsed != "" ||
		report.CreditsLimit != "" ||
		report.Currency != "GOOGLE_ONE_AI" {
		t.Fatalf("unexpected antigravity paid tier quota report: %+v", report)
	}
	assertQuotaSignalWithoutReset(t, report.QuotaSignals, "antigravity_google_one_ai_credits", "0", "25000", "50", 1)
}

func TestFetchAccountQuotaAntigravityFallsBackToCurrentTierPlan(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "object",
			body: `{"currentTier":{"id":"free-tier","name":"Free"}}`,
			want: "free-tier",
		},
		{
			name: "string",
			body: `{"currentTier":"free-tier"}`,
			want: "free-tier",
		},
		{
			name: "paid tier empty id",
			body: `{"currentTier":{"id":"free-tier"},"paidTier":{"id":"","name":""}}`,
			want: "free-tier",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()

			svc, err := service.New(upstream.Client())
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
				Provider: providercontract.Provider{
					ID:          34,
					Name:        "antigravity",
					AdapterType: "reverse-proxy-antigravity",
					Protocol:    "gemini-compatible",
					Status:      providercontract.StatusActive,
					ConfigSchema: map[string]any{
						"quota_url": upstream.URL + "/v1internal:loadCodeAssist",
						"auth_mode": "bearer",
					},
				},
				Account: accountcontract.ProviderAccount{
					ID:           34,
					ProviderID:   34,
					Name:         "antigravity-free",
					RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
					Status:       accountcontract.StatusActive,
					Metadata:     map[string]any{},
				},
				Credential: map[string]any{"access_token": "antigravity-access-token"},
			})
			if err != nil {
				t.Fatalf("fetch antigravity quota: %v", err)
			}
			if !report.Supported || report.Plan != tc.want {
				t.Fatalf("unexpected antigravity currentTier quota report: %+v", report)
			}
		})
	}
}

func TestFetchAccountQuotaAntigravityMapsAICreditsArrayFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"subscription_tier": "GOOGLE_ONE_PRO",
			"ai_credits": [
				{"credit_type": "OTHER", "amount": 10, "minimum_balance": 1},
				{"credit_type": "GOOGLE_ONE_AI", "amount": 25, "minimum_balance": 50}
			]
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          33,
			Name:        "antigravity",
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "gemini-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url": upstream.URL + "/v1internal:loadCodeAssist",
				"auth_mode": "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           33,
			ProviderID:   33,
			Name:         "antigravity-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{},
		},
		Credential: map[string]any{"access_token": "antigravity-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch antigravity quota: %v", err)
	}
	if !report.Supported ||
		report.Plan != "GOOGLE_ONE_PRO" ||
		report.CreditsRemaining != "25" ||
		report.CreditsUsed != "" ||
		report.CreditsLimit != "" ||
		report.Currency != "GOOGLE_ONE_AI" {
		t.Fatalf("unexpected antigravity ai credits quota report: %+v", report)
	}
	assertQuotaSignalWithoutReset(t, report.QuotaSignals, "antigravity_google_one_ai_credits", "25", "25", "50", 0.5)
}

func TestFetchAccountQuotaAntigravityCreditSignalBelowMinimum(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"paidTier": {
				"id": "g1-pro-tier",
				"availableCredits": [
					{"creditType": "GOOGLE_ONE_AI", "creditAmount": "25", "minimumCreditAmountForUsage": "50"}
				]
			}
		}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	report, err := svc.FetchAccountQuota(context.Background(), contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          3,
			Name:        "antigravity",
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "gemini-compatible",
			Status:      providercontract.StatusActive,
			ConfigSchema: map[string]any{
				"quota_url": upstream.URL + "/v1internal:loadCodeAssist",
				"auth_mode": "bearer",
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           32,
			ProviderID:   3,
			Name:         "antigravity-oauth",
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{},
		},
		Credential: map[string]any{"access_token": "antigravity-access-token"},
	})
	if err != nil {
		t.Fatalf("fetch antigravity quota: %v", err)
	}
	if !report.Supported || report.CreditsRemaining != "25" || report.Currency != "GOOGLE_ONE_AI" {
		t.Fatalf("unexpected antigravity paid tier quota report: %+v", report)
	}
	assertQuotaSignalWithoutReset(t, report.QuotaSignals, "antigravity_google_one_ai_credits", "25", "25", "50", 0.5)
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

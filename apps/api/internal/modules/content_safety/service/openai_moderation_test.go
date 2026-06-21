package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contentsafetycontract "github.com/srapi/srapi/apps/api/internal/modules/content_safety/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

func TestNewOpenAIModerationClientRequiresAPIKey(t *testing.T) {
	if _, err := NewOpenAIModerationClient(OpenAIModerationOptions{}); err == nil {
		t.Fatal("expected ErrModerationNotConfigured when API key is missing")
	}
}

func TestOpenAIModerationClientClassifyDeliversCategories(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/moderations" {
			t.Errorf("expected /moderations, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("missing or wrong Authorization header: %q", got)
		}
		var body openAIModerationRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Model != "omni-moderation-latest" || !strings.Contains(body.Input, "violent text") {
			t.Errorf("unexpected upstream payload: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(openAIModerationResponse{
			Model: "omni-moderation-latest",
			Results: []openAIModerationResultItem{
				{
					Flagged:        true,
					Categories:     map[string]bool{"violence": true, "harassment": false},
					CategoryScores: map[string]float64{"violence": 0.92, "harassment": 0.12},
				},
			},
		})
	}))
	defer upstream.Close()

	client, err := NewOpenAIModerationClient(OpenAIModerationOptions{
		APIKey:    "test-key",
		BaseURL:   upstream.URL,
		Model:     "omni-moderation-latest",
		Timeout:   time.Second,
		CacheSize: 4,
		CacheTTL:  time.Minute,
	})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	first, err := client.Classify(context.Background(), "this is some violent text")
	if err != nil {
		t.Fatalf("first classify: %v", err)
	}
	if !first.Flagged || !first.Categories["violence"] || first.Scores["violence"] < 0.9 {
		t.Fatalf("upstream verdict not surfaced: %+v", first)
	}
	if first.CachedHit {
		t.Fatal("first call must not report cache hit")
	}

	second, err := client.Classify(context.Background(), "this is some violent text")
	if err != nil {
		t.Fatalf("second classify: %v", err)
	}
	if !second.CachedHit {
		t.Fatal("second identical input must be served from cache")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("cache should collapse identical inputs, got %d upstream calls", got)
	}
}

func TestApplyWithContextRecordsModerationFindings(t *testing.T) {
	svc := New(DefaultConfig())
	fake := &fakeModerationProvider{
		result: contentsafetycontract.ModerationResult{
			Provider:   "openai",
			Model:      "omni-moderation-latest",
			Flagged:    true,
			Categories: map[string]bool{"violence": true},
			Scores:     map[string]float64{"violence": 0.81},
		},
	}
	config := DefaultConfig()
	config.Mode = contentsafetycontract.ModeEnforce
	config.Moderation = contentsafetycontract.ModerationOptions{
		Enabled:     true,
		BlockOnFlag: true,
		Provider:    fake,
	}
	req := gatewaycontract.CanonicalRequest{
		CanonicalModel: "gpt-4o",
		Prompt:         "please write a violent threat",
	}
	updated, result := svc.ApplyWithContext(context.Background(), req, config)
	if !result.Blocked || result.Reason != "moderation_flagged" {
		t.Fatalf("expected blocked moderation result, got %+v", result)
	}
	if len(result.Findings) == 0 || result.Findings[0].Kind != contentsafetycontract.FindingKindModerationCategory {
		t.Fatalf("expected moderation finding, got %+v", result.Findings)
	}
	if !containsWarning(result.Warnings, "content_safety_moderation_flagged") {
		t.Fatalf("expected moderation warning, got %v", result.Warnings)
	}
	if !containsWarning(updated.CompatibilityWarnings, "content_safety_moderation_flagged") {
		t.Fatalf("warning must propagate to canonical request, got %v", updated.CompatibilityWarnings)
	}
}

func TestApplyWithContextThresholdSuppressesLowScore(t *testing.T) {
	svc := New(DefaultConfig())
	fake := &fakeModerationProvider{
		result: contentsafetycontract.ModerationResult{
			Provider:   "openai",
			Flagged:    true,
			Categories: map[string]bool{"violence": true},
			Scores:     map[string]float64{"violence": 0.30},
		},
	}
	config := DefaultConfig()
	config.Mode = contentsafetycontract.ModeMonitor
	config.Moderation = contentsafetycontract.ModerationOptions{
		Enabled:    true,
		Thresholds: map[string]float64{"violence": 0.80},
		Provider:   fake,
	}
	req := gatewaycontract.CanonicalRequest{
		CanonicalModel: "gpt-4o",
		Prompt:         "borderline content",
	}
	_, result := svc.ApplyWithContext(context.Background(), req, config)
	if result.Blocked {
		t.Fatal("monitor mode must never block")
	}
	for _, finding := range result.Findings {
		if finding.Kind == contentsafetycontract.FindingKindModerationCategory {
			t.Fatalf("expected no moderation finding when below threshold, got %+v", finding)
		}
	}
}

func TestApplyWithContextSurfacesUpstreamError(t *testing.T) {
	svc := New(DefaultConfig())
	fake := &fakeModerationProvider{err: errClassifierUnavailable}
	config := DefaultConfig()
	config.Moderation = contentsafetycontract.ModerationOptions{
		Enabled:  true,
		Provider: fake,
	}
	_, result := svc.ApplyWithContext(context.Background(), gatewaycontract.CanonicalRequest{Prompt: "anything"}, config)
	if result.Blocked {
		t.Fatal("upstream failure must fail-open, never block the user request")
	}
	if !containsWarning(result.Warnings, "content_safety_moderation_failed") {
		t.Fatalf("expected upstream-failure warning, got %v", result.Warnings)
	}
}

type fakeModerationProvider struct {
	result contentsafetycontract.ModerationResult
	err    error
}

func (f *fakeModerationProvider) Classify(_ context.Context, _ string) (contentsafetycontract.ModerationResult, error) {
	if f.err != nil {
		return contentsafetycontract.ModerationResult{}, f.err
	}
	return f.result, nil
}

var errClassifierUnavailable = stringError("classifier unavailable")

type stringError string

func (s stringError) Error() string { return string(s) }

func containsWarning(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

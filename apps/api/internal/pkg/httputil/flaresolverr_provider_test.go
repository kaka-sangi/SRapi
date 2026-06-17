package httputil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFlareSolverrProviderUnconfiguredReturnsSentinel(t *testing.T) {
	p := NewFlareSolverrProvider(FlareSolverrConfig{})
	_, err := p.Resolve(ResolveRequest{TargetURL: "https://chatgpt.com"})
	if !errors.Is(err, ErrClearanceProviderNotConfigured) {
		t.Fatalf("expected ErrClearanceProviderNotConfigured, got %v", err)
	}
	if p.Configured() {
		t.Fatal("Configured() should be false for empty URL")
	}
}

func TestFlareSolverrProviderResolveOK(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotPayload)
		_, _ = w.Write([]byte(`{
			"status": "ok",
			"solution": {
				"url": "https://chatgpt.com/",
				"userAgent": "Mozilla/5.0 (Test)",
				"cookies": [
					{"name":"cf_clearance","value":"xyz","domain":".chatgpt.com"},
					{"name":"other","value":"v","domain":"example.com"}
				]
			}
		}`))
	}))
	defer server.Close()

	p := NewFlareSolverrProvider(FlareSolverrConfig{
		URL:        server.URL,
		Timeout:    2 * time.Second,
		SessionTTL: 30 * time.Minute,
	})
	if !p.Configured() {
		t.Fatal("expected Configured() == true")
	}
	bundle, err := p.Resolve(ResolveRequest{
		TargetURL: "https://chatgpt.com/",
		ProxyURL:  "http://proxy:9000",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if bundle.UserAgent != "Mozilla/5.0 (Test)" {
		t.Errorf("UserAgent = %q", bundle.UserAgent)
	}
	if bundle.Cookies["cf_clearance"] != "xyz" {
		t.Errorf("cf_clearance missing: %#v", bundle.Cookies)
	}
	if _, has := bundle.Cookies["other"]; has {
		t.Errorf("expected non-host cookie to be filtered out: %#v", bundle.Cookies)
	}
	if bundle.ExpiresAt.IsZero() {
		t.Error("expected SessionTTL to stamp ExpiresAt")
	}
	if gotProxy := gotPayload["proxy"]; gotProxy == nil {
		t.Errorf("expected proxy payload, got %v", gotPayload)
	}
}

func TestFlareSolverrProviderResolveNonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","message":"cf detected"}`))
	}))
	defer server.Close()
	p := NewFlareSolverrProvider(FlareSolverrConfig{URL: server.URL})
	_, err := p.Resolve(ResolveRequest{TargetURL: "https://chatgpt.com"})
	if !errors.Is(err, ErrClearanceResolveFailed) {
		t.Fatalf("expected ErrClearanceResolveFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), "cf detected") {
		t.Errorf("expected message in error: %v", err)
	}
}

func TestFlareSolverrEndpointNormalisation(t *testing.T) {
	if got := normaliseFlareSolverrURL("http://flare:8191"); got != "http://flare:8191/v1" {
		t.Errorf("normaliseFlareSolverrURL appended /v1: %q", got)
	}
	if got := normaliseFlareSolverrURL("http://flare:8191/v1/"); got != "http://flare:8191/v1" {
		t.Errorf("normaliseFlareSolverrURL trimmed trailing: %q", got)
	}
	if got := normaliseFlareSolverrURL(""); got != "" {
		t.Errorf("normaliseFlareSolverrURL preserved empty: %q", got)
	}
}

func TestMergeCookieHeader(t *testing.T) {
	got := MergeCookieHeader("a=1; b=2", map[string]string{"b": "should-not-override", "c": "3"})
	if !strings.Contains(got, "a=1") || !strings.Contains(got, "b=2") || !strings.Contains(got, "c=3") {
		t.Errorf("merged cookie header missing parts: %q", got)
	}
	if strings.Contains(got, "should-not-override") {
		t.Errorf("merged cookie header overrode existing: %q", got)
	}
}

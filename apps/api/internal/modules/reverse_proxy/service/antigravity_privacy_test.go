package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func TestEnrichAntigravityOAuthCredentialSkipsNonAntigravity(t *testing.T) {
	svc := newServiceForPrivacyTest(t, nil)
	codex := "codex_cli"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &codex,
	}, map[string]any{"access_token": "secret"})
	if _, ok := got["privacy_mode"]; ok {
		t.Fatalf("privacy_mode must not be set for non-antigravity client")
	}
}

func TestEnrichAntigravityOAuthCredentialSkipsAlreadySet(t *testing.T) {
	svc := newServiceForPrivacyTest(t, nil)
	upstream := "antigravity_desktop"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &upstream,
		Metadata:       map[string]any{"project_id": "proj-1"},
	}, map[string]any{"access_token": "secret", "privacy_mode": AntigravityPrivacyModeSet})
	if got["privacy_mode"] != AntigravityPrivacyModeSet {
		t.Fatalf("privacy_mode must be preserved as privacy_set, got %v", got["privacy_mode"])
	}
}

func TestEnrichAntigravityOAuthCredentialMarksFailedWithoutProjectID(t *testing.T) {
	svc := newServiceForPrivacyTest(t, nil)
	upstream := "antigravity_desktop"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &upstream,
	}, map[string]any{"access_token": "secret"})
	if got["privacy_mode"] != AntigravityPrivacyModeFailed {
		t.Fatalf("expected privacy_set_failed when project_id missing, got %v", got["privacy_mode"])
	}
}

func TestEnrichAntigravityOAuthCredentialMarksFailedWithoutAccessToken(t *testing.T) {
	svc := newServiceForPrivacyTest(t, nil)
	upstream := "antigravity_desktop"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &upstream,
		Metadata:       map[string]any{"project_id": "proj-1"},
	}, map[string]any{})
	if _, ok := got["privacy_mode"]; ok {
		t.Fatalf("missing access_token must not invent a privacy_mode, got %v", got["privacy_mode"])
	}
}

func TestEnrichAntigravityOAuthCredentialSetsPrivacySetOnSuccess(t *testing.T) {
	calls := newPrivacyServerCallRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.record(r)
		switch r.URL.Path {
		case antigravitySetUserSettingsPath:
			if r.Header.Get("Authorization") != "Bearer secret" {
				t.Fatalf("setUserSettings missing bearer, got %q", r.Header.Get("Authorization"))
			}
			body, _ := io.ReadAll(r.Body)
			if strings.TrimSpace(string(body)) != "{}" {
				t.Fatalf("setUserSettings body should be empty json, got %q", string(body))
			}
			_, _ = w.Write([]byte(`{"userSettings":{}}`))
		case antigravityFetchUserInfoPath:
			var payload map[string]string
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["project"] != "proj-1" {
				t.Fatalf("fetchUserInfo missing project, got %v", payload)
			}
			_, _ = w.Write([]byte(`{"userSettings":{},"regionCode":"us"}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := newServiceForPrivacyTest(t, nil)
	upstream := "antigravity_desktop"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &upstream,
		Metadata:       map[string]any{"project_id": "proj-1", "antigravity_base_url": srv.URL},
	}, map[string]any{"access_token": "secret"})

	if got["privacy_mode"] != AntigravityPrivacyModeSet {
		t.Fatalf("expected privacy_set after both calls succeed, got %v", got["privacy_mode"])
	}
	if calls.count() != 2 {
		t.Fatalf("expected exactly 2 backend calls, got %d", calls.count())
	}
	if got["access_token"] != "secret" {
		t.Fatalf("enrichment must not drop access_token, got %v", got)
	}
}

func TestEnrichAntigravityOAuthCredentialMarksFailedWhenSetReturnsTelemetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == antigravitySetUserSettingsPath {
			// Upstream "accepted" but echoed telemetryEnabled — sub2api
			// treats this as the toggle not actually flipping.
			_, _ = w.Write([]byte(`{"userSettings":{"telemetryEnabled":true}}`))
			return
		}
		t.Fatalf("verify step must NOT run when setUserSettings rejects")
	}))
	defer srv.Close()

	svc := newServiceForPrivacyTest(t, nil)
	upstream := "antigravity_desktop"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &upstream,
		Metadata:       map[string]any{"project_id": "proj-1", "antigravity_base_url": srv.URL},
	}, map[string]any{"access_token": "secret"})
	if got["privacy_mode"] != AntigravityPrivacyModeFailed {
		t.Fatalf("expected privacy_set_failed when telemetry stays on, got %v", got["privacy_mode"])
	}
}

func TestEnrichAntigravityOAuthCredentialMarksFailedWhenVerifyReturnsTelemetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case antigravitySetUserSettingsPath:
			_, _ = w.Write([]byte(`{"userSettings":{}}`))
		case antigravityFetchUserInfoPath:
			_, _ = w.Write([]byte(`{"userSettings":{"telemetryEnabled":true}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := newServiceForPrivacyTest(t, nil)
	upstream := "antigravity_desktop"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &upstream,
		Metadata:       map[string]any{"project_id": "proj-1", "antigravity_base_url": srv.URL},
	}, map[string]any{"access_token": "secret"})
	if got["privacy_mode"] != AntigravityPrivacyModeFailed {
		t.Fatalf("expected privacy_set_failed when verification echoes telemetry, got %v", got["privacy_mode"])
	}
}

func TestEnrichAntigravityOAuthCredentialMarksFailedOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer srv.Close()

	svc := newServiceForPrivacyTest(t, nil)
	upstream := "antigravity_desktop"
	got := svc.enrichAntigravityOAuthCredential(context.Background(), contract.AccountRuntime{
		UpstreamClient: &upstream,
		Metadata:       map[string]any{"project_id": "proj-1", "antigravity_base_url": srv.URL},
	}, map[string]any{"access_token": "secret"})
	if got["privacy_mode"] != AntigravityPrivacyModeFailed {
		t.Fatalf("expected privacy_set_failed on 403, got %v", got["privacy_mode"])
	}
}

func TestAntigravityResponseTelemetryAbsent(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"empty body counts as absent", "", true},
		{"empty userSettings counts as absent", `{"userSettings":{}}`, true},
		{"missing userSettings counts as absent", `{"regionCode":"us"}`, true},
		{"telemetry key present counts as present", `{"userSettings":{"telemetryEnabled":true}}`, false},
		{"telemetry key with false still present", `{"userSettings":{"telemetryEnabled":false}}`, false},
		{"unparseable counts as present", `not-json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := antigravityResponseTelemetryAbsent([]byte(tc.body)); got != tc.want {
				t.Fatalf("body=%q got=%v want=%v", tc.body, got, tc.want)
			}
		})
	}
}

func TestAntigravityCredentialProjectIDReadsFromMultipleSources(t *testing.T) {
	upstream := "antigravity_desktop"
	if got := antigravityCredentialProjectID(contract.AccountRuntime{UpstreamClient: &upstream}, map[string]any{"antigravity_project_id": "from-cred"}); got != "from-cred" {
		t.Fatalf("credential antigravity_project_id wins, got %q", got)
	}
	if got := antigravityCredentialProjectID(contract.AccountRuntime{UpstreamClient: &upstream, Metadata: map[string]any{"project_id": "from-metadata"}}, nil); got != "from-metadata" {
		t.Fatalf("metadata project_id wins when credential lacks one, got %q", got)
	}
	if got := antigravityCredentialProjectID(contract.AccountRuntime{UpstreamClient: &upstream, Metadata: map[string]any{"cloudaicompanion_project": "legacy-key"}}, nil); got != "legacy-key" {
		t.Fatalf("legacy cloudaicompanion_project should still be honored, got %q", got)
	}
	if got := antigravityCredentialProjectID(contract.AccountRuntime{UpstreamClient: &upstream}, nil); got != "" {
		t.Fatalf("missing project_id should yield empty string, got %q", got)
	}
}

// newServiceForPrivacyTest builds a Service wired with the default
// HTTP client — the privacy enforcement helper does not need the
// full reverse-proxy surface so we keep this minimal.
func newServiceForPrivacyTest(t *testing.T, _ map[string]any) *Service {
	t.Helper()
	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return svc
}

type privacyServerCallRecorder struct {
	mu sync.Mutex
	n  int
}

func newPrivacyServerCallRecorder() *privacyServerCallRecorder {
	return &privacyServerCallRecorder{}
}

func (r *privacyServerCallRecorder) record(_ *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
}

func (r *privacyServerCallRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

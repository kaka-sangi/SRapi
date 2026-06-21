// Antigravity privacy enforcement — on every successful OAuth refresh,
// turn off the upstream's "share usage data with Google Cloud" telemetry
// toggle. Without this, every request through a managed Antigravity
// account contributes to Google's telemetry / training pipeline, which
// is a real correctness problem for operators serving sensitive
// workloads.
//
// Ported from sub2api's two-step flow (see
// backend/internal/service/antigravity_privacy_service.go):
//
//  1. POST `…/v1internal/users/me:setUserSettings` with an empty body —
//     a 2xx response with `{"userSettings":{}}` (or empty) means the
//     toggle accepted the clear.
//  2. POST `…/v1internal:fetchUserInfo` with the project_id — a 2xx
//     response with no `telemetryEnabled` key under `userSettings`
//     confirms the toggle is off.
//
// Both calls are best-effort. A success persists `privacy_mode =
// privacy_set` into the credential so subsequent refresh passes skip
// the round-trip; a failure persists `privacy_mode =
// privacy_set_failed` so the worker can re-attempt on the next pass.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const (
	// AntigravityPrivacyModeSet marks the credential as having had
	// telemetry confirmed disabled on the upstream. Matches sub2api's
	// `AntigravityPrivacySet` verbatim so log lines and admin tooling
	// line up.
	AntigravityPrivacyModeSet = "privacy_set"
	// AntigravityPrivacyModeFailed marks the credential as having
	// attempted the enforcement and failed. The refresh worker re-tries
	// on the next pass. Matches sub2api `AntigravityPrivacyFailed`.
	AntigravityPrivacyModeFailed = "privacy_set_failed"

	antigravityPrivacyEnforcementTimeout = 10 * time.Second

	antigravityPrivacyDefaultBaseURL = "https://cloudcode-pa.googleapis.com"
	antigravitySetUserSettingsPath   = "/v1internal/users/me:setUserSettings"
	antigravityFetchUserInfoPath     = "/v1internal:fetchUserInfo"
)

// enrichAntigravityOAuthCredential is the entry point. Returns the
// (possibly new) credential and never errors — failures are surfaced
// via the persisted `privacy_mode` key, which the refresh worker
// retries on the next pass.
func (s *Service) enrichAntigravityOAuthCredential(ctx context.Context, account contract.AccountRuntime, credential map[string]any) map[string]any {
	if !antigravityUpstreamClient(account) {
		return credential
	}
	accessToken := credentialString(credential, "access_token")
	if accessToken == "" {
		return credential
	}
	if credentialString(credential, "privacy_mode") == AntigravityPrivacyModeSet {
		return credential
	}
	projectID := antigravityCredentialProjectID(account, credential)
	if projectID == "" {
		// Without the project_id the second-step verification call
		// cannot run. Mark as failed so the operator can fix the
		// metadata and retry; do not silently leave the credential as
		// "unknown".
		return setAntigravityPrivacyMode(credential, AntigravityPrivacyModeFailed)
	}
	enrichCtx, cancel := context.WithTimeout(ctx, antigravityPrivacyEnforcementTimeout)
	defer cancel()
	mode := s.enforceAntigravityPrivacy(enrichCtx, account, accessToken, projectID)
	return setAntigravityPrivacyMode(credential, mode)
}

// enforceAntigravityPrivacy runs the two-step flow and returns the
// resulting privacy_mode value. Best-effort: HTTP / parse failures
// resolve to AntigravityPrivacyModeFailed and the next refresh pass
// retries.
func (s *Service) enforceAntigravityPrivacy(ctx context.Context, account contract.AccountRuntime, accessToken, projectID string) string {
	baseURL := antigravityPrivacyBaseURL(account)
	client, err := s.clientFor(account)
	if err != nil {
		return AntigravityPrivacyModeFailed
	}

	if !s.antigravityPostSetUserSettings(ctx, client, baseURL, accessToken) {
		return AntigravityPrivacyModeFailed
	}
	if !s.antigravityVerifyTelemetryDisabled(ctx, client, baseURL, accessToken, projectID) {
		return AntigravityPrivacyModeFailed
	}
	return AntigravityPrivacyModeSet
}

// antigravityPostSetUserSettings sends the empty-body clear-toggle
// request. A 2xx response with no `telemetryEnabled` key counts as
// success.
func (s *Service) antigravityPostSetUserSettings(ctx context.Context, client *http.Client, baseURL, accessToken string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+antigravitySetUserSettingsPath, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReverseProxyResponseBytes))
	if err != nil {
		return false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	return antigravityResponseTelemetryAbsent(body)
}

// antigravityVerifyTelemetryDisabled fetches user info and confirms
// the absence of the `telemetryEnabled` key. Matches sub2api's
// FetchUserInfoResponse.IsPrivate gate verbatim.
func (s *Service) antigravityVerifyTelemetryDisabled(ctx context.Context, client *http.Client, baseURL, accessToken, projectID string) bool {
	payload, err := json.Marshal(map[string]string{"project": projectID})
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+antigravityFetchUserInfoPath, bytes.NewReader(payload))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReverseProxyResponseBytes))
	if err != nil {
		return false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	return antigravityResponseTelemetryAbsent(body)
}

// antigravityResponseTelemetryAbsent returns true when the parsed
// response has no `userSettings.telemetryEnabled` key. A missing
// `userSettings` object or an empty map both count as success — that
// is the explicit signal Antigravity uses to say "telemetry off".
func antigravityResponseTelemetryAbsent(body []byte) bool {
	var parsed struct {
		UserSettings map[string]any `json:"userSettings"`
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	if len(parsed.UserSettings) == 0 {
		return true
	}
	_, hasTelemetry := parsed.UserSettings["telemetryEnabled"]
	return !hasTelemetry
}

func antigravityCredentialProjectID(account contract.AccountRuntime, credential map[string]any) string {
	for _, key := range []string{"project_id", "antigravity_project_id", "cloudaicompanion_project"} {
		if value := strings.TrimSpace(credentialString(credential, key)); value != "" {
			return value
		}
	}
	return accountSetting(account, "project_id", "antigravity_project_id", "cloudaicompanion_project")
}

func antigravityPrivacyBaseURL(account contract.AccountRuntime) string {
	if value := accountSetting(account, "antigravity_base_url", "base_url"); value != "" {
		return strings.TrimRight(strings.TrimSpace(value), "/")
	}
	return antigravityPrivacyDefaultBaseURL
}

func setAntigravityPrivacyMode(credential map[string]any, mode string) map[string]any {
	if credential == nil {
		credential = make(map[string]any)
	}
	credential["privacy_mode"] = mode
	return credential
}

func antigravityUpstreamClient(account contract.AccountRuntime) bool {
	return upstreamClientIs(account, "antigravity_desktop") || upstreamClientIs(account, "antigravity")
}

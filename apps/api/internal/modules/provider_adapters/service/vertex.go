package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	platformvertex "github.com/srapi/srapi/apps/api/internal/platform/vertex"
)

// Vertex AI integration. The wire format is identical to Gemini's
// generateContent / streamGenerateContent, so this file only:
//
//   1. detects the Vertex account shape (RuntimeClassServiceAccountJSON),
//   2. exchanges the service account JSON for an OAuth2 access token via
//      the shared TokenSource (60s pre-expiry guard + per-(client_email,
//      kid, scope) cache),
//   3. rewrites the request so the downstream geminiCompatibleHeaders
//      branch picks up access_token + auth_mode=bearer, and
//   4. composes the region/project-templated Vertex URL so geminiEndpoint
//      can append the model + action suffix without forking the dispatch
//      pipeline.
//
// The dispatch hook in InvokeConversation reads isVertexAccount(req) and
// short-circuits into invokeVertex, which runs the same gemini-compatible
// HTTP pipeline against the synthesized URL.

const (
	vertexScope         = "https://www.googleapis.com/auth/cloud-platform"
	vertexDefaultRegion = "us-central1"
)

var (
	vertexTokenSourceOnce sync.Once
	vertexTokenSource     *platformvertex.TokenSource
	vertexProjectRotator  = newVertexProjectRotator()
)

func vertexSharedTokenSource() *platformvertex.TokenSource {
	vertexTokenSourceOnce.Do(func() {
		vertexTokenSource = platformvertex.NewTokenSource(nil)
	})
	return vertexTokenSource
}

// vertexProjectRotation tracks the current project cursor for each account
// across the in-process rotator. The cursor is bumped when the upstream
// returns RESOURCE_EXHAUSTED / 429 so the very next dispatch attempt
// targets the next project in the operator-supplied project_ids list. The
// state is per-process — Redis-style synchronization across replicas is
// not in scope here because the cursor is monotone-mod-N and converges
// to the same project quickly.
type vertexProjectRotation struct {
	mu     sync.Mutex
	cursor map[int]int
}

func newVertexProjectRotator() *vertexProjectRotation {
	return &vertexProjectRotation{cursor: map[int]int{}}
}

// pick returns the current project for accountID, applying cursor %
// len(projects) so a shrunk project_ids list still resolves cleanly.
func (r *vertexProjectRotation) pick(accountID int, projects []string) string {
	if len(projects) == 0 {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := r.cursor[accountID] % len(projects)
	return projects[idx]
}

// advance bumps the cursor so the next pick returns the next project.
// Callers pass the project they just used so a race between two
// failing dispatches against the same project only advances once.
func (r *vertexProjectRotation) advance(accountID int, usedProject string, projects []string) {
	if len(projects) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.cursor[accountID] % len(projects)
	if usedProject != "" && projects[current] != usedProject {
		// Someone else already rotated past the project we tried — leave
		// the cursor where it is so we don't double-rotate.
		return
	}
	r.cursor[accountID] = (current + 1) % len(projects)
}

func isVertexAccount(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), string(accountcontract.RuntimeClassServiceAccountJSON))
}

// prepareVertexRequest materializes the access token + bearer auth-mode on a
// copy of req. Returns the mutated request, the resolved Vertex base URL,
// and a typed error if the credential is malformed. Callers must use the
// returned req (Credential is copied, not shared) so concurrent dispatch
// against the same account does not race.
func prepareVertexRequest(ctx context.Context, req contract.ConversationRequest) (contract.ConversationRequest, string, error) {
	sa, err := vertexServiceAccountFromCredential(req.Credential)
	if err != nil {
		return req, "", contract.ProviderError{Class: "configuration_error", StatusCode: http.StatusBadGateway, Message: err.Error()}
	}
	// Resolve region + project BEFORE the network call so a misconfigured
	// account fails fast and cheap instead of consuming an upstream token
	// exchange round-trip every dispatch attempt.
	region := vertexRegion(req.Account.Metadata)
	project := vertexProject(req.Account.ID, req.Account.Metadata, sa)
	if project == "" {
		return req, "", contract.ProviderError{Class: "configuration_error", StatusCode: http.StatusBadGateway, Message: "vertex account missing project_id"}
	}
	token, err := vertexSharedTokenSource().Token(ctx, sa, vertexScope)
	if err != nil {
		return req, "", contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("vertex token exchange failed: %v", err)}
	}
	baseURL := vertexBaseURL(region, project)

	mutated := req
	mutated.Credential = cloneCredentialMap(req.Credential)
	mutated.Credential["access_token"] = token.Value
	mutated.RequestSettings = cloneAnyMap(req.RequestSettings)
	mutated.RequestSettings["auth_mode"] = "bearer"
	return mutated, baseURL, nil
}

// invalidateVertexToken drops the cached token for this credential so the
// next request forces a fresh JWT exchange. Called on 401/403 responses so
// a rotated key or revoked credential surfaces fast instead of serving a
// stale token until its TTL elapses.
func invalidateVertexToken(req contract.ConversationRequest) {
	sa, err := vertexServiceAccountFromCredential(req.Credential)
	if err != nil {
		return
	}
	vertexSharedTokenSource().Invalidate(sa, vertexScope)
}

// vertexHandleDispatchError reacts to upstream failure signals that Vertex
// emits asymmetrically:
//
//   - 401/403 → invalidate the cached access token so a rotated key
//     surfaces on the very next failover attempt.
//   - 429 / quota_exhausted → advance the project rotator so the next
//     attempt routes against a different project_ids[] entry. Operators
//     who have only configured a single project no-op gracefully because
//     the rotator skips the bump when project_ids is empty.
func vertexHandleDispatchError(req contract.ConversationRequest, err error) {
	var providerErr contract.ProviderError
	if !errors.As(err, &providerErr) {
		return
	}
	switch {
	case providerErr.StatusCode == http.StatusUnauthorized || providerErr.StatusCode == http.StatusForbidden:
		invalidateVertexToken(req)
	case providerErr.StatusCode == http.StatusTooManyRequests || providerErr.Class == "quota_exhausted" || providerErr.Class == "rate_limit":
		sa, parseErr := vertexServiceAccountFromCredential(req.Credential)
		if parseErr != nil {
			return
		}
		projects := vertexProjectList(req.Account.Metadata)
		if len(projects) <= 1 {
			return
		}
		used := vertexProject(req.Account.ID, req.Account.Metadata, sa)
		vertexProjectRotator.advance(req.Account.ID, used, projects)
	}
}

func vertexServiceAccountFromCredential(cred map[string]any) (*platformvertex.ServiceAccount, error) {
	if cred == nil {
		return nil, errors.New("vertex credential missing")
	}
	for _, key := range []string{"service_account_json", "service_account", "credentials_json"} {
		switch v := cred[key].(type) {
		case string:
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				normalized, err := platformvertex.NormalizeServiceAccountJSON([]byte(trimmed))
				if err != nil {
					return nil, fmt.Errorf("vertex service account: %w", err)
				}
				return platformvertex.ParseServiceAccount(normalized)
			}
		case map[string]any:
			normalized, err := platformvertex.NormalizeServiceAccountMap(v)
			if err != nil {
				return nil, fmt.Errorf("vertex service account: %w", err)
			}
			raw, err := mapToJSON(normalized)
			if err != nil {
				return nil, err
			}
			return platformvertex.ParseServiceAccount(raw)
		}
	}
	return nil, errors.New("vertex credential missing service_account_json")
}

func vertexBaseURL(region string, project string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		region = vertexDefaultRegion
	}
	project = strings.TrimSpace(project)
	// geminiEndpoint appends /models/{model}:generateContent (or
	// :streamGenerateContent) to whatever base we hand it, so we stop the
	// URL at the publishers/google segment.
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google", region, project, region)
}

func vertexRegion(metadata map[string]any) string {
	if region := mapString(metadata, "region"); region != "" {
		return region
	}
	if region := mapString(metadata, "location"); region != "" {
		return region
	}
	return vertexDefaultRegion
}

func vertexProject(accountID int, metadata map[string]any, sa *platformvertex.ServiceAccount) string {
	// A non-empty project_ids list takes precedence so the rotator can
	// switch between projects on quota exhaustion. Single-project
	// accounts continue to read the legacy project_id metadata field.
	if projects := vertexProjectList(metadata); len(projects) > 0 {
		return vertexProjectRotator.pick(accountID, projects)
	}
	if project := mapString(metadata, "project_id"); project != "" {
		return project
	}
	if project := mapString(metadata, "project"); project != "" {
		return project
	}
	if sa != nil {
		return sa.ProjectID
	}
	return ""
}

// vertexProjectList reads the project_ids array from metadata. Accepts
// both []any (raw JSON unmarshal) and []string (already-typed). Drops
// blank entries so partial form input doesn't ghost-rotate to "".
func vertexProjectList(metadata map[string]any) []string {
	value, ok := metadata["project_ids"]
	if !ok {
		return nil
	}
	var items []any
	switch v := value.(type) {
	case []any:
		items = v
	case []string:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if trimmed := strings.TrimSpace(entry); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func cloneCredentialMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in)+2)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in)+1)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func mapToJSON(values map[string]any) ([]byte, error) {
	return jsonMarshalForVertex(values)
}

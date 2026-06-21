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
	vertexScope        = "https://www.googleapis.com/auth/cloud-platform"
	vertexDefaultRegion = "us-central1"
)

var (
	vertexTokenSourceOnce sync.Once
	vertexTokenSource     *platformvertex.TokenSource
)

func vertexSharedTokenSource() *platformvertex.TokenSource {
	vertexTokenSourceOnce.Do(func() {
		vertexTokenSource = platformvertex.NewTokenSource(nil)
	})
	return vertexTokenSource
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
	project := vertexProject(req.Account.Metadata, sa)
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

// vertexHandleDispatchError forces a token refresh on the next call when the
// upstream rejected our access token. The actual retry happens through the
// standard gateway failover loop — this just prevents us from re-sending
// the same expired token to the next candidate retry.
func vertexHandleDispatchError(req contract.ConversationRequest, err error) {
	var providerErr contract.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode == http.StatusUnauthorized || providerErr.StatusCode == http.StatusForbidden {
			invalidateVertexToken(req)
		}
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

func vertexProject(metadata map[string]any, sa *platformvertex.ServiceAccount) string {
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

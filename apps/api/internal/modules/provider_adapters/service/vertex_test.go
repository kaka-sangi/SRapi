package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func mustVertexServiceAccountJSON(t *testing.T, tokenURI string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	body, err := json.Marshal(map[string]any{
		"type":            "service_account",
		"project_id":      "demo-project",
		"private_key_id":  "kid-vertex",
		"private_key":     string(pemBytes),
		"client_email":    "vertex@demo-project.iam.gserviceaccount.com",
		"token_uri":       tokenURI,
		"universe_domain": "googleapis.com",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(body)
}

// resetVertexTokenSource forces the next request to rebuild the singleton
// + cache. Required because Go's sync.Once provides no public reset; we
// substitute a zero value, which is the documented "not yet done" state.
func resetVertexTokenSource() {
	vertexTokenSourceOnce = sync.Once{}
	vertexTokenSource = nil
}

func TestIsVertexAccount_TrueForServiceAccountRuntime(t *testing.T) {
	req := contract.ConversationRequest{
		Account: accountcontract.ProviderAccount{RuntimeClass: accountcontract.RuntimeClassServiceAccountJSON},
	}
	if !isVertexAccount(req) {
		t.Fatal("expected service_account_json runtime class to be Vertex")
	}
	req.Account.RuntimeClass = accountcontract.RuntimeClassAPIKey
	if isVertexAccount(req) {
		t.Fatal("api_key runtime class must not be Vertex")
	}
}

func TestVertexBaseURL_HonoursRegionAndProject(t *testing.T) {
	got := vertexBaseURL("us-central1", "demo-project")
	want := "https://us-central1-aiplatform.googleapis.com/v1/projects/demo-project/locations/us-central1/publishers/google"
	if got != want {
		t.Fatalf("baseURL mismatch:\n  got: %s\n  want: %s", got, want)
	}
	if !strings.HasPrefix(vertexBaseURL("", "p"), "https://us-central1-") {
		t.Fatal("empty region must fall back to us-central1")
	}
}

func TestPrepareVertexRequest_InjectsBearerTokenAndMintsURL(t *testing.T) {
	var oauthCalls int32
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&oauthCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.vertex-tok",
			"expires_in":   3600,
		})
	}))
	defer oauth.Close()
	resetVertexTokenSource()
	t.Cleanup(resetVertexTokenSource)

	saJSON := mustVertexServiceAccountJSON(t, oauth.URL+"/")
	req := contract.ConversationRequest{
		Account: accountcontract.ProviderAccount{
			RuntimeClass: accountcontract.RuntimeClassServiceAccountJSON,
			Metadata: map[string]any{
				"region":     "europe-west4",
				"project_id": "demo-project",
			},
		},
		Credential: map[string]any{
			"service_account_json": saJSON,
		},
	}
	if !isVertexAccount(req) {
		t.Fatal("precondition: req should be detected as Vertex")
	}
	prepared, baseURL, err := prepareVertexRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if baseURL != "https://europe-west4-aiplatform.googleapis.com/v1/projects/demo-project/locations/europe-west4/publishers/google" {
		t.Fatalf("baseURL mismatch: %s", baseURL)
	}
	if prepared.Credential["access_token"] != "ya29.vertex-tok" {
		t.Fatalf("access_token not injected: %+v", prepared.Credential)
	}
	if prepared.RequestSettings["auth_mode"] != "bearer" {
		t.Fatalf("auth_mode not set: %+v", prepared.RequestSettings)
	}
	// The mutation must NOT leak back into the original request's
	// credential map — concurrent dispatches against the same account
	// would otherwise race on the shared map.
	if _, leaked := req.Credential["access_token"]; leaked {
		t.Fatal("prepareVertexRequest mutated the caller's credential map")
	}

	// Second prepare hits the in-memory token cache and does NOT call upstream.
	if _, _, err := prepareVertexRequest(context.Background(), req); err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	if got := atomic.LoadInt32(&oauthCalls); got != 1 {
		t.Fatalf("token cache should collapse identical (client_email, scope), got %d upstream calls", got)
	}
}

func TestPrepareVertexRequest_FailsClearlyOnMissingProject(t *testing.T) {
	saJSON := mustVertexServiceAccountJSON(t, "https://example.invalid/")
	var sa map[string]any
	if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(sa, "project_id")
	stripped, _ := json.Marshal(sa)

	req := contract.ConversationRequest{
		Account: accountcontract.ProviderAccount{
			RuntimeClass: accountcontract.RuntimeClassServiceAccountJSON,
			Metadata:     map[string]any{"region": "us-central1"},
		},
		Credential: map[string]any{"service_account_json": string(stripped)},
	}
	_, _, err := prepareVertexRequest(context.Background(), req)
	if err == nil {
		t.Fatal("expected configuration_error when project_id is unknown")
	}
	var providerErr contract.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Class != "configuration_error" {
		t.Fatalf("expected provider configuration_error, got %T %v", err, err)
	}
	if !strings.Contains(providerErr.Message, "project_id") {
		t.Fatalf("expected project_id mention in error, got %q", providerErr.Message)
	}
}

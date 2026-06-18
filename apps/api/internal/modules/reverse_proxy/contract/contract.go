package contract

import (
	"context"
	"io"
	"net/http"
	"time"
)

const (
	CodexOAuthTokenURL       = "https://auth.openai.com/oauth/token"
	CodexOAuthAuthorizeURL   = "https://auth.openai.com/oauth/authorize?codex_cli_simplified_flow=true&id_token_add_organizations=true&prompt=login"
	CodexOAuthClientID       = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexOAuthAuthorizeScope = "openid profile email offline_access"
	CodexOAuthRefreshScope   = "openid profile email"
	ClaudeCodeOAuthTokenURL  = "https://api.anthropic.com/v1/oauth/token"
	ClaudeCodeOAuthClientID  = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	AntigravityOAuthTokenURL = "https://oauth2.googleapis.com/token"
	AntigravityOAuthClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
)

type AccountRuntime struct {
	AccountID      int
	RuntimeClass   string
	UpstreamClient *string
	ProxyID        *string
	UserAgent      string
	Metadata       map[string]any
	Credential     map[string]any
}

type Request struct {
	Account      AccountRuntime
	Method       string
	URL          string
	Headers      http.Header
	Body         []byte
	ExpectStream bool
}

type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

type Runtime interface {
	Do(ctx context.Context, req Request) (Response, error)
	// ManagedEgressClient returns the per-account egress HTTP client (proxy + TLS
	// fingerprint + SSRF guard) and true only when the account is configured for
	// managed egress; otherwise (nil, false, nil) so the caller keeps its default
	// client.
	ManagedEgressClient(account AccountRuntime) (*http.Client, bool, error)
}

// StreamResponse carries a live, unbuffered upstream response for incremental
// streaming. The caller owns Body and MUST close it.
type StreamResponse struct {
	StatusCode int
	Headers    http.Header
	Body       io.ReadCloser
}

// StreamRuntime is implemented by reverse-proxy runtimes that can forward an
// upstream response incrementally (returning the live body) rather than
// buffering it. It is an optional capability: callers type-assert to it and
// fall back to buffered Do when unavailable.
type StreamRuntime interface {
	// DoStream performs the upstream request and returns the live response body
	// on a 2xx status. On a non-2xx status it consumes (bounded) and closes the
	// body and returns a classified RuntimeError, so callers can fail over
	// before writing anything downstream.
	DoStream(ctx context.Context, req Request) (StreamResponse, error)
}

type WebSocketMessageType string

const (
	WebSocketMessageText   WebSocketMessageType = "text"
	WebSocketMessageBinary WebSocketMessageType = "binary"
)

type WebSocketMessage struct {
	Type WebSocketMessageType
	Data []byte
}

type WebSocketRelayRequest struct {
	Account          AccountRuntime
	URL              string
	Headers          http.Header
	Subprotocols     []string
	ClientToUpstream <-chan WebSocketMessage
	UpstreamToClient chan<- WebSocketMessage
}

type WebSocketRelayResult struct {
	UpstreamStatusCode int
	Subprotocol        string
	StartedAt          time.Time
	EndedAt            time.Time
	MessagesUpstream   int
	MessagesDownstream int
	BytesUpstream      int
	BytesDownstream    int
}

type WebSocketRuntime interface {
	RelayWebSocket(ctx context.Context, req WebSocketRelayRequest) (WebSocketRelayResult, error)
}

type RefreshRequest struct {
	Account AccountRuntime
}

type RefreshResponse struct {
	AccountID   int
	Credential  map[string]any
	RefreshedAt string
}

type Refresher interface {
	Refresh(ctx context.Context, req RefreshRequest) (RefreshResponse, error)
}

type MetricsSnapshot struct {
	RequestTotal        int
	RequestSuccessTotal int
	RequestErrorTotal   map[string]int
	ChallengeTotal      map[string]int
	AccountLockedTotal  int
	AccountBannedTotal  int
	OAuthRefreshTotal   map[string]int
}

type MetricsReporter interface {
	Metrics() MetricsSnapshot
}

type RuntimeError struct {
	Class      string
	StatusCode int
	Message    string
}

func (e RuntimeError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Class != "" {
		return e.Class
	}
	return "reverse proxy runtime error"
}

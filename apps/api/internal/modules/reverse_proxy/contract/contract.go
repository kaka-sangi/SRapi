package contract

import (
	"context"
	"net/http"
	"time"
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

package contract

import (
	"context"
	"net/http"
)

type AccountRuntime struct {
	AccountID      int
	RuntimeClass   string
	UpstreamClient *string
	ProxyID        *string
	UserAgent      string
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

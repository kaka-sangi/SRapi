package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	erroreventcontract "github.com/srapi/srapi/apps/api/internal/modules/error_event_stream/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

// TestAdminErrorStreamSSEHappyPath verifies the SSE endpoint:
//   - rejects unauthenticated requests with 403
//   - upgrades authenticated admin sessions to text/event-stream
//   - delivers a published Event payload as a SSE data: frame
func TestAdminErrorStreamSSEHappyPath(t *testing.T) {
	t.Parallel()
	handler, srv := newWithServer(config.Load(), nil)
	if srv == nil || srv.runtime == nil || srv.runtime.errorEventStream == nil {
		t.Fatal("publisher not wired into runtime")
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Unauthenticated → 403.
	unauth, err := http.Get(ts.URL + "/api/v1/admin/error-stream")
	if err != nil {
		t.Fatalf("unauthenticated GET: %v", err)
	}
	if unauth.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", unauth.StatusCode)
	}
	_ = unauth.Body.Close()

	// Admin login.
	loginReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/login",
		strings.NewReader(`{"email":"admin@srapi.local","password":"password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login expected 200, got %d", loginResp.StatusCode)
	}
	cookies := loginResp.Cookies()
	_ = loginResp.Body.Close()
	if len(cookies) == 0 {
		t.Fatal("login returned no cookie")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	streamReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/admin/error-stream", nil)
	for _, c := range cookies {
		streamReq.AddCookie(c)
	}
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer streamResp.Body.Close()

	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("SSE expected 200, got %d", streamResp.StatusCode)
	}
	if got := streamResp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}

	// Give the handler a beat to register its subscriber before publishing
	// — Subscribe runs after the response headers ship.
	waitForSubscriber(t, srv, time.Second)

	ev := erroreventcontract.Event{
		AtUnixMs:          time.Now().UnixMilli(),
		RequestID:         "req-sse-1",
		AccountName:       "acct-sse",
		ProviderName:      "provider-sse",
		Model:             "canonical-sse",
		SourceEndpoint:    "/v1/chat/completions",
		SourceProtocol:    "openai-compatible",
		TargetProtocol:    "anthropic-compatible",
		AttemptNo:         2,
		StatusCode:        502,
		UpstreamRequestID: "upstream-sse",
		ErrorClass:        "server_bad",
		ErrorPhase:        "upstream",
		ErrorOwner:        "provider",
		ErrorSource:       "upstream_http",
		Message:           "live test boom",
	}
	if err := srv.runtime.errorEventStream.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	frame, err := readSSEFrame(streamResp.Body, 3*time.Second)
	if err != nil {
		t.Fatalf("read SSE frame: %v", err)
	}
	if frame.Event != "gateway_error" {
		t.Fatalf("expected gateway_error SSE event, got %q", frame.Event)
	}
	var got erroreventcontract.Event
	if err := json.Unmarshal([]byte(frame.Data), &got); err != nil {
		t.Fatalf("decode SSE payload %q: %v", frame.Data, err)
	}
	if got.RequestID != ev.RequestID || got.ErrorClass != ev.ErrorClass || got.StatusCode != ev.StatusCode {
		t.Fatalf("SSE event mismatch: got %+v want %+v", got, ev)
	}
	if got.ProviderName != ev.ProviderName || got.AccountName != ev.AccountName || got.TargetProtocol != ev.TargetProtocol || got.UpstreamRequestID != ev.UpstreamRequestID {
		t.Fatalf("SSE evidence fields mismatch: got %+v want %+v", got, ev)
	}
}

func TestPublishErrorEventIncludesRoutingEvidence(t *testing.T) {
	t.Parallel()
	_, srv := newWithServer(config.Load(), nil)
	if srv == nil || srv.runtime == nil || srv.runtime.errorEventStream == nil {
		t.Fatal("publisher not wired into runtime")
	}

	sub, err := srv.runtime.errorEventStream.Subscribe(context.Background(), erroreventcontract.SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Close() }()

	providerErr := newTestProviderError(429, "quota exhausted", http.Header{
		"X-Request-Id": []string{"upstream-live-1"},
	}, map[string]any{"code": "rate_limit_exceeded"})
	result := newTestScheduleResultForAttempt(3, 42, "acct-live", "upstream-live-model")
	result.Candidate.Provider = providercontract.Provider{
		ID:       9,
		Name:     "openai-live",
		Protocol: "openai-compatible",
	}
	result.Candidate.Mapping = modelcontract.ModelProviderMapping{
		UpstreamModelName: "upstream-live-model",
	}
	canonical := testCanonicalRequest("req-live-evidence")
	canonical.SourceEndpoint = "/v1/chat/completions"
	canonical.SourceProtocol = "openai-compatible"
	canonical.Model = "public-live-model"
	canonical.CanonicalModel = "canonical-live-model"

	srv.publishErrorEvent(context.Background(), testAuthResult(7, 8), canonical, result, providerErr, "transient", http.StatusTooManyRequests)

	select {
	case got := <-sub.Receive():
		if got.RequestID != "req-live-evidence" {
			t.Fatalf("request_id: got %q", got.RequestID)
		}
		if got.UserID == nil || *got.UserID != 7 {
			t.Fatalf("user_id: got %v want 7", got.UserID)
		}
		if got.AccountID == nil || *got.AccountID != 42 {
			t.Fatalf("account_id: got %v want 42", got.AccountID)
		}
		if got.ProviderID == nil || *got.ProviderID != 9 {
			t.Fatalf("provider_id: got %v want 9", got.ProviderID)
		}
		if got.AccountName != "acct-live" || got.ProviderName != "openai-live" {
			t.Fatalf("unexpected account/provider names: %+v", got)
		}
		if got.Model != "canonical-live-model" || got.RequestedModel != "public-live-model" || got.UpstreamModel != "upstream-live-model" {
			t.Fatalf("unexpected model evidence: %+v", got)
		}
		if got.SourceEndpoint != "/v1/chat/completions" || got.SourceProtocol != "openai-compatible" || got.TargetProtocol != "openai-compatible" {
			t.Fatalf("unexpected protocol evidence: %+v", got)
		}
		if got.AttemptNo != 3 || got.UpstreamRequestID != "upstream-live-1" {
			t.Fatalf("unexpected attempt/upstream request id: %+v", got)
		}
		if got.ErrorPhase != "upstream" || got.ErrorOwner != "provider" || got.ErrorSource != "upstream_http" {
			t.Fatalf("unexpected classification evidence: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func testCanonicalRequest(requestID string) gatewaycontract.CanonicalRequest {
	return gatewaycontract.CanonicalRequest{
		RequestID: requestID,
	}
}

func testAuthResult(userID int, keyID int) apikeycontract.AuthResult {
	return apikeycontract.AuthResult{
		UserID: userID,
		Key: apikeycontract.APIKey{
			ID: keyID,
		},
	}
}

func TestAdminErrorStreamRejectsInvalidFilters(t *testing.T) {
	t.Parallel()
	handler, _ := newWithServer(config.Load(), nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	loginReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/login",
		strings.NewReader(`{"email":"admin@srapi.local","password":"password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	cookies := loginResp.Cookies()
	_ = loginResp.Body.Close()

	invalidReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/error-stream?min_status=500&max_status=400", nil)
	for _, c := range cookies {
		invalidReq.AddCookie(c)
	}
	invalidResp, err := http.DefaultClient.Do(invalidReq)
	if err != nil {
		t.Fatalf("open filtered SSE: %v", err)
	}
	defer invalidResp.Body.Close()

	if invalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid filters, got %d", invalidResp.StatusCode)
	}
}

func waitForSubscriber(t *testing.T, srv *Server, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if srv.runtime.errorEventStream.SubscriberCount() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("subscriber never registered")
}

type sseFrame struct {
	Event string
	Data  string
}

// readSSEFrame reads one SSE frame from the stream and returns its event name
// and JSON payload. Comments are skipped.
func readSSEFrame(body io.Reader, timeout time.Duration) (sseFrame, error) {
	type result struct {
		s   sseFrame
		err error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 256)
		frame := sseFrame{}
		for {
			n, err := body.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				for {
					nl := indexNewline(buf)
					if nl < 0 {
						break
					}
					line := string(buf[:nl])
					buf = buf[nl+1:]
					line = strings.TrimSuffix(line, "\r")
					if line == "" {
						if frame.Data != "" {
							ch <- result{s: frame}
							return
						}
						continue
					}
					if strings.HasPrefix(line, ":") {
						continue
					}
					if strings.HasPrefix(line, "event: ") {
						frame.Event = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
						continue
					}
					if strings.HasPrefix(line, "data: ") {
						frame.Data = strings.TrimPrefix(line, "data: ")
						continue
					}
				}
			}
			if err != nil {
				ch <- result{err: err}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		return r.s, r.err
	case <-time.After(timeout):
		return sseFrame{}, context.DeadlineExceeded
	}
}

func indexNewline(b []byte) int {
	for i, c := range b {
		if c == '\n' {
			return i
		}
	}
	return -1
}

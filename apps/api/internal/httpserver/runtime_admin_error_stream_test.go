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
	erroreventcontract "github.com/srapi/srapi/apps/api/internal/modules/error_event_stream/contract"
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
		AtUnixMs:   time.Now().UnixMilli(),
		RequestID:  "req-sse-1",
		StatusCode: 502,
		ErrorClass: "server_bad",
		Message:    "live test boom",
	}
	if err := srv.runtime.errorEventStream.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	frame, err := readSSEFrame(streamResp.Body, 3*time.Second)
	if err != nil {
		t.Fatalf("read SSE frame: %v", err)
	}
	var got erroreventcontract.Event
	if err := json.Unmarshal([]byte(frame), &got); err != nil {
		t.Fatalf("decode SSE payload %q: %v", frame, err)
	}
	if got.RequestID != ev.RequestID || got.ErrorClass != ev.ErrorClass || got.StatusCode != ev.StatusCode {
		t.Fatalf("SSE event mismatch: got %+v want %+v", got, ev)
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

// readSSEFrame reads one "data: …" line from the SSE body and returns the JSON
// payload. Skips comments + event: lines.
func readSSEFrame(body io.Reader, timeout time.Duration) (string, error) {
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 256)
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
					if strings.HasPrefix(line, "data: ") {
						ch <- result{s: strings.TrimPrefix(line, "data: ")}
						return
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
		return "", context.DeadlineExceeded
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

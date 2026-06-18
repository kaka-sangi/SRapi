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

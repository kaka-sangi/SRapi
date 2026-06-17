package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
)

// TestClassifyUpstreamError_StatusCodes pins the directive's bucket policy:
// 401/403 -> account_bad+blacklist, 429 -> transient w/ Retry-After,
// 5xx -> server_bad+failover, 408 -> transient, other 4xx -> client_bad.
func TestClassifyUpstreamError_StatusCodes(t *testing.T) {
	cases := []struct {
		name            string
		status          int
		wantClass       string
		wantFailover    bool
		wantBlacklist   bool
	}{
		{"401 unauthorized", 401, "account_bad", true, true},
		{"403 forbidden", 403, "account_bad", true, true},
		{"429 too many requests", 429, "transient", true, false},
		{"408 request timeout", 408, "transient", true, false},
		{"400 bad request", 400, "client_bad", false, false},
		{"404 not found", 404, "client_bad", false, false},
		{"422 unprocessable", 422, "client_bad", false, false},
		{"500 internal", 500, "server_bad", true, false},
		{"502 bad gateway", 502, "server_bad", true, false},
		{"503 service unavailable", 503, "server_bad", true, false},
		{"504 gateway timeout", 504, "server_bad", true, false},
		{"599 max 5xx", 599, "server_bad", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyUpstreamError(tc.status, nil, nil)
			if got.Class != tc.wantClass {
				t.Fatalf("Class = %q, want %q", got.Class, tc.wantClass)
			}
			if got.ShouldFailover != tc.wantFailover {
				t.Fatalf("ShouldFailover = %v, want %v", got.ShouldFailover, tc.wantFailover)
			}
			if got.ShouldBlacklist != tc.wantBlacklist {
				t.Fatalf("ShouldBlacklist = %v, want %v", got.ShouldBlacklist, tc.wantBlacklist)
			}
		})
	}
}

func TestClassifyUpstreamError_RetryAfter_Seconds(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "5")
	got := classifyUpstreamErrorWithHeader(429, headers, nil, nil)
	if got.RetryAfterMs != 5000 {
		t.Fatalf("RetryAfterMs = %d, want 5000", got.RetryAfterMs)
	}
}

func TestClassifyUpstreamError_RetryAfter_NegativeIgnored(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "-3")
	got := classifyUpstreamErrorWithHeader(429, headers, nil, nil)
	if got.RetryAfterMs != 0 {
		t.Fatalf("RetryAfterMs = %d, want 0 (negative ignored)", got.RetryAfterMs)
	}
}

func TestClassifyUpstreamError_RetryAfter_Garbage(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "not-a-number")
	got := classifyUpstreamErrorWithHeader(503, headers, nil, nil)
	if got.RetryAfterMs != 0 {
		t.Fatalf("RetryAfterMs = %d, want 0", got.RetryAfterMs)
	}
}

// TestClassifyUpstreamError_NetworkErrors mirrors sub2api's
// classifyOpenAITransportError taxonomy: typed-errors first, then string markers.
func TestClassifyUpstreamError_NetworkErrors(t *testing.T) {
	cases := []struct {
		name            string
		err             error
		wantClass       string
		wantFailover    bool
		wantBlacklist   bool
	}{
		{"context canceled", context.Canceled, "transient", false, false},
		{"wrapped context canceled", fmt.Errorf("http: %w", context.Canceled), "transient", false, false},
		{"context deadline exceeded", context.DeadlineExceeded, "transient", true, false},
		{"io eof", io.EOF, "transient", true, false},
		{"unexpected eof", io.ErrUnexpectedEOF, "transient", true, false},
		{"econnrefused bare", syscall.ECONNREFUSED, "account_bad", true, true},
		{"econnrefused via opError", &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}, "account_bad", true, true},
		{"ehostunreach", syscall.EHOSTUNREACH, "account_bad", true, true},
		{"enetunreach", syscall.ENETUNREACH, "account_bad", true, true},
		{"dns not found", &net.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}, "account_bad", true, true},
		{"dns timeout (not persistent)", &net.DNSError{Err: "i/o timeout", Name: "x.invalid", IsTimeout: true}, "transient", true, false},
		{"socks5 auth failed", errors.New(`socks connect tcp 1.2.3.4:1080->host:443: username/password authentication failed`), "account_bad", true, true},
		{"proxy auth required marker", errors.New(`proxy authentication required`), "account_bad", true, true},
		{"connection reset by peer", errors.New(`read tcp x->y: read: connection reset by peer`), "transient", true, false},
		{"i/o timeout marker", errors.New(`dial tcp 1.2.3.4:443: i/o timeout`), "transient", true, false},
		{"broken pipe", errors.New(`write tcp x->y: write: broken pipe`), "transient", true, false},
		{"unknown error", errors.New(`something weird happened`), "transient", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyUpstreamError(0, nil, tc.err)
			if got.Class != tc.wantClass {
				t.Fatalf("Class = %q, want %q", got.Class, tc.wantClass)
			}
			if got.ShouldFailover != tc.wantFailover {
				t.Fatalf("ShouldFailover = %v, want %v", got.ShouldFailover, tc.wantFailover)
			}
			if got.ShouldBlacklist != tc.wantBlacklist {
				t.Fatalf("ShouldBlacklist = %v, want %v", got.ShouldBlacklist, tc.wantBlacklist)
			}
		})
	}
}

// 2xx/3xx must never be classified as failover-worthy. Defensive — callers
// shouldn't invoke the classifier on success but if they do, no blacklist.
func TestClassifyUpstreamError_NonFailureStatus(t *testing.T) {
	for _, status := range []int{200, 201, 204, 301, 304} {
		got := ClassifyUpstreamError(status, nil, nil)
		if got.ShouldFailover || got.ShouldBlacklist {
			t.Fatalf("status %d: unexpected failover=%v blacklist=%v", status, got.ShouldFailover, got.ShouldBlacklist)
		}
	}
}

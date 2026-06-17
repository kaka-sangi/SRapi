package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
)

// TestFileWriter_DisabledIsNoOp asserts the writer is silent when capture
// is off: no file is created and every method returns nil.
func TestFileWriter_DisabledIsNoOp(t *testing.T) {
	dir := t.TempDir()
	w := NewFileWriter(Config{Enabled: false, LogDir: dir})

	h, err := w.Begin(context.Background(), rlfcontract.BeginRequest{RequestID: "req_disabled"})
	if err != nil {
		t.Fatalf("Begin err: %v", err)
	}
	if h.Path() != "" {
		t.Fatalf("expected empty handle path when disabled, got %q", h.Path())
	}
	if err := w.AppendOutboundRequest(h, 1, "https://x", "GET", nil, []byte("body")); err != nil {
		t.Fatalf("AppendOutboundRequest err: %v", err)
	}
	if err := w.AppendUpstreamResponse(h, 1, 200, nil, []byte("ok")); err != nil {
		t.Fatalf("AppendUpstreamResponse err: %v", err)
	}
	if err := w.Finalize(h, rlfcontract.FinalizeResult{Success: true}); err != nil {
		t.Fatalf("Finalize err: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files when disabled, got %d", len(entries))
	}
}

// TestFileWriter_MultiAttemptSections asserts the on-disk format carries
// numbered REQUEST/RESPONSE sections so the same file holds the full
// failover history of one logical request.
func TestFileWriter_MultiAttemptSections(t *testing.T) {
	dir := t.TempDir()
	w := NewFileWriter(Config{Enabled: true, LogDir: dir})

	uid, kid, aid := 7, 11, 13
	started := time.Date(2026, 6, 18, 3, 4, 5, 0, time.UTC)
	h, err := w.Begin(context.Background(), rlfcontract.BeginRequest{
		RequestID:      "req_abc",
		UserID:         &uid,
		APIKeyID:       &kid,
		AccountID:      &aid,
		SourceProtocol: "openai",
		SourceEndpoint: "/v1/chat/completions",
		StartedAt:      started,
	})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if h.Path() == "" {
		t.Fatalf("expected non-empty path")
	}
	base := filepath.Base(h.Path())
	if !strings.HasPrefix(base, "request-") {
		t.Fatalf("expected request- prefix, got %q", base)
	}
	if !strings.Contains(base, "req_abc") {
		t.Fatalf("expected request id in filename, got %q", base)
	}
	expectedMS := started.UnixMilli()
	if !strings.Contains(base, formatInt(expectedMS)) {
		t.Fatalf("expected unix_ms %d in filename %q", expectedMS, base)
	}

	if err := w.AppendOutboundRequest(h, 1, "https://api.openai.com/v1/chat/completions", "POST",
		map[string][]string{
			"Content-Type":  {"application/json"},
			"Authorization": {"Bearer sk-secret"},
		},
		[]byte(`{"model":"gpt-4"}`),
	); err != nil {
		t.Fatalf("AppendOutboundRequest 1: %v", err)
	}
	if err := w.AppendUpstreamResponse(h, 1, 429, map[string][]string{"Retry-After": {"5"}}, []byte("rate limit")); err != nil {
		t.Fatalf("AppendUpstreamResponse 1: %v", err)
	}
	if err := w.AppendOutboundRequest(h, 2, "https://api2.openai.com/v1/chat/completions", "POST", nil, []byte("body2")); err != nil {
		t.Fatalf("AppendOutboundRequest 2: %v", err)
	}
	if err := w.AppendUpstreamResponse(h, 2, 200, map[string][]string{"Content-Type": {"application/json"}}, []byte("ok")); err != nil {
		t.Fatalf("AppendUpstreamResponse 2: %v", err)
	}
	if err := w.Finalize(h, rlfcontract.FinalizeResult{Success: true, StatusCode: 200, LatencyMS: 250}); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	body, err := os.ReadFile(h.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(body)

	for _, want := range []string{
		"=== REQUEST INFO ===",
		"Request-ID: req_abc",
		"User-ID: 7",
		"API-Key-ID: 11",
		"Account-ID: 13",
		"Source-Protocol: openai",
		"Source-Endpoint: /v1/chat/completions",
		"=== REQUEST 1 ===",
		"POST https://api.openai.com/v1/chat/completions",
		"Authorization: [REDACTED]",
		`{"model":"gpt-4"}`,
		"=== RESPONSE 1 ===",
		"Status: 429",
		"Retry-After: 5",
		"rate limit",
		"=== REQUEST 2 ===",
		"POST https://api2.openai.com/v1/chat/completions",
		"=== RESPONSE 2 ===",
		"Status: 200",
		"=== SUMMARY ===",
		"Success: true",
		"Status: 200",
		"Latency-MS: 250",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in log file; got:\n%s", want, content)
		}
	}
	// Bearer token must not leak into the dump.
	if strings.Contains(content, "Bearer sk-secret") {
		t.Errorf("credential leaked into log file")
	}
}

// TestFileWriter_FailureRenamesToError asserts a failed Finalize renames
// the file from request-* to error-* so the cleaner can apply the
// per-prefix cap and the admin filter can pick error logs out cheaply.
func TestFileWriter_FailureRenamesToError(t *testing.T) {
	dir := t.TempDir()
	w := NewFileWriter(Config{Enabled: true, LogDir: dir})
	h, err := w.Begin(context.Background(), rlfcontract.BeginRequest{RequestID: "req_fail"})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := w.Finalize(h, rlfcontract.FinalizeResult{Success: false, ErrorClass: "upstream_5xx", StatusCode: 503, LatencyMS: 42}); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if _, err := os.Stat(h.Path()); !os.IsNotExist(err) {
		t.Fatalf("expected request- file to be renamed away, stat err=%v", err)
	}
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "error-") && strings.Contains(name, "req_fail") {
			found = true
			body, _ := os.ReadFile(filepath.Join(dir, name))
			if !strings.Contains(string(body), "Error-Class: upstream_5xx") {
				t.Errorf("expected Error-Class header in error file; got %s", body)
			}
		}
	}
	if !found {
		t.Fatalf("expected an error-* file, got entries=%+v", entries)
	}
}

// TestSanitizeRequestID exercises the filename-safety stripping for ids
// that contain awkward characters. We don't permit arbitrary user input
// into the filename, but the gateway's request id format is broad enough
// that we want explicit coverage.
func TestSanitizeRequestID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"req_abc", "req_abc"},
		{"a/b\\c", "a-b-c"},
		{"hello world", "hello-world"},
		{"", ""},
		{"--xx--", "xx"},
	}
	for _, c := range cases {
		if got := sanitizeRequestID(c.in); got != c.want {
			t.Errorf("sanitizeRequestID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestResolveEnabled verifies the env override behaviour. We swap the
// env var in-process and restore it on cleanup so other tests are not
// affected.
func TestResolveEnabled(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	if ResolveEnabled(false) {
		t.Fatalf("expected disabled when env empty and config false")
	}
	if !ResolveEnabled(true) {
		t.Fatalf("expected enabled when config true")
	}
	t.Setenv(EnvEnabled, "true")
	if !ResolveEnabled(false) {
		t.Fatalf("expected enabled when env=true")
	}
	t.Setenv(EnvEnabled, "off")
	if ResolveEnabled(false) {
		t.Fatalf("expected disabled when env=off")
	}
}

// formatInt is a tiny helper so the test files do not need strconv just
// for one Itoa call in TestFileWriter_MultiAttemptSections.
func formatInt(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

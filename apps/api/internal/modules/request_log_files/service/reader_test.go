package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
)

func writeFile(t *testing.T, dir, name string, contents string, modTime time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileReader_ListFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	r := NewFileReader(dir)

	t0 := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 6, 18, 2, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 18, 3, 0, 0, 0, time.UTC)
	writeFile(t, dir, "request-1000-req_aaa.log", "x", t0)
	writeFile(t, dir, "error-2000-req_bbb.log", "x", t1)
	writeFile(t, dir, "request-3000-req_ccc.log", "x", t2)
	writeFile(t, dir, "ignore-me.log", "x", t2) // unmanaged — filtered out

	all, err := r.List(context.Background(), rlfcontract.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 managed files, got %d", len(all))
	}
	// Newest first by embedded unix_ms.
	if all[0].Name != "request-3000-req_ccc.log" || all[2].Name != "request-1000-req_aaa.log" {
		t.Fatalf("unexpected sort order: %+v", all)
	}

	errOnly, err := r.List(context.Background(), rlfcontract.ListFilter{ErrorOnly: true})
	if err != nil {
		t.Fatalf("List(ErrorOnly): %v", err)
	}
	if len(errOnly) != 1 || errOnly[0].Name != "error-2000-req_bbb.log" {
		t.Fatalf("expected single error file, got %+v", errOnly)
	}

	prefixed, err := r.List(context.Background(), rlfcontract.ListFilter{RequestIDPrefix: "req_b"})
	if err != nil {
		t.Fatalf("List(prefix): %v", err)
	}
	if len(prefixed) != 1 || prefixed[0].RequestID != "req_bbb" {
		t.Fatalf("expected prefix match, got %+v", prefixed)
	}
}

func TestFileReader_DescriptorIncludesSafeSummary(t *testing.T) {
	dir := t.TempDir()
	r := NewFileReader(dir)

	started := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	name := "error-2000-req_summary.log"
	writeFile(t, dir, name, `=== REQUEST INFO ===
Request-ID: req_summary
User-ID: 42
API-Key-ID: 7
Account-ID: 9
Source-Protocol: openai-compatible
Source-Endpoint: /v1/chat/completions
Started-At: 2026-06-18T10:00:00Z

=== REQUEST 1 ===
POST https://upstream.invalid/v1/chat/completions

=== RESPONSE 1 ===
Status: 429

=== REQUEST 2 ===
POST https://upstream.invalid/v1/chat/completions

=== RESPONSE 2 ===
Status: 503

=== SUMMARY ===
Success: false
Error-Class: server_bad
Status: 503
Latency-MS: 891
`, started)

	desc, err := r.Get(context.Background(), name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if desc.UserID != "42" || desc.APIKeyID != "7" || desc.AccountID != "9" {
		t.Fatalf("unexpected actor metadata: %+v", desc)
	}
	if desc.SourceProtocol != "openai-compatible" || desc.SourceEndpoint != "/v1/chat/completions" {
		t.Fatalf("unexpected source metadata: %+v", desc)
	}
	if desc.StartedAt == nil || !desc.StartedAt.Equal(started) {
		t.Fatalf("unexpected started_at: %v", desc.StartedAt)
	}
	if desc.Success == nil || *desc.Success {
		t.Fatalf("expected success=false, got %v", desc.Success)
	}
	if desc.StatusCode == nil || *desc.StatusCode != 503 {
		t.Fatalf("expected status 503, got %v", desc.StatusCode)
	}
	if desc.ErrorClass != "server_bad" {
		t.Fatalf("expected error class, got %q", desc.ErrorClass)
	}
	if desc.LatencyMS == nil || *desc.LatencyMS != 891 {
		t.Fatalf("expected latency 891, got %v", desc.LatencyMS)
	}
	if desc.AttemptCount != 2 || desc.ResponseCount != 2 || !desc.HasSummary {
		t.Fatalf("unexpected section summary: %+v", desc)
	}

	list, err := r.List(context.Background(), rlfcontract.ListFilter{RequestIDPrefix: "req_summary"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].StatusCode == nil || *list[0].StatusCode != 503 {
		t.Fatalf("expected list descriptor summary, got %+v", list)
	}
}

func TestFileReader_GetOpenDeleteValidatesName(t *testing.T) {
	dir := t.TempDir()
	r := NewFileReader(dir)
	p := writeFile(t, dir, "request-1000-req_get.log", "hello", time.Now())

	desc, err := r.Get(context.Background(), filepath.Base(p))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if desc.RequestID != "req_get" {
		t.Fatalf("unexpected requestID: %q", desc.RequestID)
	}

	body, err := r.Open(context.Background(), filepath.Base(p))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("unexpected body: %q", body)
	}

	// Path traversal attempts must be rejected.
	for _, bad := range []string{"../etc/passwd", "..", ".", "request-1000-../x.log", "weird name.log"} {
		if _, err := r.Get(context.Background(), bad); err == nil {
			t.Errorf("expected error for name %q", bad)
		}
		if err := r.Delete(context.Background(), bad); err == nil {
			t.Errorf("expected error on Delete %q", bad)
		}
	}

	if err := r.Delete(context.Background(), filepath.Base(p)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(context.Background(), filepath.Base(p)); !errors.Is(err, rlfcontract.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Delete, got %v", err)
	}
}

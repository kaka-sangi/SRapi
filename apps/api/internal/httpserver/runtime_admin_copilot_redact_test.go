package httpserver

import (
	"strings"
	"testing"
)

// TestRedactSensitiveJSON proves secret-bearing fields are masked before a tool
// result is fed to the copilot's LLM, while ordinary data survives.
func TestRedactSensitiveJSON(t *testing.T) {
	in := []byte(`{"data":[{"id":"1","name":"prod","api_key":"sk-leak","credential":{"token":"abc"},"nested":{"password":"hunter2","ok":"keep"}}],"plain":"visible"}`)
	out := string(redactSensitiveJSON(in))
	for _, secret := range []string{"sk-leak", "abc", "hunter2"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret %q leaked through redaction: %s", secret, out)
		}
	}
	for _, keep := range []string{"prod", "visible", "keep"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("non-secret %q was wrongly removed: %s", keep, out)
		}
	}
	if !strings.Contains(out, "***redacted***") {
		t.Fatalf("expected a redaction marker: %s", out)
	}
}

// TestRedactSensitiveJSONPassthrough leaves non-JSON bodies untouched.
func TestRedactSensitiveJSONPassthrough(t *testing.T) {
	raw := []byte("HTTP 200 plain text body")
	if got := string(redactSensitiveJSON(raw)); got != string(raw) {
		t.Fatalf("non-JSON body should pass through unchanged, got %q", got)
	}
}

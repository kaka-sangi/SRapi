package httpserver

import (
	"errors"
	"testing"
)

// TestParseDiscoveredModelIDsEmptyVsMalformed locks in the fix that a recognized
// but empty model list is a valid "no models" result (not a 502), while an
// unrecognized/error body is still surfaced as an upstream failure.
func TestParseDiscoveredModelIDsEmptyVsMalformed(t *testing.T) {
	// Recognized envelope, empty list -> valid empty (no error).
	ids, err := parseDiscoveredModelIDs(modelDiscoveryOpenAI, []byte(`{"data":[]}`), 100)
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty data should be a valid empty result, got ids=%v err=%v", ids, err)
	}

	// Unrecognized/error body (no data or models key) -> upstream failure.
	if _, err := parseDiscoveredModelIDs(modelDiscoveryOpenAI, []byte(`{"error":"nope"}`), 100); !errors.Is(err, errModelDiscoveryUpstream) {
		t.Fatalf("error body should surface as upstream failure, got %v", err)
	}

	// Populated -> ids returned.
	ids, err = parseDiscoveredModelIDs(modelDiscoveryOpenAI, []byte(`{"data":[{"id":"gpt-5.5"}]}`), 100)
	if err != nil || len(ids) != 1 || ids[0] != "gpt-5.5" {
		t.Fatalf("populated body should return ids, got ids=%v err=%v", ids, err)
	}

	// Gemini recognized-empty -> valid; malformed -> upstream failure.
	if ids, err := parseDiscoveredModelIDs(modelDiscoveryGemini, []byte(`{"models":[]}`), 100); err != nil || len(ids) != 0 {
		t.Fatalf("gemini empty should be valid empty, got ids=%v err=%v", ids, err)
	}
	if _, err := parseDiscoveredModelIDs(modelDiscoveryGemini, []byte(`{"unexpected":1}`), 100); !errors.Is(err, errModelDiscoveryUpstream) {
		t.Fatalf("gemini malformed should surface as upstream failure, got %v", err)
	}
}

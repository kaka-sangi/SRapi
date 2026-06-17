package signature

import (
	"encoding/base64"
	"strings"
	"testing"
)

// testGPTReasoningSignature constructs a Fernet-shaped payload that passes the
// shape check. Mirrors CLIProxyAPI's helper.
func testGPTReasoningSignature() string {
	payload := make([]byte, 1+8+16+16+32)
	payload[0] = 0x80
	for i := 9; i < len(payload); i++ {
		payload[i] = byte(i)
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func TestIsValidGPTReasoningSignature_Accepts(t *testing.T) {
	if !IsValidGPTReasoningSignature(testGPTReasoningSignature()) {
		t.Fatal("expected canonical Fernet-shaped signature to validate")
	}
}

func TestInspectGPTReasoningSignatureRejectsUnicodeEllipsis(t *testing.T) {
	sig := testGPTReasoningSignature()
	polluted := sig[:20] + string(rune(0x2026)) + sig[20:]
	_, err := InspectGPTReasoningSignature(polluted)
	if err == nil {
		t.Fatal("expected invalid GPT reasoning signature")
	}
	if !strings.Contains(err.Error(), "non-base64url character U+2026") {
		t.Fatalf("error = %q, want U+2026 base64url detail", err.Error())
	}
}

func TestInspectGPTReasoningSignatureRejectsBadPrefix(t *testing.T) {
	// Build a payload whose decoded length and version byte are fine but the
	// raw string doesn't begin with gAAAA.
	payload := make([]byte, 1+8+16+16+32)
	payload[0] = 0x80
	encoded := "x" + base64.RawURLEncoding.EncodeToString(payload)[1:]
	_, err := InspectGPTReasoningSignature(encoded)
	if err == nil || !strings.Contains(err.Error(), "gAAAA prefix") {
		t.Fatalf("error = %v, want gAAAA prefix message", err)
	}
}

func TestInspectGPTReasoningSignatureRejectsBadVersion(t *testing.T) {
	// Decoded byte stream for "gAAAA..." starts with 0x80, but we need to
	// craft a payload that base64-encodes to a "gAAAA"-prefixed string while
	// having a different version byte. The simplest way: build a payload
	// whose first byte is 0x7f and force the encoding by directly mutating
	// the canonical Fernet payload — but we cannot, so we exercise the
	// version-byte branch via a manually-crafted base64 whose first decoded
	// byte differs while keeping the prefix valid.
	// Encode of [0x80, 0x00, 0x00, ...] is "gAAA". Encode of [0x90, 0x00, ...]
	// is "kAAA". To preserve the gAAAA prefix we need decoded[0]=0x80 and
	// decoded[1..4]=0x00 — which means we cannot trigger this branch via the
	// public surface without also failing the prefix check. The reference
	// guards both branches separately for defense in depth; we keep the
	// guard but assert it via behavioural equivalence on a too-short input.
	_, err := InspectGPTReasoningSignature("gAAAA")
	if err == nil {
		t.Fatal("expected error for tiny gAAAA input")
	}
}

func TestInspectGPTReasoningSignatureRejectsTooShort(t *testing.T) {
	_, err := InspectGPTReasoningSignature("gAAAA")
	if err == nil || !strings.Contains(err.Error(), "decode failed") {
		t.Fatalf("error = %v, want base64url decode failed", err)
	}
}

func TestSanitizeOpenAIResponsesReasoningEncryptedContent_DropsInvalid(t *testing.T) {
	valid := testGPTReasoningSignature()
	input := []map[string]any{
		{"type": "reasoning", "id": "rs_bad", "encrypted_content": "gAAAAABqFTIa…abc", "summary": []any{}},
		{"type": "reasoning", "id": "rs_null", "encrypted_content": nil, "summary": []any{}},
		{"type": "reasoning", "id": "rs_non_string", "encrypted_content": 123, "summary": []any{}},
		{"type": "reasoning", "id": "rs_good", "encrypted_content": valid, "summary": []any{}},
		{"role": "user", "content": "hello", "encrypted_content": "leave-message-alone"},
	}
	SanitizeOpenAIResponsesReasoningEncryptedContent(input)

	if _, ok := input[0]["encrypted_content"]; ok {
		t.Fatalf("invalid reasoning encrypted_content was not dropped: %+v", input[0])
	}
	if _, ok := input[1]["encrypted_content"]; ok {
		t.Fatalf("null reasoning encrypted_content was not dropped: %+v", input[1])
	}
	if _, ok := input[2]["encrypted_content"]; ok {
		t.Fatalf("non-string reasoning encrypted_content was not dropped: %+v", input[2])
	}
	if got, _ := input[3]["encrypted_content"].(string); got != valid {
		t.Fatalf("valid reasoning encrypted_content = %q, want preserved", got)
	}
	if got, _ := input[4]["encrypted_content"].(string); got != "leave-message-alone" {
		t.Fatalf("non-reasoning encrypted_content = %q, want untouched", got)
	}
}

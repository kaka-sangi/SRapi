// Package signature ports the GPT/Codex reasoning encrypted_content shape
// check from CLIProxyAPI (internal/signature/gpt_validation.go). The check is
// a Fernet-style transport-shape validation; it does not prove decryptability.
// The original logic is preserved verbatim.
package signature

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// MaxGPTReasoningSignatureLen caps the inspected signature length to prevent
// pathological inputs from burning CPU. Mirrors CLIProxyAPI exactly.
const MaxGPTReasoningSignatureLen = 32 * 1024 * 1024

// GPTReasoningSignatureInfo describes the post-decode shape of a Fernet-like
// GPT reasoning signature. Mirrors CLIProxyAPI exactly.
type GPTReasoningSignatureInfo struct {
	DecodedLen    int
	CiphertextLen int
}

// IsValidGPTReasoningSignature reports whether rawSignature passes the
// transport-shape check.
func IsValidGPTReasoningSignature(rawSignature string) bool {
	_, err := InspectGPTReasoningSignature(rawSignature)
	return err == nil
}

// InspectGPTReasoningSignature validates the Fernet-like outer format used by
// GPT/Codex reasoning encrypted_content. The error strings here are
// load-bearing for callers that log them; preserve them verbatim from the
// reference implementation.
func InspectGPTReasoningSignature(rawSignature string) (*GPTReasoningSignatureInfo, error) {
	sig := strings.TrimSpace(rawSignature)
	if sig == "" {
		return nil, fmt.Errorf("empty GPT reasoning signature")
	}
	if len(sig) > MaxGPTReasoningSignatureLen {
		return nil, fmt.Errorf("GPT reasoning signature exceeds maximum length (%d bytes)", MaxGPTReasoningSignatureLen)
	}
	if index, r, ok := firstInvalidGPTReasoningSignatureChar(sig); ok {
		return nil, fmt.Errorf("invalid GPT reasoning signature: contains non-base64url character U+%04X at byte %d", r, index)
	}
	if !strings.HasPrefix(sig, "gAAAA") {
		return nil, fmt.Errorf("invalid GPT reasoning signature: expected gAAAA prefix")
	}

	decoded, err := decodeGPTReasoningSignature(sig)
	if err != nil {
		return nil, err
	}
	if len(decoded) < 73 {
		return nil, fmt.Errorf("invalid GPT reasoning signature: decoded payload too short")
	}
	if decoded[0] != 0x80 {
		return nil, fmt.Errorf("invalid GPT reasoning signature: expected version 0x80, got 0x%02x", decoded[0])
	}

	ciphertextLen := len(decoded) - 1 - 8 - 16 - 32
	if ciphertextLen <= 0 || ciphertextLen%16 != 0 {
		return nil, fmt.Errorf("invalid GPT reasoning signature: ciphertext length %d is not a positive AES block multiple", ciphertextLen)
	}

	return &GPTReasoningSignatureInfo{
		DecodedLen:    len(decoded),
		CiphertextLen: ciphertextLen,
	}, nil
}

func decodeGPTReasoningSignature(sig string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(sig); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(sig); err == nil {
		return decoded, nil
	}
	return nil, fmt.Errorf("invalid GPT reasoning signature: base64url decode failed")
}

func firstInvalidGPTReasoningSignatureChar(sig string) (int, rune, bool) {
	for index, r := range sig {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '=':
		default:
			return index, r, true
		}
	}
	return 0, 0, false
}

// SanitizeOpenAIResponsesReasoningEncryptedContent drops invalid
// encrypted_content from reasoning items in an OpenAI Responses-style input
// array. Mirrors the rule applied by CLIProxyAPI's
// sanitizeOpenAIResponsesReasoningEncryptedContent helper before each upstream
// request. Items that pass the shape check are preserved; items whose
// encrypted_content is missing, non-string, leading/trailing whitespace, or
// fails InspectGPTReasoningSignature have the field stripped (rather than the
// entire item removed) so the upstream still sees the reasoning placeholder.
//
// The input is mutated in place and returned for convenience.
func SanitizeOpenAIResponsesReasoningEncryptedContent(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return input
	}
	for _, item := range input {
		if item == nil {
			continue
		}
		typeValue, _ := item["type"].(string)
		if strings.TrimSpace(typeValue) != "reasoning" {
			continue
		}
		encrypted, exists := item["encrypted_content"]
		if !exists {
			continue
		}
		reason := classifyEncryptedContent(encrypted)
		if reason == "" {
			continue
		}
		delete(item, "encrypted_content")
	}
	return input
}

func classifyEncryptedContent(value any) string {
	switch typed := value.(type) {
	case nil:
		return "encrypted_content is null"
	case string:
		if typed != strings.TrimSpace(typed) {
			return "encrypted_content has leading or trailing whitespace"
		}
		if _, err := InspectGPTReasoningSignature(typed); err != nil {
			return err.Error()
		}
		return ""
	default:
		return fmt.Sprintf("encrypted_content must be a string, got %T", typed)
	}
}

package service

import "testing"

// Mirrors CLIProxyAPI TestCodexExecutorExecuteNormalizesNullInstructions:
// instructions: null on the raw caller payload becomes "" on the upstream
// request rather than being replaced with the model default.
func TestCodexInstructionsNullNormalizedToEmpty(t *testing.T) {
	payload := map[string]any{
		"model":        "gpt-5.4",
		"instructions": nil,
		"input":        "hello",
	}
	codexNormalizeInstructionsField(payload)
	if !codexInstructionsWasNormalizedEmpty(payload) {
		t.Fatal("null instructions should be marked as normalized empty")
	}
}

// Whitespace strings stay as strings so the existing per-account default
// substitution still applies (preserving srapi behaviour around configured
// account-default instructions).
func TestCodexInstructionsWhitespacePreservedForDefaultSubstitution(t *testing.T) {
	payload := map[string]any{
		"model":        "gpt-5.4",
		"instructions": "   ",
		"input":        "hello",
	}
	codexNormalizeInstructionsField(payload)
	if codexInstructionsWasNormalizedEmpty(payload) {
		t.Fatal("whitespace instructions should NOT be marked normalized empty")
	}
}

// Missing instructions stay missing.
func TestCodexInstructionsMissingNotMarked(t *testing.T) {
	payload := map[string]any{"model": "gpt-5.4", "input": "hello"}
	codexNormalizeInstructionsField(payload)
	if _, exists := payload["instructions"]; exists {
		t.Fatal("missing instructions should not be introduced by normalization")
	}
}

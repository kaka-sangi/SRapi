package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestChatGPTWebPoWGenerateUsesSentinelFNVAnswerShape(t *testing.T) {
	if got := chatGPTWebPoWFNV1a32("seed"); got != 0xd9b133b8 {
		t.Fatalf("fnv1a32 = %08x, want d9b133b8", got)
	}
	config := []any{
		"0",
		"Sun Jun 21 2026 01:00:00 GMT+0800",
		"0",
		0,
		0.0,
		"Mozilla/5.0 Test",
		"/backend-api/sentinel/sdk.js",
		"c/test/_",
		"en-US",
		0,
		"en-US",
		0.0,
		"_reactListeningtest",
		"window",
		0.0,
		"sid",
		"",
		"Win32",
		0.0,
		0,
		0,
		0,
		1,
	}
	answer, ok := chatGPTWebPoWGenerate("seed", "ffffffff", config, 1)
	if !ok {
		t.Fatal("expected trivial difficulty to solve")
	}
	if !strings.HasSuffix(answer, "~S") {
		t.Fatalf("expected sentinel answer suffix, got %q", answer)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(answer, "~S"))
	if err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	var fp []any
	if err := json.Unmarshal(raw, &fp); err != nil {
		t.Fatalf("decode fingerprint json: %v", err)
	}
	if len(fp) != 23 {
		t.Fatalf("fingerprint length = %d, want 23", len(fp))
	}
	if fp[3] != float64(0) {
		t.Fatalf("nonce slot = %#v, want 0", fp[3])
	}
	if _, ok := fp[9].(float64); !ok {
		t.Fatalf("elapsed slot = %#v, want numeric", fp[9])
	}
}

func TestChatGPTWebPoWRunCheckMatchesSentinelAlgorithm(t *testing.T) {
	start := time.Now().Add(-123 * time.Millisecond)
	config := []any{
		"0",
		"",
		"0",
		99,
		0.0,
		"",
		"",
		"",
		"en-US",
		123,
		"en-US",
		0.0,
		"",
		"",
		0.0,
		"",
		"",
		"",
		0,
		0,
		0,
		0,
		0,
	}
	answer, err := chatGPTWebPoWRunCheck(start, "seed", "ffffffff", config, 7)
	if err != nil {
		t.Fatalf("run check: %v", err)
	}
	if answer == "" {
		t.Fatal("expected trivial difficulty to solve")
	}
	if !strings.HasSuffix(answer, "~S") {
		t.Fatalf("answer suffix = %q, want ~S", answer)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(answer, "~S"))
	if err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	var fp []any
	if err := json.Unmarshal(raw, &fp); err != nil {
		t.Fatalf("decode fingerprint json: %v", err)
	}
	if fp[3] != float64(7) {
		t.Fatalf("nonce slot = %#v, want 7", fp[3])
	}
	if fp[9] != float64(123) {
		t.Fatalf("elapsed slot = %#v, want 123", fp[9])
	}
	if encoded := strings.TrimSuffix(answer, "~S"); encoded != "WyIwIiwiIiwiMCIsNywwLCIiLCIiLCIiLCJlbi1VUyIsMTIzLCJlbi1VUyIsMCwiIiwiIiwwLCIiLCIiLCIiLDAsMCwwLDAsMF0=" {
		t.Fatalf("encoded fingerprint = %q", encoded)
	}
	if got := chatGPTWebPoWFNV1aHex("seed" + strings.TrimSuffix(answer, "~S")); got != "2716110f" {
		t.Fatalf("hash = %s, want 2716110f", got)
	}
}

func TestChatGPTWebPoWRunCheckDoesNotMutateInputFingerprint(t *testing.T) {
	config := []any{
		"0",
		"",
		"0",
		99,
		0.0,
		"",
		"",
		"",
		"en-US",
		321,
		"en-US",
		0.0,
		"",
		"",
		0.0,
		"",
		"",
		"",
		0,
		0,
		0,
		0,
		0,
	}
	before, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal fingerprint: %v", err)
	}
	if _, err := chatGPTWebPoWRunCheck(time.Now(), "seed", "ffffffff", config, 7); err != nil {
		t.Fatalf("run check: %v", err)
	}
	after, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal fingerprint after run: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("fingerprint mutated: before=%s after=%s", before, after)
	}
}

func TestChatGPTWebPoWGenerateReportsUnsolvedWithoutFallback(t *testing.T) {
	config := chatGPTWebPoWConfig(contractConversationRequestForPoWTest(), nil, "")

	answer, ok := chatGPTWebPoWGenerate("seed", "0", config, 0)
	if ok {
		t.Fatal("expected zero-attempt proof to be unsolved")
	}
	if answer != "" {
		t.Fatalf("unsolved proof returned fallback answer %q", answer)
	}

	answer, ok = chatGPTWebPoWGenerate("seed", "not-hex", config, 1)
	if ok {
		t.Fatal("expected invalid difficulty to be unsolved")
	}
	if answer != "" {
		t.Fatalf("invalid difficulty returned fallback answer %q", answer)
	}
}

func TestChatGPTWebPoWConfigUsesCompactPlaceholderFingerprint(t *testing.T) {
	config := chatGPTWebPoWConfig(contractConversationRequestForPoWTest(), nil, "")
	if len(config) != 23 {
		t.Fatalf("fingerprint length = %d, want 23", len(config))
	}
	if config[1] != "" {
		t.Fatalf("date slot = %#v, want empty placeholder", config[1])
	}
	if config[12] != "" {
		t.Fatalf("random object-key slot = %#v, want empty placeholder", config[12])
	}
	if config[13] != "" {
		t.Fatalf("random window-property slot = %#v, want empty placeholder", config[13])
	}
	if config[15] != "" || config[17] != "" {
		t.Fatalf("session/platform slots = %#v/%#v, want empty placeholders", config[15], config[17])
	}
	if config[22] != 0 {
		t.Fatalf("TextEncoder slot = %#v, want 0 placeholder", config[22])
	}
}

func contractConversationRequestForPoWTest() contract.ConversationRequest {
	return contract.ConversationRequest{
		RequestID: "pow-test",
		Account: accountcontract.ProviderAccount{
			Metadata: map[string]any{"user_agent": "Mozilla/5.0 Test"},
		},
	}
}

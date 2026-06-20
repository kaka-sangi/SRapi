package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
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
	answer, err := chatGPTWebPoWRunCheck(time.Now(), "seed", "ffffffff", config, 7)
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
	if _, ok := fp[9].(float64); !ok {
		t.Fatalf("elapsed slot = %#v, want numeric", fp[9])
	}
}

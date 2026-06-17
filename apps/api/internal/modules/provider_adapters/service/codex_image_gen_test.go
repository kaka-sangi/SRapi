// Verbatim port of CLIProxyAPI's
// internal/runtime/executor/codex_executor_imagegen_test.go test cases.
// Each TestEnsure... mirrors the source name and assertion shape; the
// only deviation is that srapi keeps the payload as map[string]any and
// the auth ProviderAccount is the srapi contract type.
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

func TestEnsureCodexImageGenerationTool_NoTools(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"input": "draw a cat",
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.4", accountcontract.ProviderAccount{})

	tools, ok := payload["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools []any, got %T", payload["tools"])
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["type"].(string) != "image_generation" {
		t.Fatalf("expected type=image_generation, got %v", tool["type"])
	}
	if tool["output_format"].(string) != "png" {
		t.Fatalf("expected output_format=png, got %v", tool["output_format"])
	}
}

func TestEnsureCodexImageGenerationTool_ExistingToolsWithoutImageGen(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{
				"type":       "function",
				"name":       "get_weather",
				"parameters": map[string]any{},
			},
		},
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.4", accountcontract.ProviderAccount{})

	tools, _ := payload["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	first := tools[0].(map[string]any)
	if first["type"].(string) != "function" {
		t.Fatalf("expected first tool type=function, got %v", first["type"])
	}
	second := tools[1].(map[string]any)
	if second["type"].(string) != "image_generation" {
		t.Fatalf("expected second tool type=image_generation, got %v", second["type"])
	}
}

func TestEnsureCodexImageGenerationTool_AlreadyPresent(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "image_generation", "output_format": "webp"},
			map[string]any{"type": "function", "name": "f1"},
		},
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.4", accountcontract.ProviderAccount{})

	tools, _ := payload["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools (no duplicate), got %d", len(tools))
	}
	first := tools[0].(map[string]any)
	if first["output_format"].(string) != "webp" {
		t.Fatalf("expected original output_format=webp preserved, got %v", first["output_format"])
	}
}

func TestEnsureCodexImageGenerationTool_EmptyToolsArray(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{},
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.4", accountcontract.ProviderAccount{})

	tools, _ := payload["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["type"].(string) != "image_generation" {
		t.Fatalf("expected type=image_generation, got %v", tool["type"])
	}
}

func TestEnsureCodexImageGenerationTool_WebSearchAndImageGen(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "web_search"},
		},
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.4", accountcontract.ProviderAccount{})

	tools, _ := payload["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	first := tools[0].(map[string]any)
	if first["type"].(string) != "web_search" {
		t.Fatalf("expected first tool type=web_search, got %v", first["type"])
	}
	second := tools[1].(map[string]any)
	if second["type"].(string) != "image_generation" {
		t.Fatalf("expected second tool type=image_generation, got %v", second["type"])
	}
}

func TestEnsureCodexImageGenerationTool_GPT53CodexSparkDoesNotInjectTool(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"input": "draw a cat",
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.3-codex-spark", accountcontract.ProviderAccount{})

	if _, ok := payload["tools"]; ok {
		t.Fatalf("expected no tools for gpt-5.3-codex-spark, got %v", payload["tools"])
	}
}

func TestEnsureCodexImageGenerationTool_FreeCodexAuthDoesNotInjectTool(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"input": "draw a cat",
	}
	freeAccount := accountcontract.ProviderAccount{
		Metadata: map[string]any{"plan_type": "free"},
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.4", freeAccount)

	if _, ok := payload["tools"]; ok {
		t.Fatalf("expected no tools for free codex auth, got %v", payload["tools"])
	}
}

// Additional srapi-only test: a map[string]any (instead of []any) tools
// slice should still receive the inject. CLIProxyAPI tests this
// implicitly via gjson; we test it explicitly because the Go map shape
// can vary between codex_responses_payload.go branches.
func TestEnsureCodexImageGenerationTool_MapStringAnyToolsSlice(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"tools": []map[string]any{
			{"type": "function", "name": "foo"},
		},
	}
	ensureCodexImageGenerationTool(payload, "gpt-5.4", accountcontract.ProviderAccount{})

	tools, ok := payload["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools to be promoted to []any, got %T", payload["tools"])
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	second := tools[1].(map[string]any)
	if second["type"].(string) != "image_generation" {
		t.Fatalf("expected injected image_generation, got %v", second["type"])
	}
}

// Slot acquire/release happy path: the limiter is shared with the
// chatgpt_web flow but the key namespace is disjoint, so an acquire
// here must not collide with chatgpt_web slot state.
func TestCodexImageGenSlotAcquire_HappyPath(t *testing.T) {
	account := accountcontract.ProviderAccount{ID: 42}
	release, err := codexImageGenSlotAcquire(context.Background(), account)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if release == nil {
		t.Fatalf("expected non-nil release")
	}
	if got := codexImageGenSlotLimiter().Inflight("codex-42"); got != 1 {
		t.Fatalf("Inflight = %d, want 1", got)
	}
	release()
	if got := codexImageGenSlotLimiter().Inflight("codex-42"); got != 0 {
		t.Fatalf("Inflight after release = %d, want 0", got)
	}
}

// Slot acquire respects ctx cancellation when the cap is reached.
func TestCodexImageGenSlotAcquire_CapacityBlocks(t *testing.T) {
	account := accountcontract.ProviderAccount{
		ID:       43,
		Metadata: map[string]any{"codex_image_account_concurrency": 1},
	}
	release1, err := codexImageGenSlotAcquire(context.Background(), account)
	if err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	defer release1()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = codexImageGenSlotAcquire(ctx, account)
	if !errors.Is(err, ErrChatGPTWebImageSlotCancelled) {
		t.Fatalf("expected cancelled, got %v", err)
	}
}

// Anonymous accounts get an immediate no-op release (no key to track).
func TestCodexImageGenSlotAcquire_AnonymousNoop(t *testing.T) {
	release, err := codexImageGenSlotAcquire(context.Background(), accountcontract.ProviderAccount{ID: 0})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if release == nil {
		t.Fatalf("expected non-nil release")
	}
	// Calling release on an empty key is a no-op; no panic.
	release()
}

func TestParseCodexImageGenPollState_InProgress(t *testing.T) {
	body := []byte(`{"response":{"output":[{"type":"image_generation_call","id":"ig_abc","status":"in_progress"}],"retry_after":3}}`)
	state := parseCodexImageGenPollState(body)
	if !state.Polling {
		t.Fatalf("expected polling=true")
	}
	if state.PollID != "ig_abc" {
		t.Fatalf("expected PollID=ig_abc, got %q", state.PollID)
	}
	if state.RetryAfter != 3 {
		t.Fatalf("expected RetryAfter=3, got %d", state.RetryAfter)
	}
}

func TestParseCodexImageGenPollState_Completed(t *testing.T) {
	body := []byte(`{"response":{"output":[{"type":"image_generation_call","id":"ig_x","status":"completed"}]}}`)
	state := parseCodexImageGenPollState(body)
	if state.Polling {
		t.Fatalf("expected polling=false for completed status")
	}
}

func TestParseCodexImageGenPollState_NoImageGen(t *testing.T) {
	body := []byte(`{"response":{"output":[{"type":"message","id":"msg_1"}]}}`)
	state := parseCodexImageGenPollState(body)
	if state.Polling {
		t.Fatalf("expected polling=false when no image_generation_call present")
	}
}

func TestParseCodexImageGenPollState_TopLevelOutput(t *testing.T) {
	// Some upstream variants put the items at the top level.
	body := []byte(`{"output":[{"type":"image_generation_call","id":"ig_top","status":"queued"}]}`)
	state := parseCodexImageGenPollState(body)
	if !state.Polling {
		t.Fatalf("expected polling=true for queued status")
	}
	if state.PollID != "ig_top" {
		t.Fatalf("expected PollID=ig_top, got %q", state.PollID)
	}
}

func TestParseCodexImageGenPollState_EmptyBody(t *testing.T) {
	state := parseCodexImageGenPollState(nil)
	if state.Polling {
		t.Fatalf("expected polling=false for empty body")
	}
}

func TestParseCodexImageGenPollState_Malformed(t *testing.T) {
	state := parseCodexImageGenPollState([]byte(`{not json`))
	if state.Polling {
		t.Fatalf("expected polling=false for malformed body")
	}
}

func TestCodexImageGenSlotCapacity_DefaultAndOverride(t *testing.T) {
	if got := codexImageGenSlotCapacity(accountcontract.ProviderAccount{}); got != DefaultCodexImageGenSlotCapacity {
		t.Fatalf("default capacity = %d, want %d", got, DefaultCodexImageGenSlotCapacity)
	}
	if got := codexImageGenSlotCapacity(accountcontract.ProviderAccount{Metadata: map[string]any{"codex_image_account_concurrency": 5}}); got != 5 {
		t.Fatalf("int override = %d, want 5", got)
	}
	if got := codexImageGenSlotCapacity(accountcontract.ProviderAccount{Metadata: map[string]any{"codex_image_account_concurrency": "7"}}); got != 7 {
		t.Fatalf("string override = %d, want 7", got)
	}
	if got := codexImageGenSlotCapacity(accountcontract.ProviderAccount{Metadata: map[string]any{"codex_image_slot_capacity": 4.0}}); got != 4 {
		t.Fatalf("float64 override = %d, want 4", got)
	}
	if got := codexImageGenSlotCapacity(accountcontract.ProviderAccount{Metadata: map[string]any{"codex_image_account_concurrency": -1}}); got != DefaultCodexImageGenSlotCapacity {
		t.Fatalf("negative override should fall back to default, got %d", got)
	}
}

func TestCodexImageGenSlotKey(t *testing.T) {
	if got := codexImageGenSlotKey(7); got != "codex-7" {
		t.Fatalf("key(7) = %q, want codex-7", got)
	}
	if got := codexImageGenSlotKey(0); got != "" {
		t.Fatalf("key(0) = %q, want empty", got)
	}
}

func TestCodexPayloadInputUsesImageGenerationCall(t *testing.T) {
	if codexPayloadInputUsesImageGenerationCall(nil) {
		t.Fatalf("nil payload should report false")
	}
	if codexPayloadInputUsesImageGenerationCall(map[string]any{}) {
		t.Fatalf("empty payload should report false")
	}
	// Plain message input: no signal.
	plain := map[string]any{
		"input": []any{
			map[string]any{"type": "message", "role": "user"},
		},
	}
	if codexPayloadInputUsesImageGenerationCall(plain) {
		t.Fatalf("plain message input should report false")
	}
	// image_generation_call item present: signal fires.
	signal := map[string]any{
		"input": []any{
			map[string]any{"type": "message", "role": "user"},
			map[string]any{"type": "image_generation_call", "id": "ig_1"},
		},
	}
	if !codexPayloadInputUsesImageGenerationCall(signal) {
		t.Fatalf("payload with image_generation_call input should report true")
	}
}

func TestCodexPayloadHasImageGenerationTool(t *testing.T) {
	if codexPayloadHasImageGenerationTool(nil) {
		t.Fatalf("nil payload should report false")
	}
	if codexPayloadHasImageGenerationTool(map[string]any{}) {
		t.Fatalf("empty payload should report false")
	}
	payload := map[string]any{
		"tools": []any{map[string]any{"type": "image_generation"}},
	}
	if !codexPayloadHasImageGenerationTool(payload) {
		t.Fatalf("payload with image_generation should report true")
	}
}

func TestIsCodexFreePlanAccount(t *testing.T) {
	if isCodexFreePlanAccount(accountcontract.ProviderAccount{}) {
		t.Fatalf("empty account should report false")
	}
	if !isCodexFreePlanAccount(accountcontract.ProviderAccount{Metadata: map[string]any{"plan_type": "free"}}) {
		t.Fatalf("free plan should report true")
	}
	if !isCodexFreePlanAccount(accountcontract.ProviderAccount{Metadata: map[string]any{"plan_type": "FREE"}}) {
		t.Fatalf("FREE (case-insensitive) should report true")
	}
	if isCodexFreePlanAccount(accountcontract.ProviderAccount{Metadata: map[string]any{"plan_type": "plus"}}) {
		t.Fatalf("plus plan should report false")
	}
}

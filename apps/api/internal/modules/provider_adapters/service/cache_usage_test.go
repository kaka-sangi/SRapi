package service

import "testing"

func TestAnthropicUsageSplitsCacheCreationAndRead(t *testing.T) {
	in := 10
	out := 20
	create := 100
	read := 50
	usage := anthropicUsage{
		InputTokens:              &in,
		OutputTokens:             &out,
		CacheCreationInputTokens: &create,
		CacheReadInputTokens:     &read,
	}.ToUsage("")

	if usage.InputTokens != 10 {
		t.Fatalf("input tokens = %d, want 10 (Anthropic input excludes cache)", usage.InputTokens)
	}
	if usage.OutputTokens != 20 {
		t.Fatalf("output tokens = %d, want 20", usage.OutputTokens)
	}
	if usage.CachedTokens != 50 {
		t.Fatalf("cached (read) tokens = %d, want 50", usage.CachedTokens)
	}
	if usage.CacheCreationTokens != 100 {
		t.Fatalf("cache-creation (write) tokens = %d, want 100", usage.CacheCreationTokens)
	}
	if usage.Estimated {
		t.Fatal("usage should not be estimated when real counts are present")
	}
}

func TestOpenAIUsageHasNoCacheCreation(t *testing.T) {
	prompt := 100
	completion := 40
	cached := 30
	usage := openAIUsage{
		PromptTokens:     &prompt,
		CompletionTokens: &completion,
		CachedTokens:     &cached,
	}.ToUsage("")

	if usage.CachedTokens != 30 {
		t.Fatalf("cached (read) tokens = %d, want 30", usage.CachedTokens)
	}
	if usage.CacheCreationTokens != 0 {
		t.Fatalf("cache-creation tokens = %d, want 0 (OpenAI has no cache writes)", usage.CacheCreationTokens)
	}
}

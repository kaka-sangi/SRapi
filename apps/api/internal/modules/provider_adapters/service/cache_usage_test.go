package service

import "testing"

func TestAnthropicUsageSplitsCacheCreationAndRead(t *testing.T) {
	in := 10
	out := 20
	create := 100
	read := 50
	create5m := 40
	create1h := 60
	usage := anthropicUsage{
		InputTokens:                &in,
		OutputTokens:               &out,
		CacheCreationInputTokens:   &create,
		CacheReadInputTokens:       &read,
		CacheCreation5mInputTokens: &create5m,
		CacheCreation1hInputTokens: &create1h,
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
	if usage.CacheCreation5mTokens != 40 || usage.CacheCreation1hTokens != 60 {
		t.Fatalf("cache-creation buckets = 5m:%d 1h:%d, want 40/60", usage.CacheCreation5mTokens, usage.CacheCreation1hTokens)
	}
	if usage.Estimated {
		t.Fatal("usage should not be estimated when real counts are present")
	}
}

func TestAnthropicUsageFallsBackCacheCreationToFiveMinutes(t *testing.T) {
	create := 100
	usage := anthropicUsage{
		CacheCreationInputTokens: &create,
	}.ToUsage("")
	if usage.CacheCreationTokens != 100 || usage.CacheCreation5mTokens != 100 || usage.CacheCreation1hTokens != 0 {
		t.Fatalf("expected missing cache creation detail to fall back to 5m, got %+v", usage)
	}
}

func TestAnthropicUsageFallsBackCacheReadToCachedTokens(t *testing.T) {
	in := 12
	out := 7
	cached := 4
	usage := anthropicUsage{
		InputTokens:  &in,
		OutputTokens: &out,
		CachedTokens: &cached,
	}.ToUsage("")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when cached_tokens is present")
	}
	if usage.InputTokens != 12 || usage.OutputTokens != 7 || usage.CachedTokens != 4 {
		t.Fatalf("unexpected Anthropic cached_tokens fallback usage: %+v", usage)
	}
	if usage.CacheCreationTokens != 0 {
		t.Fatalf("cache-creation tokens = %d, want 0", usage.CacheCreationTokens)
	}
}

func TestAnthropicUsagePrefersCacheReadInputTokensOverCachedTokens(t *testing.T) {
	cacheRead := 7
	cached := 99
	usage := anthropicUsage{
		CacheReadInputTokens: &cacheRead,
		CachedTokens:         &cached,
	}.ToUsage("")

	if usage.CachedTokens != 7 {
		t.Fatalf("cached tokens = %d, want cache_read_input_tokens value 7", usage.CachedTokens)
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

	if usage.InputTokens != 70 {
		t.Fatalf("input tokens = %d, want 70 (OpenAI prompt tokens include cached tokens)", usage.InputTokens)
	}
	if usage.CachedTokens != 30 {
		t.Fatalf("cached (read) tokens = %d, want 30", usage.CachedTokens)
	}
	if usage.CacheCreationTokens != 0 {
		t.Fatalf("cache-creation tokens = %d, want 0 (OpenAI has no cache writes)", usage.CacheCreationTokens)
	}
}

func TestOpenAIUsageClampsInputWhenCachedTokensExceedInputTokens(t *testing.T) {
	input := 8
	cached := 10
	usage := openAIUsage{
		InputTokens: &input,
		InputTokensDetails: &struct {
			CachedTokens *int `json:"cached_tokens"`
		}{
			CachedTokens: &cached,
		},
	}.ToUsage("cached response")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when OpenAI cache tokens are present")
	}
	if usage.InputTokens != 0 || usage.CachedTokens != 10 {
		t.Fatalf("unexpected clamped OpenAI cache usage: %+v", usage)
	}
}

func TestOpenAIUsagePreservesImageOutputTokens(t *testing.T) {
	imageOutput := 18
	usage := openAIUsage{
		OutputTokensDetails: &struct {
			ImageTokens *int `json:"image_tokens"`
		}{
			ImageTokens: &imageOutput,
		},
	}.ToUsage("draw a quiet control room")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when image output tokens are present")
	}
	if usage.OutputTokens != 18 || usage.ImageOutputTokens != 18 {
		t.Fatalf("unexpected image usage: %+v", usage)
	}
}

func TestGeminiUsageIncludesThoughtsTokensInOutput(t *testing.T) {
	prompt := 100
	candidates := 20
	thoughts := 50
	cached := 10
	usage := geminiUsageMetadata{
		PromptTokenCount:        &prompt,
		CandidatesTokenCount:    &candidates,
		ThoughtsTokenCount:      &thoughts,
		CachedContentTokenCount: &cached,
	}.ToUsage("gemini response")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when Gemini token counts are present")
	}
	if usage.InputTokens != 90 || usage.OutputTokens != 70 || usage.CachedTokens != 10 {
		t.Fatalf("unexpected gemini usage: %+v", usage)
	}
}

func TestGeminiUsageClampsInputWhenCachedTokensExceedPromptTokens(t *testing.T) {
	prompt := 8
	cached := 10
	usage := geminiUsageMetadata{
		PromptTokenCount:        &prompt,
		CachedContentTokenCount: &cached,
	}.ToUsage("gemini response")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when Gemini cache tokens are present")
	}
	if usage.InputTokens != 0 || usage.CachedTokens != 10 {
		t.Fatalf("unexpected clamped gemini cache usage: %+v", usage)
	}
}

func TestGeminiUsageTreatsThoughtsOnlyAsRealUsage(t *testing.T) {
	thoughts := 50
	usage := geminiUsageMetadata{
		ThoughtsTokenCount: &thoughts,
	}.ToUsage("gemini thinking")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when Gemini thoughts tokens are present")
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 50 || usage.CachedTokens != 0 {
		t.Fatalf("unexpected thoughts-only gemini usage: %+v", usage)
	}
}

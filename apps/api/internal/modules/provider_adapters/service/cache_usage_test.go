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

func TestAnthropicUsageTotalsNestedCacheCreation(t *testing.T) {
	create5m := 30
	create1h := 70
	usage := anthropicUsage{
		CacheCreation: &anthropicCacheCreationUsage{
			Ephemeral5mInputTokens: &create5m,
			Ephemeral1hInputTokens: &create1h,
		},
	}.ToUsage("")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when nested cache creation details are present")
	}
	if usage.CacheCreationTokens != 100 || usage.CacheCreation5mTokens != 30 || usage.CacheCreation1hTokens != 70 {
		t.Fatalf("unexpected nested cache creation usage: %+v", usage)
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

func TestAnthropicUsagePreservesExplicitZeroTokens(t *testing.T) {
	zero := 0
	usage := anthropicUsage{
		InputTokens:  &zero,
		OutputTokens: &zero,
	}.ToUsage("anthropic response")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when Anthropic explicitly reports zero tokens")
	}
	if !usage.Observed {
		t.Fatal("usage should be marked observed when Anthropic usage is present")
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CachedTokens != 0 || usage.CacheCreationTokens != 0 {
		t.Fatalf("unexpected explicit zero anthropic usage: %+v", usage)
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

func TestOpenAIUsagePreservesExplicitZeroTokens(t *testing.T) {
	zero := 0
	usage := openAIUsage{
		PromptTokens:     &zero,
		CompletionTokens: &zero,
		TotalTokens:      &zero,
	}.ToUsage("openai response")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when OpenAI explicitly reports zero tokens")
	}
	if !usage.Observed {
		t.Fatal("usage should be marked observed when OpenAI usage is present")
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CachedTokens != 0 || usage.CacheCreationTokens != 0 {
		t.Fatalf("unexpected explicit zero OpenAI usage: %+v", usage)
	}
}

func TestOpenAIUsagePreservesCacheCreationTokens(t *testing.T) {
	create := 30
	create5m := 10
	create1h := 20
	usage := openAIUsage{
		CacheCreationInputTokens: &create,
		CacheCreation5mTokens:    &create5m,
		CacheCreation1hTokens:    &create1h,
	}.ToUsage("cache write response")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when OpenAI-style cache creation tokens are present")
	}
	if usage.CacheCreationTokens != 30 || usage.CacheCreation5mTokens != 10 || usage.CacheCreation1hTokens != 20 {
		t.Fatalf("unexpected OpenAI-style cache creation usage: %+v", usage)
	}
}

func TestOpenAIUsageAcceptsCacheReadAliases(t *testing.T) {
	input := 20
	output := 3
	cacheRead := 6
	usage := openAIUsage{
		InputTokens:          &input,
		OutputTokens:         &output,
		CacheReadInputTokens: &cacheRead,
	}.ToUsage("")

	if usage.InputTokens != 14 || usage.OutputTokens != 3 || usage.CachedTokens != 6 {
		t.Fatalf("unexpected cache_read_input_tokens usage: %+v", usage)
	}

	cacheRead = 4
	usage = openAIUsage{
		InputTokens:     &input,
		CacheReadTokens: &cacheRead,
	}.ToUsage("")
	if usage.InputTokens != 16 || usage.CachedTokens != 4 {
		t.Fatalf("unexpected cache_read_tokens usage: %+v", usage)
	}
}

func TestOpenAIUsageAcceptsCacheCreationAliases(t *testing.T) {
	create := 12
	usage := openAIUsage{
		CacheCreationTokens: &create,
	}.ToUsage("")
	if usage.CacheCreationTokens != 12 || usage.CacheCreation5mTokens != 12 || usage.CacheCreation1hTokens != 0 {
		t.Fatalf("unexpected cache_creation_tokens usage: %+v", usage)
	}

	create = 9
	usage = openAIUsage{
		InputTokensDetails: &struct {
			CachedTokens        *int `json:"cached_tokens"`
			CacheCreationTokens *int `json:"cache_creation_tokens"`
		}{
			CacheCreationTokens: &create,
		},
	}.ToUsage("")
	if usage.CacheCreationTokens != 9 || usage.CacheCreation5mTokens != 9 {
		t.Fatalf("unexpected input_tokens_details cache creation usage: %+v", usage)
	}
}

func TestOpenAIUsageClampsInputWhenCachedTokensExceedInputTokens(t *testing.T) {
	input := 8
	cached := 10
	usage := openAIUsage{
		InputTokens: &input,
		InputTokensDetails: &struct {
			CachedTokens        *int `json:"cached_tokens"`
			CacheCreationTokens *int `json:"cache_creation_tokens"`
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
			ImageTokens     *int `json:"image_tokens"`
			ReasoningTokens *int `json:"reasoning_tokens"`
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

func TestOpenAIUsagePreservesReasoningOutputTokens(t *testing.T) {
	reasoningOutput := 21
	usage := openAIUsage{
		CompletionTokensDetails: &struct {
			ReasoningTokens *int `json:"reasoning_tokens"`
		}{
			ReasoningTokens: &reasoningOutput,
		},
	}.ToUsage("think silently")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when reasoning output tokens are present")
	}
	if usage.OutputTokens != 21 {
		t.Fatalf("output tokens = %d, want reasoning tokens 21: %+v", usage.OutputTokens, usage)
	}
}

func TestOpenAIUsagePreservesOutputDetailsReasoningTokens(t *testing.T) {
	reasoningOutput := 13
	usage := openAIUsage{
		OutputTokensDetails: &struct {
			ImageTokens     *int `json:"image_tokens"`
			ReasoningTokens *int `json:"reasoning_tokens"`
		}{
			ReasoningTokens: &reasoningOutput,
		},
	}.ToUsage("think silently")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when output details reasoning tokens are present")
	}
	if usage.OutputTokens != 13 {
		t.Fatalf("output tokens = %d, want reasoning tokens 13: %+v", usage.OutputTokens, usage)
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

func TestGeminiUsagePreservesExplicitZeroTokens(t *testing.T) {
	zero := 0
	usage := geminiUsageMetadata{
		PromptTokenCount:     &zero,
		CandidatesTokenCount: &zero,
		TotalTokenCount:      &zero,
	}.ToUsage("gemini response")

	if usage.Estimated {
		t.Fatal("usage should not be estimated when Gemini explicitly reports zero tokens")
	}
	if !usage.Observed {
		t.Fatal("usage should be marked observed when Gemini usageMetadata is present")
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CachedTokens != 0 {
		t.Fatalf("unexpected explicit zero gemini usage: %+v", usage)
	}
}

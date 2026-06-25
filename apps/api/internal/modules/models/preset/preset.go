package preset

import (
	"strings"

	cap "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
)

type ModelPreset struct {
	DisplayName     string
	Family          string
	ContextWindow   int
	MaxOutputTokens int
	QualityTier     string
	Capabilities    []cap.Descriptor
}

// Lookup returns the preset for the given canonical model name, if known.
// The frontend model-presets.ts maintains a parallel copy for form auto-fill.
func Lookup(canonicalName string) (ModelPreset, bool) {
	p, ok := registry[strings.TrimSpace(canonicalName)]
	return p, ok
}

func cap1(key string) cap.Descriptor {
	return cap.Descriptor{Key: key, Level: cap.DescriptorLevelRequired, Status: cap.DescriptorStatusStable, Version: "v1"}
}

// Capability sets mirroring the provider preset capabilities in
// providers/preset/registry.go — keep in sync.
var (
	// openAICapabilities() in registry.go
	capsOpenAI = []cap.Descriptor{
		cap1(cap.KeyChatCompletions), cap1(cap.KeyResponses), cap1(cap.KeyMessages),
		cap1(cap.KeyEmbeddings), cap1(cap.KeyAudioTranscriptions), cap1(cap.KeyAudioSpeech),
		cap1(cap.KeyModerations),
		cap1(cap.KeyStreaming), cap1(cap.KeyToolCalling), cap1(cap.KeyStructuredOutput),
		cap1(cap.KeyVisionInput), cap1(cap.KeyReasoningControl),
	}
	// anthropicCapabilities() in registry.go
	capsClaude = []cap.Descriptor{
		cap1(cap.KeyChatCompletions), cap1(cap.KeyResponses), cap1(cap.KeyMessages),
		cap1(cap.KeyAnthropicCountTokens), cap1(cap.KeyTokenCounting),
		cap1(cap.KeyStreaming), cap1(cap.KeyToolCalling), cap1(cap.KeyStructuredOutput),
		cap1(cap.KeyVisionInput),
	}
	// codexCLICapabilities() in registry.go
	capsCodex = []cap.Descriptor{
		cap1(cap.KeyChatCompletions), cap1(cap.KeyResponses),
		cap1(cap.KeyResponsesCompact), cap1(cap.KeyResponsesInputItems),
		cap1(cap.KeyMessages),
		cap1(cap.KeyImageGenerations), cap1(cap.KeyImageEdits), cap1(cap.KeyImageVariations),
		cap1(cap.KeyStreaming), cap1(cap.KeyToolCalling), cap1(cap.KeyStructuredOutput),
		cap1(cap.KeyVisionInput), cap1(cap.KeyReasoningControl),
	}
	// geminiCapabilities() in registry.go
	capsGemini = []cap.Descriptor{
		cap1(cap.KeyChatCompletions), cap1(cap.KeyMessages),
		cap1(cap.KeyGeminiGenerateContent), cap1(cap.KeyGeminiCountTokens),
		cap1(cap.KeyTokenCounting),
		cap1(cap.KeyStreaming), cap1(cap.KeyToolCalling), cap1(cap.KeyVisionInput),
	}
)

var registry = map[string]ModelPreset{
	// Anthropic — source: platform.claude.com/docs/en/about-claude/models/overview
	"claude-fable-5":    {DisplayName: "Claude Fable 5", Family: "claude", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "premium", Capabilities: capsClaude},
	"claude-opus-4-8":   {DisplayName: "Claude Opus 4.8", Family: "claude", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "premium", Capabilities: capsClaude},
	"claude-opus-4-7":   {DisplayName: "Claude Opus 4.7", Family: "claude", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "premium", Capabilities: capsClaude},
	"claude-opus-4-6":   {DisplayName: "Claude Opus 4.6", Family: "claude", ContextWindow: 1000000, MaxOutputTokens: 32768, QualityTier: "premium", Capabilities: capsClaude},
	"claude-sonnet-4-6": {DisplayName: "Claude Sonnet 4.6", Family: "claude", ContextWindow: 1000000, MaxOutputTokens: 64000, QualityTier: "standard", Capabilities: capsClaude},
	"claude-haiku-4-5":  {DisplayName: "Claude Haiku 4.5", Family: "claude", ContextWindow: 200000, MaxOutputTokens: 8192, QualityTier: "economy", Capabilities: capsClaude},

	// OpenAI — source: developers.openai.com/api/docs/models
	"gpt-5":        {DisplayName: "GPT-5", Family: "gpt", ContextWindow: 128000, MaxOutputTokens: 16384, QualityTier: "standard", Capabilities: capsOpenAI},
	"gpt-5-mini":   {DisplayName: "GPT-5 Mini", Family: "gpt", ContextWindow: 128000, MaxOutputTokens: 16384, QualityTier: "economy", Capabilities: capsOpenAI},
	"gpt-5.1":      {DisplayName: "GPT-5.1", Family: "gpt", ContextWindow: 128000, MaxOutputTokens: 32768, QualityTier: "standard", Capabilities: capsOpenAI},
	"gpt-5.5":      {DisplayName: "GPT-5.5", Family: "gpt", ContextWindow: 1050000, MaxOutputTokens: 128000, QualityTier: "premium", Capabilities: capsOpenAI},
	"gpt-5.4":      {DisplayName: "GPT-5.4", Family: "gpt", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "premium", Capabilities: capsOpenAI},
	"gpt-5.4-mini": {DisplayName: "GPT-5.4 Mini", Family: "gpt", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "economy", Capabilities: capsOpenAI},
	"gpt-5.4-nano": {DisplayName: "GPT-5.4 Nano", Family: "gpt", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "economy", Capabilities: capsOpenAI},
	"gpt-5.3":      {DisplayName: "GPT-5.3", Family: "gpt", ContextWindow: 400000, MaxOutputTokens: 128000, QualityTier: "standard", Capabilities: capsOpenAI},
	"gpt-5.3-mini": {DisplayName: "GPT-5.3 Mini", Family: "gpt", ContextWindow: 200000, MaxOutputTokens: 128000, QualityTier: "economy", Capabilities: capsOpenAI},
	"gpt-5.2":      {DisplayName: "GPT-5.2", Family: "gpt", ContextWindow: 400000, MaxOutputTokens: 128000, QualityTier: "standard", Capabilities: capsOpenAI},
	"gpt-4.1":      {DisplayName: "GPT-4.1", Family: "gpt", ContextWindow: 1000000, MaxOutputTokens: 32000, QualityTier: "standard", Capabilities: capsOpenAI},
	"gpt-4.1-mini": {DisplayName: "GPT-4.1 Mini", Family: "gpt", ContextWindow: 1000000, MaxOutputTokens: 32000, QualityTier: "economy", Capabilities: capsOpenAI},
	"gpt-4.1-nano": {DisplayName: "GPT-4.1 Nano", Family: "gpt", ContextWindow: 1000000, MaxOutputTokens: 32000, QualityTier: "economy", Capabilities: capsOpenAI},
	"o4-mini":      {DisplayName: "o4-mini", Family: "o-series", ContextWindow: 200000, MaxOutputTokens: 100000, QualityTier: "standard", Capabilities: capsOpenAI},
	"o3":           {DisplayName: "o3", Family: "o-series", ContextWindow: 200000, MaxOutputTokens: 100000, QualityTier: "premium", Capabilities: capsOpenAI},
	"o3-pro":       {DisplayName: "o3-pro", Family: "o-series", ContextWindow: 200000, MaxOutputTokens: 100000, QualityTier: "premium", Capabilities: capsOpenAI},

	// Image
	"gpt-image-2": {DisplayName: "GPT Image 2", Family: "gpt-image", QualityTier: "standard", Capabilities: []cap.Descriptor{
		cap1(cap.KeyImageGenerations), cap1(cap.KeyImageEdits), cap1(cap.KeyImageVariations),
	}},

	// Codex CLI — source: developers.openai.com/codex/models
	"codex-mini-latest":   {DisplayName: "Codex Mini", Family: "codex", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "standard", Capabilities: capsCodex},
	"codex-auto-review":   {DisplayName: "Codex Auto Review", Family: "codex", ContextWindow: 1000000, MaxOutputTokens: 128000, QualityTier: "standard", Capabilities: capsCodex},
	"gpt-5.3-codex":       {DisplayName: "GPT-5.3 Codex", Family: "codex", ContextWindow: 400000, MaxOutputTokens: 128000, QualityTier: "standard", Capabilities: capsCodex},
	"gpt-5.3-codex-spark": {DisplayName: "GPT-5.3 Codex Spark", Family: "codex", ContextWindow: 128000, MaxOutputTokens: 128000, QualityTier: "economy", Capabilities: capsCodex},

	// Gemini — source: ai.google.dev/gemini-api/docs/models
	"gemini-2.5-pro":   {DisplayName: "Gemini 2.5 Pro", Family: "gemini", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "premium", Capabilities: capsGemini},
	"gemini-2.5-flash": {DisplayName: "Gemini 2.5 Flash", Family: "gemini", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "standard", Capabilities: capsGemini},
	"gemini-2.0-flash": {DisplayName: "Gemini 2.0 Flash", Family: "gemini", ContextWindow: 1048576, MaxOutputTokens: 8192, QualityTier: "economy", Capabilities: capsGemini},

	// DeepSeek
	"deepseek-r1":      {DisplayName: "DeepSeek R1", Family: "deepseek", ContextWindow: 65536, MaxOutputTokens: 8192, QualityTier: "standard"},
	"deepseek-v3-0324": {DisplayName: "DeepSeek V3", Family: "deepseek", ContextWindow: 65536, MaxOutputTokens: 8192, QualityTier: "standard"},
	"deepseek-chat":    {DisplayName: "DeepSeek Chat", Family: "deepseek", ContextWindow: 65536, MaxOutputTokens: 8192, QualityTier: "economy"},

	// Grok
	"grok-3":      {DisplayName: "Grok 3", Family: "grok", ContextWindow: 131072, MaxOutputTokens: 16384, QualityTier: "premium"},
	"grok-3-mini": {DisplayName: "Grok 3 Mini", Family: "grok", ContextWindow: 131072, MaxOutputTokens: 16384, QualityTier: "standard"},

	// Mistral
	"mistral-large-latest": {DisplayName: "Mistral Large", Family: "mistral", ContextWindow: 131072, MaxOutputTokens: 8192, QualityTier: "premium"},
	"mistral-small-latest": {DisplayName: "Mistral Small", Family: "mistral", ContextWindow: 131072, MaxOutputTokens: 8192, QualityTier: "economy"},
	"codestral-latest":     {DisplayName: "Codestral", Family: "mistral", ContextWindow: 262144, MaxOutputTokens: 8192, QualityTier: "standard"},
}

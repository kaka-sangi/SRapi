package preset

import "strings"

type ModelPreset struct {
	DisplayName     string
	Family          string
	ContextWindow   int
	MaxOutputTokens int
	QualityTier     string
}

// Lookup returns the preset for the given canonical model name, if known.
// The frontend model-presets.ts maintains a parallel copy for form auto-fill.
func Lookup(canonicalName string) (ModelPreset, bool) {
	p, ok := registry[strings.TrimSpace(canonicalName)]
	return p, ok
}

var registry = map[string]ModelPreset{
	// Anthropic
	"claude-fable-5":    {DisplayName: "Claude Fable 5", Family: "claude", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "premium"},
	"claude-opus-4-8":   {DisplayName: "Claude Opus 4.8", Family: "claude", ContextWindow: 1048576, MaxOutputTokens: 32768, QualityTier: "premium"},
	"claude-opus-4-7":   {DisplayName: "Claude Opus 4.7", Family: "claude", ContextWindow: 1048576, MaxOutputTokens: 32768, QualityTier: "premium"},
	"claude-opus-4-6":   {DisplayName: "Claude Opus 4.6", Family: "claude", ContextWindow: 1048576, MaxOutputTokens: 32768, QualityTier: "premium"},
	"claude-sonnet-4-6": {DisplayName: "Claude Sonnet 4.6", Family: "claude", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "standard"},
	"claude-haiku-4-5":  {DisplayName: "Claude Haiku 4.5", Family: "claude", ContextWindow: 1048576, MaxOutputTokens: 8192, QualityTier: "economy"},

	// OpenAI
	"gpt-5.5":      {DisplayName: "GPT-5.5", Family: "gpt", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "premium"},
	"gpt-5.4":      {DisplayName: "GPT-5.4", Family: "gpt", ContextWindow: 1048576, MaxOutputTokens: 32768, QualityTier: "premium"},
	"gpt-5.4-mini": {DisplayName: "GPT-5.4 Mini", Family: "gpt", ContextWindow: 1048576, MaxOutputTokens: 16384, QualityTier: "economy"},
	"gpt-5.4-nano": {DisplayName: "GPT-5.4 Nano", Family: "gpt", ContextWindow: 1048576, MaxOutputTokens: 8192, QualityTier: "economy"},
	"gpt-5.2":      {DisplayName: "GPT-5.2", Family: "gpt", ContextWindow: 1048576, MaxOutputTokens: 32768, QualityTier: "standard"},
	"gpt-4.1":      {DisplayName: "GPT-4.1", Family: "gpt", ContextWindow: 1047576, MaxOutputTokens: 32768, QualityTier: "standard"},
	"gpt-4.1-mini": {DisplayName: "GPT-4.1 Mini", Family: "gpt", ContextWindow: 1047576, MaxOutputTokens: 16384, QualityTier: "economy"},
	"gpt-4.1-nano": {DisplayName: "GPT-4.1 Nano", Family: "gpt", ContextWindow: 1047576, MaxOutputTokens: 16384, QualityTier: "economy"},
	"o4-mini":      {DisplayName: "o4-mini", Family: "o-series", ContextWindow: 200000, MaxOutputTokens: 100000, QualityTier: "standard"},
	"o3":           {DisplayName: "o3", Family: "o-series", ContextWindow: 200000, MaxOutputTokens: 100000, QualityTier: "premium"},
	"o3-pro":       {DisplayName: "o3-pro", Family: "o-series", ContextWindow: 200000, MaxOutputTokens: 100000, QualityTier: "premium"},

	// Codex CLI
	"codex-mini-latest":   {DisplayName: "Codex Mini", Family: "codex", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "standard"},
	"codex-auto-review":   {DisplayName: "Codex Auto Review", Family: "codex", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "standard"},
	"gpt-5.3-codex":       {DisplayName: "GPT-5.3 Codex", Family: "codex", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "standard"},
	"gpt-5.3-codex-spark": {DisplayName: "GPT-5.3 Codex Spark", Family: "codex", ContextWindow: 1048576, MaxOutputTokens: 32768, QualityTier: "economy"},

	// Gemini
	"gemini-2.5-pro":   {DisplayName: "Gemini 2.5 Pro", Family: "gemini", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "premium"},
	"gemini-2.5-flash": {DisplayName: "Gemini 2.5 Flash", Family: "gemini", ContextWindow: 1048576, MaxOutputTokens: 65536, QualityTier: "standard"},
	"gemini-2.0-flash": {DisplayName: "Gemini 2.0 Flash", Family: "gemini", ContextWindow: 1048576, MaxOutputTokens: 8192, QualityTier: "economy"},

	// DeepSeek
	"deepseek-r1":       {DisplayName: "DeepSeek R1", Family: "deepseek", ContextWindow: 65536, MaxOutputTokens: 8192, QualityTier: "standard"},
	"deepseek-v3-0324":  {DisplayName: "DeepSeek V3", Family: "deepseek", ContextWindow: 65536, MaxOutputTokens: 8192, QualityTier: "standard"},
	"deepseek-chat":     {DisplayName: "DeepSeek Chat", Family: "deepseek", ContextWindow: 65536, MaxOutputTokens: 8192, QualityTier: "economy"},

	// Grok
	"grok-3":      {DisplayName: "Grok 3", Family: "grok", ContextWindow: 131072, MaxOutputTokens: 16384, QualityTier: "premium"},
	"grok-3-mini": {DisplayName: "Grok 3 Mini", Family: "grok", ContextWindow: 131072, MaxOutputTokens: 16384, QualityTier: "standard"},

	// Mistral
	"mistral-large-latest": {DisplayName: "Mistral Large", Family: "mistral", ContextWindow: 131072, MaxOutputTokens: 8192, QualityTier: "premium"},
	"mistral-small-latest": {DisplayName: "Mistral Small", Family: "mistral", ContextWindow: 131072, MaxOutputTokens: 8192, QualityTier: "economy"},
	"codestral-latest":     {DisplayName: "Codestral", Family: "mistral", ContextWindow: 262144, MaxOutputTokens: 8192, QualityTier: "standard"},
}

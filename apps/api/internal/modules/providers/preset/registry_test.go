package preset

import (
	"reflect"
	"testing"
)

func TestDefaultRegistrySeedsCompatiblePresets(t *testing.T) {
	registry := Default()

	keys := make([]string, 0, len(registry.List()))
	for _, preset := range registry.List() {
		keys = append(keys, preset.ProviderKey)
	}
	wantKeys := []string{
		"anthropic",
		"anthropic-compatible",
		"antigravity",
		"anyrouter",
		"cerebras",
		"deepseek",
		"deepseek-anthropic",
		"grok",
		"groq",
		"kimi",
		"moonshot",
		"moonshot-anthropic",
		"openai",
		"openai-compatible",
		"openrouter",
		"rerank-compatible",
		"zai",
		"zai-anthropic",
		"zhipu",
		"zhipu-anthropic",
	}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("unexpected preset keys: want %v got %v", wantKeys, keys)
	}

	openaiPreset, ok := registry.Lookup("openai-compatible")
	if !ok {
		t.Fatalf("missing openai-compatible preset")
	}
	if openaiPreset.DefaultBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected openai-compatible base url: %s", openaiPreset.DefaultBaseURL)
	}
	if !openaiPreset.MatchesPath("/api/provider/openai-compatible/v1/chat/completions") {
		t.Fatalf("expected openai-compatible route alias to match path")
	}
	if !containsAccountType(openaiPreset.AccountTypeAllowlist, AccountTypeCustomReverseProxy) {
		t.Fatalf("expected openai-compatible allowlist to include custom_reverse_proxy")
	}
	if !openaiPreset.Capabilities["images"] || !openaiPreset.Capabilities["audio_speech"] {
		t.Fatalf("expected openai-compatible preset to advertise images and audio_speech")
	}
	if openaiPreset.Capabilities["realtime_websocket"] {
		t.Fatalf("expected realtime_websocket to require explicit provider/account capability opt-in")
	}

	anthropicPreset, ok := registry.Lookup("anthropic-compatible")
	if !ok {
		t.Fatalf("missing anthropic-compatible preset")
	}
	if anthropicPreset.DefaultBaseURL != "https://api.anthropic.com/v1" {
		t.Fatalf("unexpected anthropic-compatible base url: %s", anthropicPreset.DefaultBaseURL)
	}
	if !anthropicPreset.MatchesPath("/api/provider/anthropic-compatible/v1/messages") {
		t.Fatalf("expected anthropic-compatible route alias to match path")
	}
	if !containsAuthMode(anthropicPreset.AuthModes, AuthModeCustomHeader) {
		t.Fatalf("expected anthropic-compatible auth modes to include custom_header")
	}

	antigravityPreset, ok := registry.Lookup("antigravity")
	if !ok {
		t.Fatalf("missing antigravity preset")
	}
	if antigravityPreset.PlatformFamily != PlatformFamilyReverseProxyAntigravity {
		t.Fatalf("expected antigravity reverse proxy platform family, got %s", antigravityPreset.PlatformFamily)
	}
	if antigravityPreset.DefaultBaseURL != "" {
		t.Fatalf("expected antigravity preset to require account base_url, got %q", antigravityPreset.DefaultBaseURL)
	}
	if !antigravityPreset.MatchesPath("/api/provider/antigravity/v1/chat/completions") || !antigravityPreset.MatchesPath("/antigravity/v1/messages") {
		t.Fatalf("expected antigravity text route aliases to match paths")
	}
	if !reflect.DeepEqual(antigravityPreset.GeminiRouteAliases, []string{"/antigravity/v1beta", "/api/provider/antigravity/v1beta"}) {
		t.Fatalf("unexpected antigravity Gemini aliases: %v", antigravityPreset.GeminiRouteAliases)
	}
	if !containsAccountType(antigravityPreset.AccountTypeAllowlist, AccountTypeDesktopClientToken) || !containsAccountType(antigravityPreset.AccountTypeAllowlist, AccountTypeIdePluginToken) {
		t.Fatalf("expected antigravity allowlist to include desktop and IDE token accounts")
	}
	if !antigravityPreset.Capabilities["chat_completions"] || !antigravityPreset.Capabilities["messages"] || antigravityPreset.Capabilities["embeddings"] {
		t.Fatalf("unexpected antigravity capabilities: %+v", antigravityPreset.Capabilities)
	}

	deepseekPreset, ok := registry.Lookup("deepseek")
	if !ok {
		t.Fatalf("missing deepseek preset")
	}
	if deepseekPreset.PlatformFamily != PlatformFamilyOpenAICompatible {
		t.Fatalf("expected deepseek to be OpenAI-compatible, got %s", deepseekPreset.PlatformFamily)
	}
	if deepseekPreset.DefaultBaseURL != "https://api.deepseek.com" {
		t.Fatalf("unexpected deepseek base url: %s", deepseekPreset.DefaultBaseURL)
	}
	if !deepseekPreset.MatchesPath("/api/provider/deepseek/v1/chat/completions") {
		t.Fatalf("expected deepseek route alias to match path")
	}

	claudeAliasPreset, ok := registry.Lookup("anthropic-compatible")
	if !ok {
		t.Fatalf("missing anthropic-compatible preset")
	}
	if !claudeAliasPreset.MatchesPath("/api/provider/claude-compatible/v1/messages") {
		t.Fatalf("expected claude-compatible route alias to map to anthropic-compatible preset")
	}

	groqPreset, ok := registry.Lookup("groq")
	if !ok {
		t.Fatalf("missing groq preset")
	}
	if groqPreset.DefaultBaseURL != "https://api.groq.com/openai/v1" {
		t.Fatalf("unexpected groq base url: %s", groqPreset.DefaultBaseURL)
	}
	if !containsAccountType(groqPreset.AccountTypeAllowlist, AccountTypeAPIKey) || !containsAuthMode(groqPreset.AuthModes, AuthModeBearer) {
		t.Fatalf("expected groq preset to include bearer api_key support")
	}

	rerankPreset, ok := registry.Lookup("rerank-compatible")
	if !ok {
		t.Fatalf("missing rerank-compatible preset")
	}
	if rerankPreset.PlatformFamily != PlatformFamilyRerankCompatible || !rerankPreset.Capabilities["rerank"] {
		t.Fatalf("expected rerank-compatible preset capabilities, got %+v", rerankPreset)
	}
	if !rerankPreset.MatchesPath("/api/provider/rerank-compatible/v1/rerank") {
		t.Fatalf("expected rerank-compatible route alias to match path")
	}
}

func containsAccountType(values []AccountType, target AccountType) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsAuthMode(values []AuthMode, target AuthMode) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

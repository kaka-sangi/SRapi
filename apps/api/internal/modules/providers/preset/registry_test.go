package preset

import (
	"reflect"
	"strings"
	"testing"

	accountscontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
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
		"bedrock",
		"cerebras",
		"chatgpt-web",
		"codex-cli",
		"deepseek",
		"deepseek-anthropic",
		"gemini",
		"grok",
		"groq",
		"kimi",
		"mistral",
		"moonshot",
		"moonshot-anthropic",
		"openai",
		"openai-compatible",
		"openrouter",
		"qwen",
		"rerank-compatible",
		"together",
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
	if !containsRuntimeClass(openaiPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassCustomReverseProxy) {
		t.Fatalf("expected openai-compatible allowlist to include custom_reverse_proxy")
	}
	if containsRuntimeClass(openaiPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthRefresh) {
		t.Fatalf("expected third-party openai-compatible allowlist to exclude oauth_refresh")
	}
	if !openaiPreset.Capabilities["images"] || !openaiPreset.Capabilities["audio_speech"] {
		t.Fatalf("expected openai-compatible preset to advertise images and audio_speech")
	}
	if !openaiPreset.Capabilities["responses_compact"] {
		t.Fatalf("expected openai-compatible preset to advertise responses_compact")
	}
	if openaiPreset.Capabilities["realtime_websocket"] {
		t.Fatalf("expected realtime_websocket to require explicit provider/account capability opt-in")
	}

	openAIProviderPreset, ok := registry.Lookup("openai")
	if !ok {
		t.Fatalf("missing openai preset")
	}
	if openAIProviderPreset.MatchesPath("/openai/v1/chat/completions") {
		t.Fatalf("expected root OpenAI alias to be unregistered")
	}
	if !openAIProviderPreset.MatchesPath("/api/provider/openai/v1/chat/completions") {
		t.Fatalf("expected OpenAI provider route alias to match path")
	}
	if containsRuntimeClass(openAIProviderPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthRefresh) ||
		containsRuntimeClass(openAIProviderPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthDeviceCode) {
		t.Fatalf("expected OpenAI preset to exclude unsupported OAuth runtimes")
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

	anthropicProviderPreset, ok := registry.Lookup("anthropic")
	if !ok {
		t.Fatalf("missing anthropic preset")
	}
	if anthropicProviderPreset.MatchesPath("/anthropic/v1/messages") {
		t.Fatalf("expected root Anthropic alias to be unregistered")
	}
	if !anthropicProviderPreset.MatchesPath("/api/provider/anthropic/v1/messages") {
		t.Fatalf("expected Anthropic provider route alias to match path")
	}
	if containsRuntimeClass(anthropicProviderPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassCustomReverseProxy) {
		t.Fatalf("unexpected Anthropic runtime class allowlist: %+v", anthropicProviderPreset.RuntimeClassAllowlist)
	}
	if anthropicProviderPreset.AccountTemplate == nil || anthropicProviderPreset.AccountTemplate.UpstreamClient != "claude_code_cli" {
		t.Fatalf("expected Anthropic template upstream_client=claude_code_cli, got %+v", anthropicProviderPreset.AccountTemplate)
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
	if !antigravityPreset.MatchesPath("/api/provider/antigravity/v1/chat/completions") {
		t.Fatalf("expected antigravity text route aliases to match paths")
	}
	if antigravityPreset.MatchesPath("/antigravity/v1/messages") {
		t.Fatalf("expected root Antigravity alias to be unregistered")
	}
	if !reflect.DeepEqual(antigravityPreset.GeminiRouteAliases, []string{"/api/provider/antigravity/v1beta"}) {
		t.Fatalf("unexpected antigravity Gemini aliases: %v", antigravityPreset.GeminiRouteAliases)
	}
	if !containsRuntimeClass(antigravityPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthRefresh) {
		t.Fatalf("expected antigravity allowlist to include oauth_refresh")
	}
	if antigravityPreset.AccountTemplate == nil || antigravityPreset.AccountTemplate.UpstreamClient != "antigravity_desktop" {
		t.Fatalf("expected antigravity template upstream_client=antigravity_desktop, got %+v", antigravityPreset.AccountTemplate)
	}
	if antigravityPreset.AccountTemplate.MetadataHints["tls_profile"] == "" {
		t.Fatalf("expected antigravity template to expose tls_profile metadata hint")
	}
	if !antigravityPreset.Capabilities["chat_completions"] || !antigravityPreset.Capabilities["messages"] || antigravityPreset.Capabilities["embeddings"] {
		t.Fatalf("unexpected antigravity capabilities: %+v", antigravityPreset.Capabilities)
	}

	bedrockPreset, ok := registry.Lookup("bedrock")
	if !ok {
		t.Fatalf("missing bedrock preset")
	}
	if bedrockPreset.PlatformFamily != PlatformFamilyBedrockAnthropic {
		t.Fatalf("expected bedrock platform family, got %s", bedrockPreset.PlatformFamily)
	}
	if bedrockPreset.DefaultBaseURL != "https://bedrock-runtime.us-east-1.amazonaws.com" {
		t.Fatalf("unexpected bedrock base url: %s", bedrockPreset.DefaultBaseURL)
	}
	if !bedrockPreset.MatchesPath("/api/provider/bedrock/v1/messages") || !containsAuthMode(bedrockPreset.AuthModes, AuthModeCustomHeader) {
		t.Fatalf("unexpected bedrock routing/auth preset: %+v", bedrockPreset)
	}
	if !containsRuntimeClass(bedrockPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassAPIKey) || !bedrockPreset.Capabilities["messages"] || !bedrockPreset.Capabilities["streaming"] {
		t.Fatalf("unexpected bedrock capabilities: %+v", bedrockPreset)
	}
	if containsRuntimeClass(bedrockPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassCustomReverseProxy) {
		t.Fatalf("unexpected bedrock runtime class allowlist: %+v", bedrockPreset.RuntimeClassAllowlist)
	}

	chatGPTWebPreset, ok := registry.Lookup("chatgpt-web")
	if !ok {
		t.Fatalf("missing chatgpt-web preset")
	}
	if chatGPTWebPreset.PlatformFamily != PlatformFamilyOpenAICompatible || chatGPTWebPreset.DefaultBaseURL != "https://chatgpt.com" {
		t.Fatalf("unexpected chatgpt-web preset: %+v", chatGPTWebPreset)
	}
	if !chatGPTWebPreset.MatchesPath("/api/provider/chatgpt-web/v1/chat/completions") {
		t.Fatalf("expected chatgpt-web route alias to match path")
	}
	if chatGPTWebPreset.MatchesPath("/chatgpt-web/v1/chat/completions") {
		t.Fatalf("expected root ChatGPT Web alias to be unregistered")
	}
	if !containsRuntimeClass(chatGPTWebPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassWebSessionCookie) ||
		containsRuntimeClass(chatGPTWebPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthRefresh) ||
		chatGPTWebPreset.AccountTemplate == nil ||
		chatGPTWebPreset.AccountTemplate.UpstreamClient != "chatgpt_web" {
		t.Fatalf("unexpected chatgpt-web auth/template preset: %+v", chatGPTWebPreset)
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
	if !containsRuntimeClass(groqPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassAPIKey) || !containsAuthMode(groqPreset.AuthModes, AuthModeBearer) {
		t.Fatalf("expected groq preset to include bearer api_key support")
	}

	togetherPreset, ok := registry.Lookup("together")
	if !ok {
		t.Fatalf("missing together preset")
	}
	if togetherPreset.DefaultBaseURL != "https://api.together.ai/v1" {
		t.Fatalf("unexpected together base url: %s", togetherPreset.DefaultBaseURL)
	}

	qwenPreset, ok := registry.Lookup("qwen")
	if !ok {
		t.Fatalf("missing qwen preset")
	}
	if !qwenPreset.MatchesPath("/api/provider/tongyi/v1/chat/completions") {
		t.Fatalf("expected tongyi route alias to map to qwen preset")
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

	codexPreset, ok := registry.Lookup("codex-cli")
	if !ok {
		t.Fatalf("missing codex-cli preset")
	}
	if codexPreset.PlatformFamily != PlatformFamilyCodexCLI {
		t.Fatalf("expected codex_cli platform family, got %s", codexPreset.PlatformFamily)
	}
	if codexPreset.DefaultBaseURL != "https://chatgpt.com/backend-api/codex" {
		t.Fatalf("unexpected codex-cli base url: %s", codexPreset.DefaultBaseURL)
	}
	if containsRuntimeClass(codexPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassAPIKey) {
		t.Fatalf("expected codex-cli to exclude api_key runtime class")
	}
	if !containsRuntimeClass(codexPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthRefresh) {
		t.Fatalf("expected codex-cli to include oauth_refresh runtime class")
	}
	if !codexPreset.Capabilities["responses"] || !codexPreset.Capabilities["responses_compact"] || !codexPreset.Capabilities["streaming"] {
		t.Fatalf("unexpected codex-cli capabilities: %+v", codexPreset.Capabilities)
	}
	if codexPreset.MatchesPath("/backend-api/codex/responses") {
		t.Fatalf("expected Codex backend root alias to be unregistered")
	}
	if !codexPreset.MatchesPath("/api/provider/codex-cli/v1/responses") {
		t.Fatalf("expected Codex provider route alias to match path")
	}
	if codexPreset.AccountTemplate == nil {
		t.Fatalf("expected codex-cli to have an account template")
	}
	if codexPreset.AccountTemplate.UpstreamClient != "codex_cli" {
		t.Fatalf("expected codex-cli template upstream_client=codex_cli, got %s", codexPreset.AccountTemplate.UpstreamClient)
	}
	if len(codexPreset.AccountTemplate.ModelCatalog) == 0 {
		t.Fatalf("expected codex-cli template to have a model catalog")
	}
	if codexPreset.OAuthConfig == nil || codexPreset.OAuthConfig.ClientID == "" || codexPreset.OAuthConfig.TokenURL == "" {
		t.Fatalf("expected codex-cli to include OAuth defaults")
	}

	geminiPreset, ok := registry.Lookup("gemini")
	if !ok {
		t.Fatalf("missing gemini preset")
	}
	if geminiPreset.PlatformFamily != PlatformFamilyGeminiCompatible || geminiPreset.DefaultBaseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("unexpected gemini preset: %+v", geminiPreset)
	}
	if geminiPreset.MatchesPath("/gemini/v1beta/models/gemini-pro:generateContent") {
		t.Fatalf("expected root Gemini alias to be unregistered")
	}
	if !geminiPreset.MatchesPath("/api/provider/gemini/v1beta/models/gemini-pro:generateContent") {
		t.Fatalf("expected Gemini provider route alias to match path")
	}
	if containsRuntimeClass(geminiPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthRefresh) ||
		containsRuntimeClass(geminiPreset.RuntimeClassAllowlist, accountscontract.RuntimeClassOauthDeviceCode) ||
		geminiPreset.OAuthConfig != nil {
		t.Fatalf("expected gemini preset to exclude unsupported OAuth runtimes")
	}
}

func TestPresetRuntimeAllowlistsOnlyExposeSignableAuthMethods(t *testing.T) {
	signable := map[accountscontract.RuntimeClass]string{
		accountscontract.RuntimeClassAPIKey:             "native adapter signs API keys",
		accountscontract.RuntimeClassOauthRefresh:       "reverse proxy injects bearer tokens and supports refresh for wired upstream clients",
		accountscontract.RuntimeClassOauthDeviceCode:    "device-code provisioning mints the same refreshable OAuth credential",
		accountscontract.RuntimeClassWebSessionCookie:   "reverse proxy injects session cookies",
		accountscontract.RuntimeClassCliClientToken:     "reverse proxy injects CLI bearer tokens",
		accountscontract.RuntimeClassCustomReverseProxy: "reverse proxy injects custom bearer tokens",
	}

	for _, preset := range Default().List() {
		for _, runtimeClass := range preset.RuntimeClassAllowlist {
			if _, ok := signable[runtimeClass]; !ok {
				t.Fatalf("preset %s exposes unsupported runtime_class %s", preset.ProviderKey, runtimeClass)
			}
			if runtimeClass == accountscontract.RuntimeClassOauthRefresh || runtimeClass == accountscontract.RuntimeClassOauthDeviceCode {
				if !oauthConfigHasProvisioningDefaults(preset.OAuthConfig, runtimeClass) {
					t.Fatalf("preset %s exposes %s without complete provisioning OAuth defaults", preset.ProviderKey, runtimeClass)
				}
				if !presetUsesRefreshableUpstreamClient(preset) {
					t.Fatalf("preset %s exposes %s without a built-in refreshable upstream_client", preset.ProviderKey, runtimeClass)
				}
			}
		}
	}
}

func oauthConfigHasProvisioningDefaults(config *OAuthConfig, runtimeClass accountscontract.RuntimeClass) bool {
	if config == nil {
		return false
	}
	if strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.TokenURL) == "" || len(config.Scopes) == 0 {
		return false
	}
	if runtimeClass == accountscontract.RuntimeClassOauthDeviceCode && strings.TrimSpace(config.DeviceAuthorizeURL) == "" {
		return false
	}
	return true
}

func presetUsesRefreshableUpstreamClient(preset Preset) bool {
	if preset.AccountTemplate == nil {
		return false
	}
	switch preset.AccountTemplate.UpstreamClient {
	case "codex_cli", "claude_code_cli", "antigravity_desktop", "antigravity":
		return true
	default:
		return false
	}
}

func containsRuntimeClass(values []accountscontract.RuntimeClass, target accountscontract.RuntimeClass) bool {
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

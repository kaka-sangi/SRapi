package preset

import (
	"sort"
	"strings"

	accountscontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
)

type PlatformFamily string

const (
	PlatformFamilyOpenAICompatible        PlatformFamily = "openai_compatible"
	PlatformFamilyAnthropicCompatible     PlatformFamily = "anthropic_compatible"
	PlatformFamilyBedrockAnthropic        PlatformFamily = "bedrock_anthropic"
	PlatformFamilyReverseProxyAntigravity PlatformFamily = "reverse_proxy_antigravity"
	PlatformFamilyRerankCompatible        PlatformFamily = "rerank_compatible"
)

type AuthMode string

const (
	AuthModeBearer       AuthMode = "bearer"
	AuthModeXAPIKey      AuthMode = "x_api_key"
	AuthModeAPIKeyQuery  AuthMode = "api_key_query"
	AuthModeCustomHeader AuthMode = "custom_header"
)

type Preset struct {
	ProviderKey        string
	PlatformFamily     PlatformFamily
	DisplayName        string
	RouteAliases       []string
	GeminiRouteAliases []string
	DefaultBaseURL     string
	AuthModes          []AuthMode
	ModelCatalogOwner  string
	// RuntimeClassAllowlist is the set of authentication methods (runtime
	// classes) an admin may attach to accounts on this provider. It is the
	// single source of truth shared with the OpenAPI Provider.auth_methods
	// field and the create/update validation. An empty list means "no
	// restriction" (every runtime class is allowed) — required for legacy and
	// manually-created providers that carry no preset metadata.
	RuntimeClassAllowlist []accountscontract.RuntimeClass
	Capabilities          map[string]bool
}

func (p Preset) MatchesPath(path string) bool {
	normalized := strings.TrimRight(path, "/")
	for _, alias := range p.RouteAliases {
		if alias == "" {
			continue
		}
		prefix := strings.TrimRight(alias, "/")
		if normalized == prefix || strings.HasPrefix(normalized, prefix+"/") {
			return true
		}
	}
	return false
}

type Registry struct {
	presets map[string]Preset
	keys    []string
}

func Default() *Registry {
	return New(
		anthropicPreset("anthropic", "Anthropic", "https://api.anthropic.com/v1", []string{"/anthropic/v1", "/api/provider/anthropic", "/api/provider/anthropic/v1"}),
		anthropicPreset("anthropic-compatible", "Anthropic Compatible", "https://api.anthropic.com/v1", []string{"/api/provider/anthropic-compatible", "/api/provider/anthropic-compatible/v1", "/api/provider/claude-compatible", "/api/provider/claude-compatible/v1"}),
		antigravityPreset(),
		bedrockPreset(),
		anthropicPreset("deepseek-anthropic", "DeepSeek Anthropic Compatible", "https://api.deepseek.com/anthropic", providerAliases("deepseek-anthropic")),
		anthropicPreset("moonshot-anthropic", "Moonshot Anthropic Compatible", "https://api.moonshot.ai/anthropic", providerAliases("moonshot-anthropic")),
		anthropicPreset("zai-anthropic", "Z.AI Anthropic Compatible", "https://api.z.ai/api/anthropic", providerAliases("zai-anthropic")),
		anthropicPreset("zhipu-anthropic", "Zhipu Anthropic Compatible", "https://open.bigmodel.cn/api/anthropic", providerAliases("zhipu-anthropic")),
		openAIPreset("anyrouter", "AnyRouter", "https://anyrouter.dev/api/v1", providerAliases("anyrouter")),
		openAIPreset("cerebras", "Cerebras", "https://api.cerebras.ai/v1", providerAliases("cerebras")),
		openAIPreset("deepseek", "DeepSeek", "https://api.deepseek.com", providerAliases("deepseek")),
		openAIPreset("groq", "Groq", "https://api.groq.com/openai/v1", providerAliases("groq")),
		openAIPreset("grok", "Grok", "https://api.x.ai/v1", []string{"/grok/v1", "/api/provider/grok", "/api/provider/grok/v1"}),
		openAIPreset("kimi", "Kimi", "https://api.moonshot.ai/v1", providerAliases("kimi")),
		openAIPreset("mistral", "Mistral", "https://api.mistral.ai/v1", providerAliases("mistral")),
		openAIPreset("moonshot", "Moonshot", "https://api.moonshot.ai/v1", providerAliases("moonshot")),
		openAIPreset("openai", "OpenAI", "https://api.openai.com/v1", []string{"/openai/v1", "/api/provider/openai", "/api/provider/openai/v1"}),
		openAIPreset("openai-compatible", "OpenAI Compatible", "https://api.openai.com/v1", []string{"/api/provider/openai-compatible", "/api/provider/openai-compatible/v1"}),
		openAIPreset("openrouter", "OpenRouter", "https://openrouter.ai/api/v1", providerAliases("openrouter")),
		openAIPreset("qwen", "通义千问", "https://dashscope.aliyuncs.com/compatible-mode/v1", append(providerAliases("qwen"), providerAliases("tongyi")...)),
		rerankPreset("rerank-compatible", "Rerank Compatible", "https://api.cohere.com/v2", []string{"/api/provider/rerank-compatible", "/api/provider/rerank-compatible/v1"}),
		openAIPreset("together", "Together AI", "https://api.together.ai/v1", providerAliases("together")),
		openAIPreset("zai", "Z.AI", "https://api.z.ai/api/paas/v4", providerAliases("zai")),
		openAIPreset("zhipu", "Zhipu", "https://open.bigmodel.cn/api/paas/v4", providerAliases("zhipu")),
	)
}

func openAIPreset(providerKey string, displayName string, defaultBaseURL string, routeAliases []string) Preset {
	capabilities := openAICapabilities()
	if openAIPresetSupportsResponsesCompact(providerKey) {
		capabilities[capabilitiescontract.KeyResponsesCompact] = true
	}
	preset := compatiblePreset(providerKey, PlatformFamilyOpenAICompatible, displayName, defaultBaseURL, routeAliases, capabilities)
	if providerKey == "openai" {
		// First-party OpenAI: the ChatGPT/Codex account flows authenticate via
		// OAuth in addition to plain API keys.
		preset.RuntimeClassAllowlist = []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassAPIKey,
			accountscontract.RuntimeClassOauthRefresh,
			accountscontract.RuntimeClassOauthDeviceCode,
			accountscontract.RuntimeClassCustomReverseProxy,
		}
	}
	return preset
}

func anthropicPreset(providerKey string, displayName string, defaultBaseURL string, routeAliases []string) Preset {
	preset := compatiblePreset(providerKey, PlatformFamilyAnthropicCompatible, displayName, defaultBaseURL, routeAliases, anthropicCapabilities())
	if providerKey == "anthropic" {
		// First-party Anthropic: Claude Code (OAuth), Claude setup-token (CLI),
		// Vertex service accounts, plus plain API keys.
		preset.RuntimeClassAllowlist = []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassAPIKey,
			accountscontract.RuntimeClassOauthRefresh,
			accountscontract.RuntimeClassOauthDeviceCode,
			accountscontract.RuntimeClassCliClientToken,
			accountscontract.RuntimeClassServiceAccountJSON,
			accountscontract.RuntimeClassCustomReverseProxy,
		}
	}
	return preset
}

func rerankPreset(providerKey string, displayName string, defaultBaseURL string, routeAliases []string) Preset {
	return compatiblePreset(providerKey, PlatformFamilyRerankCompatible, displayName, defaultBaseURL, routeAliases, rerankCapabilities())
}

func antigravityPreset() Preset {
	preset := compatiblePreset(
		"antigravity",
		PlatformFamilyReverseProxyAntigravity,
		"Antigravity",
		"",
		[]string{"/antigravity/v1", "/api/provider/antigravity", "/api/provider/antigravity/v1"},
		antigravityCapabilities(),
	)
	preset.GeminiRouteAliases = []string{"/antigravity/v1beta", "/api/provider/antigravity/v1beta"}
	preset.AuthModes = []AuthMode{AuthModeBearer, AuthModeCustomHeader}
	preset.RuntimeClassAllowlist = []accountscontract.RuntimeClass{
		accountscontract.RuntimeClassDesktopClientToken,
		accountscontract.RuntimeClassIdePluginToken,
		accountscontract.RuntimeClassOauthRefresh,
		accountscontract.RuntimeClassCustomReverseProxy,
	}
	return preset
}

func bedrockPreset() Preset {
	return Preset{
		ProviderKey:       "bedrock",
		PlatformFamily:    PlatformFamilyBedrockAnthropic,
		DisplayName:       "Amazon Bedrock",
		RouteAliases:      []string{"/bedrock/v1", "/api/provider/bedrock", "/api/provider/bedrock/v1"},
		DefaultBaseURL:    "https://bedrock-runtime.us-east-1.amazonaws.com",
		AuthModes:         []AuthMode{AuthModeCustomHeader},
		ModelCatalogOwner: "bedrock",
		RuntimeClassAllowlist: []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassAPIKey,
			accountscontract.RuntimeClassServiceAccountJSON,
		},
		Capabilities: anthropicCapabilities(),
	}
}

func compatiblePreset(providerKey string, platformFamily PlatformFamily, displayName string, defaultBaseURL string, routeAliases []string, capabilities map[string]bool) Preset {
	return Preset{
		ProviderKey:           providerKey,
		PlatformFamily:        platformFamily,
		DisplayName:           displayName,
		RouteAliases:          routeAliases,
		DefaultBaseURL:        defaultBaseURL,
		AuthModes:             []AuthMode{AuthModeBearer, AuthModeXAPIKey, AuthModeAPIKeyQuery, AuthModeCustomHeader},
		ModelCatalogOwner:     providerKey,
		RuntimeClassAllowlist: standardRuntimeClasses(),
		Capabilities:          capabilities,
	}
}

func providerAliases(providerKey string) []string {
	return []string{"/api/provider/" + providerKey, "/api/provider/" + providerKey + "/v1"}
}

// standardRuntimeClasses is the default authentication-method allowlist for
// third-party OpenAI/Anthropic-compatible providers: a plain API key, or a
// custom reverse-proxy token. First-party providers (openai, anthropic,
// antigravity, bedrock) override this with their richer sets.
func standardRuntimeClasses() []accountscontract.RuntimeClass {
	return []accountscontract.RuntimeClass{
		accountscontract.RuntimeClassAPIKey,
		accountscontract.RuntimeClassCustomReverseProxy,
	}
}

func openAICapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyChatCompletions:     true,
		capabilitiescontract.KeyResponses:           true,
		capabilitiescontract.KeyMessages:            true,
		capabilitiescontract.KeyEmbeddings:          true,
		capabilitiescontract.KeyImages:              true,
		capabilitiescontract.KeyAudioTranscriptions: true,
		capabilitiescontract.KeyAudioSpeech:         true,
		capabilitiescontract.KeyModerations:         true,
		capabilitiescontract.KeyStreaming:           true,
		capabilitiescontract.KeyToolCalling:         true,
		capabilitiescontract.KeyStructuredOutput:    true,
		capabilitiescontract.KeyVisionInput:         true,
		capabilitiescontract.KeyReasoningControl:    true,
	}
}

func openAIPresetSupportsResponsesCompact(providerKey string) bool {
	switch strings.TrimSpace(providerKey) {
	case "openai", "openai-compatible":
		return true
	default:
		return false
	}
}

func anthropicCapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyMessages:         true,
		capabilitiescontract.KeyStreaming:        true,
		capabilitiescontract.KeyTokenCounting:    true,
		capabilitiescontract.KeyToolCalling:      true,
		capabilitiescontract.KeyStructuredOutput: true,
		capabilitiescontract.KeyVisionInput:      true,
	}
}

func rerankCapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyRerank: true,
	}
}

func antigravityCapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyChatCompletions:  true,
		capabilitiescontract.KeyMessages:         true,
		capabilitiescontract.KeyStreaming:        true,
		capabilitiescontract.KeyToolCalling:      true,
		capabilitiescontract.KeyStructuredOutput: true,
		capabilitiescontract.KeyVisionInput:      true,
	}
}

func New(presets ...Preset) *Registry {
	registry := &Registry{
		presets: make(map[string]Preset, len(presets)),
		keys:    make([]string, 0, len(presets)),
	}
	for _, preset := range presets {
		if preset.ProviderKey == "" {
			continue
		}
		if _, exists := registry.presets[preset.ProviderKey]; !exists {
			registry.keys = append(registry.keys, preset.ProviderKey)
		}
		registry.presets[preset.ProviderKey] = clonePreset(preset)
	}
	sort.Strings(registry.keys)
	return registry
}

func (r *Registry) Lookup(providerKey string) (Preset, bool) {
	if r == nil {
		return Preset{}, false
	}
	preset, ok := r.presets[providerKey]
	if !ok {
		return Preset{}, false
	}
	return clonePreset(preset), true
}

func (r *Registry) List() []Preset {
	if r == nil {
		return nil
	}
	out := make([]Preset, 0, len(r.keys))
	for _, key := range r.keys {
		out = append(out, clonePreset(r.presets[key]))
	}
	return out
}

func clonePreset(preset Preset) Preset {
	cloned := preset
	cloned.RouteAliases = append([]string(nil), preset.RouteAliases...)
	cloned.GeminiRouteAliases = append([]string(nil), preset.GeminiRouteAliases...)
	cloned.AuthModes = append([]AuthMode(nil), preset.AuthModes...)
	cloned.RuntimeClassAllowlist = append([]accountscontract.RuntimeClass(nil), preset.RuntimeClassAllowlist...)
	if preset.Capabilities != nil {
		cloned.Capabilities = make(map[string]bool, len(preset.Capabilities))
		for key, value := range preset.Capabilities {
			cloned.Capabilities[key] = value
		}
	}
	return cloned
}

package preset

import (
	"sort"
	"strings"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
)

type PlatformFamily string

const (
	PlatformFamilyOpenAICompatible    PlatformFamily = "openai_compatible"
	PlatformFamilyAnthropicCompatible PlatformFamily = "anthropic_compatible"
	PlatformFamilyRerankCompatible    PlatformFamily = "rerank_compatible"
)

type AuthMode string

const (
	AuthModeBearer       AuthMode = "bearer"
	AuthModeXAPIKey      AuthMode = "x_api_key"
	AuthModeAPIKeyQuery  AuthMode = "api_key_query"
	AuthModeCustomHeader AuthMode = "custom_header"
)

type AccountType string

const (
	AccountTypeAPIKey             AccountType = "api_key"
	AccountTypeUpstream           AccountType = "upstream"
	AccountTypeCustomReverseProxy AccountType = "custom_reverse_proxy"
)

type Preset struct {
	ProviderKey          string
	PlatformFamily       PlatformFamily
	DisplayName          string
	RouteAliases         []string
	DefaultBaseURL       string
	AuthModes            []AuthMode
	ModelCatalogOwner    string
	AccountTypeAllowlist []AccountType
	Capabilities         map[string]bool
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
		openAIPreset("moonshot", "Moonshot", "https://api.moonshot.ai/v1", providerAliases("moonshot")),
		openAIPreset("openai", "OpenAI", "https://api.openai.com/v1", []string{"/openai/v1", "/api/provider/openai", "/api/provider/openai/v1"}),
		openAIPreset("openai-compatible", "OpenAI Compatible", "https://api.openai.com/v1", []string{"/api/provider/openai-compatible", "/api/provider/openai-compatible/v1"}),
		openAIPreset("openrouter", "OpenRouter", "https://openrouter.ai/api/v1", providerAliases("openrouter")),
		rerankPreset("rerank-compatible", "Rerank Compatible", "https://api.cohere.com/v2", []string{"/api/provider/rerank-compatible", "/api/provider/rerank-compatible/v1"}),
		openAIPreset("zai", "Z.AI", "https://api.z.ai/api/paas/v4", providerAliases("zai")),
		openAIPreset("zhipu", "Zhipu", "https://open.bigmodel.cn/api/paas/v4", providerAliases("zhipu")),
	)
}

func openAIPreset(providerKey string, displayName string, defaultBaseURL string, routeAliases []string) Preset {
	return compatiblePreset(providerKey, PlatformFamilyOpenAICompatible, displayName, defaultBaseURL, routeAliases, openAICapabilities())
}

func anthropicPreset(providerKey string, displayName string, defaultBaseURL string, routeAliases []string) Preset {
	return compatiblePreset(providerKey, PlatformFamilyAnthropicCompatible, displayName, defaultBaseURL, routeAliases, anthropicCapabilities())
}

func rerankPreset(providerKey string, displayName string, defaultBaseURL string, routeAliases []string) Preset {
	return compatiblePreset(providerKey, PlatformFamilyRerankCompatible, displayName, defaultBaseURL, routeAliases, rerankCapabilities())
}

func compatiblePreset(providerKey string, platformFamily PlatformFamily, displayName string, defaultBaseURL string, routeAliases []string, capabilities map[string]bool) Preset {
	return Preset{
		ProviderKey:          providerKey,
		PlatformFamily:       platformFamily,
		DisplayName:          displayName,
		RouteAliases:         routeAliases,
		DefaultBaseURL:       defaultBaseURL,
		AuthModes:            []AuthMode{AuthModeBearer, AuthModeXAPIKey, AuthModeAPIKeyQuery, AuthModeCustomHeader},
		ModelCatalogOwner:    providerKey,
		AccountTypeAllowlist: standardAccountTypes(),
		Capabilities:         capabilities,
	}
}

func providerAliases(providerKey string) []string {
	return []string{"/api/provider/" + providerKey, "/api/provider/" + providerKey + "/v1"}
}

func standardAccountTypes() []AccountType {
	return []AccountType{
		AccountTypeAPIKey,
		AccountTypeUpstream,
		AccountTypeCustomReverseProxy,
	}
}

func openAICapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyChatCompletions:     true,
		capabilitiescontract.KeyResponses:           true,
		capabilitiescontract.KeyMessages:            true,
		capabilitiescontract.KeyEmbeddings:          true,
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

func anthropicCapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyMessages:         true,
		capabilitiescontract.KeyStreaming:        true,
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
	cloned.AuthModes = append([]AuthMode(nil), preset.AuthModes...)
	cloned.AccountTypeAllowlist = append([]AccountType(nil), preset.AccountTypeAllowlist...)
	if preset.Capabilities != nil {
		cloned.Capabilities = make(map[string]bool, len(preset.Capabilities))
		for key, value := range preset.Capabilities {
			cloned.Capabilities[key] = value
		}
	}
	return cloned
}

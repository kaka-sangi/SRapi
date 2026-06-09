package preset

import (
	"sort"
	"strings"

	accountscontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

type PlatformFamily string

const (
	PlatformFamilyOpenAICompatible        PlatformFamily = "openai_compatible"
	PlatformFamilyAnthropicCompatible     PlatformFamily = "anthropic_compatible"
	PlatformFamilyGeminiCompatible        PlatformFamily = "gemini_compatible"
	PlatformFamilyBedrockAnthropic        PlatformFamily = "bedrock_anthropic"
	PlatformFamilyReverseProxyAntigravity PlatformFamily = "reverse_proxy_antigravity"
	PlatformFamilyRerankCompatible        PlatformFamily = "rerank_compatible"
	PlatformFamilyCodexCLI                PlatformFamily = "codex_cli"
)

type AuthMode string

const (
	AuthModeBearer       AuthMode = "bearer"
	AuthModeXAPIKey      AuthMode = "x_api_key"
	AuthModeAPIKeyQuery  AuthMode = "api_key_query"
	AuthModeCustomHeader AuthMode = "custom_header"
)

// AccountTemplate carries provider-specific defaults that the Quick Setup
// wizard and the account form use to auto-fill fields. Serialized into the
// provider's config_schema.account_template so the frontend can read it.
type AccountTemplate struct {
	UpstreamClient  string            `json:"upstream_client,omitempty"`
	DefaultMetadata map[string]any    `json:"default_metadata,omitempty"`
	ModelCatalog    []string          `json:"model_catalog,omitempty"`
	MetadataHints   map[string]string `json:"metadata_hints,omitempty"`
}

// OAuthConfig carries non-secret OAuth defaults for interactive upstream-account
// provisioning. Client secrets are intentionally not part of provider presets.
type OAuthConfig struct {
	ClientID           string
	AuthorizeURL       string
	TokenURL           string
	DeviceAuthorizeURL string
	RedirectURI        string
	Scopes             []string
	UsePKCE            bool
}

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
	AccountTemplate       *AccountTemplate
	QuotaConfig           map[string]string
	OAuthConfig           *OAuthConfig
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
		chatGPTWebPreset(),
		codexCLIPreset(),
		geminiPreset(),
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
		preset.RuntimeClassAllowlist = []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassAPIKey,
			accountscontract.RuntimeClassCustomReverseProxy,
		}
		preset.AccountTemplate = &AccountTemplate{
			ModelCatalog: []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-4.1", "gpt-4.1-mini", "gpt-4.1-nano", "o4-mini", "o3", "o3-pro"},
		}
	}
	switch providerKey {
	case "deepseek":
		preset.AccountTemplate = &AccountTemplate{
			ModelCatalog: []string{"deepseek-r1", "deepseek-v3-0324", "deepseek-chat", "deepseek-reasoner"},
		}
	case "groq":
		preset.AccountTemplate = &AccountTemplate{
			ModelCatalog: []string{"llama-4-scout-17b-16e-instruct", "llama-4-maverick-17b-128e-instruct", "qwen-qwq-32b", "deepseek-r1-distill-llama-70b", "llama-3.3-70b-versatile", "llama-3.1-8b-instant"},
		}
	case "mistral":
		preset.AccountTemplate = &AccountTemplate{
			ModelCatalog: []string{"mistral-large-latest", "mistral-medium-latest", "mistral-small-latest", "codestral-latest", "open-mistral-nemo"},
		}
	case "openrouter":
		preset.AccountTemplate = &AccountTemplate{
			ModelCatalog: []string{"openai/gpt-4.1", "anthropic/claude-sonnet-4-6", "google/gemini-2.5-pro", "deepseek/deepseek-r1", "meta-llama/llama-4-scout"},
		}
	}
	return preset
}

func codexCLIPreset() Preset {
	return Preset{
		ProviderKey:    "codex-cli",
		PlatformFamily: PlatformFamilyCodexCLI,
		DisplayName:    "Codex CLI",
		RouteAliases:   []string{"/api/provider/codex-cli", "/api/provider/codex-cli/v1", "/backend-api/codex"},
		DefaultBaseURL: "https://chatgpt.com/backend-api/codex",
		AuthModes:      []AuthMode{AuthModeBearer},
		RuntimeClassAllowlist: []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassOauthRefresh,
			accountscontract.RuntimeClassOauthDeviceCode,
			accountscontract.RuntimeClassCustomReverseProxy,
		},
		Capabilities: codexCLICapabilities(),
		AccountTemplate: &AccountTemplate{
			UpstreamClient:  "codex_cli",
			DefaultMetadata: map[string]any{"base_url": "https://chatgpt.com/backend-api/codex"},
			ModelCatalog:    []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2", "codex-mini-latest"},
			MetadataHints: map[string]string{
				"base_url":           "Codex upstream (adapter appends /responses)",
				"chatgpt_account_id": "From session JWT (optional)",
			},
		},
		QuotaConfig: map[string]string{
			"quota_url":                    "https://chatgpt.com/backend-api/accounts/check/v4-2023-04-27",
			"quota_plan_path":              "account_plan.account_plan_id",
			"quota_credits_remaining_path": "account_plan.subscription_plan.allowance",
			"quota_credits_used_path":      "account_plan.subscription_plan.usage",
			"quota_credits_limit_path":     "account_plan.subscription_plan.limit",
		},
		OAuthConfig: &OAuthConfig{
			ClientID:           reverseproxycontract.CodexOAuthClientID,
			AuthorizeURL:       "https://auth.openai.com/oauth/authorize",
			TokenURL:           reverseproxycontract.CodexOAuthTokenURL,
			DeviceAuthorizeURL: "https://auth.openai.com/oauth/device/code",
			Scopes:             strings.Fields(reverseproxycontract.CodexOAuthRefreshScope),
			UsePKCE:            true,
		},
	}
}

func anthropicPreset(providerKey string, displayName string, defaultBaseURL string, routeAliases []string) Preset {
	preset := compatiblePreset(providerKey, PlatformFamilyAnthropicCompatible, displayName, defaultBaseURL, routeAliases, anthropicCapabilities())
	if providerKey == "anthropic" {
		preset.RuntimeClassAllowlist = []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassAPIKey,
			accountscontract.RuntimeClassOauthRefresh,
			accountscontract.RuntimeClassOauthDeviceCode,
			accountscontract.RuntimeClassCliClientToken,
			accountscontract.RuntimeClassCustomReverseProxy,
		}
		preset.AccountTemplate = &AccountTemplate{
			UpstreamClient: "claude_code_cli",
			ModelCatalog:   []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"},
		}
		preset.QuotaConfig = map[string]string{
			"quota_url": "https://api.anthropic.com/api/oauth/usage",
		}
		preset.OAuthConfig = &OAuthConfig{
			ClientID:           reverseproxycontract.ClaudeCodeOAuthClientID,
			AuthorizeURL:       "https://console.anthropic.com/oauth/authorize",
			TokenURL:           reverseproxycontract.ClaudeCodeOAuthTokenURL,
			DeviceAuthorizeURL: "https://console.anthropic.com/oauth/device/code",
			Scopes:             []string{"org:create_api_key", "user:profile"},
			UsePKCE:            true,
		}
	}
	return preset
}

func geminiPreset() Preset {
	return Preset{
		ProviderKey:       "gemini",
		PlatformFamily:    PlatformFamilyGeminiCompatible,
		DisplayName:       "Gemini",
		RouteAliases:      []string{"/gemini/v1beta", "/api/provider/gemini/v1beta"},
		DefaultBaseURL:    "https://generativelanguage.googleapis.com/v1beta",
		AuthModes:         []AuthMode{AuthModeAPIKeyQuery, AuthModeBearer, AuthModeCustomHeader},
		ModelCatalogOwner: "gemini",
		RuntimeClassAllowlist: []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassAPIKey,
			accountscontract.RuntimeClassCustomReverseProxy,
		},
		Capabilities: geminiCapabilities(),
		AccountTemplate: &AccountTemplate{
			ModelCatalog: []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"},
		},
	}
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
		accountscontract.RuntimeClassOauthRefresh,
		accountscontract.RuntimeClassCustomReverseProxy,
	}
	preset.AccountTemplate = &AccountTemplate{
		UpstreamClient: "antigravity_desktop",
		DefaultMetadata: map[string]any{
			"project_id": "",
		},
		ModelCatalog: []string{"gemini-3-pro-preview", "claude-sonnet-4-6"},
		MetadataHints: map[string]string{
			"base_url":   "Antigravity / Google Cloud Code upstream URL",
			"project_id": "Google Cloud project id for Antigravity requests",
		},
	}
	preset.OAuthConfig = googleOAuthConfig()
	return preset
}

func chatGPTWebPreset() Preset {
	return Preset{
		ProviderKey:    "chatgpt-web",
		PlatformFamily: PlatformFamilyOpenAICompatible,
		DisplayName:    "ChatGPT Web",
		RouteAliases:   []string{"/chatgpt-web/v1", "/api/provider/chatgpt-web", "/api/provider/chatgpt-web/v1"},
		DefaultBaseURL: "https://chatgpt.com",
		AuthModes:      []AuthMode{AuthModeBearer, AuthModeCustomHeader},
		RuntimeClassAllowlist: []accountscontract.RuntimeClass{
			accountscontract.RuntimeClassWebSessionCookie,
			accountscontract.RuntimeClassCustomReverseProxy,
		},
		Capabilities: openAICapabilities(),
		AccountTemplate: &AccountTemplate{
			UpstreamClient: "chatgpt_web",
			DefaultMetadata: map[string]any{
				"base_url": "https://chatgpt.com",
			},
			ModelCatalog: []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini"},
			MetadataHints: map[string]string{
				"chatgpt_requirements_token": "OpenAI Sentinel chat requirements token, or enable requirements_auto",
				"user_agent":                 "Browser user agent for ChatGPT web requests",
			},
		},
	}
}

func googleOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		ClientID:           reverseproxycontract.AntigravityOAuthClientID,
		AuthorizeURL:       "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:           reverseproxycontract.AntigravityOAuthTokenURL,
		DeviceAuthorizeURL: "https://oauth2.googleapis.com/device/code",
		Scopes:             []string{"openid", "email", "profile", "https://www.googleapis.com/auth/cloud-platform"},
		UsePKCE:            true,
	}
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

func geminiCapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyStreaming:     true,
		capabilitiescontract.KeyTokenCounting: true,
		capabilitiescontract.KeyToolCalling:   true,
		capabilitiescontract.KeyVisionInput:   true,
	}
}

func codexCLICapabilities() map[string]bool {
	return map[string]bool{
		capabilitiescontract.KeyResponses:        true,
		capabilitiescontract.KeyResponsesCompact: true,
		capabilitiescontract.KeyStreaming:        true,
		capabilitiescontract.KeyToolCalling:      true,
		capabilitiescontract.KeyStructuredOutput: true,
		capabilitiescontract.KeyVisionInput:      true,
		capabilitiescontract.KeyReasoningControl: true,
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

func (r *Registry) CapabilitiesForPlatformFamily(family PlatformFamily) map[string]bool {
	if r == nil {
		return nil
	}
	for _, preset := range r.presets {
		if preset.PlatformFamily == family {
			out := make(map[string]bool, len(preset.Capabilities))
			for k, v := range preset.Capabilities {
				out[k] = v
			}
			return out
		}
	}
	return nil
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
	if preset.OAuthConfig != nil {
		oauthConfig := *preset.OAuthConfig
		oauthConfig.Scopes = append([]string(nil), preset.OAuthConfig.Scopes...)
		cloned.OAuthConfig = &oauthConfig
	}
	return cloned
}

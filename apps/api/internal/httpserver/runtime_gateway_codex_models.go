package httpserver

import (
	"sort"
	"strings"

	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type codexClientTemplate struct {
	slug                  string
	displayName           string
	description           string
	priority              int
	visibility            string
	minimalClientVersion  string
	contextWindow         int
	maxContextWindow      int
	defaultVerbosity      string
	webSearchToolType     string
	defaultReasoningLevel string
	serviceTiers          []map[string]any
	reasoningLevels       []map[string]any
}

var codexClientTemplates = []codexClientTemplate{
	{
		slug:                  "gpt-5.5",
		displayName:           "GPT-5.5",
		description:           "Frontier model for complex coding, research, and real-world work.",
		priority:              0,
		visibility:            "list",
		minimalClientVersion:  "0.124.0",
		contextWindow:         272000,
		maxContextWindow:      272000,
		defaultVerbosity:      "low",
		webSearchToolType:     "text_and_image",
		defaultReasoningLevel: "medium",
		serviceTiers:          codexClientPriorityServiceTiers(),
		reasoningLevels:       codexClientDefaultReasoningLevels(),
	},
	{
		slug:                  "gpt-5.4",
		displayName:           "gpt-5.4",
		description:           "Strong model for everyday coding.",
		priority:              2,
		visibility:            "list",
		minimalClientVersion:  "0.98.0",
		contextWindow:         272000,
		maxContextWindow:      1000000,
		defaultVerbosity:      "low",
		webSearchToolType:     "text_and_image",
		defaultReasoningLevel: "xhigh",
		serviceTiers:          codexClientPriorityServiceTiers(),
		reasoningLevels:       codexClientDefaultReasoningLevels(),
	},
	{
		slug:                  "gpt-5.4-mini",
		displayName:           "GPT-5.4-Mini",
		description:           "Small, fast, and cost-efficient model for simpler coding tasks.",
		priority:              4,
		visibility:            "list",
		minimalClientVersion:  "0.98.0",
		contextWindow:         272000,
		maxContextWindow:      272000,
		defaultVerbosity:      "medium",
		webSearchToolType:     "text_and_image",
		defaultReasoningLevel: "medium",
		reasoningLevels:       codexClientDefaultReasoningLevels(),
	},
	{
		slug:                  "gpt-5.3-codex",
		displayName:           "gpt-5.3-codex",
		description:           "Coding-optimized model.",
		priority:              6,
		visibility:            "list",
		minimalClientVersion:  "0.98.0",
		contextWindow:         272000,
		maxContextWindow:      272000,
		defaultVerbosity:      "low",
		webSearchToolType:     "text",
		defaultReasoningLevel: "medium",
		reasoningLevels:       codexClientDefaultReasoningLevels(),
	},
	{
		slug:                  "gpt-5.2",
		displayName:           "gpt-5.2",
		description:           "Optimized for professional work and long-running agents.",
		priority:              10,
		visibility:            "list",
		minimalClientVersion:  "0.0.1",
		contextWindow:         272000,
		maxContextWindow:      272000,
		defaultVerbosity:      "low",
		webSearchToolType:     "text",
		defaultReasoningLevel: "medium",
		reasoningLevels: []map[string]any{
			{"effort": "low", "description": "Balances speed with some reasoning; useful for straightforward queries and short explanations"},
			{"effort": "medium", "description": "Provides a solid balance of reasoning depth and latency for general-purpose tasks"},
			{"effort": "high", "description": "Maximizes reasoning depth for complex or ambiguous problems"},
			{"effort": "xhigh", "description": "Extra high reasoning for complex problems"},
		},
	},
	{
		slug:                  "codex-auto-review",
		displayName:           "Codex Auto Review",
		description:           "Automatic approval review model for Codex.",
		priority:              29,
		visibility:            "hide",
		minimalClientVersion:  "0.98.0",
		contextWindow:         272000,
		maxContextWindow:      1000000,
		defaultVerbosity:      "low",
		webSearchToolType:     "text_and_image",
		defaultReasoningLevel: "medium",
		reasoningLevels:       codexClientDefaultReasoningLevels(),
	},
}

func codexClientModelsResponse(models []modelcontract.Model, visible []apiopenapi.OpenAIModel) map[string]any {
	return map[string]any{
		"models": buildCodexClientModels(codexClientModelSources(models, visible)),
	}
}

func codexClientModelSources(models []modelcontract.Model, visible []apiopenapi.OpenAIModel) []map[string]any {
	metadata := make(map[string]modelcontract.Model, len(models))
	for _, model := range models {
		metadata[model.CanonicalName] = model
	}
	out := make([]map[string]any, 0, len(visible))
	for _, visibleModel := range visible {
		id := strings.TrimSpace(visibleModel.Id)
		if id == "" {
			continue
		}
		source := map[string]any{
			"id":       id,
			"object":   "model",
			"owned_by": visibleModel.OwnedBy,
		}
		if visibleModel.Created != nil {
			source["created"] = *visibleModel.Created
		}
		if model, ok := metadata[id]; ok {
			if displayName := strings.TrimSpace(model.DisplayName); displayName != "" {
				source["display_name"] = displayName
				source["description"] = displayName
			}
			if model.ContextWindow != nil && *model.ContextWindow > 0 {
				source["context_length"] = *model.ContextWindow
			}
		}
		out = append(out, source)
	}
	return out
}

func buildCodexClientModels(models []map[string]any) []map[string]any {
	templates := codexClientTemplatesBySlug()
	defaultTemplate := codexClientTemplateBySlug("gpt-5.5")
	result := make([]map[string]any, 0, len(models))
	for _, model := range models {
		id := codexClientStringValue(model, "id")
		if id == "" {
			continue
		}
		if template, ok := templates[id]; ok {
			entry := codexClientEntryFromTemplate(template)
			applyCodexClientVisibilityOverride(entry, id)
			result = append(result, entry)
			continue
		}
		entry := codexClientEntryFromTemplate(defaultTemplate)
		applyCodexClientModelMetadata(entry, id, model)
		applyCodexClientVisibilityOverride(entry, id)
		result = append(result, entry)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return codexClientIntValue(result[i], "priority") < codexClientIntValue(result[j], "priority")
	})
	return result
}

func codexClientTemplatesBySlug() map[string]codexClientTemplate {
	out := make(map[string]codexClientTemplate, len(codexClientTemplates))
	for _, template := range codexClientTemplates {
		out[template.slug] = template
	}
	return out
}

func codexClientTemplateBySlug(slug string) codexClientTemplate {
	for _, template := range codexClientTemplates {
		if template.slug == slug {
			return template
		}
	}
	return codexClientTemplate{}
}

func codexClientEntryFromTemplate(template codexClientTemplate) map[string]any {
	serviceTiers := cloneMapSlice(template.serviceTiers)
	if serviceTiers == nil {
		serviceTiers = []map[string]any{}
	}
	entry := map[string]any{
		"prefer_websockets":              true,
		"support_verbosity":              true,
		"default_verbosity":              template.defaultVerbosity,
		"apply_patch_tool_type":          "freeform",
		"web_search_tool_type":           template.webSearchToolType,
		"input_modalities":               []any{"text", "image"},
		"supports_image_detail_original": true,
		"truncation_policy":              map[string]any{"mode": "tokens", "limit": 10000},
		"supports_parallel_tool_calls":   true,
		"context_window":                 template.contextWindow,
		"max_context_window":             template.maxContextWindow,
		"auto_compact_token_limit":       nil,
		"reasoning_summary_format":       "experimental",
		"default_reasoning_summary":      "none",
		"slug":                           template.slug,
		"display_name":                   template.displayName,
		"description":                    template.description,
		"default_reasoning_level":        template.defaultReasoningLevel,
		"supported_reasoning_levels":     cloneMapSlice(template.reasoningLevels),
		"shell_type":                     "shell_command",
		"visibility":                     template.visibility,
		"minimal_client_version":         template.minimalClientVersion,
		"supported_in_api":               true,
		"availability_nux":               nil,
		"upgrade":                        nil,
		"priority":                       template.priority,
		"base_instructions":              provideradapterservice.CodexBaseInstructionsForModel(template.slug),
		"available_in_plans":             codexClientAvailablePlans(),
		"service_tiers":                  serviceTiers,
	}
	return entry
}

func applyCodexClientModelMetadata(entry map[string]any, id string, model map[string]any) {
	displayName := codexClientStringValue(model, "display_name")
	description := codexClientStringValue(model, "description")
	contextWindow := codexClientIntValue(model, "context_length")
	if displayName == "" {
		displayName = id
	}
	if description == "" {
		description = id
	}
	entry["slug"] = id
	entry["display_name"] = displayName
	entry["description"] = description
	entry["priority"] = 100
	entry["prefer_websockets"] = false
	entry["base_instructions"] = provideradapterservice.CodexBaseInstructionsForModel(id)
	delete(entry, "apply_patch_tool_type")
	delete(entry, "upgrade")
	delete(entry, "availability_nux")
	if contextWindow > 0 {
		entry["context_window"] = contextWindow
		entry["max_context_window"] = contextWindow
	}
}

func applyCodexClientVisibilityOverride(entry map[string]any, id string) {
	switch strings.TrimSpace(id) {
	case "grok-imagine-image-quality", "gpt-image-2", "grok-imagine-image", "grok-imagine-video", "grok-imagine-video-1.5-preview":
		entry["visibility"] = "hide"
	}
}

func codexClientDefaultReasoningLevels() []map[string]any {
	return []map[string]any{
		{"effort": "low", "description": "Fast responses with lighter reasoning"},
		{"effort": "medium", "description": "Balances speed and reasoning depth for everyday tasks"},
		{"effort": "high", "description": "Greater reasoning depth for complex problems"},
		{"effort": "xhigh", "description": "Extra high reasoning depth for complex problems"},
	}
}

func codexClientPriorityServiceTiers() []map[string]any {
	return []map[string]any{
		{"id": "priority", "name": "Fast", "description": "1.5x speed, increased usage"},
	}
}

func codexClientAvailablePlans() []any {
	return []any{
		"business",
		"edu",
		"education",
		"enterprise",
		"enterprise_cbp_usage_based",
		"finserv",
		"free",
		"free_workspace",
		"go",
		"hc",
		"k12",
		"plus",
		"pro",
		"prolite",
		"quorum",
		"self_serve_business_usage_based",
		"team",
	}
}

func codexClientStringValue(model map[string]any, key string) string {
	if model == nil {
		return ""
	}
	value, ok := model[key]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func codexClientIntValue(model map[string]any, key string) int {
	if model == nil {
		return 0
	}
	switch value := model[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

package contract

import (
	"fmt"
	"sort"
	"strings"
)

type DescriptorLevel string

const (
	DescriptorLevelRequired    DescriptorLevel = "required"
	DescriptorLevelOptional    DescriptorLevel = "optional"
	DescriptorLevelUnsupported DescriptorLevel = "unsupported"
)

type DescriptorStatus string

const (
	DescriptorStatusStable       DescriptorStatus = "stable"
	DescriptorStatusExperimental DescriptorStatus = "experimental"
	DescriptorStatusDeprecated   DescriptorStatus = "deprecated"
)

type Descriptor struct {
	Key      string           `json:"key"`
	Level    DescriptorLevel  `json:"level"`
	Metadata map[string]any   `json:"metadata,omitempty"`
	Status   DescriptorStatus `json:"status"`
	Version  string           `json:"version"`
}

const (
	KeyTextInput         = "text_input"
	KeyTextOutput        = "text_output"
	KeyStreaming         = "streaming"
	KeyToolCalling       = "tool_calling"
	KeyParallelToolCalls = "parallel_tool_calls"
	KeyVisionInput       = "vision_input"
	KeyJSONMode          = "json_mode"
	KeyStructuredOutput  = "structured_output"
	KeyReasoningControl  = "reasoning_control"
	KeyPromptCache       = "prompt_cache"
	KeyContextCache      = "context_cache"
	KeyUsageInStream     = "usage_in_stream"

	KeyChatCompletions = "chat_completions"
	KeyResponses       = "responses"
	KeyMessages        = "messages"
	KeyEmbeddings      = "embeddings"
	KeyImages          = "images"
	KeyModerations     = "moderations"
	KeyRerank          = "rerank"
)

type DefinitionStatus string

const (
	DefinitionStatusStable       DefinitionStatus = "stable"
	DefinitionStatusExperimental DefinitionStatus = "experimental"
	DefinitionStatusDeprecated   DefinitionStatus = "deprecated"
)

type Definition struct {
	Category       string           `json:"category"`
	Description    string           `json:"description"`
	Key            string           `json:"key"`
	ReplacementKey *string          `json:"replacement_key,omitempty"`
	Schema         map[string]any   `json:"schema,omitempty"`
	Status         DefinitionStatus `json:"status"`
	Version        string           `json:"version"`
}

var defaultDefinitions = []Definition{
	{Key: KeyTextInput, Version: "v1", Category: "input", Status: DefinitionStatusStable, Description: "Model can consume text input."},
	{Key: KeyTextOutput, Version: "v1", Category: "output", Status: DefinitionStatusStable, Description: "Model can produce text output."},
	{Key: KeyStreaming, Version: "v1", Category: "interaction", Status: DefinitionStatusStable, Description: "Model or provider can return streaming token events."},
	{Key: KeyToolCalling, Version: "v1", Category: "interaction", Status: DefinitionStatusStable, Description: "Model or provider can accept tool definitions and produce tool calls."},
	{Key: KeyParallelToolCalls, Version: "v1", Category: "interaction", Status: DefinitionStatusStable, Description: "Model or provider can produce parallel tool calls."},
	{Key: KeyVisionInput, Version: "v1", Category: "input", Status: DefinitionStatusStable, Description: "Model or provider can consume image inputs."},
	{Key: KeyJSONMode, Version: "v1", Category: "control", Status: DefinitionStatusStable, Description: "Model or provider can follow JSON mode output constraints."},
	{Key: KeyStructuredOutput, Version: "v1", Category: "output", Status: DefinitionStatusStable, Description: "Model or provider can follow structured output schemas."},
	{Key: KeyReasoningControl, Version: "v1", Category: "control", Status: DefinitionStatusStable, Description: "Model or provider can expose reasoning or thinking controls."},
	{Key: KeyPromptCache, Version: "v1", Category: "cache", Status: DefinitionStatusStable, Description: "Model or provider can use prompt cache controls."},
	{Key: KeyContextCache, Version: "v1", Category: "cache", Status: DefinitionStatusStable, Description: "Model or provider can use context cache affinity."},
	{Key: KeyUsageInStream, Version: "v1", Category: "usage", Status: DefinitionStatusStable, Description: "Provider can emit usage in streaming responses."},
	{Key: KeyChatCompletions, Version: "v1", Category: "endpoint", Status: DefinitionStatusStable, Description: "Provider supports Chat Completions-compatible generation."},
	{Key: KeyResponses, Version: "v1", Category: "endpoint", Status: DefinitionStatusStable, Description: "Provider supports Responses-compatible generation."},
	{Key: KeyMessages, Version: "v1", Category: "endpoint", Status: DefinitionStatusStable, Description: "Provider supports Messages-compatible generation."},
	{Key: KeyEmbeddings, Version: "v1", Category: "endpoint", Status: DefinitionStatusStable, Description: "Provider supports embeddings."},
	{Key: KeyImages, Version: "v1", Category: "endpoint", Status: DefinitionStatusStable, Description: "Provider supports image generation."},
	{Key: KeyModerations, Version: "v1", Category: "endpoint", Status: DefinitionStatusStable, Description: "Provider supports moderation classification."},
	{Key: KeyRerank, Version: "v1", Category: "endpoint", Status: DefinitionStatusStable, Description: "Provider supports document reranking."},
}

var knownKeys = func() map[string]Definition {
	values := make(map[string]Definition, len(defaultDefinitions))
	for _, def := range defaultDefinitions {
		values[def.Key] = def
	}
	return values
}()

var convenienceKeys = map[string]string{
	"supports_stream":              KeyStreaming,
	"stream":                       KeyStreaming,
	"supports_tools":               KeyToolCalling,
	"tools":                        KeyToolCalling,
	"supports_parallel_tool_calls": KeyParallelToolCalls,
	"supports_vision":              KeyVisionInput,
	"vision":                       KeyVisionInput,
	"supports_json":                KeyJSONMode,
	"json":                         KeyJSONMode,
	"supports_json_mode":           KeyJSONMode,
	"supports_structured_output":   KeyStructuredOutput,
	"supports_reasoning":           KeyReasoningControl,
	"reasoning":                    KeyReasoningControl,
	"thinking":                     KeyReasoningControl,
	"supports_prompt_cache":        KeyPromptCache,
	"supports_context_cache":       KeyContextCache,
	"supports_usage_in_stream":     KeyUsageInStream,
	"supports_chat_completions":    KeyChatCompletions,
	"supports_responses":           KeyResponses,
	"supports_messages":            KeyMessages,
	"supports_embeddings":          KeyEmbeddings,
	"supports_images":              KeyImages,
	"supports_moderations":         KeyModerations,
	"supports_moderation":          KeyModerations,
	"moderation":                   KeyModerations,
	"supports_rerank":              KeyRerank,
	"supports_reranking":           KeyRerank,
	"rerank":                       KeyRerank,
	"reranking":                    KeyRerank,
}

func DefaultDefinitions() []Definition {
	out := append([]Definition(nil), defaultDefinitions...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func IsKnownKey(key string) bool {
	_, ok := knownKeys[strings.TrimSpace(key)]
	return ok
}

func CanonicalKeyFromConvenience(key string) (string, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	if IsKnownKey(key) {
		return key, true
	}
	canonical, ok := convenienceKeys[key]
	return canonical, ok
}

func NormalizeDescriptor(value Descriptor) (Descriptor, error) {
	value.Key = strings.TrimSpace(value.Key)
	if !IsKnownKey(value.Key) {
		return Descriptor{}, fmt.Errorf("unknown capability key %q", value.Key)
	}
	value.Version = strings.TrimSpace(value.Version)
	if value.Version == "" {
		value.Version = "v1"
	}
	if value.Level == "" {
		value.Level = DescriptorLevelRequired
	}
	if !validDescriptorLevel(value.Level) {
		return Descriptor{}, fmt.Errorf("unknown capability level %q", value.Level)
	}
	if value.Status == "" {
		value.Status = DescriptorStatusStable
	}
	if !validDescriptorStatus(value.Status) {
		return Descriptor{}, fmt.Errorf("unknown capability status %q", value.Status)
	}
	return value, nil
}

func NormalizeDescriptors(values []Descriptor) ([]Descriptor, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]Descriptor, 0, len(values))
	for _, value := range values {
		normalized, err := NormalizeDescriptor(value)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}

func validDescriptorLevel(value DescriptorLevel) bool {
	switch value {
	case DescriptorLevelRequired, DescriptorLevelOptional, DescriptorLevelUnsupported:
		return true
	default:
		return false
	}
}

func validDescriptorStatus(value DescriptorStatus) bool {
	switch value {
	case DescriptorStatusStable, DescriptorStatusExperimental, DescriptorStatusDeprecated:
		return true
	default:
		return false
	}
}

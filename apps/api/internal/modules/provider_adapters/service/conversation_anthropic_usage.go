package service

import (
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func anthropicStopReason(reason string) contract.StopReason {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "max_tokens":
		return contract.StopReasonMaxTokens
	case "tool_use":
		return contract.StopReasonToolUse
	case "refusal":
		return contract.StopReasonRefusal
	case "content_filter", "safety":
		return contract.StopReasonContentFilter
	}
	return contract.StopReasonEndTurn
}

type anthropicUsage struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

func (u anthropicUsage) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.InputTokens)
	output := valueOrZero(u.OutputTokens)
	// Anthropic reports input_tokens EXCLUDING cache tokens, and cache
	// creation (write) vs read separately. Keep them distinct so they bill at
	// their (different) rates: writes cost more than input, reads less.
	cacheRead := valueOrZero(u.CacheReadInputTokens)
	cacheCreation := valueOrZero(u.CacheCreationInputTokens)
	if input == 0 && output == 0 && cacheRead == 0 && cacheCreation == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:         input,
		OutputTokens:        output,
		CachedTokens:        cacheRead,
		CacheCreationTokens: cacheCreation,
		Estimated:           false,
	}
}

func (u *anthropicUsage) Merge(next anthropicUsage) {
	if u == nil {
		return
	}
	if next.InputTokens != nil {
		u.InputTokens = cloneIntPtr(next.InputTokens)
	}
	if next.OutputTokens != nil {
		u.OutputTokens = cloneIntPtr(next.OutputTokens)
	}
	if next.CacheCreationInputTokens != nil {
		u.CacheCreationInputTokens = cloneIntPtr(next.CacheCreationInputTokens)
	}
	if next.CacheReadInputTokens != nil {
		u.CacheReadInputTokens = cloneIntPtr(next.CacheReadInputTokens)
	}
}

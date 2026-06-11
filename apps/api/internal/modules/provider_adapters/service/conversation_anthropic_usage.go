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
	InputTokens                *int                         `json:"input_tokens"`
	OutputTokens               *int                         `json:"output_tokens"`
	CacheCreationInputTokens   *int                         `json:"cache_creation_input_tokens"`
	CacheReadInputTokens       *int                         `json:"cache_read_input_tokens"`
	CachedTokens               *int                         `json:"cached_tokens"`
	CacheCreation5mInputTokens *int                         `json:"cache_creation_ephemeral_5m_input_tokens"`
	CacheCreation1hInputTokens *int                         `json:"cache_creation_ephemeral_1h_input_tokens"`
	CacheCreation              *anthropicCacheCreationUsage `json:"cache_creation"`
}

type anthropicCacheCreationUsage struct {
	Ephemeral5mInputTokens *int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens *int `json:"ephemeral_1h_input_tokens"`
}

func (u anthropicUsage) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.InputTokens)
	output := valueOrZero(u.OutputTokens)
	// Anthropic reports input_tokens EXCLUDING cache tokens, and cache
	// creation (write) vs read separately. Keep them distinct so they bill at
	// their (different) rates: writes cost more than input, reads less.
	cacheRead := valueOrZero(u.CacheReadInputTokens)
	if cacheRead == 0 {
		cacheRead = valueOrZero(u.CachedTokens)
	}
	cacheCreation := valueOrZero(u.CacheCreationInputTokens)
	cacheCreation5m, cacheCreation1h := u.cacheCreationBuckets(cacheCreation)
	if input == 0 && output == 0 && cacheRead == 0 && cacheCreation == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:           input,
		OutputTokens:          output,
		CachedTokens:          cacheRead,
		CacheCreationTokens:   cacheCreation,
		CacheCreation5mTokens: cacheCreation5m,
		CacheCreation1hTokens: cacheCreation1h,
		Estimated:             false,
	}
}

func (u anthropicUsage) cacheCreationBuckets(total int) (int, int) {
	fiveMinutes := valueOrZero(u.CacheCreation5mInputTokens)
	oneHour := valueOrZero(u.CacheCreation1hInputTokens)
	if u.CacheCreation != nil {
		fiveMinutes += valueOrZero(u.CacheCreation.Ephemeral5mInputTokens)
		oneHour += valueOrZero(u.CacheCreation.Ephemeral1hInputTokens)
	}
	if total > 0 && fiveMinutes == 0 && oneHour == 0 {
		return total, 0
	}
	if fiveMinutes+oneHour < total {
		fiveMinutes += total - fiveMinutes - oneHour
	}
	return fiveMinutes, oneHour
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
	if next.CachedTokens != nil {
		u.CachedTokens = cloneIntPtr(next.CachedTokens)
	}
	if next.CacheCreation5mInputTokens != nil {
		u.CacheCreation5mInputTokens = cloneIntPtr(next.CacheCreation5mInputTokens)
	}
	if next.CacheCreation1hInputTokens != nil {
		u.CacheCreation1hInputTokens = cloneIntPtr(next.CacheCreation1hInputTokens)
	}
	if next.CacheCreation != nil {
		u.CacheCreation = next.CacheCreation
	}
}

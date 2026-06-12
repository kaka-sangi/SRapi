package service

import (
	"math"
	"net/http"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

type codexQuotaHeaderWindow struct {
	usedPercent  float64
	resetSeconds *int
	windowMin    int
}

type codexQuotaWindowKind string

const (
	codexQuotaWindow5h codexQuotaWindowKind = "5h"
	codexQuotaWindow7d codexQuotaWindowKind = "7d"
)

type codexQuotaHeaderMapping struct {
	kind   codexQuotaWindowKind
	window codexQuotaHeaderWindow
}

func codexQuotaSignalsFromHeaders(headers http.Header, now time.Time) []contract.QuotaSignal {
	if headers == nil {
		return nil
	}
	now = now.UTC()
	metadata := codexQuotaMetadataFromHeaders(headers, now)
	windows := codexQuotaHeaderMappings(
		codexQuotaHeaderWindowFromHeaders(headers, "x-codex-primary"),
		codexQuotaHeaderWindowFromHeaders(headers, "x-codex-secondary"),
	)
	signals := make([]contract.QuotaSignal, 0, len(windows))
	for _, mapping := range windows {
		if !codexQuotaHeaderWindowValid(mapping.window) {
			continue
		}
		quotaType := codexQuotaTypeForWindow(mapping.kind)
		if quotaType == "" {
			continue
		}
		used := clampFloat(mapping.window.usedPercent, 0, 100)
		remaining := 100 - used
		var resetAt *time.Time
		if mapping.window.resetSeconds != nil {
			seconds := *mapping.window.resetSeconds
			if seconds < 0 {
				seconds = 0
			}
			value := now.Add(time.Duration(seconds) * time.Second)
			resetAt = &value
		}
		signals = append(signals, contract.QuotaSignal{
			QuotaType:      quotaType,
			Remaining:      formatPercentQuotaValue(remaining),
			Used:           formatPercentQuotaValue(used),
			QuotaLimit:     "100",
			RemainingRatio: float32(remaining / 100),
			ResetAt:        resetAt,
			SnapshotAt:     now,
			Metadata:       cloneMap(metadata),
		})
	}
	return signals
}

func codexQuotaMetadataFromHeaders(headers http.Header, now time.Time) map[string]any {
	metadata := map[string]any{}
	if value, ok := parseHeaderFloat(headers, "x-codex-primary-used-percent"); ok {
		metadata["codex_primary_used_percent"] = value
	}
	if value, ok := parseHeaderInt(headers, "x-codex-primary-reset-after-seconds"); ok {
		metadata["codex_primary_reset_after_seconds"] = value
	}
	if value, ok := parseHeaderInt(headers, "x-codex-primary-window-minutes"); ok {
		metadata["codex_primary_window_minutes"] = value
	}
	if value, ok := parseHeaderFloat(headers, "x-codex-secondary-used-percent"); ok {
		metadata["codex_secondary_used_percent"] = value
	}
	if value, ok := parseHeaderInt(headers, "x-codex-secondary-reset-after-seconds"); ok {
		metadata["codex_secondary_reset_after_seconds"] = value
	}
	if value, ok := parseHeaderInt(headers, "x-codex-secondary-window-minutes"); ok {
		metadata["codex_secondary_window_minutes"] = value
	}
	if value, ok := parseHeaderFloat(headers, "x-codex-primary-over-secondary-limit-percent"); ok {
		metadata["codex_primary_over_secondary_percent"] = value
	}
	if len(metadata) == 0 {
		return nil
	}
	metadata["codex_usage_updated_at"] = now.UTC().Format(time.RFC3339)
	return metadata
}

func codexQuotaHeaderWindowFromHeaders(headers http.Header, prefix string) codexQuotaHeaderWindow {
	window := codexQuotaHeaderWindow{usedPercent: math.NaN()}
	if value, ok := parseHeaderFloat(headers, prefix+"-used-percent"); ok {
		window.usedPercent = value
	}
	if value, ok := parseHeaderInt(headers, prefix+"-reset-after-seconds"); ok {
		window.resetSeconds = &value
	}
	if value, ok := parseHeaderInt(headers, prefix+"-window-minutes"); ok {
		window.windowMin = value
	}
	return window
}

func codexQuotaHeaderWindowValid(window codexQuotaHeaderWindow) bool {
	return !math.IsNaN(window.usedPercent)
}

func codexQuotaHeaderMappings(primary codexQuotaHeaderWindow, secondary codexQuotaHeaderWindow) []codexQuotaHeaderMapping {
	primaryValid := codexQuotaHeaderWindowValid(primary)
	secondaryValid := codexQuotaHeaderWindowValid(secondary)
	if !primaryValid && !secondaryValid {
		return nil
	}
	if primary.windowMin > 0 && secondary.windowMin > 0 && primary.windowMin != secondary.windowMin {
		if primary.windowMin < secondary.windowMin {
			return []codexQuotaHeaderMapping{{kind: codexQuotaWindow5h, window: primary}, {kind: codexQuotaWindow7d, window: secondary}}
		}
		return []codexQuotaHeaderMapping{{kind: codexQuotaWindow7d, window: primary}, {kind: codexQuotaWindow5h, window: secondary}}
	}
	if primary.windowMin > 0 {
		primaryKind := codexQuotaKindForWindowMinutes(primary.windowMin)
		secondaryKind := codexQuotaOppositeKind(primaryKind)
		return []codexQuotaHeaderMapping{{kind: primaryKind, window: primary}, {kind: secondaryKind, window: secondary}}
	}
	if secondary.windowMin > 0 {
		secondaryKind := codexQuotaKindForWindowMinutes(secondary.windowMin)
		primaryKind := codexQuotaOppositeKind(secondaryKind)
		return []codexQuotaHeaderMapping{{kind: primaryKind, window: primary}, {kind: secondaryKind, window: secondary}}
	}
	// Older Codex responses omitted window minutes; sub2api treats primary as
	// the long window and secondary as the short 5h window in that legacy shape.
	return []codexQuotaHeaderMapping{{kind: codexQuotaWindow7d, window: primary}, {kind: codexQuotaWindow5h, window: secondary}}
}

func codexQuotaKindForWindowMinutes(minutes int) codexQuotaWindowKind {
	if minutes <= 360 {
		return codexQuotaWindow5h
	}
	return codexQuotaWindow7d
}

func codexQuotaOppositeKind(kind codexQuotaWindowKind) codexQuotaWindowKind {
	if kind == codexQuotaWindow5h {
		return codexQuotaWindow7d
	}
	return codexQuotaWindow5h
}

func codexQuotaTypeForWindow(kind codexQuotaWindowKind) string {
	switch kind {
	case codexQuotaWindow5h:
		return "codex_5h_percent"
	case codexQuotaWindow7d:
		return "codex_7d_percent"
	default:
		return ""
	}
}

func withCodexQuotaSignals(resp contract.ConversationResponse, headers http.Header) contract.ConversationResponse {
	resp.QuotaSignals = codexQuotaSignalsFromHeaders(headers, time.Now())
	return resp
}

func withCodexInputItemsQuotaSignals(resp contract.ResponseInputItemsResponse, headers http.Header) contract.ResponseInputItemsResponse {
	resp.QuotaSignals = codexQuotaSignalsFromHeaders(headers, time.Now())
	return resp
}

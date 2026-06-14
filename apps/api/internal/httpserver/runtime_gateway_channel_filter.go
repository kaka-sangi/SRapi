package httpserver

import (
	"context"
	"strings"

	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func (rt *runtimeState) filterCandidatesByEnabledChannels(ctx context.Context, candidates []schedulercontract.Candidate) []schedulercontract.Candidate {
	if len(candidates) == 0 || rt.adminControl == nil {
		return candidates
	}
	settings, err := rt.adminControl.GetAdminSettings(ctx)
	if err != nil {
		// Fail OPEN, not closed: a transient settings-read failure must not drop
		// every candidate and take the whole gateway down with "no available
		// account". Skip channel filtering for this request (same effect as no
		// channel restriction configured) and let it route normally.
		if rt.logger != nil {
			rt.logger.Warn("channel filter skipped: admin settings unavailable", "error", err)
		}
		return candidates
	}
	enabled := enabledChannelSet(settings.Features.EnabledChannels)
	if len(enabled) == 0 {
		return candidates
	}
	out := make([]schedulercontract.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		protocol := strings.ToLower(strings.TrimSpace(candidate.Provider.Protocol))
		if enabled[protocol] {
			out = append(out, candidate)
		}
	}
	return out
}

func enabledChannelSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		out[normalized] = true
	}
	return out
}

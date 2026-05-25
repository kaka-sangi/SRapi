package httpserver

import (
	"context"
	"strings"
	"time"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func (rt *runtimeState) applyGatewayQualityScores(ctx context.Context, candidates []schedulercontract.Candidate, model string) []schedulercontract.Candidate {
	if rt.qualityEval == nil || len(candidates) == 0 || strings.TrimSpace(model) == "" {
		return candidates
	}
	out := make([]schedulercontract.Candidate, len(candidates))
	copy(out, candidates)
	for idx := range out {
		aggregate, err := rt.qualityEval.AggregateScore(ctx, out[idx].Account.ID, model)
		if err != nil || aggregate.SampleCount == 0 {
			continue
		}
		out[idx].Mapping.PricingOverride = qualityPricingOverride(out[idx].Mapping.PricingOverride, aggregate)
	}
	return out
}

func qualityPricingOverride(existing map[string]any, aggregate qualitycontract.AggregateScore) map[string]any {
	out := cloneAnyMap(existing)
	if out == nil {
		out = map[string]any{}
	}
	out["quality_score"] = aggregate.Score
	out["quality_eval_score"] = aggregate.Score
	out["quality_eval_samples"] = aggregate.SampleCount
	out["quality_tier"] = qualityTier(aggregate.Score)
	if !aggregate.UpdatedAt.IsZero() {
		out["quality_eval_updated_at"] = aggregate.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func qualityTier(score float64) string {
	switch {
	case score >= 0.80:
		return "premium"
	case score >= 0.50:
		return "standard"
	default:
		return "basic"
	}
}

func (rt *runtimeState) captureGatewayQualitySample(ctx context.Context, rec gatewayUsageRecord, prompt string, output string) {
	if rt.qualityEval == nil || !rt.cfg.QualityEval.Enabled || rec.FeedbackID <= 0 || rec.DecisionID <= 0 || rec.AccountID == nil || rec.ProviderID == nil || !rec.Success {
		return
	}
	_, _, err := rt.qualityEval.CaptureSample(ctx, qualitycontract.CaptureSampleRequest{
		FeedbackID:      rec.FeedbackID,
		RequestID:       rec.RequestID,
		DecisionID:      rec.DecisionID,
		AttemptNo:       rec.AttemptNo,
		AccountID:       *rec.AccountID,
		ProviderID:      *rec.ProviderID,
		Model:           fallbackModelName(rec.Model),
		SourceEndpoint:  rec.SourceEndpoint,
		SanitizedPrompt: gatewayQualityPrompt(prompt),
		SanitizedOutput: output,
		CapturedAt:      time.Now().UTC(),
	})
	if err != nil {
		rt.logger.Warn("failed to capture quality evaluation sample", "error", err, "request_id", rec.RequestID)
	}
}

func gatewayQualityPrompt(prompt string) string {
	return strings.TrimSpace(prompt)
}

func gatewayTextForQuality(req gatewaycontract.CanonicalRequest) string {
	return gatewayRequestText(req)
}

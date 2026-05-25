package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// SimulateStrategy evaluates current and shadow strategies without writing decisions or leases.
func (s *Service) SimulateStrategy(ctx context.Context, req contract.StrategySimulationRequest) (contract.StrategySimulationResult, error) {
	scheduleReq := req.Request
	if err := validateScheduleRequest(scheduleReq); err != nil {
		return contract.StrategySimulationResult{}, ErrInvalidInput
	}
	if err := normalizeScheduleCapabilities(&scheduleReq); err != nil {
		return contract.StrategySimulationResult{}, ErrInvalidInput
	}
	if err := s.RefreshStrategies(ctx); err != nil {
		return contract.StrategySimulationResult{}, err
	}

	currentStrategy := req.CurrentStrategy
	if currentStrategy == "" {
		currentStrategy = scheduleReq.Strategy
	}
	if currentStrategy == "" {
		currentStrategy = contract.StrategyBalanced
	}
	shadowStrategy := req.ShadowStrategy
	if shadowStrategy == "" {
		return contract.StrategySimulationResult{}, ErrInvalidInput
	}
	rollout, err := simulationRollout(req, scheduleReq)
	if err != nil {
		return contract.StrategySimulationResult{}, err
	}

	current, err := s.simulateSingleStrategy(scheduleReq, currentStrategy)
	if err != nil {
		return contract.StrategySimulationResult{}, err
	}
	shadow, err := s.simulateSingleStrategy(scheduleReq, shadowStrategy)
	if err != nil {
		return contract.StrategySimulationResult{}, err
	}
	return contract.StrategySimulationResult{
		Current: current,
		Shadow:  shadow,
		Diff:    simulationDiff(current.Decision, shadow.Decision),
		Rollout: rollout,
		DryRun:  true,
	}, nil
}

func (s *Service) simulateSingleStrategy(req contract.ScheduleRequest, strategyName contract.StrategyName) (contract.SimulatedStrategyDecision, error) {
	strategy, err := s.registry.Resolve(strategyName)
	if err != nil {
		return contract.SimulatedStrategyDecision{}, err
	}
	req.Strategy = strategy.Name
	evaluation := s.evaluateSchedule(req, strategy)
	return contract.SimulatedStrategyDecision{
		Decision: evaluation.decision,
		Error:    simulationError(evaluation.err),
	}, nil
}

func simulationError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNoAvailableAccount):
		return "no_available_account"
	case errors.Is(err, ErrUserBalanceInsufficient):
		return "user_balance_insufficient"
	default:
		return "scheduler_error"
	}
}

// ReplayStrategies evaluates persisted request snapshots without writing decisions or acquiring leases.
func (s *Service) ReplayStrategies(ctx context.Context, req contract.StrategyReplayRequest) (contract.StrategyReplayResult, error) {
	if req.ShadowStrategy == "" {
		return contract.StrategyReplayResult{}, ErrInvalidInput
	}
	if req.Limit < 0 {
		return contract.StrategyReplayResult{}, ErrInvalidInput
	}
	if req.Since != nil && req.Until != nil && req.Since.After(*req.Until) {
		return contract.StrategyReplayResult{}, ErrInvalidInput
	}
	if err := s.RefreshStrategies(ctx); err != nil {
		return contract.StrategyReplayResult{}, err
	}
	snapshots, err := s.store.ListRequestSnapshots(ctx)
	if err != nil {
		return contract.StrategyReplayResult{}, err
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt) })
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	items := make([]contract.StrategyReplayItem, 0, limit)
	skipped := 0
	for _, snapshot := range snapshots {
		if len(items) >= limit {
			break
		}
		if !snapshotMatchesReplayRequest(snapshot, req) {
			continue
		}
		item, ok, err := s.replaySnapshot(ctx, snapshot, req)
		if err != nil {
			return contract.StrategyReplayResult{}, err
		}
		if !ok {
			skipped++
			continue
		}
		items = append(items, item)
	}
	return summarizeReplay(len(items)+skipped, skipped, items), nil
}

func (s *Service) replaySnapshot(ctx context.Context, snapshot contract.RequestSnapshot, req contract.StrategyReplayRequest) (contract.StrategyReplayItem, bool, error) {
	scheduleReq, ok := scheduleRequestFromSnapshot(snapshot)
	if !ok {
		return contract.StrategyReplayItem{}, false, nil
	}
	currentStrategy := req.CurrentStrategy
	if currentStrategy == "" {
		currentStrategy = snapshot.Strategy
	}
	if currentStrategy == "" {
		currentStrategy = scheduleReq.Strategy
	}
	if currentStrategy == "" {
		currentStrategy = contract.StrategyBalanced
	}
	simulationReq := contract.StrategySimulationRequest{
		Request:              scheduleReq,
		CurrentStrategy:      currentStrategy,
		ShadowStrategy:       req.ShadowStrategy,
		ShadowRolloutPercent: req.ShadowRolloutPercent,
		RolloutKey:           snapshot.RequestID,
	}
	result, err := s.SimulateStrategy(ctx, simulationReq)
	if err != nil {
		return contract.StrategyReplayItem{}, false, err
	}
	return contract.StrategyReplayItem{
		SnapshotID:                snapshot.ID,
		DecisionID:                snapshot.DecisionID,
		RequestID:                 snapshot.RequestID,
		AttemptNo:                 snapshot.AttemptNo,
		CreatedAt:                 snapshot.CreatedAt,
		OriginalStrategy:          snapshot.Strategy,
		OriginalSelectedAccountID: cloneIntPtr(snapshot.SelectedAccountID),
		Current:                   result.Current,
		Shadow:                    result.Shadow,
		Diff:                      result.Diff,
		Rollout:                   result.Rollout,
	}, true, nil
}

func simulationDiff(current contract.Decision, shadow contract.Decision) contract.StrategySimulationDiff {
	currentScore := selectedScore(current)
	shadowScore := selectedScore(shadow)
	return contract.StrategySimulationDiff{
		WinnerChanged:             intPtrValue(current.SelectedAccountID) != intPtrValue(shadow.SelectedAccountID),
		CurrentSelectedAccountID:  cloneIntPtr(current.SelectedAccountID),
		ShadowSelectedAccountID:   cloneIntPtr(shadow.SelectedAccountID),
		CurrentSelectedProviderID: cloneIntPtr(current.SelectedProviderID),
		ShadowSelectedProviderID:  cloneIntPtr(shadow.SelectedProviderID),
		FinalScoreDelta:           shadowScore.Final - currentScore.Final,
		CostScoreDelta:            shadowScore.Cost - currentScore.Cost,
		LatencyScoreDelta:         shadowScore.Latency - currentScore.Latency,
		QualityScoreDelta:         shadowScore.Quality - currentScore.Quality,
		RiskPenaltyDelta:          shadowScore.RiskPenalty - currentScore.RiskPenalty,
	}
}

func snapshotMatchesReplayRequest(snapshot contract.RequestSnapshot, req contract.StrategyReplayRequest) bool {
	if req.Since != nil && snapshot.CreatedAt.Before(*req.Since) {
		return false
	}
	if req.Until != nil && snapshot.CreatedAt.After(*req.Until) {
		return false
	}
	if requestID := strings.TrimSpace(req.RequestID); requestID != "" && snapshot.RequestID != requestID {
		return false
	}
	if model := strings.TrimSpace(req.Model); model != "" && snapshotString(snapshot.RequestProfile, "model") != model {
		return false
	}
	return true
}

func scheduleRequestFromSnapshot(snapshot contract.RequestSnapshot) (contract.ScheduleRequest, bool) {
	profile := snapshot.RequestProfile
	if len(profile) == 0 || len(snapshot.CandidateSnapshot) == 0 {
		return contract.ScheduleRequest{}, false
	}
	req := contract.ScheduleRequest{
		RequestID:               defaultString(snapshotString(profile, "request_id"), snapshot.RequestID),
		AttemptNo:               snapshotInt(profile, "attempt_no", snapshot.AttemptNo),
		FallbackFromDecisionID:  snapshotOptionalInt(profile, "fallback_from_decision_id"),
		UserID:                  snapshotInt(profile, "user_id", 0),
		APIKeyID:                snapshotInt(profile, "api_key_id", 0),
		SourceProtocol:          snapshotString(profile, "source_protocol"),
		SourceEndpoint:          snapshotString(profile, "source_endpoint"),
		TargetProtocol:          snapshotString(profile, "target_protocol"),
		Model:                   snapshotString(profile, "model"),
		ModelAlias:              snapshotString(profile, "model_alias"),
		FallbackModels:          snapshotStrings(profile, "fallback_models"),
		SessionAffinitySource:   snapshotString(profile, "session_affinity_source"),
		AccountGroupScope:       snapshotInts(profile, "account_group_scope"),
		UserTier:                contract.UserTier(snapshotString(profile, "user_tier")),
		UserBalanceInsufficient: snapshotBool(profile, "user_balance_insufficient"),
		EstimatedInputTokens:    snapshotInt(profile, "estimated_input_tokens", 0),
		EstimatedOutputTokens:   snapshotInt(profile, "estimated_output_tokens", 0),
		EstimatedCost:           snapshotString(profile, "estimated_cost"),
		Currency:                snapshotString(profile, "currency"),
		PricingRuleID:           snapshotOptionalInt(profile, "pricing_rule_id"),
		PricingSource:           snapshotString(profile, "pricing_source"),
		PricingEstimated:        snapshotBool(profile, "pricing_estimated"),
		IsStream:                snapshotBool(profile, "is_stream"),
		StickyAccountID:         snapshotOptionalInt(profile, "sticky_account_id"),
		StickyStrength:          contract.StickyStrength(snapshotString(profile, "sticky_strength")),
		Strategy:                contract.StrategyName(defaultString(snapshotString(profile, "strategy"), string(snapshot.Strategy))),
		StrategyRollout:         snapshotStrategyRollout(profile),
		Warnings:                snapshotStrings(profile, "warnings"),
		RequestCapabilities:     snapshotCapabilities(profile, "request_capabilities"),
		ExcludedAccountIDs:      snapshotInts(profile, "excluded_account_ids"),
		Candidates:              candidatesFromSnapshots(snapshot.CandidateSnapshot),
	}
	if req.RequestID == "" || req.UserID <= 0 || req.APIKeyID <= 0 || strings.TrimSpace(req.SourceEndpoint) == "" || strings.TrimSpace(req.Model) == "" {
		return contract.ScheduleRequest{}, false
	}
	return req, true
}

func candidatesFromSnapshots(values []contract.CandidateSnapshot) []contract.Candidate {
	out := make([]contract.Candidate, 0, len(values))
	for _, value := range values {
		credential := "snapshot-replay-credential"
		if value.AccountHasCredential != nil && !*value.AccountHasCredential {
			credential = ""
		}
		out = append(out, contract.Candidate{
			Account: accountcontract.ProviderAccount{
				ID:                   value.AccountID,
				ProviderID:           value.ProviderID,
				RuntimeClass:         accountcontract.RuntimeClass(defaultString(value.RuntimeClass, string(accountcontract.RuntimeClassAPIKey))),
				CredentialCiphertext: credential,
				Status:               accountcontract.Status(defaultString(value.AccountStatus, string(accountcontract.StatusActive))),
				Weight:               replayAccountWeight(value.AccountWeight),
				RiskLevel:            cloneStringPtr(value.AccountRiskLevel),
				Metadata:             cloneMapAny(value.AccountMetadata),
			},
			Provider: providercontract.Provider{
				ID:           value.ProviderID,
				Protocol:     defaultString(value.ProviderProtocol, "openai-compatible"),
				Status:       providercontract.Status(defaultString(value.ProviderStatus, string(providercontract.StatusActive))),
				Capabilities: cloneMapAny(value.ProviderCapabilities),
				ConfigSchema: cloneMapAny(value.ProviderConfig),
			},
			Mapping: modelcontract.ModelProviderMapping{
				ID:                value.MappingID,
				ModelID:           value.ModelID,
				ProviderID:        value.ProviderID,
				UpstreamModelName: strings.TrimSpace(value.UpstreamModelName),
				Status:            modelcontract.Status(defaultString(value.MappingStatus, string(modelcontract.StatusActive))),
				PricingOverride:   cloneMapAny(value.PricingOverride),
			},
			EffectiveCapabilities: cloneCapabilityDescriptors(value.EffectiveCapabilities),
			RuntimeState:          value.RuntimeState,
			Limits:                cloneRuntimeLimits(value.Limits),
		})
	}
	return out
}

func replayAccountWeight(value float32) float32 {
	if value <= 0 {
		return 1
	}
	return value
}

func summarizeReplay(requested int, skipped int, items []contract.StrategyReplayItem) contract.StrategyReplayResult {
	result := contract.StrategyReplayResult{
		DryRun:           true,
		Requested:        requested,
		Replayed:         len(items),
		Skipped:          skipped,
		CurrentWinCounts: map[string]int{},
		ShadowWinCounts:  map[string]int{},
		Items:            items,
	}
	if len(items) == 0 {
		return result
	}
	for _, item := range items {
		if item.Diff.WinnerChanged {
			result.WinnerChanged++
		}
		result.CurrentWinCounts[replayWinnerKey(item.Current.Decision.SelectedAccountID)]++
		result.ShadowWinCounts[replayWinnerKey(item.Shadow.Decision.SelectedAccountID)]++
		result.AverageFinalScoreDelta += item.Diff.FinalScoreDelta
		result.AverageCostScoreDelta += item.Diff.CostScoreDelta
		result.AverageLatencyScoreDelta += item.Diff.LatencyScoreDelta
		result.AverageQualityScoreDelta += item.Diff.QualityScoreDelta
		result.AverageRiskPenaltyDelta += item.Diff.RiskPenaltyDelta
	}
	count := float64(len(items))
	result.AverageFinalScoreDelta /= count
	result.AverageCostScoreDelta /= count
	result.AverageLatencyScoreDelta /= count
	result.AverageQualityScoreDelta /= count
	result.AverageRiskPenaltyDelta /= count
	return result
}

func replayWinnerKey(value *int) string {
	if value == nil {
		return "none"
	}
	return strconv.Itoa(*value)
}

func snapshotString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmtAny(typed))
	}
}

func snapshotBool(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func snapshotInt(values map[string]any, key string, fallback int) int {
	value, ok := values[key]
	if !ok || value == nil {
		return fallback
	}
	if parsed, ok := anyToInt(value); ok {
		return parsed
	}
	return fallback
}

func snapshotOptionalInt(values map[string]any, key string) *int {
	value, ok := values[key]
	if !ok || value == nil {
		return nil
	}
	parsed, ok := anyToInt(value)
	if !ok {
		return nil
	}
	return &parsed
}

func snapshotInts(values map[string]any, key string) []int {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []int:
		return cloneInts(typed)
	case []any:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			if parsed, ok := anyToInt(item); ok {
				out = append(out, parsed)
			}
		}
		return out
	default:
		return nil
	}
}

func snapshotStrings(values map[string]any, key string) []string {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return cloneStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value := strings.TrimSpace(fmtAny(item))
			if value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func snapshotCapabilities(values map[string]any, key string) []capabilitiescontract.Descriptor {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var descriptors []capabilitiescontract.Descriptor
	if err := json.Unmarshal(encoded, &descriptors); err != nil {
		return nil
	}
	return descriptors
}

func snapshotStrategyRollout(values map[string]any) contract.StrategyRollout {
	raw, ok := values["routing_hints"].(map[string]any)
	if !ok || raw == nil {
		return contract.StrategyRollout{}
	}
	rawRollout, ok := raw["strategy_rollout"].(map[string]any)
	if !ok || rawRollout == nil {
		return contract.StrategyRollout{}
	}
	return contract.StrategyRollout{
		Enabled:        true,
		ShadowStrategy: contract.StrategyName(snapshotString(rawRollout, "shadow_strategy")),
		Percent:        snapshotFloat(rawRollout, "percent"),
		Bucket:         snapshotFloat(rawRollout, "bucket"),
		ShadowSelected: snapshotBool(rawRollout, "shadow_selected"),
		KeyHash:        snapshotString(rawRollout, "rollout_key_hash"),
	}
}

func snapshotFloat(values map[string]any, key string) float64 {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func anyToInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), typed == math.Trunc(typed)
	case float32:
		return int(typed), typed == float32(math.Trunc(float64(typed)))
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func fmtAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func selectedScore(decision contract.Decision) scoreBreakdown {
	if decision.SelectedAccountID == nil {
		return scoreBreakdown{}
	}
	raw, ok := decision.Scores[accountKey(*decision.SelectedAccountID)]
	if !ok {
		return scoreBreakdown{}
	}
	score, ok := raw.(scoreBreakdown)
	if ok {
		return score
	}
	return scoreBreakdown{}
}

func intPtrValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func simulationRollout(req contract.StrategySimulationRequest, scheduleReq contract.ScheduleRequest) (contract.StrategySimulationRollout, error) {
	if req.ShadowRolloutPercent == nil {
		return contract.StrategySimulationRollout{}, nil
	}
	percent := *req.ShadowRolloutPercent
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 100 {
		return contract.StrategySimulationRollout{}, ErrInvalidInput
	}
	key := strings.TrimSpace(req.RolloutKey)
	if key == "" {
		key = strings.TrimSpace(scheduleReq.RequestID)
	}
	bucket := rolloutBucket(key)
	return contract.StrategySimulationRollout{
		Enabled:        true,
		Percent:        percent,
		Bucket:         bucket,
		ShadowSelected: bucket < percent || percent >= 100,
		KeyHash:        affinityKeyHash(key),
	}, nil
}

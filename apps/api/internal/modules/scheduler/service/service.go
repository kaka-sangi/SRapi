package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store    contract.Store
	clock    Clock
	registry *StrategyRegistry
}

type StrategyRegistry struct {
	descriptors map[contract.StrategyName]contract.StrategyDescriptor
}

func NewStrategyRegistry() *StrategyRegistry {
	descriptors := map[contract.StrategyName]contract.StrategyDescriptor{}
	for _, descriptor := range []contract.StrategyDescriptor{
		newStrategyDescriptor(contract.StrategyBalanced, "v1", "Balanced default scheduler strategy.", map[string]float64{
			"health":   0.30,
			"quota":    0.20,
			"latency":  0.15,
			"sticky":   0.10,
			"cache":    0.10,
			"cost":     0.10,
			"fairness": 0.05,
			"priority": 0.00,
		}),
		newStrategyDescriptor(contract.StrategyCostSaver, "v1", "Cost-saving scheduler strategy.", map[string]float64{
			"cost":     0.30,
			"quota":    0.20,
			"cache":    0.15,
			"health":   0.15,
			"fairness": 0.10,
			"latency":  0.05,
			"sticky":   0.05,
			"priority": 0.00,
		}),
	} {
		descriptors[descriptor.Name] = descriptor
	}
	return &StrategyRegistry{descriptors: descriptors}
}

func (r *StrategyRegistry) Resolve(name contract.StrategyName) (contract.StrategyDescriptor, error) {
	if name == "" {
		name = contract.StrategyBalanced
	}
	descriptor, ok := r.descriptors[name]
	if !ok || descriptor.Status != "active" {
		return contract.StrategyDescriptor{}, ErrInvalidInput
	}
	return cloneStrategyDescriptor(descriptor), nil
}

func (r *StrategyRegistry) List() []contract.StrategyDescriptor {
	out := make([]contract.StrategyDescriptor, 0, len(r.descriptors))
	for _, descriptor := range r.descriptors {
		out = append(out, cloneStrategyDescriptor(descriptor))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func newStrategyDescriptor(name contract.StrategyName, version string, description string, weights map[string]float64) contract.StrategyDescriptor {
	config := map[string]any{
		"weights":           weightsPayload(weights),
		"hard_rules":        []string{"account_disabled", "credential_invalid", "quota_exhausted", "rpm_limit_exceeded", "tpm_limit_exceeded", "concurrency_full", "circuit_open", "cooldown_active", "capability_mismatch"},
		"fallback_rules":    map[string]any{"max_attempts": 1},
		"randomization":     map[string]any{"top_n": 1, "seeded_tests": true},
		"risk_controls":     []string{"runtime_class", "account_status", "circuit_breaker"},
		"observability":     []string{"decision", "feedback", "usage"},
		"strategy_registry": "seed",
	}
	return contract.StrategyDescriptor{
		Name:        name,
		Version:     version,
		Status:      "active",
		Config:      config,
		ConfigHash:  configHash(config),
		Weights:     cloneWeights(weights),
		Description: description,
	}
}

func configHash(config map[string]any) string {
	raw, err := json.Marshal(config)
	if err != nil {
		return "sha256:invalid"
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneStrategyDescriptor(value contract.StrategyDescriptor) contract.StrategyDescriptor {
	value.Config = cloneMapAny(value.Config)
	value.Weights = cloneWeights(value.Weights)
	return value
}

func cloneWeights(values map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneMapAny(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func weightsPayload(weights map[string]float64) map[string]any {
	out := make(map[string]any, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock, registry: NewStrategyRegistry()}, nil
}

func (s *Service) Schedule(ctx context.Context, req contract.ScheduleRequest) (contract.ScheduleResult, error) {
	requestID := strings.TrimSpace(req.RequestID)
	model := strings.TrimSpace(req.Model)
	sourceEndpoint := strings.TrimSpace(req.SourceEndpoint)
	if requestID == "" || req.UserID <= 0 || req.APIKeyID <= 0 || model == "" || sourceEndpoint == "" {
		return contract.ScheduleResult{}, ErrInvalidInput
	}
	if err := normalizeScheduleCapabilities(&req); err != nil {
		return contract.ScheduleResult{}, ErrInvalidInput
	}

	strategy, err := s.registry.Resolve(req.Strategy)
	if err != nil {
		return contract.ScheduleResult{}, err
	}
	if req.UserBalanceInsufficient {
		rejectReasons := rejectAllCandidates(req.Candidates, "user_balance_insufficient")
		decision, err := s.store.CreateDecision(ctx, s.buildDecision(req, strategy, nil, len(req.Candidates), len(rejectReasons), nil, rejectReasons))
		if err != nil {
			return contract.ScheduleResult{}, err
		}
		return contract.ScheduleResult{Decision: decision}, ErrUserBalanceInsufficient
	}

	scores := make([]candidateScore, 0, len(req.Candidates))
	rejectReasons := map[string]any{}
	for _, candidate := range req.Candidates {
		reason := rejectReason(candidate, req)
		if reason != "" {
			rejectReasons[accountKey(candidate.Account.ID)] = reason
			continue
		}
		scores = append(scores, candidateScore{
			Candidate: candidate,
			Score:     scoreCandidate(candidate, req, strategy),
		})
	}
	addStickyBrokenReason(rejectReasons, req)
	if len(scores) == 0 {
		decision, err := s.store.CreateDecision(ctx, s.buildDecision(req, strategy, nil, len(req.Candidates), len(rejectReasons), nil, rejectReasons))
		if err != nil {
			return contract.ScheduleResult{}, err
		}
		return contract.ScheduleResult{Decision: decision}, ErrNoAvailableAccount
	}

	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score.Final == scores[j].Score.Final {
			return scores[i].Candidate.Account.ID < scores[j].Candidate.Account.ID
		}
		return scores[i].Score.Final > scores[j].Score.Final
	})
	selected := scores[0].Candidate
	scorePayload := map[string]any{}
	for _, score := range scores {
		scorePayload[accountKey(score.Candidate.Account.ID)] = score.Score
	}
	lease, err := s.acquireLease(ctx, req, 1, selected)
	if err != nil {
		rejectReasons[accountKey(selected.Account.ID)] = "concurrency_full"
		decision, decisionErr := s.store.CreateDecision(ctx, s.buildDecision(req, strategy, nil, len(req.Candidates), len(rejectReasons), scorePayload, rejectReasons))
		if decisionErr != nil {
			return contract.ScheduleResult{}, decisionErr
		}
		return contract.ScheduleResult{Decision: decision}, ErrNoAvailableAccount
	}
	decision, err := s.store.CreateDecision(ctx, s.buildDecision(req, strategy, &selected, len(req.Candidates), len(rejectReasons), scorePayload, rejectReasons))
	if err != nil {
		_, _ = s.store.UpdateLeaseStatus(ctx, requestID, contract.LeaseStatusReleased)
		return contract.ScheduleResult{}, err
	}
	return contract.ScheduleResult{Decision: decision, Candidate: selected, Lease: lease}, nil
}

func normalizeScheduleCapabilities(req *contract.ScheduleRequest) error {
	requestCapabilities, err := capabilitiescontract.NormalizeDescriptors(req.RequestCapabilities)
	if err != nil {
		return err
	}
	req.RequestCapabilities = requestCapabilities
	for idx := range req.Candidates {
		effectiveCapabilities, err := capabilitiescontract.NormalizeDescriptors(req.Candidates[idx].EffectiveCapabilities)
		if err != nil {
			return err
		}
		req.Candidates[idx].EffectiveCapabilities = effectiveCapabilities
	}
	return nil
}

func (s *Service) ListDecisions(ctx context.Context) ([]contract.Decision, error) {
	return s.store.ListDecisions(ctx)
}

func (s *Service) RecordFeedback(ctx context.Context, req contract.RecordFeedbackRequest) (contract.Feedback, error) {
	if strings.TrimSpace(req.RequestID) == "" || req.DecisionID <= 0 || req.AttemptNo <= 0 || req.AccountID <= 0 || req.ProviderID <= 0 {
		return contract.Feedback{}, ErrInvalidInput
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "USD"
	}
	actualCost := strings.TrimSpace(req.ActualCost)
	if actualCost == "" {
		actualCost = "0.00000000"
	}
	feedback, err := s.store.CreateFeedback(ctx, contract.Feedback{
		RequestID:    strings.TrimSpace(req.RequestID),
		DecisionID:   req.DecisionID,
		AttemptNo:    req.AttemptNo,
		AccountID:    req.AccountID,
		ProviderID:   req.ProviderID,
		Model:        strings.TrimSpace(req.Model),
		Success:      req.Success,
		ErrorClass:   req.ErrorClass,
		StatusCode:   req.StatusCode,
		LatencyMS:    req.LatencyMS,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		CachedTokens: req.CachedTokens,
		ActualCost:   actualCost,
		Currency:     currency,
		CreatedAt:    s.clock.Now(),
	})
	if err != nil {
		return contract.Feedback{}, err
	}
	status := contract.LeaseStatusCommitted
	if !req.Success {
		status = contract.LeaseStatusFailed
	}
	if _, err := s.store.UpdateLeaseStatus(ctx, strings.TrimSpace(req.RequestID), status); err != nil {
		return feedback, nil
	}
	return feedback, nil
}

func (s *Service) ListFeedbacks(ctx context.Context) ([]contract.Feedback, error) {
	return s.store.ListFeedbacks(ctx)
}

func (s *Service) ListStrategies() []contract.StrategyDescriptor {
	return s.registry.List()
}

func (s *Service) ListLeases(ctx context.Context) ([]contract.Lease, error) {
	return s.store.ListLeases(ctx)
}

func (s *Service) acquireLease(ctx context.Context, req contract.ScheduleRequest, attemptNo int, selected contract.Candidate) (contract.Lease, error) {
	ttl := req.LeaseTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := s.clock.Now()
	return s.store.AcquireLease(ctx, contract.Lease{
		ID:        fmt.Sprintf("lease_%s_%d_%d", strings.TrimSpace(req.RequestID), attemptNo, selected.Account.ID),
		RequestID: strings.TrimSpace(req.RequestID),
		AccountID: selected.Account.ID,
		Status:    contract.LeaseStatusPending,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
		UpdatedAt: now,
	}, selected.Limits.MaxConcurrency)
}

func (s *Service) buildDecision(req contract.ScheduleRequest, strategy contract.StrategyDescriptor, selected *contract.Candidate, candidateCount int, rejectedCount int, scores map[string]any, rejectReasons map[string]any) contract.Decision {
	sourceProtocol := strings.TrimSpace(req.SourceProtocol)
	if sourceProtocol == "" {
		sourceProtocol = "openai-compatible"
	}
	targetProtocol := strings.TrimSpace(req.TargetProtocol)
	if targetProtocol == "" && selected != nil {
		targetProtocol = selected.Provider.Protocol
	}
	decision := contract.Decision{
		RequestID:             strings.TrimSpace(req.RequestID),
		AttemptNo:             1,
		UserID:                req.UserID,
		APIKeyID:              req.APIKeyID,
		SourceProtocol:        sourceProtocol,
		SourceEndpoint:        strings.TrimSpace(req.SourceEndpoint),
		TargetProtocol:        targetProtocol,
		Model:                 strings.TrimSpace(req.Model),
		Strategy:              strategy.Name,
		StrategyVersion:       strategy.Version,
		StrategyConfigHash:    strategy.ConfigHash,
		CandidateCount:        candidateCount,
		RejectedCount:         rejectedCount,
		Scores:                scores,
		RejectReasons:         rejectReasons,
		StrategyWeights:       strategyWeightsPayload(strategy),
		CompatibilityWarnings: cloneStrings(req.Warnings),
		EstimatedCost:         "0.00000000",
		Currency:              "USD",
		CreatedAt:             s.clock.Now(),
	}
	if selected != nil {
		providerID := selected.Provider.ID
		accountID := selected.Account.ID
		decision.SelectedProviderID = &providerID
		decision.SelectedAccountID = &accountID
		decision.StickyHit = stickyScore(*selected, req) > 0
	}
	return decision
}

type candidateScore struct {
	Candidate contract.Candidate
	Score     scoreBreakdown
}

type scoreBreakdown struct {
	AccountID         int     `json:"account_id"`
	Final             float64 `json:"final_score"`
	Health            float64 `json:"health_score"`
	Quota             float64 `json:"quota_score"`
	Latency           float64 `json:"latency_score"`
	Sticky            float64 `json:"sticky_score"`
	Cache             float64 `json:"cache_score"`
	Cost              float64 `json:"cost_score"`
	Fairness          float64 `json:"fairness_score"`
	RiskPenalty       float64 `json:"risk_penalty"`
	SaturationPenalty float64 `json:"saturation_penalty"`
}

func scoreCandidate(candidate contract.Candidate, req contract.ScheduleRequest, strategy contract.StrategyDescriptor) scoreBreakdown {
	weights := strategy.Weights
	health := healthScore(candidate)
	quota := quotaScore(candidate)
	latency := latencyScore(candidate)
	sticky := stickyScore(candidate, req)
	cache := 0.0
	cost := costScore(candidate)
	fairness := normalizeWeight(candidate.Account.Weight)
	riskPenalty := riskPenalty(candidate)
	saturationPenalty := saturationPenalty(candidate)
	final := health*weights["health"] + quota*weights["quota"] + latency*weights["latency"] + sticky*weights["sticky"] + cache*weights["cache"] + cost*weights["cost"] + fairness*weights["fairness"] - riskPenalty - saturationPenalty
	return scoreBreakdown{
		AccountID:         candidate.Account.ID,
		Final:             final,
		Health:            health,
		Quota:             quota,
		Latency:           latency,
		Sticky:            sticky,
		Cache:             cache,
		Cost:              cost,
		Fairness:          fairness,
		RiskPenalty:       riskPenalty,
		SaturationPenalty: saturationPenalty,
	}
}

func rejectReason(candidate contract.Candidate, req contract.ScheduleRequest) string {
	if candidate.Provider.Status != "active" {
		return "provider_disabled"
	}
	switch candidate.Account.Status {
	case accountcontract.StatusActive:
	case accountcontract.StatusNeedsReauth:
		return "needs_reauth"
	default:
		return "account_disabled"
	}
	if candidate.Mapping.Status != "active" {
		return "model_not_supported"
	}
	if candidate.Account.ProviderID != candidate.Provider.ID || candidate.Mapping.ProviderID != candidate.Provider.ID {
		return "model_not_supported"
	}
	if requestCapabilityMismatch(req.RequestCapabilities, candidate.EffectiveCapabilities) {
		return "capability_mismatch"
	}
	if strings.TrimSpace(candidate.Account.CredentialCiphertext) == "" {
		return "credential_invalid"
	}
	if candidate.RuntimeState.QuotaExhausted {
		return "quota_exhausted"
	}
	if quotaProtected(candidate, req) {
		return "quota_protected"
	}
	if candidate.RuntimeState.CircuitOpen {
		return "circuit_open"
	}
	if candidate.RuntimeState.CooldownActive {
		return "cooldown_active"
	}
	if limitReached(candidate.Limits.MaxConcurrency, candidate.RuntimeState.CurrentConcurrency) {
		return "concurrency_full"
	}
	if limitReached(candidate.Limits.RPMLimit, candidate.RuntimeState.RPMUsed) {
		return "rpm_limit_exceeded"
	}
	if limitReached(candidate.Limits.TPMLimit, candidate.RuntimeState.TPMUsed) {
		return "tpm_limit_exceeded"
	}
	return ""
}

func requestCapabilityMismatch(requested []capabilitiescontract.Descriptor, effective []capabilitiescontract.Descriptor) bool {
	if len(requested) == 0 {
		return false
	}
	effectiveByKey := map[string]capabilitiescontract.Descriptor{}
	for _, descriptor := range effective {
		if descriptor.Status == capabilitiescontract.DescriptorStatusDeprecated {
			continue
		}
		key := strings.TrimSpace(descriptor.Key)
		if key == "" {
			continue
		}
		effectiveByKey[key] = descriptor
	}
	for _, descriptor := range requested {
		if descriptor.Level != capabilitiescontract.DescriptorLevelRequired {
			continue
		}
		effectiveDescriptor, ok := effectiveByKey[strings.TrimSpace(descriptor.Key)]
		if !ok {
			return true
		}
		if effectiveDescriptor.Level == capabilitiescontract.DescriptorLevelUnsupported {
			return true
		}
	}
	return false
}

func rejectAllCandidates(candidates []contract.Candidate, reason string) map[string]any {
	rejectReasons := make(map[string]any, len(candidates))
	for _, candidate := range candidates {
		rejectReasons[accountKey(candidate.Account.ID)] = reason
	}
	return rejectReasons
}

func addStickyBrokenReason(rejectReasons map[string]any, req contract.ScheduleRequest) {
	if req.StickyAccountID == nil {
		return
	}
	reason, ok := rejectReasons[accountKey(*req.StickyAccountID)]
	if !ok {
		return
	}
	switch req.StickyStrength {
	case contract.StickyStrengthHard:
		rejectReasons["hard_sticky_unavailable"] = reason
	case contract.StickyStrengthSoft:
		rejectReasons["sticky_broken_reason"] = reason
	}
}

func healthScore(candidate contract.Candidate) float64 {
	if candidate.RuntimeState.CircuitOpen || candidate.RuntimeState.CooldownActive {
		return 0
	}
	if candidate.RuntimeState.HealthScore != nil {
		return clamp01(*candidate.RuntimeState.HealthScore)
	}
	return 0.70
}

func quotaScore(candidate contract.Candidate) float64 {
	if candidate.RuntimeState.QuotaExhausted {
		return 0
	}
	if candidate.RuntimeState.QuotaRemainingRatio == nil {
		return 1
	}
	ratio := clamp01(*candidate.RuntimeState.QuotaRemainingRatio)
	switch {
	case ratio >= 0.70:
		return 1.00
	case ratio >= 0.30:
		return 0.70
	case ratio >= 0.10:
		return 0.35
	case ratio > 0:
		return 0.10
	default:
		return 0
	}
}

func latencyScore(candidate contract.Candidate) float64 {
	if candidate.RuntimeState.LatencyP95MS == nil {
		return 0.60
	}
	return clamp01(1 - float64(*candidate.RuntimeState.LatencyP95MS)/10000)
}

func stickyScore(candidate contract.Candidate, req contract.ScheduleRequest) float64 {
	if req.StickyAccountID == nil || candidate.Account.ID != *req.StickyAccountID {
		return 0
	}
	switch req.StickyStrength {
	case contract.StickyStrengthHard:
		return 1.0
	case contract.StickyStrengthSoft:
		return 0.7
	default:
		return 0
	}
}

func quotaProtected(candidate contract.Candidate, req contract.ScheduleRequest) bool {
	if !freeTier(req.UserTier) || !protectedAccount(candidate) || candidate.RuntimeState.QuotaRemainingRatio == nil {
		return false
	}
	return *candidate.RuntimeState.QuotaRemainingRatio > 0 && *candidate.RuntimeState.QuotaRemainingRatio < 0.10
}

func freeTier(tier contract.UserTier) bool {
	return tier == "" || tier == contract.UserTierFree
}

func protectedAccount(candidate contract.Candidate) bool {
	if metadataBool(candidate.Account.Metadata, "quota_protected") || metadataBool(candidate.Account.Metadata, "protected") {
		return true
	}
	value, ok := candidate.Account.Metadata["quality_tier"].(string)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "premium", "protected":
		return true
	default:
		return false
	}
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata[key]
	if !ok {
		return false
	}
	switch value := value.(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil && parsed
	default:
		return false
	}
}

func limitReached(limit *int, used int) bool {
	return limit != nil && *limit >= 0 && used >= *limit
}

func saturationPenalty(candidate contract.Candidate) float64 {
	limit := candidate.Limits.MaxConcurrency
	if limit == nil || *limit <= 0 {
		return 0
	}
	ratio := float64(candidate.RuntimeState.CurrentConcurrency) / float64(*limit)
	if ratio <= 0.75 {
		return 0
	}
	if ratio >= 1 {
		return 0.25
	}
	return (ratio - 0.75)
}

func costScore(candidate contract.Candidate) float64 {
	if value, ok := candidate.Mapping.PricingOverride["relative_cost"]; ok {
		switch v := value.(type) {
		case float64:
			return clamp01(1 - v)
		case float32:
			return clamp01(1 - float64(v))
		case int:
			return clamp01(1 - float64(v))
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err == nil {
				return clamp01(1 - parsed)
			}
		}
	}
	return 0.6
}

func riskPenalty(candidate contract.Candidate) float64 {
	if candidate.Account.RiskLevel == nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(*candidate.Account.RiskLevel)) {
	case "high":
		return 0.15
	case "medium":
		return 0.05
	default:
		return 0
	}
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func strategyWeightsPayload(strategy contract.StrategyDescriptor) map[string]any {
	weights := strategy.Weights
	out := make(map[string]any, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}

func normalizeWeight(weight float32) float64 {
	if weight <= 0 {
		return 0
	}
	if weight >= 1 {
		return 1
	}
	return float64(weight)
}

func accountKey(id int) string {
	return strings.TrimSpace("account_" + strconv.Itoa(id))
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

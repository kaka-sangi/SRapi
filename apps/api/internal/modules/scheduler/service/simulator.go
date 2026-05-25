package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"strings"

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

func rolloutBucket(key string) float64 {
	sum := sha256Sum(key)
	value := binary.BigEndian.Uint64(sum[:8])
	return float64(value%10000) / 100
}

func sha256Sum(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

package service

import (
	"context"
	"errors"

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

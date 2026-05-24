package service

const paretoEpsilon = 0.000000001

type paretoObjective struct {
	cost       float64
	hasCost    bool
	latency    float64
	hasLatency bool
	quality    float64
	hasQuality bool
}

func paretoFrontier(scores []candidateScore) []candidateScore {
	if len(scores) <= 1 {
		return append([]candidateScore(nil), scores...)
	}
	frontier := make([]candidateScore, 0, len(scores))
	for idx, score := range scores {
		if dominatedByAny(scoreObjective(score), scores, idx) {
			continue
		}
		frontier = append(frontier, score)
	}
	if len(frontier) == 0 {
		return append([]candidateScore(nil), scores...)
	}
	return frontier
}

func dominatedByAny(candidate paretoObjective, scores []candidateScore, candidateIndex int) bool {
	for idx, score := range scores {
		if idx == candidateIndex {
			continue
		}
		if dominates(scoreObjective(score), candidate) {
			return true
		}
	}
	return false
}

func scoreObjective(score candidateScore) paretoObjective {
	return paretoObjective{
		cost:       score.Score.Cost,
		hasCost:    hasExplicitCost(score),
		latency:    score.Score.Latency,
		hasLatency: score.Candidate.RuntimeState.LatencyP95MS != nil,
		quality:    score.Score.Quality,
		hasQuality: hasExplicitQuality(score),
	}
}

func dominates(left paretoObjective, right paretoObjective) bool {
	comparable := 0
	strictlyBetter := 0
	if left.hasCost && right.hasCost {
		comparable++
		if left.cost+paretoEpsilon < right.cost {
			return false
		}
		if left.cost > right.cost+paretoEpsilon {
			strictlyBetter++
		}
	}
	if left.hasLatency && right.hasLatency {
		comparable++
		if left.latency+paretoEpsilon < right.latency {
			return false
		}
		if left.latency > right.latency+paretoEpsilon {
			strictlyBetter++
		}
	}
	if left.hasQuality && right.hasQuality {
		comparable++
		if left.quality+paretoEpsilon < right.quality {
			return false
		}
		if left.quality > right.quality+paretoEpsilon {
			strictlyBetter++
		}
	}
	return comparable >= 2 && strictlyBetter > 0
}

func paretoFrontierAccountIDs(scores []candidateScore) []int {
	out := make([]int, 0, len(scores))
	for _, score := range scores {
		out = append(out, score.Candidate.Account.ID)
	}
	return out
}

func hasExplicitCost(score candidateScore) bool {
	_, ok := metadataValue(score.Candidate.Mapping.PricingOverride, "relative_cost")
	return ok
}

func hasExplicitQuality(score candidateScore) bool {
	valueMaps := []map[string]any{score.Candidate.Mapping.PricingOverride, score.Candidate.Account.Metadata, score.Candidate.Provider.Capabilities, score.Candidate.Provider.ConfigSchema}
	for _, key := range []string{"quality_score", "quality_eval_score", "online_eval_score", "judge_score", "quality_tier", "quality"} {
		for _, metadata := range valueMaps {
			if _, ok := metadataValue(metadata, key); ok {
				return true
			}
		}
	}
	return false
}

package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
)

type Store struct {
	mu                   sync.Mutex
	nextSampleID         int
	nextEvaluationID     int
	samples              map[int]qualitycontract.Sample
	sampleByFeedback     map[int]int
	evaluations          map[int]qualitycontract.Evaluation
	evaluationByFeedback map[int]int
}

func New() *Store {
	return &Store{
		nextSampleID:         1,
		nextEvaluationID:     1,
		samples:              map[int]qualitycontract.Sample{},
		sampleByFeedback:     map[int]int{},
		evaluations:          map[int]qualitycontract.Evaluation{},
		evaluationByFeedback: map[int]int{},
	}
}

func (s *Store) CreateSample(_ context.Context, sample qualitycontract.Sample) (qualitycontract.Sample, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.sampleByFeedback[sample.FeedbackID]; ok {
		return cloneSample(s.samples[id]), false, nil
	}
	stored := cloneSample(sample)
	stored.ID = s.nextSampleID
	if stored.AttemptNo <= 0 {
		stored.AttemptNo = 1
	}
	if stored.CapturedAt.IsZero() {
		stored.CapturedAt = time.Now().UTC()
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = stored.CapturedAt
	}
	if stored.UpdatedAt.IsZero() {
		stored.UpdatedAt = stored.CreatedAt
	}
	s.samples[stored.ID] = stored
	s.sampleByFeedback[stored.FeedbackID] = stored.ID
	s.nextSampleID++
	return cloneSample(stored), true, nil
}

func (s *Store) ListPendingSamples(_ context.Context, filter qualitycontract.PendingSampleFilter) ([]qualitycontract.Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	percent := filter.SamplePercent
	if percent <= 0 {
		return nil, nil
	}
	if percent > 100 {
		percent = 100
	}
	out := make([]qualitycontract.Sample, 0)
	for _, sample := range s.samples {
		if _, ok := s.evaluationByFeedback[sample.FeedbackID]; ok {
			continue
		}
		if !sampleSelected(sample.SampleRequestHash, percent) {
			continue
		}
		out = append(out, cloneSample(sample))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CapturedAt.Before(out[j].CapturedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) CreateEvaluation(_ context.Context, evaluation qualitycontract.Evaluation) (qualitycontract.Evaluation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.evaluationByFeedback[evaluation.FeedbackID]; ok {
		return cloneEvaluation(s.evaluations[id]), false, nil
	}
	stored := cloneEvaluation(evaluation)
	stored.ID = s.nextEvaluationID
	if stored.AttemptNo <= 0 {
		stored.AttemptNo = 1
	}
	if stored.JudgedAt.IsZero() {
		stored.JudgedAt = time.Now().UTC()
	}
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = stored.JudgedAt
	}
	if stored.UpdatedAt.IsZero() {
		stored.UpdatedAt = stored.CreatedAt
	}
	s.evaluations[stored.ID] = stored
	s.evaluationByFeedback[stored.FeedbackID] = stored.ID
	s.nextEvaluationID++
	return cloneEvaluation(stored), true, nil
}

func (s *Store) AggregateScore(_ context.Context, accountID int, model string, since time.Time) (qualitycontract.AggregateScore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	model = strings.TrimSpace(model)
	var total float64
	count := 0
	var updatedAt time.Time
	for _, evaluation := range s.evaluations {
		if evaluation.AccountID != accountID || evaluation.Model != model || evaluation.JudgedAt.Before(since) {
			continue
		}
		total += evaluation.Score
		count++
		if evaluation.JudgedAt.After(updatedAt) {
			updatedAt = evaluation.JudgedAt
		}
	}
	if count == 0 {
		return qualitycontract.AggregateScore{AccountID: accountID, Model: model}, nil
	}
	return qualitycontract.AggregateScore{
		AccountID:   accountID,
		Model:       model,
		Score:       total / float64(count),
		SampleCount: count,
		UpdatedAt:   updatedAt,
	}, nil
}

func (s *Store) ListEvaluations(_ context.Context) ([]qualitycontract.Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]qualitycontract.Evaluation, 0, len(s.evaluations))
	for _, evaluation := range s.evaluations {
		out = append(out, cloneEvaluation(evaluation))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func sampleSelected(hash string, percent float64) bool {
	if percent >= 100 {
		return true
	}
	hash = strings.TrimPrefix(strings.TrimSpace(hash), "sha256:")
	if len(hash) < 4 {
		return false
	}
	value := 0
	for _, ch := range hash[:4] {
		value <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			value += int(ch - '0')
		case ch >= 'a' && ch <= 'f':
			value += int(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			value += int(ch-'A') + 10
		default:
			return false
		}
	}
	bucket := float64(value) / 65535 * 100
	return bucket < percent
}

func cloneSample(value qualitycontract.Sample) qualitycontract.Sample {
	return value
}

func cloneEvaluation(value qualitycontract.Evaluation) qualitycontract.Evaluation {
	value.Rubric = cloneMap(value.Rubric)
	return value
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

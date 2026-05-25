package qualityeval

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entqualityevalsample "github.com/srapi/srapi/apps/api/ent/qualityevalsample"
	entqualityevaluation "github.com/srapi/srapi/apps/api/ent/qualityevaluation"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
)

var ErrInvalidStore = errors.New("invalid quality evaluation ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateSample(ctx context.Context, sample qualitycontract.Sample) (qualitycontract.Sample, bool, error) {
	if existing, err := s.findSampleByFeedback(ctx, sample.FeedbackID); err == nil {
		return existing, false, nil
	} else if !ent.IsNotFound(err) {
		return qualitycontract.Sample{}, false, err
	}
	create := s.client.QualityEvalSample.Create().
		SetFeedbackID(sample.FeedbackID).
		SetRequestID(sample.RequestID).
		SetDecisionID(sample.DecisionID).
		SetAttemptNo(attemptNo(sample.AttemptNo)).
		SetAccountID(sample.AccountID).
		SetProviderID(sample.ProviderID).
		SetModel(sample.Model).
		SetSourceEndpoint(sample.SourceEndpoint).
		SetSampleRequestHash(sample.SampleRequestHash).
		SetSamplePayloadCiphertext([]byte(sample.SamplePayloadCiphertext)).
		SetPayloadVersion(sample.PayloadVersion).
		SetCapturedAt(sample.CapturedAt)
	if !sample.CreatedAt.IsZero() {
		create.SetCreatedAt(sample.CreatedAt).SetUpdatedAt(sample.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := s.findSampleByFeedback(ctx, sample.FeedbackID); findErr == nil {
				return existing, false, nil
			}
		}
		return qualitycontract.Sample{}, false, err
	}
	return toSample(created), true, nil
}

func (s *Store) ListPendingSamples(ctx context.Context, filter qualitycontract.PendingSampleFilter) ([]qualitycontract.Sample, error) {
	if filter.SamplePercent <= 0 {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	evaluatedRows, err := s.client.QualityEvaluation.Query().
		Select(entqualityevaluation.FieldFeedbackID).
		All(ctx)
	if err != nil {
		return nil, err
	}
	evaluated := map[int]bool{}
	for _, row := range evaluatedRows {
		evaluated[row.FeedbackID] = true
	}
	rows, err := s.client.QualityEvalSample.Query().
		Order(entqualityevalsample.ByCapturedAt(), entqualityevalsample.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]qualitycontract.Sample, 0, len(rows))
	for _, row := range rows {
		if evaluated[row.FeedbackID] || !sampleSelected(row.SampleRequestHash, filter.SamplePercent) {
			continue
		}
		out = append(out, toSample(row))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) CreateEvaluation(ctx context.Context, evaluation qualitycontract.Evaluation) (qualitycontract.Evaluation, bool, error) {
	if existing, err := s.findEvaluationByFeedback(ctx, evaluation.FeedbackID); err == nil {
		return existing, false, nil
	} else if !ent.IsNotFound(err) {
		return qualitycontract.Evaluation{}, false, err
	}
	create := s.client.QualityEvaluation.Create().
		SetFeedbackID(evaluation.FeedbackID).
		SetRequestID(evaluation.RequestID).
		SetDecisionID(evaluation.DecisionID).
		SetAttemptNo(attemptNo(evaluation.AttemptNo)).
		SetAccountID(evaluation.AccountID).
		SetProviderID(evaluation.ProviderID).
		SetModel(evaluation.Model).
		SetSourceEndpoint(evaluation.SourceEndpoint).
		SetSampleRequestHash(evaluation.SampleRequestHash).
		SetJudgeModel(evaluation.JudgeModel).
		SetScore(evaluation.Score).
		SetRubricJSON(cloneMap(evaluation.Rubric)).
		SetJudgedAt(evaluation.JudgedAt)
	if !evaluation.CreatedAt.IsZero() {
		create.SetCreatedAt(evaluation.CreatedAt).SetUpdatedAt(evaluation.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := s.findEvaluationByFeedback(ctx, evaluation.FeedbackID); findErr == nil {
				return existing, false, nil
			}
		}
		return qualitycontract.Evaluation{}, false, err
	}
	return toEvaluation(created), true, nil
}

func (s *Store) AggregateScore(ctx context.Context, accountID int, model string, since time.Time) (qualitycontract.AggregateScore, error) {
	model = strings.TrimSpace(model)
	rows, err := s.client.QualityEvaluation.Query().
		Where(
			entqualityevaluation.AccountIDEQ(accountID),
			entqualityevaluation.ModelEQ(model),
			entqualityevaluation.JudgedAtGTE(since),
		).
		Order(entqualityevaluation.ByJudgedAt()).
		All(ctx)
	if err != nil {
		return qualitycontract.AggregateScore{}, err
	}
	var total float64
	var updatedAt time.Time
	for _, row := range rows {
		total += row.Score
		if row.JudgedAt.After(updatedAt) {
			updatedAt = row.JudgedAt
		}
	}
	if len(rows) == 0 {
		return qualitycontract.AggregateScore{AccountID: accountID, Model: model}, nil
	}
	return qualitycontract.AggregateScore{
		AccountID:   accountID,
		Model:       model,
		Score:       total / float64(len(rows)),
		SampleCount: len(rows),
		UpdatedAt:   updatedAt,
	}, nil
}

func (s *Store) ListEvaluations(ctx context.Context) ([]qualitycontract.Evaluation, error) {
	rows, err := s.client.QualityEvaluation.Query().
		Order(entqualityevaluation.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]qualitycontract.Evaluation, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEvaluation(row))
	}
	return out, nil
}

func (s *Store) findSampleByFeedback(ctx context.Context, feedbackID int) (qualitycontract.Sample, error) {
	row, err := s.client.QualityEvalSample.Query().
		Where(entqualityevalsample.FeedbackIDEQ(feedbackID)).
		Only(ctx)
	if err != nil {
		return qualitycontract.Sample{}, err
	}
	return toSample(row), nil
}

func (s *Store) findEvaluationByFeedback(ctx context.Context, feedbackID int) (qualitycontract.Evaluation, error) {
	row, err := s.client.QualityEvaluation.Query().
		Where(entqualityevaluation.FeedbackIDEQ(feedbackID)).
		Only(ctx)
	if err != nil {
		return qualitycontract.Evaluation{}, err
	}
	return toEvaluation(row), nil
}

func toSample(row *ent.QualityEvalSample) qualitycontract.Sample {
	return qualitycontract.Sample{
		ID:                      row.ID,
		FeedbackID:              row.FeedbackID,
		RequestID:               row.RequestID,
		DecisionID:              row.DecisionID,
		AttemptNo:               row.AttemptNo,
		AccountID:               row.AccountID,
		ProviderID:              row.ProviderID,
		Model:                   row.Model,
		SourceEndpoint:          row.SourceEndpoint,
		SampleRequestHash:       row.SampleRequestHash,
		SamplePayloadCiphertext: string(row.SamplePayloadCiphertext),
		PayloadVersion:          row.PayloadVersion,
		CapturedAt:              row.CapturedAt,
		CreatedAt:               row.CreatedAt,
		UpdatedAt:               row.UpdatedAt,
	}
}

func toEvaluation(row *ent.QualityEvaluation) qualitycontract.Evaluation {
	return qualitycontract.Evaluation{
		ID:                row.ID,
		FeedbackID:        row.FeedbackID,
		RequestID:         row.RequestID,
		DecisionID:        row.DecisionID,
		AttemptNo:         row.AttemptNo,
		AccountID:         row.AccountID,
		ProviderID:        row.ProviderID,
		Model:             row.Model,
		SourceEndpoint:    row.SourceEndpoint,
		SampleRequestHash: row.SampleRequestHash,
		JudgeModel:        row.JudgeModel,
		Score:             row.Score,
		Rubric:            cloneMap(row.RubricJSON),
		JudgedAt:          row.JudgedAt,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
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

func attemptNo(value int) int {
	if value <= 0 {
		return 1
	}
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

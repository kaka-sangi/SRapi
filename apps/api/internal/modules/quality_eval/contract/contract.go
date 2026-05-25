package contract

import (
	"context"
	"time"
)

// DefaultJudgeModel is the fallback LLM-as-judge model for quality evaluation.
const DefaultJudgeModel = "gpt-4o-mini"

// Sample is an encrypted prompt/output sample linked to one scheduler feedback row.
type Sample struct {
	ID                      int
	FeedbackID              int
	RequestID               string
	DecisionID              int
	AttemptNo               int
	AccountID               int
	ProviderID              int
	Model                   string
	SourceEndpoint          string
	SampleRequestHash       string
	SamplePayloadCiphertext string
	PayloadVersion          string
	CapturedAt              time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// CaptureSampleRequest describes a sanitized Gateway result that can be judged later.
type CaptureSampleRequest struct {
	FeedbackID      int
	RequestID       string
	DecisionID      int
	AttemptNo       int
	AccountID       int
	ProviderID      int
	Model           string
	SourceEndpoint  string
	SanitizedPrompt string
	SanitizedOutput string
	CapturedAt      time.Time
}

// Evaluation stores one judge result for a captured sample.
type Evaluation struct {
	ID                int
	FeedbackID        int
	RequestID         string
	DecisionID        int
	AttemptNo         int
	AccountID         int
	ProviderID        int
	Model             string
	SourceEndpoint    string
	SampleRequestHash string
	JudgeModel        string
	Score             float64
	Rubric            map[string]any
	JudgedAt          time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// EvaluationSample is the decrypted, sanitized input passed to a judge.
type EvaluationSample struct {
	SampleID          int
	FeedbackID        int
	RequestID         string
	DecisionID        int
	AttemptNo         int
	AccountID         int
	ProviderID        int
	Model             string
	SourceEndpoint    string
	SampleRequestHash string
	SanitizedPrompt   string
	SanitizedOutput   string
	CapturedAt        time.Time
}

// JudgeResult is the normalized rubric returned by an LLM-as-judge.
type JudgeResult struct {
	JudgeModel  string
	Score       float64
	Correctness int
	Coherence   int
	Safety      int
	Rationale   string
	Rubric      map[string]any
	JudgedAt    time.Time
}

// AggregateScore is the account+model quality signal consumed by Scheduler.
type AggregateScore struct {
	AccountID   int
	Model       string
	Score       float64
	SampleCount int
	UpdatedAt   time.Time
}

// PendingSampleFilter controls worker sample selection.
type PendingSampleFilter struct {
	SamplePercent float64
	Limit         int
	Now           time.Time
}

// Store persists samples and evaluations behind the quality_eval service boundary.
type Store interface {
	CreateSample(ctx context.Context, sample Sample) (Sample, bool, error)
	ListPendingSamples(ctx context.Context, filter PendingSampleFilter) ([]Sample, error)
	CreateEvaluation(ctx context.Context, evaluation Evaluation) (Evaluation, bool, error)
	AggregateScore(ctx context.Context, accountID int, model string, since time.Time) (AggregateScore, error)
	ListEvaluations(ctx context.Context) ([]Evaluation, error)
}

// Judge evaluates a sanitized sample and returns a normalized quality result.
type Judge interface {
	Evaluate(ctx context.Context, sample EvaluationSample) (JudgeResult, error)
}

package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

const (
	payloadVersionV1       = "v1"
	defaultSampleTextLimit = 4000
	defaultScoreWindow     = 30 * 24 * time.Hour
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store     qualitycontract.Store
	masterKey []byte
	clock     Clock
	textLimit int
}

func New(store qualitycontract.Store, masterKey string, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	derivedKey, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, masterKey: derivedKey, clock: clock, textLimit: defaultSampleTextLimit}, nil
}

func (s *Service) CaptureSample(ctx context.Context, req qualitycontract.CaptureSampleRequest) (qualitycontract.Sample, bool, error) {
	if err := validateCaptureRequest(req); err != nil {
		return qualitycontract.Sample{}, false, err
	}
	now := s.clock.Now()
	capturedAt := req.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = now
	}
	payload := samplePayload{
		Prompt:   truncateText(req.SanitizedPrompt, s.textLimit),
		Output:   truncateText(req.SanitizedOutput, s.textLimit),
		Captured: capturedAt.UTC().Format(time.RFC3339Nano),
	}
	payloadHash := sampleHash(req, payload)
	ciphertext, err := s.encryptPayload(payload)
	if err != nil {
		return qualitycontract.Sample{}, false, err
	}
	return s.store.CreateSample(ctx, qualitycontract.Sample{
		FeedbackID:              req.FeedbackID,
		RequestID:               strings.TrimSpace(req.RequestID),
		DecisionID:              req.DecisionID,
		AttemptNo:               attemptNo(req.AttemptNo),
		AccountID:               req.AccountID,
		ProviderID:              req.ProviderID,
		Model:                   strings.TrimSpace(req.Model),
		SourceEndpoint:          strings.TrimSpace(req.SourceEndpoint),
		SampleRequestHash:       payloadHash,
		SamplePayloadCiphertext: ciphertext,
		PayloadVersion:          payloadVersionV1,
		CapturedAt:              capturedAt,
		CreatedAt:               now,
		UpdatedAt:               now,
	})
}

func (s *Service) ListPendingSamples(ctx context.Context, filter qualitycontract.PendingSampleFilter) ([]qualitycontract.Sample, error) {
	normalized := filter
	if normalized.SamplePercent <= 0 {
		return nil, nil
	}
	if normalized.SamplePercent > 100 {
		normalized.SamplePercent = 100
	}
	if normalized.Now.IsZero() {
		normalized.Now = s.clock.Now()
	}
	return s.store.ListPendingSamples(ctx, normalized)
}

func (s *Service) EvaluationSample(sample qualitycontract.Sample) (qualitycontract.EvaluationSample, error) {
	if sample.ID <= 0 || sample.FeedbackID <= 0 {
		return qualitycontract.EvaluationSample{}, ErrInvalidInput
	}
	payload, err := s.decryptPayload(sample.SamplePayloadCiphertext)
	if err != nil {
		return qualitycontract.EvaluationSample{}, err
	}
	return qualitycontract.EvaluationSample{
		SampleID:          sample.ID,
		FeedbackID:        sample.FeedbackID,
		RequestID:         sample.RequestID,
		DecisionID:        sample.DecisionID,
		AttemptNo:         sample.AttemptNo,
		AccountID:         sample.AccountID,
		ProviderID:        sample.ProviderID,
		Model:             sample.Model,
		SourceEndpoint:    sample.SourceEndpoint,
		SampleRequestHash: sample.SampleRequestHash,
		SanitizedPrompt:   payload.Prompt,
		SanitizedOutput:   payload.Output,
		CapturedAt:        sample.CapturedAt,
	}, nil
}

func (s *Service) RecordEvaluation(ctx context.Context, sample qualitycontract.Sample, result qualitycontract.JudgeResult) (qualitycontract.Evaluation, bool, error) {
	if sample.FeedbackID <= 0 || sample.DecisionID <= 0 || strings.TrimSpace(result.JudgeModel) == "" {
		return qualitycontract.Evaluation{}, false, ErrInvalidInput
	}
	judgedAt := result.JudgedAt
	if judgedAt.IsZero() {
		judgedAt = s.clock.Now()
	}
	rubric := normalizedRubric(result)
	return s.store.CreateEvaluation(ctx, qualitycontract.Evaluation{
		FeedbackID:        sample.FeedbackID,
		RequestID:         sample.RequestID,
		DecisionID:        sample.DecisionID,
		AttemptNo:         sample.AttemptNo,
		AccountID:         sample.AccountID,
		ProviderID:        sample.ProviderID,
		Model:             sample.Model,
		SourceEndpoint:    sample.SourceEndpoint,
		SampleRequestHash: sample.SampleRequestHash,
		JudgeModel:        strings.TrimSpace(result.JudgeModel),
		Score:             clamp01(result.Score),
		Rubric:            rubric,
		JudgedAt:          judgedAt,
		CreatedAt:         judgedAt,
		UpdatedAt:         judgedAt,
	})
}

func (s *Service) AggregateScore(ctx context.Context, accountID int, model string) (qualitycontract.AggregateScore, error) {
	if accountID <= 0 || strings.TrimSpace(model) == "" {
		return qualitycontract.AggregateScore{}, ErrInvalidInput
	}
	return s.store.AggregateScore(ctx, accountID, strings.TrimSpace(model), s.clock.Now().Add(-defaultScoreWindow))
}

type samplePayload struct {
	Prompt   string `json:"prompt"`
	Output   string `json:"output"`
	Captured string `json:"captured_at"`
}

func validateCaptureRequest(req qualitycontract.CaptureSampleRequest) error {
	if req.FeedbackID <= 0 || req.DecisionID <= 0 || req.AccountID <= 0 || req.ProviderID <= 0 {
		return ErrInvalidInput
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(req.SanitizedPrompt) == "" || strings.TrimSpace(req.SanitizedOutput) == "" {
		return ErrInvalidInput
	}
	return nil
}

func sampleHash(req qualitycontract.CaptureSampleRequest, payload samplePayload) string {
	parts := []string{
		strings.TrimSpace(req.RequestID),
		fmt.Sprint(attemptNo(req.AttemptNo)),
		strings.TrimSpace(req.Model),
		payload.Prompt,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (s *Service) encryptPayload(payload samplePayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, raw, []byte(payloadVersionV1))
	return fmt.Sprintf("%s:%s:%s", payloadVersionV1, base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(ciphertext)), nil
}

func (s *Service) decryptPayload(ciphertext string) (samplePayload, error) {
	parts := strings.Split(ciphertext, ":")
	if len(parts) != 3 || parts[0] != payloadVersionV1 {
		return samplePayload{}, ErrInvalidInput
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return samplePayload{}, err
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return samplePayload{}, err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return samplePayload{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return samplePayload{}, err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, []byte(payloadVersionV1))
	if err != nil {
		return samplePayload{}, err
	}
	var payload samplePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return samplePayload{}, err
	}
	return payload, nil
}

func normalizedRubric(result qualitycontract.JudgeResult) map[string]any {
	rubric := cloneMap(result.Rubric)
	if rubric == nil {
		rubric = map[string]any{}
	}
	rubric["correctness"] = clampRubric(result.Correctness)
	rubric["coherence"] = clampRubric(result.Coherence)
	rubric["safety"] = clampRubric(result.Safety)
	if rationale := strings.TrimSpace(result.Rationale); rationale != "" {
		rubric["rationale"] = truncateText(rationale, 1000)
	}
	return rubric
}

func clampRubric(value int) int {
	switch {
	case value < 0:
		return 0
	case value > 5:
		return 5
	default:
		return value
	}
}

func clamp01(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func attemptNo(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return strings.TrimSpace(value[:limit])
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

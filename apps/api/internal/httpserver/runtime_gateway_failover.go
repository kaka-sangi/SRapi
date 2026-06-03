package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const maxGatewayFailoverAttempts = 3

const maxGatewayProviderErrorMessageLength = 300

type gatewayFailoverResult[T any] struct {
	Response        T
	ScheduleResult  schedulercontract.ScheduleResult
	Err             error
	FailureRecorded bool
}

type gatewayCandidateInvoker[T any] func(context.Context, schedulercontract.Candidate) (T, error)

func (s *Server) reserveGatewayAccountQuotaForScheduledRequest(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	result schedulercontract.ScheduleResult,
	admission gatewayAdmission,
	startedAt time.Time,
) error {
	// Per-user, per-platform spend cap (WP sub2api M142 parity): block before
	// reserving account quota when the user's spend on the scheduled platform
	// would exceed a configured window cap. 402 is a non-failover class, so this
	// hard-denies rather than rerouting to another platform.
	if err := s.runtime.enforceUserPlatformQuota(ctx, canonical.UserID, result.Candidate.Provider.ID, result.Candidate.Provider.Name, admission); err != nil {
		errorClass, upstreamStatus, _ := providerGatewayError(err)
		s.recordGatewayProviderAttemptFailure(r, authed, canonical, result, err, errorClass, upstreamStatus, elapsedMillis(startedAt), admission)
		return err
	}
	if err := s.runtime.reserveGatewayAccountQuota(ctx, admission.EstimatedUsage, result.Candidate); err != nil {
		errorClass, upstreamStatus, _ := providerGatewayError(err)
		s.recordGatewayProviderAttemptFailure(r, authed, canonical, result, err, errorClass, upstreamStatus, elapsedMillis(startedAt), admission)
		return err
	}
	return nil
}

func (s *Server) writeProviderGatewayError(w http.ResponseWriter, err error) {
	s.writeProviderGatewayErrorForCandidate(w, err, nil)
}

func (s *Server) writeProviderGatewayErrorForCandidate(w http.ResponseWriter, err error, candidate *schedulercontract.Candidate) {
	errorClass, upstreamStatus, errorType := providerGatewayError(err)
	writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, s.gatewayPublicMessage(err, errorClass, upstreamStatus, candidate), errorClass)
}

func (s *Server) writeGeminiProviderGatewayError(w http.ResponseWriter, err error) {
	s.writeGeminiProviderGatewayErrorForCandidate(w, err, nil)
}

func (s *Server) writeGeminiProviderGatewayErrorForCandidate(w http.ResponseWriter, err error, candidate *schedulercontract.Candidate) {
	errorClass, upstreamStatus, _ := providerGatewayError(err)
	status := providerGatewayHTTPStatus(upstreamStatus)
	writeGeminiGatewayError(w, status, geminiStatusForGatewayErrorClass(errorClass, status), s.gatewayPublicMessage(err, errorClass, upstreamStatus, candidate))
}

// gatewayPublicMessage decides the caller-facing message. Global admin-managed
// error-passthrough rules take precedence; when no rule matches it falls back to
// the per-account / per-provider metadata behavior in gatewayProviderPublicMessage.
func (s *Server) gatewayPublicMessage(err error, errorClass string, upstreamStatus int, candidate *schedulercontract.Candidate) string {
	if s.runtime != nil && s.runtime.errorPassthrough != nil {
		if raw := gatewayProviderErrorMessage(err); raw != "" {
			if action, matched := s.runtime.errorPassthrough.Resolve(context.Background(), errorClass, upstreamStatus, raw); matched {
				if action == errorpassthroughcontract.ActionExpose {
					return raw
				}
				return providerGatewayMessage(errorClass)
			}
		}
	}
	return gatewayProviderPublicMessage(err, errorClass, upstreamStatus, candidate)
}

func (s *Server) writeGatewayFailoverFailure(
	w http.ResponseWriter,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	result schedulercontract.ScheduleResult,
	failureRecorded bool,
	err error,
	admission gatewayAdmission,
	startedAt time.Time,
) {
	if !failureRecorded {
		s.recordGatewayNoAvailableAccount(r, authed, canonical, result, admission, startedAt)
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available account", "no_available_account")
		return
	}
	s.writeProviderGatewayErrorForCandidate(w, err, gatewayFailureCandidate(result))
}

func (s *Server) writeGeminiGatewayFailoverFailure(
	w http.ResponseWriter,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	result schedulercontract.ScheduleResult,
	failureRecorded bool,
	err error,
	admission gatewayAdmission,
	startedAt time.Time,
) {
	if !failureRecorded {
		s.recordGatewayNoAvailableAccount(r, authed, canonical, result, admission, startedAt)
		writeGeminiGatewayError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "no available account")
		return
	}
	s.writeGeminiProviderGatewayErrorForCandidate(w, err, gatewayFailureCandidate(result))
}

func gatewayFailureCandidate(result schedulercontract.ScheduleResult) *schedulercontract.Candidate {
	if result.Candidate.Provider.ID == 0 && result.Candidate.Account.ID == 0 {
		return nil
	}
	candidate := result.Candidate
	return &candidate
}

func (s *Server) invokeProviderConversationWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.ConversationResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.ConversationResponse, error) {
			return s.runtime.invokeProviderConversation(ctx, providerConversationRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderEmbeddingsWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.EmbeddingResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.EmbeddingResponse, error) {
			return s.runtime.invokeProviderEmbeddings(ctx, providerEmbeddingRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderTokenCountWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	rawBody []byte,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.TokenCountResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.TokenCountResponse, error) {
			return s.runtime.invokeProviderTokenCount(ctx, providerTokenCountRequest(canonical, rawBody, candidate))
		})
}

func (s *Server) invokeProviderImageGenerationWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.ImageGenerationResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.ImageGenerationResponse, error) {
			return s.runtime.invokeProviderImageGeneration(ctx, providerImageGenerationRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderImageEditWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.ImageGenerationResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.ImageGenerationResponse, error) {
			return s.runtime.invokeProviderImageEdit(ctx, providerImageEditRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderImageVariationWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.ImageGenerationResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.ImageGenerationResponse, error) {
			return s.runtime.invokeProviderImageVariation(ctx, providerImageVariationRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderAudioTranscriptionWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.AudioTranscriptionResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.AudioTranscriptionResponse, error) {
			return s.runtime.invokeProviderAudioTranscription(ctx, providerAudioTranscriptionRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderAudioSpeechWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.AudioSpeechResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.AudioSpeechResponse, error) {
			return s.runtime.invokeProviderAudioSpeech(ctx, providerAudioSpeechRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderModerationsWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.ModerationResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.ModerationResponse, error) {
			return s.runtime.invokeProviderModerations(ctx, providerModerationRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderRerankWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.RerankResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.RerankResponse, error) {
			return s.runtime.invokeProviderRerank(ctx, providerRerankRequest(canonical, candidate))
		})
}

func (s *Server) invokeProviderResponseInputItemsWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	responseID string,
	query map[string][]string,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.ResponseInputItemsResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.ResponseInputItemsResponse, error) {
			return s.runtime.invokeProviderResponseInputItems(ctx, providerResponseInputItemsRequest(canonical, responseID, query, candidate))
		})
}

func invokeGatewayCandidateWithFailover[T any](
	s *Server,
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
	invoke gatewayCandidateInvoker[T],
) gatewayFailoverResult[T] {
	initialExcluded := append([]int(nil), scheduleReq.ExcludedAccountIDs...)
	excluded := append([]int(nil), initialExcluded...)
	var fallbackFromDecisionID *int
	var lastFailure gatewayFailoverResult[T]

NextCandidate:
	for attemptNo := 1; attemptNo <= maxGatewayFailoverAttempts; attemptNo++ {
		attemptReq := scheduleReq
		attemptReq.AttemptNo = attemptNo
		attemptReq.FallbackFromDecisionID = cloneIntPtr(fallbackFromDecisionID)
		attemptReq.ExcludedAccountIDs = append([]int(nil), excluded...)

		result, err := s.runtime.scheduleGatewayRequest(ctx, attemptReq, modelID, forcedProviderKey, authed.Key)
		if err != nil {
			if lastFailure.Err != nil {
				return lastFailure
			}
			return gatewayFailoverResult[T]{ScheduleResult: result, Err: err}
		}
		if err := s.reserveGatewayAccountQuotaForScheduledRequest(ctx, r, authed, canonical, result, admission, startedAt); err != nil {
			errorClass, upstreamStatus, _ := providerGatewayError(err)
			lastFailure = gatewayFailoverResult[T]{
				ScheduleResult:  result,
				Err:             err,
				FailureRecorded: true,
			}
			if !gatewayShouldFailover(errorClass, upstreamStatus, attemptNo, len(result.Candidates)) {
				return lastFailure
			}
			excluded = append(excluded, result.Candidate.Account.ID)
			decisionID := result.Decision.ID
			fallbackFromDecisionID = &decisionID
			continue
		}

		retryPolicy := gatewaySameCandidateRetryPolicyFor(result.Candidate)
		for sameCandidateRetries := 0; ; {
			response, err := invoke(ctx, result.Candidate)
			if err == nil {
				return gatewayFailoverResult[T]{Response: response, ScheduleResult: result}
			}

			errorClass, upstreamStatus, _ := providerGatewayError(err)
			if gatewayShouldRetrySameCandidate(retryPolicy, errorClass, upstreamStatus, sameCandidateRetries) {
				sameCandidateRetries++
				delay := gatewaySameCandidateRetryDelay(retryPolicy, sameCandidateRetries, err)
				s.runtime.logger.Info("retrying gateway provider candidate", "request_id", canonical.RequestID, "attempt_no", result.Decision.AttemptNo, "retry_no", sameCandidateRetries, "account_id", result.Candidate.Account.ID, "provider_id", result.Candidate.Provider.ID, "error_class", errorClass, "status_code", upstreamStatus, "delay_ms", int(delay/time.Millisecond))
				if err := sleepGatewayRetryDelay(ctx, delay); err != nil {
					s.recordGatewayProviderAttemptFailure(r, authed, canonical, result, err, errorClass, upstreamStatus, elapsedMillis(startedAt), admission)
					return gatewayFailoverResult[T]{
						ScheduleResult:  result,
						Err:             err,
						FailureRecorded: true,
					}
				}
				continue
			}

			s.recordGatewayProviderAttemptFailure(r, authed, canonical, result, err, errorClass, upstreamStatus, elapsedMillis(startedAt), admission)
			lastFailure = gatewayFailoverResult[T]{
				ScheduleResult:  result,
				Err:             err,
				FailureRecorded: true,
			}
			if !gatewayShouldFailover(errorClass, upstreamStatus, attemptNo, len(result.Candidates)) {
				return lastFailure
			}

			excluded = append(excluded, result.Candidate.Account.ID)
			decisionID := result.Decision.ID
			fallbackFromDecisionID = &decisionID
			continue NextCandidate
		}
	}

	return lastFailure
}

func (s *Server) recordGatewayProviderAttemptFailure(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, providerErr error, errorClass string, upstreamStatus int, latencyMS int, admission gatewayAdmission) {
	s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		ProviderID:            ptrInt(result.Candidate.Provider.ID),
		AccountID:             ptrInt(result.Candidate.Account.ID),
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		TargetProtocol:        result.Candidate.Provider.Protocol,
		Model:                 canonical.CanonicalModel,
		Success:               false,
		ErrorClass:            ptrStringValue(errorClass),
		StatusCode:            ptrInt(upstreamStatus),
		LatencyMS:             latencyMS,
		InputTokens:           admission.EstimatedUsage.InputTokens,
		OutputTokens:          admission.EstimatedUsage.OutputTokens,
		CachedTokens:          admission.EstimatedUsage.CachedTokens,
		UsageEstimated:        true,
		Pricing:               admission.Pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
		ProviderRetryAfter:    providerRetryAfterFromError(providerErr),
		ProviderErrorMessage:  providerErrorMessage(providerErr),
	})
}

func (s *Server) recordGatewayNoAvailableAccount(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, admission gatewayAdmission, startedAt time.Time) {
	s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		Model:                 canonical.CanonicalModel,
		Success:               false,
		ErrorClass:            ptrStringValue("no_available_account"),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           admission.EstimatedUsage.InputTokens,
		OutputTokens:          admission.EstimatedUsage.OutputTokens,
		CachedTokens:          admission.EstimatedUsage.CachedTokens,
		UsageEstimated:        true,
		Pricing:               admission.Pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
	})
}

func providerRetryAfterFromError(err error) *time.Time {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) || providerErr.RetryAfter == nil {
		return nil
	}
	retryAfter := providerErr.RetryAfter.UTC()
	return &retryAfter
}

func providerErrorMessage(err error) string {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return ""
	}
	return strings.TrimSpace(providerErr.Message)
}

func gatewayProviderErrorMessage(err error) string {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return ""
	}
	message := gatewayProviderErrorJSONMessage(providerErr.Message)
	if message == "" {
		trimmed := strings.TrimSpace(providerErr.Message)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return ""
		}
		message = providerErr.Message
	}
	message = gatewayNormalizeProviderErrorMessage(message)
	if message == "" || gatewayProviderErrorMessageSensitive(message) {
		return ""
	}
	if len([]rune(message)) <= maxGatewayProviderErrorMessageLength {
		return message
	}
	return string([]rune(message)[:maxGatewayProviderErrorMessageLength]) + "..."
}

func gatewayProviderErrorJSONMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return ""
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ""
	}
	for _, path := range [][]string{
		{"error", "message"},
		{"error", "details"},
		{"message"},
		{"detail"},
		{"error"},
	} {
		if message := gatewayJSONPathString(decoded, path...); message != "" {
			return message
		}
	}
	return ""
}

func gatewayJSONPathString(value any, path ...string) string {
	if len(path) == 0 {
		switch value := value.(type) {
		case string:
			return strings.TrimSpace(value)
		default:
			return ""
		}
	}
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	next, ok := object[path[0]]
	if !ok || next == nil {
		return ""
	}
	return gatewayJSONPathString(next, path[1:]...)
}

func gatewayNormalizeProviderErrorMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, strings.TrimSpace(message))
	return strings.Join(strings.Fields(message), " ")
}

func gatewayProviderErrorMessageSensitive(message string) bool {
	lower := strings.ToLower(message)
	for _, marker := range []string{
		"authorization",
		"bearer ",
		"api key",
		"api-key",
		"api_key",
		"apikey",
		"access_token",
		"refresh_token",
		"secret",
		"set-cookie",
		"cookie",
		"password",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func gatewayProviderGatewayMetadata(candidate schedulercontract.Candidate) []map[string]any {
	return []map[string]any{
		candidate.Account.Metadata,
		candidate.Provider.ConfigSchema,
		candidate.Provider.Capabilities,
	}
}

func gatewayProviderErrorMessageEnabled(metadata map[string]any) bool {
	for _, key := range []string{
		"expose_provider_error_messages",
		"provider_error_passthrough_enabled",
		"upstream_error_message_passthrough",
		"passthrough_provider_error_message",
	} {
		if metadataBool(metadata, key) {
			return true
		}
	}
	return false
}

func gatewayProviderErrorMessageStatusAllowed(metadata map[string]any, upstreamStatus int) bool {
	value, ok := metadataValue(metadata,
		"provider_error_passthrough_status_codes",
		"exposed_provider_error_status_codes",
		"provider_error_message_status_codes",
	)
	if !ok {
		return true
	}
	statusCodes := gatewayStatusCodeList(value)
	return len(statusCodes) == 0 || gatewayIntInList(statusCodes, upstreamStatus)
}

func gatewayIntInList(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func gatewayProviderErrorMessageClassAllowed(metadata map[string]any, errorClass string) bool {
	for _, key := range []string{
		"provider_error_passthrough_classes",
		"exposed_provider_error_classes",
		"provider_error_message_classes",
	} {
		classes, ok := metadataStringList(metadata, key)
		if !ok {
			continue
		}
		hasFilter := false
		for _, class := range classes {
			class = strings.TrimSpace(class)
			if class == "" {
				continue
			}
			hasFilter = true
			if strings.EqualFold(class, errorClass) {
				return true
			}
		}
		return !hasFilter
	}
	return true
}

func gatewayProviderErrorMessageKeywordAllowed(metadata map[string]any, message string) bool {
	for _, key := range []string{
		"provider_error_passthrough_keywords",
		"exposed_provider_error_keywords",
		"provider_error_message_keywords",
	} {
		keywords, ok := metadataStringList(metadata, key)
		if !ok {
			continue
		}
		lowerMessage := strings.ToLower(message)
		hasFilter := false
		for _, keyword := range keywords {
			keyword = strings.ToLower(strings.TrimSpace(keyword))
			if keyword == "" {
				continue
			}
			hasFilter = true
			if strings.Contains(lowerMessage, keyword) {
				return true
			}
		}
		return !hasFilter
	}
	return true
}

func gatewayProviderErrorMessageAllowed(candidate schedulercontract.Candidate, errorClass string, upstreamStatus int, message string) bool {
	for _, metadata := range gatewayProviderGatewayMetadata(candidate) {
		if !gatewayProviderErrorMessageEnabled(metadata) {
			continue
		}
		if !gatewayProviderErrorMessageStatusAllowed(metadata, upstreamStatus) {
			continue
		}
		if !gatewayProviderErrorMessageClassAllowed(metadata, errorClass) {
			continue
		}
		if !gatewayProviderErrorMessageKeywordAllowed(metadata, message) {
			continue
		}
		return true
	}
	return false
}

func gatewayProviderPublicMessage(err error, errorClass string, upstreamStatus int, candidate *schedulercontract.Candidate) string {
	if candidate == nil {
		return providerGatewayMessage(errorClass)
	}
	message := gatewayProviderErrorMessage(err)
	if message == "" {
		return providerGatewayMessage(errorClass)
	}
	if gatewayProviderErrorMessageAllowed(*candidate, errorClass, upstreamStatus, message) {
		return message
	}
	return providerGatewayMessage(errorClass)
}

func gatewayShouldFailover(errorClass string, upstreamStatus int, attemptNo int, candidateCount int) bool {
	if attemptNo >= maxGatewayFailoverAttempts || candidateCount <= 1 {
		return false
	}
	if upstreamStatus == http.StatusTooManyRequests ||
		upstreamStatus == http.StatusGatewayTimeout ||
		upstreamStatus == http.StatusRequestTimeout ||
		upstreamStatus >= http.StatusInternalServerError {
		return true
	}
	switch errorClass {
	case "rate_limit", "timeout", "network_error", "provider_5xx", "model_unavailable", "configuration_error", "credential_error", "auth_failed", "auth_error", "permission_denied", "session_invalid", "account_locked", "account_banned", "abuse_detected", "device_unrecognized", "stream_interrupted", "empty_completion":
		return true
	default:
		return false
	}
}

type gatewaySameCandidateRetryPolicy struct {
	Enabled           bool
	MaxRetries        int
	BaseDelay         time.Duration
	MaxDelay          time.Duration
	RetryAuthFailures bool
}

const (
	defaultGatewaySameCandidateRetries = 3
	maxGatewaySameCandidateRetries     = 10
	defaultGatewayRetryBaseDelay       = 100 * time.Millisecond
	defaultGatewayRetryMaxDelay        = 2 * time.Second
)

func gatewaySameCandidateRetryPolicyFor(candidate schedulercontract.Candidate) gatewaySameCandidateRetryPolicy {
	metadata := candidate.Account.Metadata
	poolMode := metadataBool(metadata, "pool_mode")
	enabled := poolMode ||
		metadataBool(metadata, "same_candidate_retry_enabled") ||
		metadataBool(metadata, "same_account_retry_enabled") ||
		metadataBool(metadata, "transient_retry_enabled")
	count, hasCount := gatewayRetryCount(metadata,
		"same_candidate_retry_count",
		"same_account_retry_count",
		"transient_retry_count",
		"pool_mode_retry_count",
	)
	if hasCount {
		enabled = true
	} else if enabled {
		count = defaultGatewaySameCandidateRetries
	}
	if !enabled {
		return gatewaySameCandidateRetryPolicy{}
	}
	return gatewaySameCandidateRetryPolicy{
		Enabled:           true,
		MaxRetries:        clampGatewayRetryCount(count),
		BaseDelay:         gatewayRetryDelay(metadata, defaultGatewayRetryBaseDelay, "same_candidate_retry_base_delay_ms", "same_account_retry_base_delay_ms", "pool_mode_retry_base_delay_ms"),
		MaxDelay:          gatewayRetryDelay(metadata, defaultGatewayRetryMaxDelay, "same_candidate_retry_max_delay_ms", "same_account_retry_max_delay_ms", "pool_mode_retry_max_delay_ms"),
		RetryAuthFailures: poolMode || metadataBool(metadata, "same_candidate_retry_auth_errors") || metadataBool(metadata, "same_account_retry_auth_errors"),
	}
}

func gatewayShouldRetrySameCandidate(policy gatewaySameCandidateRetryPolicy, errorClass string, upstreamStatus int, retriesUsed int) bool {
	if !policy.Enabled || retriesUsed >= policy.MaxRetries {
		return false
	}
	if upstreamStatus == http.StatusTooManyRequests ||
		upstreamStatus == http.StatusGatewayTimeout ||
		upstreamStatus == http.StatusRequestTimeout ||
		upstreamStatus == 529 ||
		upstreamStatus >= http.StatusInternalServerError {
		return true
	}
	if policy.RetryAuthFailures && (upstreamStatus == http.StatusUnauthorized || upstreamStatus == http.StatusForbidden) {
		return true
	}
	switch errorClass {
	case "rate_limit", "timeout", "network_error", "provider_5xx", "overloaded", "stream_interrupted", "empty_completion":
		return true
	case "auth_failed", "auth_error", "permission_denied":
		return policy.RetryAuthFailures
	default:
		return false
	}
}

func gatewaySameCandidateRetryDelay(policy gatewaySameCandidateRetryPolicy, retryNo int, err error) time.Duration {
	if retryAfter := providerRetryAfterFromError(err); retryAfter != nil {
		delay := time.Until(*retryAfter)
		if delay > 0 {
			return capGatewayRetryDelay(delay, policy.MaxDelay)
		}
	}
	if retryNo <= 0 || policy.BaseDelay <= 0 {
		return 0
	}
	delay := policy.BaseDelay
	for i := 1; i < retryNo; i++ {
		delay *= 2
		if delay >= policy.MaxDelay {
			return policy.MaxDelay
		}
	}
	return capGatewayRetryDelay(delay, policy.MaxDelay)
}

func capGatewayRetryDelay(delay time.Duration, maxDelay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	if maxDelay <= 0 {
		return 0
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func sleepGatewayRetryDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func gatewayRetryCount(metadata map[string]any, keys ...string) (int, bool) {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0, false
	}
	count, ok := gatewayRetryCountValue(value)
	if !ok {
		return defaultGatewaySameCandidateRetries, true
	}
	return count, true
}

func gatewayRetryCountValue(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint:
		return int(value), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return int(value), true
	case uint64:
		return int(value), true
	case float32:
		return int(value), true
	case float64:
		return int(value), true
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return int(parsed), true
		}
		floatValue, err := value.Float64()
		if err == nil {
			return int(floatValue), true
		}
	case string:
		raw := strings.TrimSpace(value)
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			return parsed, true
		}
		floatValue, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return int(floatValue), true
		}
	}
	return 0, false
}

func clampGatewayRetryCount(count int) int {
	if count < 0 {
		return 0
	}
	if count > maxGatewaySameCandidateRetries {
		return maxGatewaySameCandidateRetries
	}
	return count
}

func gatewayRetryDelay(metadata map[string]any, fallback time.Duration, keys ...string) time.Duration {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return fallback
	}
	millis, ok := gatewayRetryCountValue(value)
	if !ok || millis < 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}

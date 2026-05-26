package httpserver

import (
	"context"
	"net/http"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

const maxGatewayFailoverAttempts = 3

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
	if err := s.runtime.reserveGatewayAccountQuota(ctx, admission.EstimatedUsage, result.Candidate); err != nil {
		errorClass, upstreamStatus, _ := providerGatewayError(err)
		s.recordGatewayProviderAttemptFailure(r, authed, canonical, result, errorClass, upstreamStatus, elapsedMillis(startedAt), admission)
		return err
	}
	return nil
}

func writeProviderGatewayError(w http.ResponseWriter, err error) {
	errorClass, upstreamStatus, errorType := providerGatewayError(err)
	writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, providerGatewayMessage(errorClass), errorClass)
}

func writeGeminiProviderGatewayError(w http.ResponseWriter, err error) {
	errorClass, upstreamStatus, _ := providerGatewayError(err)
	status := providerGatewayHTTPStatus(upstreamStatus)
	writeGeminiGatewayError(w, status, geminiStatusForGatewayErrorClass(errorClass, status), providerGatewayMessage(errorClass))
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

		response, err := invoke(ctx, result.Candidate)
		if err == nil {
			return gatewayFailoverResult[T]{Response: response, ScheduleResult: result}
		}

		errorClass, upstreamStatus, _ := providerGatewayError(err)
		s.recordGatewayProviderAttemptFailure(r, authed, canonical, result, errorClass, upstreamStatus, elapsedMillis(startedAt), admission)
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
	}

	return lastFailure
}

func (s *Server) recordGatewayProviderAttemptFailure(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, errorClass string, upstreamStatus int, latencyMS int, admission gatewayAdmission) {
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
	case "rate_limit", "timeout", "network_error", "provider_5xx", "model_unavailable", "credential_error", "auth_failed", "auth_error", "permission_denied", "session_invalid", "account_locked", "account_banned", "abuse_detected", "device_unrecognized", "stream_interrupted", "empty_completion":
		return true
	default:
		return false
	}
}

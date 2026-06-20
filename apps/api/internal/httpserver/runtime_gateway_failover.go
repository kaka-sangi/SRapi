package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/platform/circuitbreaker"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/metacoerce"
)

// defaultGatewayFailoverAttempts is the fallback cross-candidate failover cap
// used when the operator-tunable AdminSettingsGateway.retry_count is unset or
// the settings read fails. Operators tune this via admin settings; per-account
// metadata still governs same-candidate retries on top of it.
const defaultGatewayFailoverAttempts = 3

const defaultGatewayMaxRetryIntervalMS = 2000

const maxGatewayProviderErrorMessageLength = 300

type gatewayNoAvailableDiagnostic struct {
	BodyExcerpt string
	Metadata    map[string]any
}

func gatewayNoAccountMessage(decision schedulercontract.Decision) string {
	counts := gatewaySchedulerRejectReasonCounts(decision, false)
	if len(counts) == 0 {
		return "no available account"
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s(%d)", k, counts[k]))
	}
	return fmt.Sprintf("no available account: %d candidate(s) rejected [%s]", decision.CandidateCount, strings.Join(parts, ", "))
}

func gatewayNoAvailableDiagnosticForDecision(decision schedulercontract.Decision) gatewayNoAvailableDiagnostic {
	counts := gatewaySchedulerRejectReasonCounts(decision, true)
	primaryReason, primaryCount := gatewayPrimaryRejectReason(counts)
	action := gatewayNoAvailableOperatorAction(primaryReason)
	body := map[string]any{
		"response_status":                 http.StatusServiceUnavailable,
		"scheduler_decision_id":           decision.ID,
		"scheduler_candidate_count":       decision.CandidateCount,
		"scheduler_rejected_count":        decision.RejectedCount,
		"scheduler_primary_reject_reason": primaryReason,
		"scheduler_primary_reject_count":  primaryCount,
		"scheduler_operator_action":       action,
	}
	if len(counts) > 0 {
		body["scheduler_reject_reason_counts"] = gatewayRejectReasonCountsMetadata(counts)
	}
	if rationale := strings.TrimSpace(decision.SelectionRationale); rationale != "" {
		body["scheduler_selection_rationale"] = rationale
	}
	raw, err := json.Marshal(body)
	if err != nil {
		raw = []byte(`{"response_status":503,"scheduler_operator_action":"inspect_scheduler_decision"}`)
	}
	return gatewayNoAvailableDiagnostic{
		BodyExcerpt: string(raw),
		Metadata:    body,
	}
}

func gatewayRejectReasonCountsMetadata(counts map[string]int) map[string]any {
	if len(counts) == 0 {
		return nil
	}
	out := make(map[string]any, len(counts))
	for key, value := range counts {
		out[key] = value
	}
	return out
}

func gatewaySchedulerRejectReasonCounts(decision schedulercontract.Decision, includeSynthetic bool) map[string]int {
	counts := map[string]int{}
	for _, raw := range decision.RejectReasons {
		reason := strings.TrimSpace(fmt.Sprint(raw))
		if reason == "" || reason == "<nil>" {
			continue
		}
		counts[reason]++
	}
	if includeSynthetic && len(counts) == 0 && decision.CandidateCount == 0 {
		counts["no_schedulable_candidates"] = 1
	}
	return counts
}

func gatewayPrimaryRejectReason(counts map[string]int) (string, int) {
	if len(counts) == 0 {
		return "unknown", 0
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	primary := keys[0]
	primaryCount := counts[primary]
	for _, key := range keys[1:] {
		if counts[key] > primaryCount {
			primary = key
			primaryCount = counts[key]
		}
	}
	return primary, primaryCount
}

func gatewayNoAvailableOperatorAction(reason string) string {
	base := strings.TrimSpace(reason)
	if idx := strings.Index(base, ":"); idx >= 0 {
		base = base[:idx]
	}
	switch base {
	case "capability_mismatch":
		return "check_model_capabilities_or_mapping"
	case "credential_invalid", "needs_reauth", "auth_error", "auth_failed":
		return "check_account_credentials"
	case "quota_exhausted", "quota_protected":
		return "refresh_or_recover_quota"
	case "rate_limited", "rpm_limit_exceeded", "tpm_limit_exceeded", "concurrency_full", "cooldown_active":
		return "wait_or_reduce_load"
	case "circuit_open":
		return "recover_account_or_inspect_upstream"
	case "fallback_excluded":
		return "inspect_previous_attempts"
	case "lower_priority_tier":
		return "inspect_priority_strategy"
	case "user_balance_insufficient":
		return "check_user_balance"
	case "no_schedulable_candidates":
		return "check_provider_account_scope"
	default:
		return "inspect_scheduler_decision"
	}
}

// gatewayRetrySettings is the resolved, per-request snapshot of the
// operator-tunable failover/retry policy. It is read once at the top of the
// failover loop so the hot path never re-reads admin settings mid-flight.
type gatewayRetrySettings struct {
	// MaxAttempts caps the number of cross-candidate gateway attempts.
	MaxAttempts int
	// MaxRetryCredentials caps how many distinct credentials the loop may
	// exclude and retry across. 0 means unlimited (bounded only by MaxAttempts
	// and available candidates).
	MaxRetryCredentials int
	// MaxRetryIntervalMS is the default ceiling for same-candidate retry backoff
	// when per-account metadata does not override it.
	MaxRetryIntervalMS int
}

// resolveGatewayRetrySettings loads the operator-tunable failover policy,
// falling back to the historical hardcoded defaults when admin settings are
// unavailable so the hot path keeps behaving identically.
func (rt *runtimeState) resolveGatewayRetrySettings(ctx context.Context) gatewayRetrySettings {
	settings := gatewayRetrySettings{
		MaxAttempts:         defaultGatewayFailoverAttempts,
		MaxRetryCredentials: 0,
		MaxRetryIntervalMS:  defaultGatewayMaxRetryIntervalMS,
	}
	if rt == nil || rt.adminControl == nil {
		return settings
	}
	adminSettings, err := rt.adminControl.GetAdminSettings(ctx)
	if err != nil {
		if rt.logger != nil {
			rt.logger.Warn("failed to load gateway retry settings; using defaults", "error", err)
		}
		return settings
	}
	gateway := adminSettings.Gateway
	if gateway.RetryCount > 0 {
		settings.MaxAttempts = gateway.RetryCount
	}
	if gateway.MaxRetryCredentials > 0 {
		settings.MaxRetryCredentials = gateway.MaxRetryCredentials
	}
	if gateway.MaxRetryIntervalMS > 0 {
		settings.MaxRetryIntervalMS = gateway.MaxRetryIntervalMS
	}
	return settings
}

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
	response := s.gatewayPublicErrorResponse(err, errorClass, upstreamStatus, candidate)
	s.forwardProviderErrorHeaders(w, err)
	setRetryAfterFromProviderError(w, err)
	writeGatewayError(w, response.Status, errorType, response.Message, errorClass)
}

func (s *Server) writeGeminiProviderGatewayError(w http.ResponseWriter, err error) {
	s.writeGeminiProviderGatewayErrorForCandidate(w, err, nil)
}

func (s *Server) writeGeminiProviderGatewayErrorForCandidate(w http.ResponseWriter, err error, candidate *schedulercontract.Candidate) {
	errorClass, upstreamStatus, _ := providerGatewayError(err)
	response := s.gatewayPublicErrorResponse(err, errorClass, upstreamStatus, candidate)
	s.forwardProviderErrorHeaders(w, err)
	setRetryAfterFromProviderError(w, err)
	writeGeminiGatewayError(w, response.Status, geminiStatusForGatewayErrorClass(errorClass, response.Status), response.Message)
}

// gatewayPublicMessage decides the caller-facing message. Global admin-managed
// error-passthrough rules take precedence; when no rule matches it falls back to
// the per-account / per-provider metadata behavior in gatewayProviderPublicMessage.
func (s *Server) gatewayPublicMessage(err error, errorClass string, upstreamStatus int, candidate *schedulercontract.Candidate) string {
	return s.gatewayPublicErrorResponse(err, errorClass, upstreamStatus, candidate).Message
}

type gatewayPublicErrorResponse struct {
	Status  int
	Message string
}

func (s *Server) gatewayPublicErrorResponse(err error, errorClass string, upstreamStatus int, candidate *schedulercontract.Candidate) gatewayPublicErrorResponse {
	response := gatewayPublicErrorResponse{
		Status:  providerGatewayHTTPStatus(upstreamStatus),
		Message: providerGatewayMessage(errorClass),
	}
	if s.runtime != nil && s.runtime.errorPassthrough != nil {
		raw := gatewayProviderErrorMessage(err)
		if resolution, matched := s.runtime.errorPassthrough.Resolve(context.Background(), errorClass, upstreamStatus, raw); matched {
			if resolution.ResponseStatus != nil {
				response.Status = *resolution.ResponseStatus
			}
			if customMessage := gatewayNormalizeProviderErrorMessage(resolution.CustomMessage); customMessage != "" && !gatewayProviderErrorMessageSensitive(customMessage) {
				if len([]rune(customMessage)) > maxGatewayProviderErrorMessageLength {
					customMessage = string([]rune(customMessage)[:maxGatewayProviderErrorMessageLength]) + "..."
				}
				response.Message = customMessage
				return response
			}
			if resolution.Action == errorpassthroughcontract.ActionExpose && raw != "" {
				response.Message = raw
				return response
			}
			return response
		}
	}
	return gatewayProviderPublicErrorResponse(err, errorClass, upstreamStatus, candidate)
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
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, gatewayNoAccountMessage(result.Decision), "no_available_account")
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
		writeGeminiGatewayError(w, http.StatusServiceUnavailable, "UNAVAILABLE", gatewayNoAccountMessage(result.Decision))
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

func gatewaySkippedScheduleResult(result schedulercontract.ScheduleResult, reason string) schedulercontract.ScheduleResult {
	reason = strings.TrimSpace(reason)
	if reason == "" || result.Candidate.Account.ID <= 0 {
		return result
	}
	if result.Decision.RejectReasons == nil {
		result.Decision.RejectReasons = map[string]any{}
	}
	result.Decision.RejectReasons["account_"+strconv.Itoa(result.Candidate.Account.ID)] = reason
	result.Decision.RejectedCount = len(result.Decision.RejectReasons)
	return result
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
			return s.runtime.invokeProviderConversation(ctx, providerConversationRequest(canonical, candidate, r))
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
			return s.runtime.invokeProviderTokenCount(ctx, providerTokenCountRequest(canonical, rawBody, candidate, r))
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
			return s.runtime.invokeProviderImageGeneration(ctx, providerImageGenerationRequest(canonical, candidate, r))
		})
}

func (s *Server) invokeProviderVideoWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
	operation provideradaptercontract.VideoOperation,
) gatewayFailoverResult[provideradaptercontract.VideoResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.VideoResponse, error) {
			return s.runtime.invokeProviderVideo(ctx, providerVideoRequest(canonical, candidate, operation, r))
		})
}

func (s *Server) invokeProviderVideoContentWithFailover(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	scheduleReq schedulercontract.ScheduleRequest,
	modelID int,
	forcedProviderKey string,
	admission gatewayAdmission,
	startedAt time.Time,
) gatewayFailoverResult[provideradaptercontract.VideoContentResponse] {
	return invokeGatewayCandidateWithFailover(s, ctx, r, authed, canonical, scheduleReq, modelID, forcedProviderKey, admission, startedAt,
		func(ctx context.Context, candidate schedulercontract.Candidate) (provideradaptercontract.VideoContentResponse, error) {
			return s.runtime.invokeProviderVideoContent(ctx, providerVideoRequest(canonical, candidate, provideradaptercontract.VideoOperationContent, r))
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
			return s.runtime.invokeProviderImageEdit(ctx, providerImageEditRequest(canonical, candidate, r))
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
			return s.runtime.invokeProviderImageVariation(ctx, providerImageVariationRequest(canonical, candidate, r))
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
			return s.runtime.invokeProviderResponseInputItems(ctx, providerResponseInputItemsRequest(canonical, responseID, query, candidate, r))
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
	ctx = withGatewayInboundClient(ctx, r)
	retrySettings := s.runtime.resolveGatewayRetrySettings(ctx)
	initialExcluded := append([]int(nil), scheduleReq.ExcludedAccountIDs...)
	excluded := append([]int(nil), initialExcluded...)
	var fallbackFromDecisionID *int
	var lastFailure gatewayFailoverResult[T]
	// upstreamErrors accumulates one entry per failed candidate attempt across
	// the whole failover loop so the persisted usage_log can carry the timeline
	// for the admin error-log surface (sub2api ops_upstream_error_events parity).
	upstreamErrors := make([]gatewayUpstreamErrorEvent, 0, retrySettings.MaxAttempts)

NextCandidate:
	for attemptNo := 1; attemptNo <= retrySettings.MaxAttempts; attemptNo++ {
		attemptReq := scheduleReq
		attemptReq.AttemptNo = attemptNo
		attemptReq.FallbackFromDecisionID = cloneIntPtr(fallbackFromDecisionID)
		attemptReq.ExcludedAccountIDs = append([]int(nil), excluded...)
		// Append accounts currently in the in-process 429 cooldown to the
		// excluded set so the scheduler skips them. The asynchronous
		// metadata write to cooldown_until is the source of truth — this
		// cooldown is a per-process synchronous accelerator.
		for _, id := range s.runtime.gatewayCooldownedAccountIDs() {
			attemptReq.ExcludedAccountIDs = append(attemptReq.ExcludedAccountIDs, id)
		}

		// The first selection waits briefly for a concurrency slot when every
		// account is saturated; failover attempts (after a real failure) schedule
		// immediately so total added latency stays bounded.
		var result schedulercontract.ScheduleResult
		var err error
		if attemptNo == 1 {
			result, err = s.runtime.scheduleGatewayRequestWaitingForSlot(ctx, attemptReq, modelID, forcedProviderKey, authed.Key)
		} else {
			result, err = s.runtime.scheduleGatewayRequest(ctx, attemptReq, modelID, forcedProviderKey, authed.Key)
		}
		if err != nil {
			if lastFailure.Err != nil {
				return lastFailure
			}
			return gatewayFailoverResult[T]{ScheduleResult: result, Err: err}
		}
		breaker := s.runtime.accountBreaker(result.Candidate.Account.ID)
		breakerDone, breakerErr := breaker.Allow()
		if errors.Is(breakerErr, circuitbreaker.ErrCircuitOpen) {
			s.runtime.logger.Info("circuit breaker open, skipping account",
				"request_id", canonical.RequestID,
				"account_id", result.Candidate.Account.ID,
				"attempt_no", attemptNo)
			s.runtime.releaseGatewaySchedulerLease(ctx, result, "circuit_open")
			excluded = append(excluded, result.Candidate.Account.ID)
			decisionID := result.Decision.ID
			fallbackFromDecisionID = &decisionID
			if !lastFailure.FailureRecorded {
				lastFailure = gatewayFailoverResult[T]{
					ScheduleResult: gatewaySkippedScheduleResult(result, "circuit_open"),
					Err:            errors.New("no available account"),
				}
			}
			continue
		}

		if err := s.reserveGatewayAccountQuotaForScheduledRequest(ctx, r, authed, canonical, result, admission, startedAt); err != nil {
			breakerDone(false)
			errorClass, upstreamStatus, _ := providerGatewayError(err)
			upstreamErrors = append(upstreamErrors, buildGatewayUpstreamErrorEvent(attemptNo, result, err, upstreamStatus))
			lastFailure = gatewayFailoverResult[T]{
				ScheduleResult:  result,
				Err:             err,
				FailureRecorded: true,
			}
			if !gatewayShouldFailover(errorClass, upstreamStatus, attemptNo, len(result.Candidates), retrySettings.MaxAttempts) ||
				gatewayRetryCredentialBudgetExhausted(retrySettings, excluded, initialExcluded) {
				return lastFailure
			}
			excluded = append(excluded, result.Candidate.Account.ID)
			decisionID := result.Decision.ID
			fallbackFromDecisionID = &decisionID
			continue
		}

		// Per-account in-process concurrency slot (sub2api parity). Acquired
		// AFTER scheduler+breaker pass so we don't waste a slot on a path
		// that would have been rejected anyway. Held for the full candidate
		// invoke including same-candidate retries; released exactly once
		// when this candidate either succeeds, gives up, or fails over.
		slotRelease, slotAcquired, slotErr := s.runtime.acquireGatewayAccountConcurrencySlot(ctx, result.Candidate.Account)
		if slotErr != nil {
			breakerDone(false)
			s.runtime.releaseGatewayAccountQuota(ctx, admission.EstimatedUsage, result.Candidate)
			s.runtime.releaseGatewaySchedulerLease(ctx, result, "concurrency_slot_failed")
			skippedResult := gatewaySkippedScheduleResult(result, "concurrency_full")
			if errIsConcurrencySlotTransient(slotErr) {
				// Treat as transient — failover to a different candidate.
				excluded = append(excluded, result.Candidate.Account.ID)
				decisionID := result.Decision.ID
				fallbackFromDecisionID = &decisionID
				lastFailure = gatewayFailoverResult[T]{ScheduleResult: skippedResult, Err: slotErr}
				continue
			}
			return gatewayFailoverResult[T]{ScheduleResult: skippedResult, Err: slotErr}
		}
		releaseConcurrencySlotOnce := func() {
			if slotAcquired && slotRelease != nil {
				slotRelease()
				slotRelease = nil
			}
		}

		retryPolicy := gatewaySameCandidateRetryPolicyFor(result.Candidate, retrySettings.MaxRetryIntervalMS)
		for sameCandidateRetries := 0; ; {
			response, err := invoke(ctx, result.Candidate)
			if err == nil {
				breakerDone(true)
				releaseConcurrencySlotOnce()
				s.runtime.bindGatewaySessionAffinity(ctx, scheduleReq.APIKeyID, scheduleReq.SessionAffinityKey, result.Candidate.Account.ID)
				if conversationResp, ok := any(response).(provideradaptercontract.ConversationResponse); ok {
					s.runtime.bindGatewayPreviousResponseAffinity(ctx, scheduleReq.APIKeyID, conversationResp.ID, result.Candidate.Account.ID)
				}
				return gatewayFailoverResult[T]{Response: response, ScheduleResult: result}
			}

			// On failure we feed the structured ClassifyUpstreamError into
			// the in-process cooldown — captures the Retry-After window so
			// subsequent attempts (across this request and the next) skip
			// the account immediately. Gated by per-account metadata flag.
			if errorClass, upstreamStatus, _ := providerGatewayError(err); errorClass != "" || upstreamStatus > 0 {
				decision := ClassifyUpstreamError(upstreamStatus, nil, err)
				if decision.Class == "transient" && decision.RetryAfterMs > 0 {
					s.runtime.recordGatewayAccountRateLimitCooldown(result.Candidate.Account, time.Duration(decision.RetryAfterMs)*time.Millisecond)
				} else if upstreamStatus == http.StatusTooManyRequests {
					// 429 without a Retry-After header still counts toward
					// the consecutive-disable threshold.
					s.runtime.recordGatewayAccountRateLimitCooldown(result.Candidate.Account, 0)
				}
			}

			errorClass, upstreamStatus, _ := providerGatewayError(err)
			if gatewayShouldRetrySameCandidate(retryPolicy, errorClass, upstreamStatus, sameCandidateRetries) {
				sameCandidateRetries++
				delay := gatewaySameCandidateRetryDelay(retryPolicy, sameCandidateRetries, err)
				s.runtime.logger.Info("retrying gateway provider candidate", "request_id", canonical.RequestID, "attempt_no", result.Decision.AttemptNo, "retry_no", sameCandidateRetries, "account_id", result.Candidate.Account.ID, "provider_id", result.Candidate.Provider.ID, "error_class", errorClass, "status_code", upstreamStatus, "delay_ms", int(delay/time.Millisecond))
				if err := sleepGatewayRetryDelay(ctx, delay); err != nil {
					breakerDone(false)
					releaseConcurrencySlotOnce()
					s.runtime.releaseGatewayAccountQuota(ctx, admission.EstimatedUsage, result.Candidate)
					upstreamErrors = append(upstreamErrors, buildGatewayUpstreamErrorEvent(attemptNo, result, err, upstreamStatus))
					s.recordGatewayProviderAttemptFailureWithHistory(r, authed, canonical, result, err, errorClass, upstreamStatus, elapsedMillis(startedAt), admission, upstreamErrors)
					return gatewayFailoverResult[T]{
						ScheduleResult:  result,
						Err:             err,
						FailureRecorded: true,
					}
				}
				continue
			}

			breakerDone(false)
			releaseConcurrencySlotOnce()
			s.runtime.releaseGatewayAccountQuota(ctx, admission.EstimatedUsage, result.Candidate)
			upstreamErrors = append(upstreamErrors, buildGatewayUpstreamErrorEvent(attemptNo, result, err, upstreamStatus))
			s.recordGatewayProviderAttemptFailureWithHistory(r, authed, canonical, result, err, errorClass, upstreamStatus, elapsedMillis(startedAt), admission, upstreamErrors)
			lastFailure = gatewayFailoverResult[T]{
				ScheduleResult:  result,
				Err:             err,
				FailureRecorded: true,
			}
			if !gatewayShouldFailover(errorClass, upstreamStatus, attemptNo, len(result.Candidates), retrySettings.MaxAttempts) ||
				gatewayRetryCredentialBudgetExhausted(retrySettings, excluded, initialExcluded) {
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
	s.recordGatewayProviderAttemptFailureWithHistory(r, authed, canonical, result, providerErr, errorClass, upstreamStatus, latencyMS, admission, nil)
}

// recordGatewayProviderAttemptFailureWithHistory is the variant that carries the
// cumulative per-attempt UpstreamErrorEvent timeline into the usage layer so the
// admin error-log surface can render the failover history of the request.
func (s *Server) recordGatewayProviderAttemptFailureWithHistory(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, providerErr error, errorClass string, upstreamStatus int, latencyMS int, admission gatewayAdmission, upstreamErrors []gatewayUpstreamErrorEvent) {
	// Mirror the same failure onto the in-memory error-event stream so the
	// admin SSE subscribers (Stream C / CLIProxyAPI SubscribeErrors port) see
	// it live without polling. Best-effort: a publish error is never allowed
	// to fail the request, and the underlying MemoryPublisher already swallows
	// per-subscriber overflows internally.
	s.publishErrorEvent(r.Context(), authed, canonical, result, providerErr, errorClass, upstreamStatus)
	headers := providerHeadersFromError(providerErr)
	upstreamRequestID := upstreamRequestIDFromHeaders(headers)
	phase := classifyErrorPhase(errorClass, upstreamStatus)
	owner := classifyErrorOwner(phase)
	source := classifyErrorSource(phase)
	rec := gatewayUsageRecord{
		RequestID:                canonical.RequestID,
		Authed:                   authed,
		DecisionID:               result.Decision.ID,
		AttemptNo:                result.Decision.AttemptNo,
		ProviderID:               ptrInt(result.Candidate.Provider.ID),
		AccountID:                ptrInt(result.Candidate.Account.ID),
		SourceProtocol:           string(canonical.SourceProtocol),
		SourceEndpoint:           canonical.SourceEndpoint,
		TargetProtocol:           result.Candidate.Provider.Protocol,
		Model:                    canonical.CanonicalModel,
		RequestedModel:           gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:            gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:                  false,
		ErrorClass:               ptrStringValue(errorClass),
		StatusCode:               ptrInt(upstreamStatus),
		LatencyMS:                latencyMS,
		InputTokens:              admission.EstimatedUsage.InputTokens,
		OutputTokens:             admission.EstimatedUsage.OutputTokens,
		CachedTokens:             admission.EstimatedUsage.CachedTokens,
		UsageEstimated:           true,
		Pricing:                  admission.Pricing,
		CompatibilityWarnings:    canonical.CompatibilityWarnings,
		ProviderQuotaSignals:     providerQuotaSignalsFromError(providerErr),
		ProviderRetryAfter:       providerRetryAfterFromError(providerErr),
		ProviderErrorMessage:     providerErrorMessage(providerErr),
		ProviderErrorBodyExcerpt: providerErrorBodyExcerpt(providerErr),
		Headers:                  headers,
		UpstreamRequestID:        upstreamRequestID,
		ErrorPhase:               phase,
		ErrorOwner:               owner,
		ErrorSource:              source,
		UpstreamErrors:           append([]gatewayUpstreamErrorEvent(nil), upstreamErrors...),
	}
	s.recordGatewaySystemLog(r.Context(), rec)
	s.recordOpsErrorLog(r.Context(), rec)
	s.runtime.recordGatewayUsage(r.Context(), rec)
}

// buildGatewayUpstreamErrorEvent assembles one history entry for the failing
// attempt. AccountID/Name come from the chosen candidate; status/request id come
// from the provider error headers; kind is "http_error" when a status was
// received, else "request_error" (transport failure before any HTTP response).
func buildGatewayUpstreamErrorEvent(attemptNo int, result schedulercontract.ScheduleResult, providerErr error, statusCode int) gatewayUpstreamErrorEvent {
	headers := providerHeadersFromError(providerErr)
	kind := "http_error"
	if statusCode == 0 {
		kind = "request_error"
	}
	var accountID *int
	if result.Candidate.Account.ID > 0 {
		accountID = ptrInt(result.Candidate.Account.ID)
	}
	return gatewayUpstreamErrorEvent{
		AtUnixMs:           time.Now().UTC().UnixMilli(),
		AttemptNo:          attemptNo,
		AccountID:          accountID,
		AccountName:        result.Candidate.Account.Name,
		UpstreamStatusCode: statusCode,
		UpstreamRequestID:  upstreamRequestIDFromHeaders(headers),
		UpstreamURL:        result.Candidate.Mapping.UpstreamModelName,
		Kind:               kind,
		Message:            providerErrorMessage(providerErr),
		BodyExcerpt:        providerErrorBodyExcerpt(providerErr),
	}
}

func (s *Server) recordGatewayNoAvailableAccount(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, admission gatewayAdmission, startedAt time.Time) {
	// Also surface the scheduler-side "no candidate" decision on the system-log
	// panel so operators see the reason (e.g. capability_mismatch:responses_compact)
	// alongside actual upstream rejections.
	diagnostic := gatewayNoAvailableDiagnosticForDecision(result.Decision)
	phase := classifyErrorPhase("no_available_account", 0)
	rec := gatewayUsageRecord{
		RequestID:                canonical.RequestID,
		Authed:                   authed,
		DecisionID:               result.Decision.ID,
		AttemptNo:                result.Decision.AttemptNo,
		SourceProtocol:           string(canonical.SourceProtocol),
		SourceEndpoint:           canonical.SourceEndpoint,
		TargetProtocol:           result.Decision.TargetProtocol,
		Model:                    canonical.CanonicalModel,
		RequestedModel:           gatewayRequestedModel(canonical),
		Success:                  false,
		ErrorClass:               ptrStringValue("no_available_account"),
		LatencyMS:                elapsedMillis(startedAt),
		InputTokens:              admission.EstimatedUsage.InputTokens,
		OutputTokens:             admission.EstimatedUsage.OutputTokens,
		CachedTokens:             admission.EstimatedUsage.CachedTokens,
		UsageEstimated:           true,
		Pricing:                  admission.Pricing,
		CompatibilityWarnings:    canonical.CompatibilityWarnings,
		ProviderErrorMessage:     gatewayNoAccountMessage(result.Decision),
		ProviderErrorBodyExcerpt: diagnostic.BodyExcerpt,
		ErrorPhase:               phase,
		ErrorOwner:               classifyErrorOwner(phase),
		ErrorSource:              classifyErrorSource(phase),
		DiagnosticMetadata:       diagnostic.Metadata,
	}
	s.recordGatewaySystemLog(r.Context(), rec)
	s.recordOpsErrorLog(r.Context(), rec)
	s.runtime.recordGatewayUsage(r.Context(), rec)
}

func providerRetryAfterFromError(err error) *time.Time {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) || providerErr.RetryAfter == nil {
		return nil
	}
	retryAfter := providerErr.RetryAfter.UTC()
	return &retryAfter
}

func (s *Server) forwardProviderErrorHeaders(w http.ResponseWriter, err error) {
	forwardProviderErrorHeaders(w, err, s.gatewayPassthroughHeaderConfig(context.Background()))
}

func forwardProviderErrorHeaders(w http.ResponseWriter, err error, cfg gatewayPassthroughHeaderConfig) {
	headers := providerHeadersFromError(err)
	if len(headers) == 0 {
		return
	}
	forwardUpstreamResponseHeaders(w, headers, cfg)
}

func providerHeadersFromError(err error) http.Header {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) || len(providerErr.Headers) == 0 {
		return nil
	}
	return cloneHTTPHeader(providerErr.Headers)
}

// codexUsageHeaderWindow is one Codex rate-limit window (primary or secondary)
// parsed from the x-codex-* response headers. valid is false when the upstream
// did not emit a used-percent value for that window.
type codexUsageHeaderWindow struct {
	usedPercent  float64
	resetSeconds *int
	windowMin    int
	valid        bool
}

type codexUsageWindowKind int

const (
	codexUsageWindow5h codexUsageWindowKind = iota
	codexUsageWindow7d
)

type codexUsageHeaderMapping struct {
	kind   codexUsageWindowKind
	window codexUsageHeaderWindow
}

// codexCooldownMetadataUpdates parses the x-codex-* rate-limit telemetry headers
// into the account-metadata fields used by the cooldown stage. It is a faithful
// port of sub2api's ParseCodexRateLimitHeaders + (*OpenAICodexUsageSnapshot).
// Normalize + buildCodexUsageExtraUpdates: it preserves the raw primary/secondary
// values for troubleshooting and normalizes them to the canonical 5h/7d fields by
// comparing window-minutes (smaller window = 5h, larger = 7d; classified by a
// <=360 minute threshold when only one window is known; legacy responses without
// window-minutes treat primary as 7d and secondary as 5h). It returns nil when no
// recognized header is present so the caller can skip the metadata write entirely.
func codexCooldownMetadataUpdates(headers http.Header, now time.Time) map[string]any {
	if headers == nil {
		return nil
	}
	primary := codexUsageHeaderWindowFromHeaders(headers, "x-codex-primary")
	secondary := codexUsageHeaderWindowFromHeaders(headers, "x-codex-secondary")
	overflow, hasOverflow := parseCodexHeaderFloat(headers, "x-codex-primary-over-secondary-limit-percent")

	if !primary.valid && !secondary.valid && !primaryWindowHasData(primary) && !primaryWindowHasData(secondary) && !hasOverflow {
		return nil
	}

	baseTime := now.UTC()
	updates := make(map[string]any)

	// Preserve the raw primary/secondary fields for troubleshooting.
	if primary.valid {
		updates["codex_primary_used_percent"] = primary.usedPercent
	}
	if primary.resetSeconds != nil {
		updates["codex_primary_reset_after_seconds"] = *primary.resetSeconds
	}
	if primary.windowMin > 0 {
		updates["codex_primary_window_minutes"] = primary.windowMin
	}
	if secondary.valid {
		updates["codex_secondary_used_percent"] = secondary.usedPercent
	}
	if secondary.resetSeconds != nil {
		updates["codex_secondary_reset_after_seconds"] = *secondary.resetSeconds
	}
	if secondary.windowMin > 0 {
		updates["codex_secondary_window_minutes"] = secondary.windowMin
	}
	if hasOverflow {
		updates["codex_primary_over_secondary_percent"] = overflow
	}
	if len(updates) == 0 {
		return nil
	}
	updates["codex_usage_updated_at"] = baseTime.Format(time.RFC3339)

	// Normalize to the canonical 5h/7d fields.
	for _, mapping := range codexUsageHeaderMappings(primary, secondary) {
		prefix := codexUsageWindowFieldPrefix(mapping.kind)
		if prefix == "" {
			continue
		}
		if mapping.window.valid {
			updates[prefix+"_used_percent"] = mapping.window.usedPercent
		}
		if mapping.window.resetSeconds != nil {
			updates[prefix+"_reset_after_seconds"] = *mapping.window.resetSeconds
			if resetAt := codexResetAtRFC3339(baseTime, mapping.window.resetSeconds); resetAt != "" {
				updates[prefix+"_reset_at"] = resetAt
			}
		}
		if mapping.window.windowMin > 0 {
			updates[prefix+"_window_minutes"] = mapping.window.windowMin
		}
	}

	return updates
}

func primaryWindowHasData(window codexUsageHeaderWindow) bool {
	return window.resetSeconds != nil || window.windowMin > 0
}

func codexUsageHeaderWindowFromHeaders(headers http.Header, prefix string) codexUsageHeaderWindow {
	window := codexUsageHeaderWindow{}
	if value, ok := parseCodexHeaderFloat(headers, prefix+"-used-percent"); ok {
		window.usedPercent = value
		window.valid = true
	}
	if value, ok := parseCodexHeaderInt(headers, prefix+"-reset-after-seconds"); ok {
		window.resetSeconds = &value
	}
	if value, ok := parseCodexHeaderInt(headers, prefix+"-window-minutes"); ok {
		window.windowMin = value
	}
	return window
}

// codexUsageHeaderMappings classifies the primary/secondary windows into the
// canonical 5h/7d slots, mirroring sub2api's Normalize() strategy.
func codexUsageHeaderMappings(primary codexUsageHeaderWindow, secondary codexUsageHeaderWindow) []codexUsageHeaderMapping {
	hasPrimaryWindow := primary.windowMin > 0
	hasSecondaryWindow := secondary.windowMin > 0

	switch {
	case hasPrimaryWindow && hasSecondaryWindow:
		// Both known: smaller window is 5h, larger is 7d.
		if primary.windowMin < secondary.windowMin {
			return []codexUsageHeaderMapping{{kind: codexUsageWindow5h, window: primary}, {kind: codexUsageWindow7d, window: secondary}}
		}
		return []codexUsageHeaderMapping{{kind: codexUsageWindow7d, window: primary}, {kind: codexUsageWindow5h, window: secondary}}
	case hasPrimaryWindow:
		// Only primary known: classify by the <=360 minute threshold.
		if primary.windowMin <= 360 {
			return []codexUsageHeaderMapping{{kind: codexUsageWindow5h, window: primary}, {kind: codexUsageWindow7d, window: secondary}}
		}
		return []codexUsageHeaderMapping{{kind: codexUsageWindow7d, window: primary}, {kind: codexUsageWindow5h, window: secondary}}
	case hasSecondaryWindow:
		// Only secondary known: classify by threshold; primary takes the opposite.
		if secondary.windowMin <= 360 {
			return []codexUsageHeaderMapping{{kind: codexUsageWindow7d, window: primary}, {kind: codexUsageWindow5h, window: secondary}}
		}
		return []codexUsageHeaderMapping{{kind: codexUsageWindow5h, window: primary}, {kind: codexUsageWindow7d, window: secondary}}
	default:
		// No window-minutes: legacy assumption (primary=7d, secondary=5h).
		return []codexUsageHeaderMapping{{kind: codexUsageWindow7d, window: primary}, {kind: codexUsageWindow5h, window: secondary}}
	}
}

func codexUsageWindowFieldPrefix(kind codexUsageWindowKind) string {
	switch kind {
	case codexUsageWindow5h:
		return "codex_5h"
	case codexUsageWindow7d:
		return "codex_7d"
	default:
		return ""
	}
}

func codexResetAtRFC3339(base time.Time, resetAfterSeconds *int) string {
	if resetAfterSeconds == nil {
		return ""
	}
	seconds := *resetAfterSeconds
	if seconds < 0 {
		seconds = 0
	}
	return base.Add(time.Duration(seconds) * time.Second).Format(time.RFC3339)
}

func parseCodexHeaderFloat(headers http.Header, key string) (float64, bool) {
	raw := strings.TrimSpace(headers.Get(key))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func parseCodexHeaderInt(headers http.Header, key string) (int, bool) {
	raw := strings.TrimSpace(headers.Get(key))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func setRetryAfterFromProviderError(w http.ResponseWriter, err error) {
	if w == nil || strings.TrimSpace(w.Header().Get("Retry-After")) != "" {
		return
	}
	retryAfter := providerRetryAfterFromError(err)
	if retryAfter == nil {
		return
	}
	delay := time.Until(*retryAfter)
	if delay <= 0 {
		return
	}
	seconds := int((delay + time.Second - time.Nanosecond) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func providerQuotaSignalsFromError(err error) []provideradaptercontract.QuotaSignal {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) || len(providerErr.QuotaSignals) == 0 {
		return nil
	}
	out := make([]provideradaptercontract.QuotaSignal, 0, len(providerErr.QuotaSignals))
	for _, signal := range providerErr.QuotaSignals {
		if signal.QuotaType == "" {
			continue
		}
		cloned := signal
		cloned.ResetAt = cloneTimePtr(signal.ResetAt)
		out = append(out, cloned)
	}
	return out
}

func providerErrorMessage(err error) string {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return ""
	}
	return strings.TrimSpace(providerErr.Message)
}

// providerErrorBodyExcerpt composes a compact upstream-error envelope that
// mirrors sub2api's ops_error_logs.upstream_error_detail field. The intent
// is to give operators the four facts they actually need when triaging an
// upstream rejection — class, status, type/code, message — in a single
// string the admin panel can render verbatim. Sensitive material in the
// raw response body is NOT included here; that path goes through the
// passthrough metadata gate (gatewayProviderErrorMessageEnabled). The
// result is truncated to providerErrorBodyExcerptMaxLength runes so it is
// safe to inline in lists.
const providerErrorBodyExcerptMaxLength = 2048

func providerErrorBodyExcerpt(err error) string {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return ""
	}
	parts := make([]string, 0, 4)
	if class := strings.TrimSpace(providerErr.Class); class != "" {
		parts = append(parts, "class="+class)
	}
	if providerErr.StatusCode > 0 {
		parts = append(parts, "status="+strconv.Itoa(providerErr.StatusCode))
	}
	if providerErr.Metadata != nil {
		if value := strings.TrimSpace(metadataString(providerErr.Metadata, "type")); value != "" {
			parts = append(parts, "type="+value)
		}
		if value := strings.TrimSpace(metadataString(providerErr.Metadata, "code")); value != "" {
			parts = append(parts, "code="+value)
		}
	}
	if message := strings.TrimSpace(providerErr.Message); message != "" {
		parts = append(parts, "message="+message)
	}
	excerpt := strings.Join(parts, " | ")
	if runes := []rune(excerpt); len(runes) > providerErrorBodyExcerptMaxLength {
		excerpt = string(runes[:providerErrorBodyExcerptMaxLength]) + "..."
	}
	return excerpt
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

func gatewayProviderErrorMetadataAllowed(metadata map[string]any, errorClass string, upstreamStatus int, message string) bool {
	if !gatewayProviderErrorMessageEnabled(metadata) {
		return false
	}
	if !gatewayProviderErrorMessageStatusAllowed(metadata, upstreamStatus) {
		return false
	}
	if !gatewayProviderErrorMessageClassAllowed(metadata, errorClass) {
		return false
	}
	if !gatewayProviderErrorMessageKeywordAllowed(metadata, message) {
		return false
	}
	return true
}

func gatewayProviderErrorMessageStatusAllowed(metadata map[string]any, upstreamStatus int) bool {
	value, ok := metacoerce.Value(metadata,
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
		if gatewayProviderErrorMetadataAllowed(metadata, errorClass, upstreamStatus, message) {
			return true
		}
	}
	return false
}

func gatewayProviderErrorMetadataStatus(metadata map[string]any, upstreamStatus int) (int, bool) {
	status := metadataInt(metadata,
		"provider_error_passthrough_response_code",
		"provider_error_response_code",
		"upstream_error_response_code",
		"error_passthrough_response_code",
	)
	if status >= 100 && status <= 599 {
		return status, true
	}
	for _, key := range []string{
		"provider_error_passthrough_code",
		"upstream_error_status_passthrough",
		"passthrough_provider_error_code",
		"error_passthrough_code",
	} {
		if metadataBool(metadata, key) && upstreamStatus >= 100 && upstreamStatus <= 599 {
			return upstreamStatus, true
		}
	}
	return 0, false
}

func gatewayProviderErrorMetadataMessage(metadata map[string]any) (string, bool) {
	for _, key := range []string{
		"provider_error_passthrough_message",
		"provider_error_passthrough_custom_message",
		"provider_error_custom_message",
		"upstream_error_custom_message",
		"error_passthrough_custom_message",
	} {
		message := gatewayNormalizeProviderErrorMessage(metadataString(metadata, key))
		if message == "" || gatewayProviderErrorMessageSensitive(message) {
			continue
		}
		if len([]rune(message)) <= maxGatewayProviderErrorMessageLength {
			return message, true
		}
		return string([]rune(message)[:maxGatewayProviderErrorMessageLength]) + "...", true
	}
	return "", false
}

func gatewayProviderPublicErrorResponse(err error, errorClass string, upstreamStatus int, candidate *schedulercontract.Candidate) gatewayPublicErrorResponse {
	response := gatewayPublicErrorResponse{
		Status:  providerGatewayHTTPStatus(upstreamStatus),
		Message: providerGatewayMessage(errorClass),
	}
	if candidate == nil {
		return response
	}
	message := gatewayProviderErrorMessage(err)
	for _, metadata := range gatewayProviderGatewayMetadata(*candidate) {
		if !gatewayProviderErrorMetadataAllowed(metadata, errorClass, upstreamStatus, message) {
			continue
		}
		if status, ok := gatewayProviderErrorMetadataStatus(metadata, upstreamStatus); ok {
			response.Status = status
		}
		if customMessage, ok := gatewayProviderErrorMetadataMessage(metadata); ok {
			response.Message = customMessage
		} else if message != "" {
			response.Message = message
		}
		return response
	}
	return response
}

func gatewayProviderPublicMessage(err error, errorClass string, upstreamStatus int, candidate *schedulercontract.Candidate) string {
	return gatewayProviderPublicErrorResponse(err, errorClass, upstreamStatus, candidate).Message
}

// gatewayRetryCredentialBudgetExhausted reports whether excluding one more
// credential would exceed the operator-tunable max_retry_credentials cap. A cap
// of 0 means unlimited (bounded only by MaxAttempts and available candidates).
// The number of credentials this request has already failed over across equals
// the credentials excluded since the loop began (entries beyond the scheduler's
// initial exclusion list).
func gatewayRetryCredentialBudgetExhausted(settings gatewayRetrySettings, excluded []int, initialExcluded []int) bool {
	if settings.MaxRetryCredentials <= 0 {
		return false
	}
	credentialsTried := len(excluded) - len(initialExcluded)
	return credentialsTried >= settings.MaxRetryCredentials
}

func gatewayShouldFailover(errorClass string, upstreamStatus int, attemptNo int, candidateCount int, maxAttempts int) bool {
	if attemptNo >= maxAttempts || candidateCount <= 1 {
		return false
	}
	if upstreamStatus == http.StatusTooManyRequests ||
		upstreamStatus == http.StatusGatewayTimeout ||
		upstreamStatus == http.StatusRequestTimeout ||
		upstreamStatus >= http.StatusInternalServerError {
		return true
	}
	switch errorClass {
	case "rate_limit", "quota_exhausted", "timeout", "network_error", "provider_5xx", "model_unavailable", "configuration_error", "credential_error", "auth_failed", "auth_error", "permission_denied", "session_invalid", "account_locked", "account_banned", "abuse_detected", "device_unrecognized", "stream_interrupted", "empty_completion", "invalid_response":
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

// gatewaySameCandidateRetryPolicyFor resolves the per-account same-candidate
// retry policy. The MaxDelay ceiling falls back to the operator-tunable
// defaultMaxRetryIntervalMS (AdminSettingsGateway.max_retry_interval_ms) when
// per-account metadata does not override it; metadata overrides always win.
func gatewaySameCandidateRetryPolicyFor(candidate schedulercontract.Candidate, defaultMaxRetryIntervalMS int) gatewaySameCandidateRetryPolicy {
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
		MaxDelay:          gatewayRetryDelay(metadata, gatewayDefaultMaxRetryInterval(defaultMaxRetryIntervalMS), "same_candidate_retry_max_delay_ms", "same_account_retry_max_delay_ms", "pool_mode_retry_max_delay_ms"),
		RetryAuthFailures: poolMode || metadataBool(metadata, "same_candidate_retry_auth_errors") || metadataBool(metadata, "same_account_retry_auth_errors"),
	}
}

// gatewayDefaultMaxRetryInterval converts the operator-tunable
// max_retry_interval_ms into a duration, falling back to the historical 2s
// ceiling when it is unset or non-positive so behavior stays identical.
func gatewayDefaultMaxRetryInterval(maxRetryIntervalMS int) time.Duration {
	if maxRetryIntervalMS <= 0 {
		return defaultGatewayRetryMaxDelay
	}
	return time.Duration(maxRetryIntervalMS) * time.Millisecond
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
	value, ok := metacoerce.Value(metadata, keys...)
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
	value, ok := metacoerce.Value(metadata, keys...)
	if !ok {
		return fallback
	}
	millis, ok := gatewayRetryCountValue(value)
	if !ok || millis < 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}

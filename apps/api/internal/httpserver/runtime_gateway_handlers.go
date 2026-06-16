package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}

	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to list models", "internal_error")
		return
	}
	hidden := s.runtime.modelsHiddenByExclusion(r.Context(), models, authed.Key)
	gatewayModels := toGatewayModelsWithAliases(r.Context(), s.runtime.models, models, hidden)
	gatewayModels = filterGatewayModels(gatewayModels, authed.Key.AllowedModels)
	if len(hidden) > 0 {
		gatewayModels = hideGatewayModels(gatewayModels, hidden)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpenAIModelList{
		Object: apiopenapi.OpenAIModelListObjectList,
		Data:   gatewayModels,
	})
	_ = requestID
}

func (s *Server) handleGatewayUsage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	days, ok := gatewayUsageDays(r)
	if !ok {
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, "days must be an integer between 1 and 90", "invalid_request")
		return
	}
	summary, err := s.runtime.usage.SummarizeAPIKey(r.Context(), authed.Key.ID, days)
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to load usage", "internal_error")
		return
	}
	user, err := s.runtime.users.FindByID(r.Context(), authed.UserID)
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to load usage", "internal_error")
		return
	}
	writeJSONAny(w, http.StatusOK, gatewayUsageResponse(authed.Key, user.User, summary))
}

func (s *Server) handleCreateChatCompletion(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/chat/completions")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.ChatCompletionRequest
	rawBody, err := s.decodeJSONBodyWithRaw(w, r, &body)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid chat completion request", "invalid_request")
		return
	}
	s.serveChatCompletion(w, r, authed, body, rawBody, sourceEndpoint, forcedProviderKey, startedAt)
}

// serveChatCompletion runs the gateway chat pipeline AFTER authentication: model
// resolution, per-key entitlement, admission (balance / quota / subscription),
// provider scheduling + failover, and streaming/metering+billing. It is shared
// by the API-key path (handleCreateChatCompletion) and the session-billed user
// playground (交界地) so both bill identically.
func (s *Server) serveChatCompletion(w http.ResponseWriter, r *http.Request, authed apikeycontract.AuthResult, body apiopenapi.ChatCompletionRequest, rawBody []byte, sourceEndpoint, forcedProviderKey string, startedAt time.Time) {
	requestID := requestIDFromContext(r.Context())
	modelSuffix := chatRequestModelSuffix(body)
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelSuffix.BaseModel)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(modelSuffix.RequestedModel),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical := s.runtime.gateway.NormalizeChatCompletions(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
		RawBody:        rawBody,
	})
	applyGatewayModelSuffix(&canonical, modelSuffix)
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderConversationWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	if providerResp.StreamBody != nil {
		s.writeConversationStreamPassthrough(w, r, authed, canonical, result, providerResp, admission, model.ID, startedAt)
		return
	}
	usage := gatewayUsageFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalConversationResponse(canonical, gatewayContentBlocksFromProvider(providerResp.Parts), gatewayStopReasonFromProvider(providerResp.StopReason), usage, providerResp.Warnings, providerResp.Raw, gatewayStreamEventsFromProvider(providerResp.StreamEvents))
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), canonicalResp.Usage.Estimated)
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
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		CacheCreationTokens:   canonicalResp.Usage.CacheCreationTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
		QualityPrompt:         gatewayTextForQuality(canonical),
		QualityOutput:         canonicalResp.Message,
	})
	if canonical.Stream {
		if sameProtocolRawConversationStream(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
			s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
			writeRawSSEResponse(w, canonicalResp.RawProviderMetadata)
			return
		}
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeSSEJSONChunks(w, s.runtime.gateway.RenderChatStreamChunks(canonicalResp))
		return
	}
	if sameProtocolRawConversationResponse(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeRawJSONResponse(w, http.StatusOK, canonicalResp.RawProviderMetadata)
		return
	}
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderChatCompletions(canonicalResp))
}

func (s *Server) handleCreateResponse(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/responses")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.ResponsesRequest
	rawBody, err := s.decodeJSONBodyWithRaw(w, r, &body)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid responses request", "invalid_request")
		return
	}
	if err := s.runtime.gateway.ValidateResponsesRequest(rawBody); err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(body.Model),
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, err.Error(), "invalid_request")
		return
	}
	modelSuffix := responsesRequestModelSuffix(body)
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelSuffix.BaseModel)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(modelSuffix.RequestedModel),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical := s.runtime.gateway.NormalizeResponses(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
		RawBody:        rawBody,
	})
	applyGatewayModelSuffix(&canonical, modelSuffix)
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderConversationWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	if providerResp.StreamBody != nil {
		s.writeConversationStreamPassthrough(w, r, authed, canonical, result, providerResp, admission, model.ID, startedAt)
		return
	}
	usage := gatewayUsageFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalConversationResponse(canonical, gatewayContentBlocksFromProvider(providerResp.Parts), gatewayStopReasonFromProvider(providerResp.StopReason), usage, providerResp.Warnings, providerResp.Raw, gatewayStreamEventsFromProvider(providerResp.StreamEvents))
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), canonicalResp.Usage.Estimated)
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
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		CacheCreationTokens:   canonicalResp.Usage.CacheCreationTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
		QualityPrompt:         gatewayTextForQuality(canonical),
		QualityOutput:         canonicalResp.Message,
	})
	response := s.runtime.gateway.RenderResponses(canonicalResp)
	if canonical.Stream {
		if sameProtocolRawConversationStream(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
			s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
			writeRawSSEResponse(w, canonicalResp.RawProviderMetadata)
			return
		}
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeSSEEvents(w, s.runtime.gateway.RenderResponsesStreamEvents(canonicalResp))
		return
	}
	if sameProtocolRawConversationResponse(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeRawJSONResponse(w, http.StatusOK, canonicalResp.RawProviderMetadata)
		return
	}
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, response)
}

func (s *Server) handleListResponseInputItems(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), string(gatewaycontract.EndpointResponseInputItems))
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	responseID := strings.TrimSpace(r.PathValue("response_id"))
	modelName := strings.TrimSpace(r.URL.Query().Get("model"))
	if responseID == "" || modelName == "" {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(modelName),
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, "response input_items requests require response_id and model", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelName)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(modelName),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical := s.runtime.gateway.NormalizeResponseInputItems(modelName, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	admission, err := s.runtime.prepareGatewayAdmissionWithoutContentSafety(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	admission.EstimatedUsage = gatewaycontract.Usage{}
	admission.Pricing = zeroGatewayPricing()
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderResponseInputItemsWithFailover(r.Context(), r, authed, canonical, responseID, r.URL.Query(), scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
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
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		UsageEstimated:        false,
		Pricing:               zeroGatewayPricing(),
		CompatibilityWarnings: canonical.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
	})
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeRawJSONResponse(w, providerResp.StatusCode, providerResp.Raw)
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/messages")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.AnthropicMessagesRequest
	rawBody, err := s.decodeJSONBodyWithRaw(w, r, &body)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "anthropic-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid messages request", "invalid_request")
		return
	}
	modelSuffix := gatewayModelSuffixFromModel(body.Model)
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelSuffix.BaseModel)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "anthropic-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(modelSuffix.RequestedModel),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "anthropic-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical := s.runtime.gateway.NormalizeAnthropicMessages(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
		RawBody:        rawBody,
	})
	applyGatewayModelSuffix(&canonical, modelSuffix)
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderConversationWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	if providerResp.StreamBody != nil {
		s.writeConversationStreamPassthrough(w, r, authed, canonical, result, providerResp, admission, model.ID, startedAt)
		return
	}
	usage := gatewayUsageFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalConversationResponse(canonical, gatewayContentBlocksFromProvider(providerResp.Parts), gatewayStopReasonFromProvider(providerResp.StopReason), usage, providerResp.Warnings, providerResp.Raw, gatewayStreamEventsFromProvider(providerResp.StreamEvents))
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), canonicalResp.Usage.Estimated)
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
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		CacheCreationTokens:   canonicalResp.Usage.CacheCreationTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
		QualityPrompt:         gatewayTextForQuality(canonical),
		QualityOutput:         canonicalResp.Message,
	})
	response := s.runtime.gateway.RenderAnthropicMessages(canonicalResp)
	if canonical.Stream {
		if sameProtocolRawConversationStream(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
			s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
			writeRawSSEResponse(w, canonicalResp.RawProviderMetadata)
			return
		}
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		// Anthropic streams terminate with message_stop; do not append the
		// OpenAI-only [DONE] sentinel.
		writeSSEEventsNoDone(w, s.runtime.gateway.RenderAnthropicMessagesStreamEvents(canonicalResp))
		return
	}
	if sameProtocolRawConversationResponse(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeRawJSONResponse(w, http.StatusOK, canonicalResp.RawProviderMetadata)
		return
	}
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, response)
}

func (s *Server) handleCreateEmbedding(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/embeddings")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.EmbeddingRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid embeddings request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), body.Model)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(body.Model),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical, err := s.runtime.gateway.NormalizeEmbeddings(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, err.Error(), "invalid_request")
		return
	}
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderEmbeddingsWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	usage := gatewayUsageFromEmbeddingProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalEmbeddingResponse(canonical, gatewayEmbeddingsFromProvider(providerResp), usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), canonicalResp.Usage.Estimated)
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
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		CacheCreationTokens:   canonicalResp.Usage.CacheCreationTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
	})
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderEmbeddings(canonicalResp))
}

func (s *Server) handleGeminiModelAction(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseGeminiCountTokens(r.URL.Path); ok {
		s.handleGeminiCountTokens(w, r)
		return
	}
	requestID := requestIDFromContext(r.Context())
	modelRef, stream, ok := parseGeminiModelAction(r.URL.Path)
	if !ok {
		writeGeminiGatewayError(w, http.StatusNotFound, "NOT_FOUND", "Gemini route not found")
		return
	}
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), r.URL.Path)
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGeminiGatewayKey(r)
	if err != nil {
		writeGeminiGatewayAuthError(w, err)
		return
	}
	var body apiopenapi.GeminiGenerateContentRequest
	rawBody, err := s.decodeJSONBodyWithRaw(w, r, &body)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: string(gatewaycontract.ProtocolGeminiCompatible),
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(modelRef),
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGeminiGatewayError(w, jsonDecodeStatus(err), "INVALID_ARGUMENT", "invalid generateContent request")
		return
	}
	modelSuffix := gatewayModelSuffixFromModel(modelRef)
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelSuffix.BaseModel)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: string(gatewaycontract.ProtocolGeminiCompatible),
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(modelSuffix.RequestedModel),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGeminiGatewayError(w, http.StatusNotFound, "NOT_FOUND", "model not found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: string(gatewaycontract.ProtocolGeminiCompatible),
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGeminiGatewayError(w, http.StatusForbidden, "PERMISSION_DENIED", "model not allowed for this api key")
		return
	}
	canonical := s.runtime.gateway.NormalizeGeminiGenerateContent(body, modelRef, stream, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
		RawBody:        rawBody,
	})
	applyGatewayModelSuffix(&canonical, modelSuffix)
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGeminiGatewayError(w, http.StatusInternalServerError, "INTERNAL", "failed to check gateway entitlement")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGeminiGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), geminiStatusForGatewayErrorClass(errorClass, gatewayEntitlementHTTPStatus(errorClass)), gatewayEntitlementMessage(errorClass))
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderConversationWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGeminiGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	if providerResp.StreamBody != nil {
		s.writeConversationStreamPassthrough(w, r, authed, canonical, result, providerResp, admission, model.ID, startedAt)
		return
	}
	usage := gatewayUsageFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalConversationResponse(canonical, gatewayContentBlocksFromProvider(providerResp.Parts), gatewayStopReasonFromProvider(providerResp.StopReason), usage, providerResp.Warnings, providerResp.Raw, gatewayStreamEventsFromProvider(providerResp.StreamEvents))
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), canonicalResp.Usage.Estimated)
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
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		CacheCreationTokens:   canonicalResp.Usage.CacheCreationTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
		QualityPrompt:         gatewayTextForQuality(canonical),
		QualityOutput:         canonicalResp.Message,
	})
	if canonical.Stream {
		if sameProtocolRawConversationStream(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
			s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
			writeRawSSEResponse(w, canonicalResp.RawProviderMetadata)
			return
		}
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeSSEEvents(w, s.runtime.gateway.RenderGeminiGenerateContentStreamEvents(canonicalResp))
		return
	}
	if sameProtocolRawConversationResponse(canonical, result.Candidate.Provider.Protocol, result.Candidate.Provider.AdapterType, result.Candidate.Provider.Name, result.Candidate.Provider.ConfigSchema, result.Candidate.Provider.Capabilities, result.Candidate.Account.Metadata, canonicalResp.RawProviderMetadata) {
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeRawJSONResponse(w, http.StatusOK, canonicalResp.RawProviderMetadata)
		return
	}
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderGeminiGenerateContent(canonicalResp))
}

func sameProtocolRawConversationResponse(req gatewaycontract.CanonicalRequest, targetProtocol, adapterType, providerName string, providerConfig, providerCapabilities, accountMetadata map[string]any, raw []byte) bool {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false
	}
	if req.Stream {
		return false
	}
	sourceProtocol := strings.ToLower(strings.TrimSpace(string(req.SourceProtocol)))
	targetProtocol = strings.ToLower(strings.TrimSpace(targetProtocol))
	adapterType = strings.ToLower(strings.TrimSpace(adapterType))
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	sourceEndpoint := strings.ToLower(strings.TrimSpace(req.SourceEndpoint))
	if sourceProtocol == "" || sourceProtocol != targetProtocol {
		return false
	}
	switch sourceProtocol {
	case string(gatewaycontract.ProtocolOpenAICompatible):
		if strings.HasSuffix(sourceEndpoint, "/responses/compact") {
			return adapterType == "openai-compatible" ||
				adapterType == "native-openai" ||
				adapterType == "reverse-proxy-openai-compatible" ||
				adapterType == "reverse-proxy-codex-cli"
		}
		if strings.HasSuffix(sourceEndpoint, "/responses") {
			return openAIResponsesRawPassthroughEnabled(adapterType, providerName, providerConfig, providerCapabilities, accountMetadata)
		}
		return strings.HasSuffix(sourceEndpoint, "/chat/completions") &&
			(adapterType == "openai-compatible" || adapterType == "reverse-proxy-openai-compatible")
	case string(gatewaycontract.ProtocolAnthropicCompatible):
		return strings.HasSuffix(sourceEndpoint, "/messages") &&
			(adapterType == "anthropic-compatible" || adapterType == "reverse-proxy-claude-code-cli")
	case string(gatewaycontract.ProtocolGeminiCompatible):
		return (strings.Contains(sourceEndpoint, ":generatecontent") || strings.Contains(sourceEndpoint, ":streamgeneratecontent")) &&
			(adapterType == "gemini-compatible" || adapterType == "native-gemini" || adapterType == "reverse-proxy-gemini-cli")
	default:
		return false
	}
}

func sameProtocolRawConversationStream(req gatewaycontract.CanonicalRequest, targetProtocol, adapterType, providerName string, providerConfig, providerCapabilities, accountMetadata map[string]any, raw []byte) bool {
	if !req.Stream || !looksLikeSSE(raw) {
		return false
	}
	sourceProtocol := strings.ToLower(strings.TrimSpace(string(req.SourceProtocol)))
	targetProtocol = strings.ToLower(strings.TrimSpace(targetProtocol))
	adapterType = strings.ToLower(strings.TrimSpace(adapterType))
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	sourceEndpoint := strings.ToLower(strings.TrimSpace(req.SourceEndpoint))
	if sourceProtocol == "" || sourceProtocol != targetProtocol {
		return false
	}
	switch sourceProtocol {
	case string(gatewaycontract.ProtocolOpenAICompatible):
		if strings.HasSuffix(sourceEndpoint, "/responses/compact") {
			return adapterType == "reverse-proxy-codex-cli"
		}
		if strings.HasSuffix(sourceEndpoint, "/responses") {
			return adapterType == "reverse-proxy-codex-cli" ||
				openAIResponsesRawPassthroughEnabled(adapterType, providerName, providerConfig, providerCapabilities, accountMetadata)
		}
		return strings.HasSuffix(sourceEndpoint, "/chat/completions") &&
			(adapterType == "openai-compatible" || adapterType == "reverse-proxy-openai-compatible")
	case string(gatewaycontract.ProtocolAnthropicCompatible):
		return strings.HasSuffix(sourceEndpoint, "/messages") &&
			(adapterType == "anthropic-compatible" || adapterType == "reverse-proxy-claude-code-cli")
	case string(gatewaycontract.ProtocolGeminiCompatible):
		return strings.Contains(sourceEndpoint, ":streamgeneratecontent") &&
			(adapterType == "gemini-compatible" || adapterType == "native-gemini" || adapterType == "reverse-proxy-gemini-cli")
	default:
		return false
	}
}

func openAIResponsesRawPassthroughEnabled(adapterType, providerName string, providerConfig, providerCapabilities, accountMetadata map[string]any) bool {
	if adapterType == "native-openai" || providerName == "openai" {
		return true
	}
	for _, values := range []map[string]any{accountMetadata, providerConfig, providerCapabilities} {
		if metadataBool(values, "native_responses") ||
			metadataBool(values, "responses_native") ||
			metadataBool(values, "responses_passthrough") ||
			metadataBool(values, "openai_responses_passthrough") {
			return true
		}
	}
	return false
}

func looksLikeSSE(raw []byte) bool {
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("data:")) {
			return true
		}
	}
	return false
}

func writeRawJSONResponse(w http.ResponseWriter, status int, raw []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(bytes.TrimSpace(raw))
}

func (s *Server) handleGeminiCountTokens(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	modelRef, ok := parseGeminiCountTokens(r.URL.Path)
	if !ok {
		writeGeminiGatewayError(w, http.StatusNotFound, "NOT_FOUND", "Gemini route not found")
		return
	}
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), r.URL.Path)
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGeminiGatewayKey(r)
	if err != nil {
		writeGeminiGatewayAuthError(w, err)
		return
	}
	rawBody, err := readTokenCountBody(r)
	if err != nil || len(bytes.TrimSpace(rawBody)) == 0 {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolGeminiCompatible), modelRef, "invalid_request", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGeminiGatewayError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid countTokens request")
		return
	}
	var body apiopenapi.GeminiCountTokensRequest
	if err := json.Unmarshal(rawBody, &body); err != nil {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolGeminiCompatible), modelRef, "invalid_request", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGeminiGatewayError(w, jsonDecodeStatus(err), "INVALID_ARGUMENT", "invalid countTokens request")
		return
	}
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelRef)
	if err != nil {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolGeminiCompatible), modelRef, "model_not_found", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGeminiGatewayError(w, http.StatusNotFound, "NOT_FOUND", "model not found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolGeminiCompatible), model.CanonicalName, "model_not_allowed", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGeminiGatewayError(w, http.StatusForbidden, "PERMISSION_DENIED", "model not allowed for this api key")
		return
	}
	canonical, err := s.runtime.gateway.NormalizeGeminiCountTokens(body, modelRef, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	if err != nil {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolGeminiCompatible), model.CanonicalName, "invalid_request", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGeminiGatewayError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.recordTokenCountFailure(r, authed, canonical.RequestID, canonical.SourceEndpoint, string(canonical.SourceProtocol), canonical.CanonicalModel, "entitlement_check_failed", elapsedMillis(startedAt), &canonical, admission)
		writeGeminiGatewayError(w, http.StatusInternalServerError, "INTERNAL", "failed to check gateway entitlement")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.recordTokenCountFailure(r, authed, canonical.RequestID, canonical.SourceEndpoint, string(canonical.SourceProtocol), canonical.CanonicalModel, errorClass, elapsedMillis(startedAt), &canonical, admission)
		writeGeminiGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), geminiStatusForGatewayErrorClass(errorClass, gatewayEntitlementHTTPStatus(errorClass)), gatewayEntitlementMessage(errorClass))
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderTokenCountWithFailover(r.Context(), r, authed, canonical, rawBody, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGeminiGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	tokenCount := gatewayTokenCountFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalTokenCountResponse(canonical, tokenCount.TotalTokens, tokenCount.CachedContentTokenCount, tokenCount.PromptTokensDetails, tokenCount.CacheTokensDetails, tokenCount.Metadata)
	s.recordTokenCountSuccess(r, authed, canonical, result, canonicalResp, elapsedMillis(startedAt), providerResp.QuotaSignals)
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderGeminiCountTokens(canonicalResp))
}

func zeroGatewayPricing() gatewayPricingEvidence {
	return gatewayPricingEvidence{Amount: "0.00000000", Currency: "USD", PricingSource: "token_count_no_charge", PricingEstimated: false}
}

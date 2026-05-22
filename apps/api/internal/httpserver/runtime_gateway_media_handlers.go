package httpserver

import (
	"net/http"
	"time"

	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleCreateImageGeneration(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/images/generations")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.ImageGenerationRequest
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
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid image generation request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), body.Model)
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
	canonical, err := s.runtime.gateway.NormalizeImageGeneration(body, gatewayservice.RequestMeta{
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
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), canonical, modelResolution, model.ID)
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
	result, err := s.runtime.scheduleGatewayRequest(r.Context(), scheduleReq, model.ID, forcedProviderKey, authed.Key)
	if err != nil {
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
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available account", "no_available_account")
		return
	}
	providerResp, err := s.runtime.invokeProviderImageGeneration(r.Context(), providerImageGenerationRequest(canonical, result.Candidate))
	if err != nil {
		errorClass, upstreamStatus, errorType := providerGatewayError(err)
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
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, providerGatewayMessage(errorClass), errorClass)
		return
	}
	usage := gatewayUsageFromImageProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalImageGenerationResponse(canonical, gatewayImagesFromProvider(providerResp), providerResp.Created, usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequest(model.ID, result.Candidate, canonicalResp.Usage), canonicalResp.Usage.Estimated)
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
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
	})
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderImageGeneration(canonicalResp))
}

func (s *Server) handleCreateModeration(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/moderations")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.ModerationRequest
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
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid moderation request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), body.Model)
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
	canonical, err := s.runtime.gateway.NormalizeModerations(body, gatewayservice.RequestMeta{
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
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), canonical, modelResolution, model.ID)
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
	result, err := s.runtime.scheduleGatewayRequest(r.Context(), scheduleReq, model.ID, forcedProviderKey, authed.Key)
	if err != nil {
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
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available account", "no_available_account")
		return
	}
	providerResp, err := s.runtime.invokeProviderModerations(r.Context(), providerModerationRequest(canonical, result.Candidate))
	if err != nil {
		errorClass, upstreamStatus, errorType := providerGatewayError(err)
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
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, providerGatewayMessage(errorClass), errorClass)
		return
	}
	usage := gatewayUsageFromModerationProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalModerationResponse(canonical, providerResp.ID, gatewayModerationResultsFromProvider(providerResp), usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequest(model.ID, result.Candidate, canonicalResp.Usage), canonicalResp.Usage.Estimated)
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
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
	})
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderModerations(canonicalResp))
}

package httpserver

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
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

func (s *Server) handleCreateImageEdit(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/images/edits")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	body, imageMeta, err := s.decodeImageEditMultipart(w, r)
	if err != nil {
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
		writeGatewayError(w, imageEditDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid image edit request", "invalid_request")
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
	canonical, err := s.runtime.gateway.NormalizeImageEdit(body, gatewayservice.RequestMeta{
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
	applyImageEditMultipartMetadata(&canonical, imageMeta)
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
	providerResp, err := s.runtime.invokeProviderImageEdit(r.Context(), providerImageEditRequest(canonical, result.Candidate))
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

func (s *Server) handleCreateImageVariation(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/images/variations")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	body, imageContentType, err := s.decodeImageVariationMultipart(w, r)
	if err != nil {
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
		writeGatewayError(w, imageEditDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid image variation request", "invalid_request")
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
	canonical, err := s.runtime.gateway.NormalizeImageVariation(body, gatewayservice.RequestMeta{
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
	if len(canonical.ImageInputs) > 0 {
		canonical.ImageInputs[0].ContentType = imageContentType
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
	providerResp, err := s.runtime.invokeProviderImageVariation(r.Context(), providerImageVariationRequest(canonical, result.Candidate))
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

type imageEditMultipartMetadata struct {
	ImageContentTypes []string
	MaskContentType   string
}

func (s *Server) decodeImageEditMultipart(w http.ResponseWriter, r *http.Request) (apiopenapi.ImageEditRequest, imageEditMultipartMetadata, error) {
	limited := http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize)
	r.Body = limited
	if err := r.ParseMultipartForm(s.cfg.Gateway.MaxBodySize); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return apiopenapi.ImageEditRequest{}, imageEditMultipartMetadata{}, errRequestTooLarge
		}
		return apiopenapi.ImageEditRequest{}, imageEditMultipartMetadata{}, err
	}
	imageHeaders := append([]*multipart.FileHeader(nil), r.MultipartForm.File["image"]...)
	imageHeaders = append(imageHeaders, r.MultipartForm.File["image[]"]...)
	images := make([]openapi_types.File, 0, len(imageHeaders))
	meta := imageEditMultipartMetadata{ImageContentTypes: make([]string, 0, len(imageHeaders))}
	for _, header := range imageHeaders {
		var file openapi_types.File
		file.InitFromMultipart(header)
		images = append(images, file)
		meta.ImageContentTypes = append(meta.ImageContentTypes, strings.TrimSpace(header.Header.Get("Content-Type")))
	}
	body := apiopenapi.ImageEditRequest{
		Image:                images,
		Model:                strings.TrimSpace(r.FormValue("model")),
		Prompt:               strings.TrimSpace(r.FormValue("prompt")),
		N:                    optionalFormInt(r, "n"),
		Size:                 optionalFormString(r, "size"),
		Quality:              optionalFormString(r, "quality"),
		ResponseFormat:       optionalImageEditResponseFormat(r.FormValue("response_format")),
		OutputFormat:         optionalFormString(r, "output_format"),
		OutputCompression:    optionalFormInt(r, "output_compression"),
		Background:           optionalFormString(r, "background"),
		Moderation:           optionalFormString(r, "moderation"),
		InputFidelity:        optionalFormString(r, "input_fidelity"),
		Stream:               optionalFormBool(r, "stream"),
		PartialImages:        optionalFormInt(r, "partial_images"),
		User:                 optionalFormString(r, "user"),
		AdditionalProperties: imageEditAdditionalProperties(r),
	}
	if maskHeaders := r.MultipartForm.File["mask"]; len(maskHeaders) > 0 {
		var mask openapi_types.File
		mask.InitFromMultipart(maskHeaders[0])
		body.Mask = &mask
		meta.MaskContentType = strings.TrimSpace(maskHeaders[0].Header.Get("Content-Type"))
	}
	return body, meta, nil
}

func applyImageEditMultipartMetadata(canonical *gatewaycontract.CanonicalRequest, meta imageEditMultipartMetadata) {
	for idx := range canonical.ImageInputs {
		if idx < len(meta.ImageContentTypes) {
			canonical.ImageInputs[idx].ContentType = meta.ImageContentTypes[idx]
		}
	}
	if canonical.ImageMask != nil {
		canonical.ImageMask.ContentType = meta.MaskContentType
	}
}

func (s *Server) decodeImageVariationMultipart(w http.ResponseWriter, r *http.Request) (apiopenapi.ImageVariationRequest, string, error) {
	limited := http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize)
	r.Body = limited
	if err := r.ParseMultipartForm(s.cfg.Gateway.MaxBodySize); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return apiopenapi.ImageVariationRequest{}, "", errRequestTooLarge
		}
		return apiopenapi.ImageVariationRequest{}, "", err
	}
	imageHeaders := r.MultipartForm.File["image"]
	var image openapi_types.File
	contentType := ""
	if len(imageHeaders) > 0 {
		image.InitFromMultipart(imageHeaders[0])
		contentType = strings.TrimSpace(imageHeaders[0].Header.Get("Content-Type"))
	}
	body := apiopenapi.ImageVariationRequest{
		Image:                image,
		Model:                strings.TrimSpace(r.FormValue("model")),
		N:                    optionalFormInt(r, "n"),
		Size:                 optionalImageVariationSize(r.FormValue("size")),
		ResponseFormat:       optionalImageVariationResponseFormat(r.FormValue("response_format")),
		User:                 optionalFormString(r, "user"),
		AdditionalProperties: imageVariationAdditionalProperties(r),
	}
	return body, contentType, nil
}

func imageEditDecodeStatus(err error) int {
	if errors.Is(err, errRequestTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func optionalImageEditResponseFormat(value string) *apiopenapi.ImageEditRequestResponseFormat {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	format := apiopenapi.ImageEditRequestResponseFormat(value)
	return &format
}

func optionalImageVariationResponseFormat(value string) *apiopenapi.ImageVariationRequestResponseFormat {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	format := apiopenapi.ImageVariationRequestResponseFormat(value)
	return &format
}

func optionalImageVariationSize(value string) *apiopenapi.ImageVariationRequestSize {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	size := apiopenapi.ImageVariationRequestSize(value)
	return &size
}

func optionalFormInt(r *http.Request, key string) *int {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		invalid := -1
		return &invalid
	}
	return &parsed
}

func optionalFormBool(r *http.Request, key string) *bool {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		invalid := false
		return &invalid
	}
	return &parsed
}

func imageEditAdditionalProperties(r *http.Request) map[string]interface{} {
	if r.MultipartForm == nil {
		return nil
	}
	known := map[string]struct{}{
		"image":              {},
		"image[]":            {},
		"mask":               {},
		"model":              {},
		"prompt":             {},
		"n":                  {},
		"size":               {},
		"quality":            {},
		"response_format":    {},
		"output_format":      {},
		"output_compression": {},
		"background":         {},
		"moderation":         {},
		"input_fidelity":     {},
		"stream":             {},
		"partial_images":     {},
		"user":               {},
	}
	out := map[string]interface{}{}
	for key, values := range r.MultipartForm.Value {
		if _, ok := known[key]; ok || len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			out[key] = values[0]
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func imageVariationAdditionalProperties(r *http.Request) map[string]interface{} {
	if r.MultipartForm == nil {
		return nil
	}
	known := map[string]struct{}{
		"image":           {},
		"model":           {},
		"n":               {},
		"size":            {},
		"response_format": {},
		"user":            {},
	}
	out := map[string]interface{}{}
	for key, values := range r.MultipartForm.Value {
		if _, ok := known[key]; ok || len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			out[key] = values[0]
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func (s *Server) handleCreateRerank(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/rerank")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.RerankRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "rerank-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid rerank request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), body.Model)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "rerank-compatible",
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
			SourceProtocol: "rerank-compatible",
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
	canonical, err := s.runtime.gateway.NormalizeRerank(body, gatewayservice.RequestMeta{
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
			SourceProtocol: "rerank-compatible",
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
	providerResp, err := s.runtime.invokeProviderRerank(r.Context(), providerRerankRequest(canonical, result.Candidate))
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
	usage := gatewayUsageFromRerankProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalRerankResponse(canonical, providerResp.ID, gatewayRerankResultsFromProvider(providerResp), usage)
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
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderRerank(canonicalResp))
}

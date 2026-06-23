package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
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
			UsageEstimated: false,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid image generation request", "invalid_request")
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated:        false,
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
			InputTokens:           0,
			OutputTokens:          0,
			CachedTokens:          0,
			UsageEstimated:        false,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderImageGenerationWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	if providerResp.StreamBody != nil {
		s.writeImageGenerationStreamPassthrough(w, r, authed, canonical, result, providerResp, admission, model.ID, startedAt)
		return
	}
	usage := gatewayUsageFromImageProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalImageGenerationResponse(canonical, gatewayImagesFromProvider(providerResp), providerResp.Created, usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), ptrInt(result.Candidate.Account.ID), canonicalResp.Usage.Estimated)
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
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
	})
	if canonical.ImageStream {
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeSSEEvents(w, s.runtime.gateway.RenderImageGenerationStreamEvents(canonicalResp))
		return
	}
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
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
	body, imageMeta, err := s.decodeImageEditRequest(w, r)
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
			UsageEstimated: false,
		})
		writeGatewayError(w, imageRequestDecodeStatus(err), apiopenapi.InvalidRequestError, imageEditDecodeMessage(err), "invalid_request")
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated: false,
		})
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, err.Error(), "invalid_request")
		return
	}
	applyImageEditMultipartMetadata(&canonical, imageMeta)
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
			UsageEstimated:        false,
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
			InputTokens:           0,
			OutputTokens:          0,
			CachedTokens:          0,
			UsageEstimated:        false,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderImageEditWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	if providerResp.StreamBody != nil {
		s.writeImageGenerationStreamPassthrough(w, r, authed, canonical, result, providerResp, admission, model.ID, startedAt)
		return
	}
	usage := gatewayUsageFromImageProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalImageGenerationResponse(canonical, gatewayImagesFromProvider(providerResp), providerResp.Created, usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), ptrInt(result.Candidate.Account.ID), canonicalResp.Usage.Estimated)
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
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
	})
	streamRequested := body.Stream != nil && *body.Stream
	if streamRequested {
		s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
		writeSSEEvents(w, s.runtime.gateway.RenderImageGenerationStreamEvents(canonicalResp))
		return
	}
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
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
	body, imageContentType, err := s.decodeImageVariationRequest(w, r)
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
			UsageEstimated: false,
		})
		writeGatewayError(w, imageRequestDecodeStatus(err), apiopenapi.InvalidRequestError, imageVariationDecodeMessage(err), "invalid_request")
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated: false,
		})
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, err.Error(), "invalid_request")
		return
	}
	if len(canonical.ImageInputs) > 0 {
		canonical.ImageInputs[0].ContentType = imageContentType
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
			UsageEstimated:        false,
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
			InputTokens:           0,
			OutputTokens:          0,
			CachedTokens:          0,
			UsageEstimated:        false,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderImageVariationWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	usage := gatewayUsageFromImageProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalImageGenerationResponse(canonical, gatewayImagesFromProvider(providerResp), providerResp.Created, usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), ptrInt(result.Candidate.Account.ID), canonicalResp.Usage.Estimated)
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
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
	})
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderImageGeneration(canonicalResp))
}

type imageEditMultipartMetadata struct {
	ImageContentTypes []string
	MaskContentType   string
}

func (s *Server) decodeImageEditRequest(w http.ResponseWriter, r *http.Request) (apiopenapi.ImageEditRequest, imageEditMultipartMetadata, error) {
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return s.decodeImageEditJSON(w, r)
	}
	return s.decodeImageEditMultipart(w, r)
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

func (s *Server) decodeImageEditJSON(w http.ResponseWriter, r *http.Request) (apiopenapi.ImageEditRequest, imageEditMultipartMetadata, error) {
	var raw imageEditJSONRequest
	if err := s.decodeJSONBody(w, r, &raw); err != nil {
		return apiopenapi.ImageEditRequest{}, imageEditMultipartMetadata{}, err
	}
	refs := append([]json.RawMessage(nil), raw.Images...)
	if len(raw.Image) > 0 && string(raw.Image) != "null" {
		refs = append([]json.RawMessage{raw.Image}, refs...)
	}
	images, contentTypes, err := imageFilesFromJSONReferences(refs)
	if err != nil {
		return apiopenapi.ImageEditRequest{}, imageEditMultipartMetadata{}, err
	}
	var mask *openapi_types.File
	maskContentType := ""
	if len(raw.Mask) > 0 && string(raw.Mask) != "null" {
		maskFile, contentType, err := imageFileFromJSONReference(raw.Mask, 1)
		if err != nil {
			return apiopenapi.ImageEditRequest{}, imageEditMultipartMetadata{}, err
		}
		mask = &maskFile
		maskContentType = contentType
	}
	body := apiopenapi.ImageEditRequest{
		Image:                images,
		Mask:                 mask,
		Model:                strings.TrimSpace(raw.Model),
		Prompt:               strings.TrimSpace(raw.Prompt),
		N:                    raw.N,
		Size:                 optionalJSONString(raw.Size),
		Quality:              optionalJSONString(raw.Quality),
		ResponseFormat:       optionalImageEditJSONResponseFormat(raw.ResponseFormat),
		OutputFormat:         optionalJSONString(raw.OutputFormat),
		OutputCompression:    raw.OutputCompression,
		Background:           optionalJSONString(raw.Background),
		Moderation:           optionalJSONString(raw.Moderation),
		InputFidelity:        optionalJSONString(raw.InputFidelity),
		Stream:               raw.Stream,
		PartialImages:        raw.PartialImages,
		User:                 optionalJSONString(raw.User),
		AdditionalProperties: imageEditJSONAdditionalProperties(raw.AdditionalProperties),
	}
	return body, imageEditMultipartMetadata{ImageContentTypes: contentTypes, MaskContentType: maskContentType}, nil
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

type imageEditJSONRequest struct {
	Image                json.RawMessage        `json:"image"`
	Images               []json.RawMessage      `json:"images"`
	Mask                 json.RawMessage        `json:"mask"`
	Model                string                 `json:"model"`
	Prompt               string                 `json:"prompt"`
	N                    *int                   `json:"n"`
	Size                 string                 `json:"size"`
	Quality              string                 `json:"quality"`
	ResponseFormat       string                 `json:"response_format"`
	OutputFormat         string                 `json:"output_format"`
	OutputCompression    *int                   `json:"output_compression"`
	Background           string                 `json:"background"`
	Moderation           string                 `json:"moderation"`
	InputFidelity        string                 `json:"input_fidelity"`
	Stream               *bool                  `json:"stream"`
	PartialImages        *int                   `json:"partial_images"`
	User                 string                 `json:"user"`
	AdditionalProperties map[string]interface{} `json:"-"`
}

func (r *imageEditJSONRequest) UnmarshalJSON(data []byte) error {
	type alias imageEditJSONRequest
	var known alias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var all map[string]interface{}
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for _, key := range []string{"image", "images", "mask", "model", "prompt", "n", "size", "quality", "response_format", "output_format", "output_compression", "background", "moderation", "input_fidelity", "stream", "partial_images", "user"} {
		delete(all, key)
	}
	*r = imageEditJSONRequest(known)
	if len(all) > 0 {
		r.AdditionalProperties = all
	}
	return nil
}

func imageFilesFromJSONReferences(refs []json.RawMessage) ([]openapi_types.File, []string, error) {
	if len(refs) == 0 {
		return nil, nil, fmt.Errorf("image file is required")
	}
	images := make([]openapi_types.File, 0, len(refs))
	contentTypes := make([]string, 0, len(refs))
	for idx, raw := range refs {
		image, contentType, err := imageFileFromJSONReference(raw, idx+1)
		if err != nil {
			return nil, nil, err
		}
		images = append(images, image)
		contentTypes = append(contentTypes, contentType)
	}
	return images, contentTypes, nil
}

func imageFileFromJSONReference(raw json.RawMessage, index int) (openapi_types.File, string, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return imageFileFromDataURL(asString, "", index)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return openapi_types.File{}, "", fmt.Errorf("image reference is invalid")
	}
	if _, ok := obj["file_id"]; ok {
		return openapi_types.File{}, "", fmt.Errorf("file_id image references are not supported")
	}
	filename := jsonRawString(obj["filename"])
	mimeType := jsonRawString(obj["mime_type"])
	if b64Value := jsonRawString(obj["b64_json"]); b64Value != "" {
		return imageFileFromBase64(b64Value, mimeType, filename, index)
	}
	if imageURLRaw, ok := obj["image_url"]; ok {
		imageURL, err := imageURLFromJSONReference(imageURLRaw)
		if err != nil {
			return openapi_types.File{}, "", err
		}
		return imageFileFromDataURL(imageURL, filename, index)
	}
	return openapi_types.File{}, "", fmt.Errorf("image reference is unsupported")
}

func imageURLFromJSONReference(raw json.RawMessage) (string, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("image_url reference is invalid")
	}
	urlValue := jsonRawString(obj["url"])
	if urlValue == "" {
		return "", fmt.Errorf("image_url reference is invalid")
	}
	return urlValue, nil
}

func imageFileFromDataURL(value, filename string, index int) (openapi_types.File, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return openapi_types.File{}, "", fmt.Errorf("image reference is empty")
	}
	if !strings.HasPrefix(value, "data:") {
		if parsed, err := url.Parse(value); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			return openapi_types.File{}, "", fmt.Errorf("remote image URLs are not supported")
		}
		return openapi_types.File{}, "", fmt.Errorf("image_url must be a data URL")
	}
	header, encoded, ok := strings.Cut(value, ",")
	if !ok || !strings.Contains(header, ";base64") {
		return openapi_types.File{}, "", fmt.Errorf("image data URL must be base64 encoded")
	}
	mimeType := strings.TrimSpace(strings.Split(strings.TrimPrefix(header, "data:"), ";")[0])
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return imageFileFromBase64(encoded, mimeType, filename, index)
}

func imageFileFromBase64(value, mimeType, filename string, index int) (openapi_types.File, string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return openapi_types.File{}, "", fmt.Errorf("image reference base64 is invalid")
	}
	if len(data) == 0 {
		return openapi_types.File{}, "", fmt.Errorf("image reference is empty")
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	filename = imageJSONReferenceFilename(filename, mimeType, index)
	var file openapi_types.File
	file.InitFromBytes(data, filename)
	return file, mimeType, nil
}

func imageJSONReferenceFilename(filename, mimeType string, index int) string {
	filename = strings.TrimSpace(filename)
	if filename != "" {
		return filepath.Base(filename)
	}
	ext := ".bin"
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		ext = ".png"
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	}
	return fmt.Sprintf("image_%d%s", index, ext)
}

func jsonRawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func optionalJSONString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalImageEditJSONResponseFormat(value string) *apiopenapi.ImageEditRequestResponseFormat {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	format := apiopenapi.ImageEditRequestResponseFormat(value)
	return &format
}

func imageEditJSONAdditionalProperties(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for key, value := range values {
		if key == "" || value == nil {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Server) decodeImageVariationRequest(w http.ResponseWriter, r *http.Request) (apiopenapi.ImageVariationRequest, string, error) {
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return s.decodeImageVariationJSON(w, r)
	}
	return s.decodeImageVariationMultipart(w, r)
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

type imageVariationJSONRequest struct {
	Image                json.RawMessage        `json:"image"`
	Images               []json.RawMessage      `json:"images"`
	Model                string                 `json:"model"`
	N                    *int                   `json:"n"`
	Size                 string                 `json:"size"`
	ResponseFormat       string                 `json:"response_format"`
	User                 string                 `json:"user"`
	AdditionalProperties map[string]interface{} `json:"-"`
}

func (r *imageVariationJSONRequest) UnmarshalJSON(data []byte) error {
	type alias imageVariationJSONRequest
	var known alias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var all map[string]interface{}
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for _, key := range []string{"image", "images", "model", "n", "size", "response_format", "user"} {
		delete(all, key)
	}
	*r = imageVariationJSONRequest(known)
	if len(all) > 0 {
		r.AdditionalProperties = all
	}
	return nil
}

func (s *Server) decodeImageVariationJSON(w http.ResponseWriter, r *http.Request) (apiopenapi.ImageVariationRequest, string, error) {
	var raw imageVariationJSONRequest
	if err := s.decodeJSONBody(w, r, &raw); err != nil {
		return apiopenapi.ImageVariationRequest{}, "", err
	}
	refs := append([]json.RawMessage(nil), raw.Images...)
	if len(raw.Image) > 0 && string(raw.Image) != "null" {
		refs = append([]json.RawMessage{raw.Image}, refs...)
	}
	if len(refs) > 1 {
		return apiopenapi.ImageVariationRequest{}, "", fmt.Errorf("image variation accepts exactly one image reference")
	}
	images, contentTypes, err := imageFilesFromJSONReferences(refs)
	if err != nil {
		return apiopenapi.ImageVariationRequest{}, "", err
	}
	body := apiopenapi.ImageVariationRequest{
		Image:                images[0],
		Model:                strings.TrimSpace(raw.Model),
		N:                    raw.N,
		Size:                 optionalImageVariationSize(raw.Size),
		ResponseFormat:       optionalImageVariationResponseFormat(raw.ResponseFormat),
		User:                 optionalJSONString(raw.User),
		AdditionalProperties: imageEditJSONAdditionalProperties(raw.AdditionalProperties),
	}
	return body, contentTypes[0], nil
}

func imageRequestDecodeStatus(err error) int {
	if errors.Is(err, errRequestTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func imageEditDecodeMessage(err error) string {
	if err == nil {
		return "invalid image edit request"
	}
	if errors.Is(err, errRequestTooLarge) {
		return "image edit request is too large"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "invalid image edit request"
	}
	return message
}

func imageVariationDecodeMessage(err error) string {
	if err == nil {
		return "invalid image variation request"
	}
	if errors.Is(err, errRequestTooLarge) {
		return "image variation request is too large"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "invalid image variation request"
	}
	return message
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
			UsageEstimated: false,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid moderation request", "invalid_request")
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated:        false,
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
			InputTokens:           0,
			OutputTokens:          0,
			CachedTokens:          0,
			UsageEstimated:        false,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderModerationsWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	usage := gatewayUsageFromModerationProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalModerationResponse(canonical, providerResp.ID, gatewayModerationResultsFromProvider(providerResp), usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), ptrInt(result.Candidate.Account.ID), canonicalResp.Usage.Estimated)
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
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
	})
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
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
			UsageEstimated: false,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid rerank request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), body.Model)
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated: false,
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
			UsageEstimated:        false,
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
			InputTokens:           0,
			OutputTokens:          0,
			CachedTokens:          0,
			UsageEstimated:        false,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderRerankWithFailover(r.Context(), r, authed, canonical, scheduleReq, model.ID, forcedProviderKey, admission, startedAt)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, admission, startedAt)
		return
	}
	providerResp := failover.Response
	usage := gatewayUsageFromRerankProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalRerankResponse(canonical, providerResp.ID, gatewayRerankResultsFromProvider(providerResp), usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(model.ID, result.Candidate, canonical, canonicalResp.Usage), ptrInt(result.Candidate.Account.ID), canonicalResp.Usage.Estimated)
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
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
		ProviderQuotaSignals:  providerResp.QuotaSignals,
	})
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderRerank(canonicalResp))
}

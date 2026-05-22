package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleCreateAudioTranscription(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/audio/transcriptions")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	body, contentType, err := s.decodeAudioTranscriptionMultipart(w, r)
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
		writeGatewayError(w, audioTranscriptionDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid audio transcription request", "invalid_request")
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
	canonical, err := s.runtime.gateway.NormalizeAudioTranscription(body, gatewayservice.RequestMeta{
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
	canonical.AudioContentType = contentType
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
	providerResp, err := s.runtime.invokeProviderAudioTranscription(r.Context(), providerAudioTranscriptionRequest(canonical, result.Candidate))
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
	usage := gatewayUsageFromAudioTranscriptionProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalAudioTranscriptionResponse(canonical, providerResp.ID, providerResp.Text, providerResp.Task, providerResp.Language, providerResp.Duration, gatewayAudioTranscriptionSegmentsFromProvider(providerResp), usage)
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
	writeAudioTranscriptionResponse(w, canonical.AudioResponseFormat, s.runtime.gateway.RenderAudioTranscription(canonicalResp))
}

func (s *Server) decodeAudioTranscriptionMultipart(w http.ResponseWriter, r *http.Request) (apiopenapi.AudioTranscriptionRequest, string, error) {
	limited := http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize)
	r.Body = limited
	if err := r.ParseMultipartForm(s.cfg.Gateway.MaxBodySize); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return apiopenapi.AudioTranscriptionRequest{}, "", errRequestTooLarge
		}
		return apiopenapi.AudioTranscriptionRequest{}, "", err
	}
	fileHeaders := r.MultipartForm.File["file"]
	if len(fileHeaders) == 0 {
		return apiopenapi.AudioTranscriptionRequest{Model: strings.TrimSpace(r.FormValue("model"))}, "", errors.New("audio file missing")
	}
	var file openapi_types.File
	file.InitFromMultipart(fileHeaders[0])
	body := apiopenapi.AudioTranscriptionRequest{
		File:                 file,
		Model:                strings.TrimSpace(r.FormValue("model")),
		Language:             optionalFormString(r, "language"),
		Prompt:               optionalFormString(r, "prompt"),
		ResponseFormat:       optionalAudioTranscriptionResponseFormat(r.FormValue("response_format")),
		Temperature:          optionalFormFloat32(r, "temperature"),
		User:                 optionalFormString(r, "user"),
		AdditionalProperties: audioTranscriptionAdditionalProperties(r),
	}
	contentType := strings.TrimSpace(fileHeaders[0].Header.Get("Content-Type"))
	return body, contentType, nil
}

func audioTranscriptionDecodeStatus(err error) int {
	if errors.Is(err, errRequestTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func optionalFormString(r *http.Request, key string) *string {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil
	}
	return &value
}

func optionalAudioTranscriptionResponseFormat(value string) *apiopenapi.AudioTranscriptionRequestResponseFormat {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	format := apiopenapi.AudioTranscriptionRequestResponseFormat(value)
	return &format
}

func optionalFormFloat32(r *http.Request, key string) *float32 {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil {
		invalid := float32(-1)
		return &invalid
	}
	out := float32(parsed)
	return &out
}

func audioTranscriptionAdditionalProperties(r *http.Request) map[string]interface{} {
	if r.MultipartForm == nil {
		return nil
	}
	known := map[string]struct{}{
		"file":            {},
		"model":           {},
		"language":        {},
		"prompt":          {},
		"response_format": {},
		"temperature":     {},
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

func writeAudioTranscriptionResponse(w http.ResponseWriter, responseFormat string, body apiopenapi.AudioTranscriptionResponse) {
	switch strings.TrimSpace(responseFormat) {
	case "text", "srt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body.Text))
	case "vtt":
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body.Text))
	default:
		writeJSONAny(w, http.StatusOK, body)
	}
}

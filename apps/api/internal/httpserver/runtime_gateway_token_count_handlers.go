package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/messages/count_tokens")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	rawBody, err := readTokenCountBody(r)
	if err != nil || len(bytes.TrimSpace(rawBody)) == 0 {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolAnthropicCompatible), "unknown", "invalid_request", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, "invalid count_tokens request", "invalid_request")
		return
	}
	var body apiopenapi.AnthropicCountTokensRequest
	if err := json.Unmarshal(rawBody, &body); err != nil {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolAnthropicCompatible), fallbackModelName(body.Model), "invalid_request", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid count_tokens request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), body.Model)
	if err != nil {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolAnthropicCompatible), fallbackModelName(body.Model), "model_not_found", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolAnthropicCompatible), model.CanonicalName, "model_not_allowed", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical, err := s.runtime.gateway.NormalizeAnthropicCountTokens(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	if err != nil {
		s.recordTokenCountFailure(r, authed, requestID, sourceEndpoint, string(gatewaycontract.ProtocolAnthropicCompatible), model.CanonicalName, "invalid_request", elapsedMillis(startedAt), nil, gatewayAdmission{})
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, err.Error(), "invalid_request")
		return
	}
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), canonical, modelResolution, model.ID)
	if err != nil {
		s.recordTokenCountFailure(r, authed, canonical.RequestID, canonical.SourceEndpoint, string(canonical.SourceProtocol), canonical.CanonicalModel, "entitlement_check_failed", elapsedMillis(startedAt), &canonical, admission)
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.recordTokenCountFailure(r, authed, canonical.RequestID, canonical.SourceEndpoint, string(canonical.SourceProtocol), canonical.CanonicalModel, errorClass, elapsedMillis(startedAt), &canonical, admission)
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	result, err := s.runtime.scheduleGatewayRequest(r.Context(), scheduleReq, model.ID, forcedProviderKey, authed.Key)
	if err != nil {
		s.recordTokenCountScheduled(r, authed, canonical, result, nil, "no_available_account", http.StatusServiceUnavailable, elapsedMillis(startedAt), admission)
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available account", "no_available_account")
		return
	}
	providerResp, err := s.runtime.invokeProviderTokenCount(r.Context(), providerTokenCountRequest(canonical, rawBody, result.Candidate))
	if err != nil {
		errorClass, upstreamStatus, errorType := providerGatewayError(err)
		s.recordTokenCountScheduled(r, authed, canonical, result, &result.Candidate, errorClass, upstreamStatus, elapsedMillis(startedAt), admission)
		writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, providerGatewayMessage(errorClass), errorClass)
		return
	}
	tokenCount := gatewayTokenCountFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalTokenCountResponse(canonical, tokenCount.TotalTokens, tokenCount.CachedContentTokenCount, tokenCount.PromptTokensDetails, tokenCount.CacheTokensDetails, tokenCount.Metadata)
	s.recordTokenCountSuccess(r, authed, canonical, result, canonicalResp, elapsedMillis(startedAt))
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderAnthropicCountTokens(canonicalResp))
}

func readTokenCountBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 4<<20))
}

func (s *Server) recordTokenCountFailure(r *http.Request, authed apikeycontract.AuthResult, requestID string, sourceEndpoint string, sourceProtocol string, model string, errorClass string, latencyMS int, canonical *gatewaycontract.CanonicalRequest, admission gatewayAdmission) {
	rec := gatewayUsageRecord{
		RequestID:      requestID,
		Authed:         authed,
		SourceProtocol: sourceProtocol,
		SourceEndpoint: sourceEndpoint,
		Model:          fallbackModelName(model),
		Success:        false,
		ErrorClass:     ptrStringValue(errorClass),
		LatencyMS:      latencyMS,
		UsageEstimated: true,
		Pricing:        admission.Pricing,
	}
	if canonical != nil {
		rec.SourceProtocol = string(canonical.SourceProtocol)
		rec.SourceEndpoint = canonical.SourceEndpoint
		rec.Model = canonical.CanonicalModel
		rec.InputTokens = admission.EstimatedUsage.InputTokens
		rec.OutputTokens = admission.EstimatedUsage.OutputTokens
		rec.CachedTokens = admission.EstimatedUsage.CachedTokens
		rec.CompatibilityWarnings = canonical.CompatibilityWarnings
	}
	s.runtime.recordGatewayUsage(r.Context(), rec)
}

func (s *Server) recordTokenCountScheduled(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, candidate *schedulercontract.Candidate, errorClass string, statusCode int, latencyMS int, admission gatewayAdmission) {
	rec := gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		Model:                 canonical.CanonicalModel,
		Success:               false,
		ErrorClass:            ptrStringValue(errorClass),
		StatusCode:            ptrInt(statusCode),
		LatencyMS:             latencyMS,
		InputTokens:           admission.EstimatedUsage.InputTokens,
		OutputTokens:          admission.EstimatedUsage.OutputTokens,
		CachedTokens:          admission.EstimatedUsage.CachedTokens,
		UsageEstimated:        true,
		Pricing:               admission.Pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
	}
	if candidate != nil {
		rec.ProviderID = ptrInt(candidate.Provider.ID)
		rec.AccountID = ptrInt(candidate.Account.ID)
		rec.TargetProtocol = candidate.Provider.Protocol
	}
	s.runtime.recordGatewayUsage(r.Context(), rec)
}

func (s *Server) recordTokenCountSuccess(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, resp gatewaycontract.TokenCountResponse, latencyMS int) {
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
		LatencyMS:             latencyMS,
		UsageEstimated:        false,
		Pricing:               zeroGatewayPricing(),
		CompatibilityWarnings: resp.CompatibilityWarnings,
	})
}

package httpserver

import (
	"io"
	"net/http"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const defaultVideoGatewayModel = "sora-2"

func (s *Server) handleCreateVideo(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), string(gatewaycontract.EndpointVideos))
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.VideoCreateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, fallbackModelName(body.Model), "invalid_request", http.StatusBadRequest, startedAt)
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid video request", "invalid_request")
		return
	}
	prepared, ok := s.prepareVideoGatewayRequest(w, r, authed, body.Model, requestID, sourceEndpoint, startedAt)
	if !ok {
		return
	}
	canonical, err := s.runtime.gateway.NormalizeVideoCreate(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: prepared.Model.CanonicalName,
	})
	if err != nil {
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, prepared.Model.CanonicalName, "invalid_request", http.StatusBadRequest, startedAt)
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, err.Error(), "invalid_request")
		return
	}
	failover := s.invokeProviderVideoWithFailover(r.Context(), r, authed, canonical, prepared.ScheduleReq, prepared.Model.ID, forcedProviderKey, prepared.Admission, startedAt, provideradaptercontract.VideoOperationCreate)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, prepared.Admission, startedAt)
		return
	}
	providerResp := failover.Response
	usage := gatewayUsageFromVideoProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalVideoResponse(canonical, gatewayVideoFromProvider(providerResp), usage)
	if canonicalResp.ID != "" {
		s.runtime.bindGatewayVideoResultAffinity(r.Context(), authed.Key.ID, canonicalResp.ID, result.Candidate.Account.ID)
	}
	s.recordVideoGatewaySuccess(r, authed, canonical, result, prepared.Model.ID, canonicalResp.Usage, providerResp.QuotaSignals, startedAt, http.StatusOK)
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderVideo(canonicalResp))
}

func (s *Server) handleRetrieveVideo(w http.ResponseWriter, r *http.Request) {
	s.handleVideoRead(w, r, provideradaptercontract.VideoOperationRetrieve)
}

func (s *Server) handleDownloadVideoContent(w http.ResponseWriter, r *http.Request) {
	s.handleVideoRead(w, r, provideradaptercontract.VideoOperationContent)
}

func (s *Server) handleVideoRead(w http.ResponseWriter, r *http.Request, operation provideradaptercontract.VideoOperation) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := string(gatewaycontract.EndpointVideoRetrieve)
	if operation == provideradaptercontract.VideoOperationContent {
		sourceEndpoint = string(gatewaycontract.EndpointVideoContent)
	}
	sourceEndpoint = gatewaySourceEndpoint(r.Context(), sourceEndpoint)
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	videoID := strings.TrimSpace(r.PathValue("video_id"))
	if videoID == "" {
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, defaultVideoGatewayModel, "invalid_request", http.StatusBadRequest, startedAt)
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, "video id is required", "invalid_request")
		return
	}
	boundAccountID, ok := s.runtime.lookupGatewayVideoResultAffinity(r.Context(), authed.Key.ID, videoID)
	if !ok {
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, defaultVideoGatewayModel, "video_binding_not_found", http.StatusNotFound, startedAt)
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "video binding not found or expired", "video_binding_not_found")
		return
	}
	modelRef := strings.TrimSpace(r.URL.Query().Get("model"))
	if modelRef == "" {
		modelRef = defaultVideoGatewayModel
	}
	prepared, ok := s.prepareVideoGatewayRequest(w, r, authed, modelRef, requestID, sourceEndpoint, startedAt)
	if !ok {
		return
	}
	canonical := gatewaycontract.CanonicalRequest{
		RequestID:           requestID,
		SourceProtocol:      gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint:      sourceEndpoint,
		ResponseProtocol:    gatewaycontract.ProtocolOpenAICompatible,
		UserID:              authed.UserID,
		APIKeyID:            authed.Key.ID,
		Model:               modelRef,
		CanonicalModel:      prepared.Model.CanonicalName,
		VideoID:             videoID,
		RequestCapabilities: []gatewaycontract.RequestCapability{{Key: capabilitiescontract.KeyVideos, Version: "v1"}},
	}
	prepared.ScheduleReq.StickyAccountID = &boundAccountID
	prepared.ScheduleReq.StickyStrength = schedulercontract.StickyStrengthHard
	prepared.ScheduleReq.SessionAffinityKey = gatewayVideoResultSessionKey(videoID)
	prepared.ScheduleReq.SessionAffinitySource = "video_result"
	prepared.ScheduleReq.Strategy = schedulercontract.StrategyStickyFirst
	if operation == provideradaptercontract.VideoOperationContent {
		failover := s.invokeProviderVideoContentWithFailover(r.Context(), r, authed, canonical, prepared.ScheduleReq, prepared.Model.ID, forcedProviderKey, prepared.Admission, startedAt)
		result := failover.ScheduleResult
		if failover.Err != nil {
			s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, prepared.Admission, startedAt)
			return
		}
		providerResp := failover.Response
		defer providerResp.Content.Close()
		s.recordVideoGatewaySuccess(r, authed, canonical, result, prepared.Model.ID, gatewaycontract.Usage{Estimated: true}, providerResp.QuotaSignals, startedAt, http.StatusOK)
		writeVideoContentResponse(w, providerResp)
		return
	}
	failover := s.invokeProviderVideoWithFailover(r.Context(), r, authed, canonical, prepared.ScheduleReq, prepared.Model.ID, forcedProviderKey, prepared.Admission, startedAt, operation)
	result := failover.ScheduleResult
	if failover.Err != nil {
		s.writeGatewayFailoverFailure(w, r, authed, canonical, result, failover.FailureRecorded, failover.Err, prepared.Admission, startedAt)
		return
	}
	providerResp := failover.Response
	usage := gatewayUsageFromVideoProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalVideoResponse(canonical, gatewayVideoFromProvider(providerResp), usage)
	s.recordVideoGatewaySuccess(r, authed, canonical, result, prepared.Model.ID, canonicalResp.Usage, providerResp.QuotaSignals, startedAt, http.StatusOK)
	s.forwardBufferedPassthroughHeaders(w, r, providerResp.Headers)
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderVideo(canonicalResp))
}

type preparedVideoGatewayRequest struct {
	Model struct {
		ID            int
		CanonicalName string
	}
	Admission   gatewayAdmission
	ScheduleReq schedulercontract.ScheduleRequest
}

func (s *Server) prepareVideoGatewayRequest(w http.ResponseWriter, r *http.Request, authed apikeycontract.AuthResult, modelRef string, requestID string, sourceEndpoint string, startedAt time.Time) (preparedVideoGatewayRequest, bool) {
	var prepared preparedVideoGatewayRequest
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelRef)
	if err != nil {
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, fallbackModelName(modelRef), "model_not_found", http.StatusNotFound, startedAt)
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return prepared, false
	}
	model := modelResolution.Model
	prepared.Model.ID = model.ID
	prepared.Model.CanonicalName = model.CanonicalName
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, model.CanonicalName, "model_not_allowed", http.StatusForbidden, startedAt)
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return prepared, false
	}
	canonical := gatewaycontract.CanonicalRequest{
		RequestID:           requestID,
		SourceProtocol:      gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint:      sourceEndpoint,
		ResponseProtocol:    gatewaycontract.ProtocolOpenAICompatible,
		UserID:              authed.UserID,
		APIKeyID:            authed.Key.ID,
		Model:               modelRef,
		CanonicalModel:      model.CanonicalName,
		RequestCapabilities: []gatewaycontract.RequestCapability{{Key: capabilitiescontract.KeyVideos, Version: "v1"}},
	}
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, model.CanonicalName, "entitlement_check_failed", http.StatusInternalServerError, startedAt)
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return prepared, false
	}
	prepared.Admission = admission
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.recordVideoGatewayFailure(r, authed, requestID, sourceEndpoint, model.CanonicalName, errorClass, gatewayEntitlementHTTPStatus(errorClass), startedAt)
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return prepared, false
	}
	prepared.ScheduleReq = gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(r.Context(), &prepared.ScheduleReq, admission)
	return prepared, true
}

func (s *Server) recordVideoGatewayFailure(r *http.Request, authed apikeycontract.AuthResult, requestID string, sourceEndpoint string, model string, errorClass string, statusCode int, startedAt time.Time) {
	s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
		RequestID:      requestID,
		Authed:         authed,
		SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		SourceEndpoint: sourceEndpoint,
		Model:          model,
		Success:        false,
		ErrorClass:     ptrStringValue(errorClass),
		StatusCode:     ptrInt(statusCode),
		LatencyMS:      elapsedMillis(startedAt),
		UsageEstimated: false,
	})
}

func (s *Server) recordVideoGatewaySuccess(r *http.Request, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, modelID int, usage gatewaycontract.Usage, quotaSignals []provideradaptercontract.QuotaSignal, startedAt time.Time, statusCode int) {
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(modelID, result.Candidate, canonical, usage), ptrInt(result.Candidate.Account.ID), authed.Key.GroupIDs, usage.Estimated)
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
		StatusCode:            ptrInt(statusCode),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CachedTokens:          usage.CachedTokens,
		UsageEstimated:        usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
		ProviderQuotaSignals:  quotaSignals,
	})
}

func writeVideoContentResponse(w http.ResponseWriter, resp provideradaptercontract.VideoContentResponse) {
	for key, values := range resp.Headers {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				w.Header().Add(key, value)
			}
		}
	}
	if strings.TrimSpace(resp.ContentType) != "" {
		w.Header().Set("Content-Type", resp.ContentType)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Content)
}

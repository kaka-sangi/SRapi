package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

type gatewayConversationInvocation struct {
	Canonical      gatewaycontract.CanonicalRequest
	Model          modelcontract.Model
	Admission      gatewayAdmission
	Failover       gatewayFailoverResult[provideradaptercontract.ConversationResponse]
	RequestedModel string
}

func (s *Server) invokeProviderConversationWithModelFallback(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	resolution modelcontract.ModelResolution,
	admission gatewayAdmission,
	forcedProviderKey string,
	startedAt time.Time,
) gatewayConversationInvocation {
	primary := s.invokeGatewayConversationAttempt(ctx, r, authed, canonical, resolution, admission, forcedProviderKey, startedAt)
	if primary.Failover.Err == nil || !gatewayShouldAttemptModelFallback(primary.Failover) {
		return primary
	}

	last := primary
	for _, fallbackModel := range uniqueNonEmptyStrings(gatewayConfiguredModelFallbacks(resolution)) {
		if strings.EqualFold(fallbackModel, resolution.Model.CanonicalName) {
			continue
		}
		fallbackResolution, err := s.runtime.resolveModelCached(ctx, fallbackModel)
		if err != nil {
			continue
		}
		if fallbackResolution.Model.ID == resolution.Model.ID || strings.EqualFold(fallbackResolution.Model.CanonicalName, resolution.Model.CanonicalName) {
			continue
		}
		if !gatewayFallbackModelAllowed(authed.Key.AllowedModels, resolution, fallbackResolution) {
			continue
		}

		fallbackCanonical := gatewayCanonicalForModelFallback(canonical, fallbackResolution.Model.CanonicalName)
		fallbackAdmission, err := s.runtime.prepareGatewayAdmission(ctx, &fallbackCanonical, fallbackResolution, fallbackResolution.Model.ID)
		if err != nil || !fallbackAdmission.Entitlement.Allowed {
			s.runtime.releaseGatewayReservation(ctx, authed.UserID, fallbackCanonical.RequestID)
			continue
		}
		attempt := s.invokeGatewayConversationAttempt(ctx, r, authed, fallbackCanonical, fallbackResolution, fallbackAdmission, forcedProviderKey, startedAt)
		attempt.RequestedModel = primary.RequestedModel
		if attempt.Failover.Err == nil {
			return attempt
		}
		if !attempt.Failover.FailureRecorded {
			s.runtime.releaseGatewayReservation(ctx, authed.UserID, fallbackCanonical.RequestID)
			continue
		}
		last = attempt
		if !gatewayShouldAttemptModelFallback(attempt.Failover) {
			return attempt
		}
	}
	return last
}

func (s *Server) invokeGatewayConversationAttempt(
	ctx context.Context,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	resolution modelcontract.ModelResolution,
	admission gatewayAdmission,
	forcedProviderKey string,
	startedAt time.Time,
) gatewayConversationInvocation {
	scheduleReq := gatewayScheduleRequest(r, canonical, resolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	failover := s.invokeProviderConversationWithFailover(ctx, r, authed, canonical, scheduleReq, resolution.Model.ID, forcedProviderKey, admission, startedAt)
	return gatewayConversationInvocation{
		Canonical:      canonical,
		Model:          resolution.Model,
		Admission:      admission,
		Failover:       failover,
		RequestedModel: gatewayRequestedModel(canonical),
	}
}

func gatewayConfiguredModelFallbacks(resolution modelcontract.ModelResolution) []string {
	if resolution.Alias == nil {
		return nil
	}
	return resolution.Alias.FallbackModels
}

func gatewayFallbackModelAllowed(allowed []string, root modelcontract.ModelResolution, fallback modelcontract.ModelResolution) bool {
	if apiKeyAllowsModelReference(allowed, fallback) {
		return true
	}
	if len(allowed) == 0 || root.Alias == nil || !apiKeyAllowsModel(allowed, root.Alias.Alias) {
		return false
	}
	for _, candidate := range root.Alias.FallbackModels {
		if strings.EqualFold(strings.TrimSpace(candidate), fallback.Model.CanonicalName) {
			return true
		}
		if fallback.Alias != nil && strings.EqualFold(strings.TrimSpace(candidate), fallback.Alias.Alias) {
			return true
		}
	}
	return false
}

func gatewayCanonicalForModelFallback(canonical gatewaycontract.CanonicalRequest, model string) gatewaycontract.CanonicalRequest {
	model = strings.TrimSpace(model)
	if model == "" {
		return canonical
	}
	updated := canonical
	updated.Model = model
	updated.CanonicalModel = model
	updated.RawBody = gatewayReplaceRawBodyModel(canonical.RawBody, canonical.SourceProtocol, model)
	return updated
}

func gatewayReplaceRawBodyModel(raw []byte, protocol gatewaycontract.Protocol, model string) []byte {
	model = strings.TrimSpace(model)
	if len(raw) == 0 || model == "" {
		return raw
	}
	if protocol != gatewaycontract.ProtocolOpenAICompatible && protocol != gatewaycontract.ProtocolAnthropicCompatible {
		return raw
	}
	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil || doc == nil {
		return raw
	}
	doc["model"] = model
	updated, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return updated
}

func gatewayShouldAttemptModelFallback(result gatewayFailoverResult[provideradaptercontract.ConversationResponse]) bool {
	if result.Err == nil || !result.FailureRecorded {
		return false
	}
	errorClass, upstreamStatus, _ := providerGatewayError(result.Err)
	switch errorClass {
	case "invalid_request",
		"model_not_found",
		"model_not_allowed",
		"auth_failed",
		"auth_error",
		"credential_error",
		"permission_denied",
		"configuration_error",
		"session_invalid",
		"account_locked",
		"account_banned",
		"abuse_detected",
		"device_unrecognized",
		"platform_quota_exceeded":
		return false
	case "rate_limit",
		"quota_exhausted",
		"timeout",
		"network_error",
		"provider_5xx",
		"model_unavailable",
		"overloaded",
		"stream_interrupted",
		"empty_completion",
		"invalid_response":
		return true
	}
	switch upstreamStatus {
	case http.StatusTooManyRequests, http.StatusRequestTimeout, http.StatusGatewayTimeout, 529:
		return true
	}
	return upstreamStatus >= 500 && errorClass == "upstream_error"
}

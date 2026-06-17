package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	realtimeservice "github.com/srapi/srapi/apps/api/internal/modules/realtime/service"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"nhooyr.io/websocket"
)

const responsesWebSocketSourceEndpoint = "/v1/responses/ws"
const realtimeWebSocketSourceEndpoint = "/v1/realtime"
const statusClientClosedRequest = 499

type gatewayCaptureResponse struct {
	headers http.Header
	body    bytes.Buffer
	status  int
}

func newGatewayCaptureResponse() *gatewayCaptureResponse {
	return &gatewayCaptureResponse{headers: make(http.Header)}
}

func (r *gatewayCaptureResponse) Header() http.Header {
	return r.headers
}

func (r *gatewayCaptureResponse) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *gatewayCaptureResponse) Write(payload []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(payload)
}

func (r *gatewayCaptureResponse) Flush() {}

func (r *gatewayCaptureResponse) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (s *Server) handleResponsesWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	slot, err := s.acquireResponsesWebSocketSlot(r.Context(), r, authed)
	if err != nil {
		status := http.StatusInternalServerError
		code := "internal_error"
		message := "failed to acquire realtime slot"
		errorType := apiopenapi.InternalError
		if errors.Is(err, realtimeservice.ErrLimitExceeded) {
			status = http.StatusTooManyRequests
			code = "rate_limit"
			message = "realtime websocket slot limit exceeded"
			errorType = apiopenapi.RateLimitError
		}
		writeGatewayError(w, status, errorType, message, code)
		return
	}
	defer s.releaseResponsesWebSocketSlot(r.Context(), slot.ID)

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		s.logger.Warn("failed to accept responses websocket", "error", err, "request_id", requestID)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	if s.cfg.Gateway.MaxBodySize > 0 {
		conn.SetReadLimit(s.cfg.Gateway.MaxBodySize)
	}

	// Register this connection with the WS idle-timeout janitor (5min,
	// ported from CLIProxyAPI codex_websockets_executor.go). The janitor
	// will close the client connection if no frame is read or written
	// within the idle window.
	idleCtx, cancelIdle := context.WithCancel(r.Context())
	defer cancelIdle()
	idleSession := globalWSIdleSessionStore.Register(slot.ID, conn, nil, cancelIdle)
	defer globalWSIdleSessionStore.Unregister(slot.ID)

	var lastTurn responsesWebSocketTurnState
	for {
		messageType, payload, err := conn.Read(idleCtx)
		if err != nil {
			if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				return
			}
			if !errors.Is(err, context.Canceled) {
				s.logger.Debug("responses websocket closed", "error", err, "request_id", requestID)
			}
			return
		}
		idleSession.SetActive()
		if messageType != websocket.MessageText {
			if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusBadRequest, "invalid_request", "responses websocket only accepts JSON text frames", nil); err != nil {
				return
			}
			idleSession.SetActive()
			continue
		}
		if handled, err := handleResponsesWebSocketControl(r.Context(), conn, payload); handled || err != nil {
			if err != nil {
				return
			}
			continue
		}

		requestPayload, err := responsesWebSocketRequestPayload(payload, r.URL.Query().Get("model"))
		if err != nil {
			if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusBadRequest, "invalid_request", err.Error(), nil); err != nil {
				return
			}
			continue
		}
		requestPayload, err = responsesWebSocketInferPreviousResponseID(requestPayload, lastTurn)
		if err != nil {
			if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusBadRequest, "invalid_request", err.Error(), nil); err != nil {
				return
			}
			continue
		}
		if err := s.runtime.gateway.ValidateResponsesRequest(requestPayload); err != nil {
			if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusBadRequest, "invalid_request", err.Error(), nil); err != nil {
				return
			}
			continue
		}
		if s.shouldUseCodexWebSocketRelay(r, requestPayload) {
			relayed, turnState, err := s.relayCodexResponsesWebSocket(r, conn, requestPayload, authed, idleSession)
			if err != nil {
				status, code, message := responsesWebSocketRelayError(err)
				if err := writeResponsesWebSocketError(r.Context(), conn, status, code, message, nil); err != nil {
					return
				}
				continue
			}
			if relayed {
				if turnState.ResponseID != "" {
					lastTurn = turnState
				}
				continue
			}
		}

		captured, err := s.captureResponsesRequest(r, requestPayload)
		if err != nil {
			if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusInternalServerError, "internal_error", "failed to execute responses request", nil); err != nil {
				return
			}
			continue
		}
		turnState, err := writeCapturedResponsesWebSocket(r.Context(), conn, captured, idleSession)
		if err != nil {
			return
		}
		if turnState.ResponseID != "" {
			lastTurn = turnState
		}
	}
}

func (s *Server) acquireResponsesWebSocketSlot(ctx context.Context, r *http.Request, authed apikeycontract.AuthResult) (realtimecontract.Slot, error) {
	stickyAccountID, stickyStrength, affinityKey, affinitySource := gatewaySessionAffinity(r)
	return s.runtime.realtime.Acquire(ctx, realtimecontract.AcquireRequest{
		Kind:                  realtimecontract.SlotKindResponsesWebSocket,
		RequestID:             requestIDFromContext(ctx),
		UserID:                authed.UserID,
		APIKeyID:              authed.Key.ID,
		SourceEndpoint:        responsesWebSocketSourceEndpoint,
		SessionAffinityKey:    affinityKey,
		SessionAffinitySource: affinitySource,
		StickyAccountID:       stickyAccountID,
		StickyStrength:        string(stickyStrength),
	})
}

func (s *Server) releaseResponsesWebSocketSlot(ctx context.Context, slotID string) {
	if _, err := s.runtime.realtime.Release(ctx, slotID); err != nil && !errors.Is(err, realtimeservice.ErrSlotNotFound) {
		s.logger.Warn("failed to release responses websocket slot", "error", err, "slot_id", slotID)
	}
}

func (s *Server) handleRealtimeWebSocket(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	requestID := requestIDFromContext(r.Context())
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	modelName := strings.TrimSpace(r.URL.Query().Get("model"))
	if modelName == "" {
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, "realtime websocket model query parameter is required", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelName)
	if err != nil {
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	slot, err := s.acquireRealtimeWebSocketSlot(r.Context(), r, authed)
	if err != nil {
		status := http.StatusInternalServerError
		code := "internal_error"
		message := "failed to acquire realtime slot"
		errorType := apiopenapi.InternalError
		if errors.Is(err, realtimeservice.ErrLimitExceeded) {
			status = http.StatusTooManyRequests
			code = "rate_limit"
			message = "realtime websocket slot limit exceeded"
			errorType = apiopenapi.RateLimitError
		}
		writeGatewayError(w, status, errorType, message, code)
		return
	}
	defer s.releaseResponsesWebSocketSlot(r.Context(), slot.ID)

	canonical := s.runtime.gateway.NormalizeRealtimeWebSocket(modelName, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: realtimeWebSocketSourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: modelResolution.Model.CanonicalName,
	})
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, modelResolution.Model.ID)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, err.Error(), "invalid_request")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	result, err := s.runtime.scheduleGatewayRequest(withGatewayInboundClient(r.Context(), r), scheduleReq, modelResolution.Model.ID, gatewayForcedProviderKey(r.Context()), authed.Key)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, nil, false, "no_available_account", http.StatusServiceUnavailable, elapsedMillis(startedAt), admission, nil, ""))
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available provider account", "no_available_account")
		return
	}
	if err := s.reserveGatewayAccountQuotaForScheduledRequest(r.Context(), r, authed, canonical, result, admission, startedAt); err != nil {
		s.writeProviderGatewayError(w, err)
		return
	}
	s.runtime.bindGatewaySessionAffinity(r.Context(), scheduleReq.APIKeyID, scheduleReq.SessionAffinityKey, result.Candidate.Account.ID)
	session, credential, err := s.runtime.prepareProviderRealtime(r.Context(), providerRealtimeRequest(canonical, result.Candidate, nil, r, realtimeWebSocketHeaders(r)))
	if err != nil {
		errorClass, upstreamStatus, _ := providerGatewayError(err)
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, false, errorClass, upstreamStatus, elapsedMillis(startedAt), admission, nil, providerErrorMessage(err)))
		writeGatewayError(w, providerStatusFromError(err), gatewayErrorTypeForProviderClass(errorClass), providerGatewayMessage(errorClass), errorClass)
		return
	}
	session.Account.Credential = credential

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		s.logger.Warn("failed to accept realtime websocket", "error", err, "request_id", requestID)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	if s.cfg.Gateway.MaxBodySize > 0 {
		conn.SetReadLimit(s.cfg.Gateway.MaxBodySize)
	}
	// Register the realtime WS with the idle-timeout janitor (5min, ported
	// from CLIProxyAPI codex_websockets_executor.go).
	idleCtx, cancelIdle := context.WithCancel(r.Context())
	defer cancelIdle()
	idleSession := globalWSIdleSessionStore.Register(slot.ID, conn, nil, cancelIdle)
	defer globalWSIdleSessionStore.Unregister(slot.ID)
	success, errorClass, statusCode := s.relayRealtimeWebSocket(idleCtx, conn, session, idleSession)
	s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, success, errorClass, statusCode, elapsedMillis(startedAt), admission, nil, ""))
}

func (s *Server) acquireRealtimeWebSocketSlot(ctx context.Context, r *http.Request, authed apikeycontract.AuthResult) (realtimecontract.Slot, error) {
	stickyAccountID, stickyStrength, affinityKey, affinitySource := gatewaySessionAffinity(r)
	return s.runtime.realtime.Acquire(ctx, realtimecontract.AcquireRequest{
		Kind:                  realtimecontract.SlotKindRealtimeWebSocket,
		RequestID:             requestIDFromContext(ctx),
		UserID:                authed.UserID,
		APIKeyID:              authed.Key.ID,
		SourceEndpoint:        realtimeWebSocketSourceEndpoint,
		SessionAffinityKey:    affinityKey,
		SessionAffinitySource: affinitySource,
		StickyAccountID:       stickyAccountID,
		StickyStrength:        string(stickyStrength),
	})
}

func realtimeWebSocketHeaders(r *http.Request) http.Header {
	headers := http.Header{}
	if safetyID := strings.TrimSpace(r.Header.Get("OpenAI-Safety-Identifier")); safetyID != "" {
		headers.Set("OpenAI-Safety-Identifier", safetyID)
	}
	return headers
}

func (s *Server) relayRealtimeWebSocket(ctx context.Context, conn *websocket.Conn, session providerRealtimeSession, idleSession *wsIdleSession) (bool, string, int) {
	if strings.EqualFold(strings.TrimSpace(session.Account.RuntimeClass), string(accountcontract.RuntimeClassAPIKey)) {
		return s.relayOfficialAPIKeyRealtimeWebSocket(ctx, conn, session, idleSession)
	}
	clientToUpstream := make(chan reverseproxycontract.WebSocketMessage, 32)
	upstreamToClient := make(chan reverseproxycontract.WebSocketMessage, 32)
	relayCtx, cancelRelay := context.WithCancel(ctx)
	defer cancelRelay()
	relayDone := make(chan responsesWebSocketRelayResult, 1)
	go func() {
		result, err := s.runtime.reverseProxy.RelayWebSocket(relayCtx, reverseproxycontract.WebSocketRelayRequest{
			Account:          session.Account,
			URL:              session.URL,
			Headers:          session.Headers,
			ClientToUpstream: clientToUpstream,
			UpstreamToClient: upstreamToClient,
		})
		relayDone <- responsesWebSocketRelayResult{result: result, err: err}
	}()
	clientDone := make(chan error, 1)
	go readRealtimeWebSocketClient(ctx, conn, clientToUpstream, clientDone, idleSession)

	var relayResult *responsesWebSocketRelayResult
	relayDoneCh := relayDone
	forwardedUpstreamMessage := false
	for {
		select {
		case msg, ok := <-upstreamToClient:
			if !ok {
				if relayResult == nil && relayDoneCh != nil {
					select {
					case result := <-relayDoneCh:
						relayResult = &result
					case <-ctx.Done():
						if forwardedUpstreamMessage {
							return true, "", http.StatusOK
						}
						return false, "client_closed", statusClientClosedRequest
					}
				}
				if relayResult != nil && relayResult.err != nil {
					if forwardedUpstreamMessage {
						return true, "", http.StatusOK
					}
					return false, errorClassName(relayResult.err), providerStatusFromError(relayResult.err)
				}
				return true, "", http.StatusOK
			}
			messageType := websocket.MessageText
			if msg.Type == reverseproxycontract.WebSocketMessageBinary {
				messageType = websocket.MessageBinary
			}
			if err := conn.Write(ctx, messageType, msg.Data); err != nil {
				cancelRelay()
				if forwardedUpstreamMessage {
					return true, "", http.StatusOK
				}
				return false, "client_closed", statusClientClosedRequest
			}
			idleSession.SetActive()
			forwardedUpstreamMessage = true
		case err := <-clientDone:
			cancelRelay()
			if err != nil {
				if forwardedUpstreamMessage {
					return true, "", http.StatusOK
				}
				return false, "client_closed", statusClientClosedRequest
			}
			if relayDoneCh != nil {
				select {
				case result := <-relayDoneCh:
					relayResult = &result
					relayDoneCh = nil
				case <-time.After(100 * time.Millisecond):
				}
			}
			if relayResult != nil && relayResult.err != nil {
				if forwardedUpstreamMessage {
					return true, "", http.StatusOK
				}
				return false, errorClassName(relayResult.err), providerStatusFromError(relayResult.err)
			}
			return true, "", http.StatusOK
		case result := <-relayDoneCh:
			relayResult = &result
			relayDoneCh = nil
			if relayResult.err != nil {
				cancelRelay()
				if forwardedUpstreamMessage {
					return true, "", http.StatusOK
				}
				return false, errorClassName(relayResult.err), providerStatusFromError(relayResult.err)
			}
		case <-ctx.Done():
			cancelRelay()
			if forwardedUpstreamMessage {
				return true, "", http.StatusOK
			}
			return false, "client_closed", statusClientClosedRequest
		}
	}
}

func (s *Server) relayOfficialAPIKeyRealtimeWebSocket(ctx context.Context, conn *websocket.Conn, session providerRealtimeSession, idleSession *wsIdleSession) (bool, string, int) {
	apiKey := firstNonEmpty(
		mapString(session.Account.Credential, "api_key"),
		mapString(session.Account.Credential, "openai_api_key"),
	)
	if apiKey == "" {
		return false, "auth_failed", http.StatusUnauthorized
	}
	headers := cloneHTTPHeader(session.Headers)
	if headers == nil {
		headers = http.Header{}
	}
	headers.Set("Authorization", "Bearer "+apiKey)
	if headers.Get("User-Agent") == "" {
		if userAgent := mapString(session.Account.Metadata, "user_agent"); userAgent != "" {
			headers.Set("User-Agent", userAgent)
		}
	}
	client, err := realtimeAPIKeyHTTPClient(session.Account)
	if err != nil {
		return false, "network_error", http.StatusBadGateway
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, 30*time.Second)
	upstream, resp, err := websocket.Dial(dialCtx, session.URL, &websocket.DialOptions{
		HTTPClient:      client,
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionDisabled,
	})
	cancelDial()
	if err != nil {
		return false, realtimeWebSocketDialErrorClass(dialCtx, err), realtimeWebSocketDialStatus(dialCtx, resp)
	}
	defer upstream.Close(websocket.StatusNormalClosure, "")

	clientDone := make(chan error, 1)
	upstreamDone := make(chan realtimeDirectRelayResult, 1)
	upstreamForwarded := make(chan struct{}, 1)
	forwardedUpstreamMessage := false
	go relayWebSocketClientToUpstream(ctx, conn, upstream, clientDone, idleSession)
	go relayWebSocketUpstreamToClient(ctx, upstream, conn, upstreamDone, upstreamForwarded, idleSession)
	for {
		select {
		case <-upstreamForwarded:
			forwardedUpstreamMessage = true
		case result := <-upstreamDone:
			if result.forwarded {
				forwardedUpstreamMessage = true
			}
			if result.err != nil {
				if forwardedUpstreamMessage {
					return true, "", http.StatusOK
				}
				return false, realtimeWebSocketRelayErrorClass(ctx, result.err), realtimeWebSocketRelayStatus(ctx, result.err)
			}
			return true, "", http.StatusOK
		case err := <-clientDone:
			upstream.Close(websocket.StatusNormalClosure, "")
			if err != nil {
				if forwardedUpstreamMessage {
					return true, "", http.StatusOK
				}
				return false, "client_closed", statusClientClosedRequest
			}
			return true, "", http.StatusOK
		case <-ctx.Done():
			upstream.Close(websocket.StatusNormalClosure, "")
			if forwardedUpstreamMessage {
				return true, "", http.StatusOK
			}
			return false, "client_closed", statusClientClosedRequest
		}
	}
}

type realtimeDirectRelayResult struct {
	err       error
	forwarded bool
}

func relayWebSocketClientToUpstream(ctx context.Context, client *websocket.Conn, upstream *websocket.Conn, done chan<- error, idleSession *wsIdleSession) {
	for {
		messageType, payload, err := client.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				done <- nil
				return
			}
			if errors.Is(err, context.Canceled) {
				done <- nil
				return
			}
			done <- err
			return
		}
		idleSession.SetActive()
		if err := upstream.Write(ctx, messageType, payload); err != nil {
			done <- err
			return
		}
	}
}

func relayWebSocketUpstreamToClient(ctx context.Context, upstream *websocket.Conn, client *websocket.Conn, done chan<- realtimeDirectRelayResult, forwardedOnce chan<- struct{}, idleSession *wsIdleSession) {
	forwarded := false
	for {
		messageType, payload, err := upstream.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				done <- realtimeDirectRelayResult{forwarded: forwarded}
				return
			}
			if errors.Is(err, context.Canceled) {
				done <- realtimeDirectRelayResult{forwarded: forwarded}
				return
			}
			done <- realtimeDirectRelayResult{err: err, forwarded: forwarded}
			return
		}
		idleSession.SetActive()
		if err := client.Write(ctx, messageType, payload); err != nil {
			done <- realtimeDirectRelayResult{err: err, forwarded: forwarded}
			return
		}
		if !forwarded {
			select {
			case forwardedOnce <- struct{}{}:
			default:
			}
		}
		forwarded = true
	}
}

func realtimeAPIKeyHTTPClient(account reverseproxycontract.AccountRuntime) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	if account.ProxyID != nil && strings.TrimSpace(*account.ProxyID) != "" {
		proxyURL, err := url.Parse(strings.TrimSpace(*account.ProxyID))
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: transport}, nil
}

func realtimeWebSocketDialErrorClass(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	return "network_error"
}

func realtimeWebSocketDialStatus(ctx context.Context, resp *http.Response) int {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	if resp != nil && resp.StatusCode > 0 {
		return resp.StatusCode
	}
	return http.StatusBadGateway
}

func realtimeWebSocketRelayErrorClass(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "client_closed"
	}
	return "network_error"
}

func realtimeWebSocketRelayStatus(ctx context.Context, err error) int {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return statusClientClosedRequest
	}
	return http.StatusBadGateway
}

func readRealtimeWebSocketClient(ctx context.Context, conn *websocket.Conn, clientToUpstream chan<- reverseproxycontract.WebSocketMessage, done chan<- error, idleSession *wsIdleSession) {
	defer close(clientToUpstream)
	defer close(done)
	for {
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				done <- nil
				return
			}
			if errors.Is(err, context.Canceled) {
				done <- nil
				return
			}
			done <- err
			return
		}
		idleSession.SetActive()
		msgType := reverseproxycontract.WebSocketMessageText
		if messageType == websocket.MessageBinary {
			msgType = reverseproxycontract.WebSocketMessageBinary
		}
		select {
		case clientToUpstream <- reverseproxycontract.WebSocketMessage{Type: msgType, Data: payload}:
		case <-ctx.Done():
			done <- nil
			return
		}
	}
}

func (s *Server) shouldUseCodexWebSocketRelay(r *http.Request, payload []byte) bool {
	if !responsesWebSocketRelayRequested(r) {
		return false
	}
	return responsesWebSocketPayloadModel(payload, r.URL.Query().Get("model")) != ""
}

func responsesWebSocketRelayRequested(r *http.Request) bool {
	return boolValue(firstNonEmpty(
		r.URL.Query().Get("upstream_ws"),
		r.URL.Query().Get("codex_responses_websocket"),
		r.Header.Get("X-SRapi-Upstream-WS"),
		r.Header.Get("X-SRapi-Codex-Responses-WebSocket"),
	))
}

func (s *Server) relayCodexResponsesWebSocket(r *http.Request, conn *websocket.Conn, payload []byte, authed apikeycontract.AuthResult, idleSession *wsIdleSession) (bool, responsesWebSocketTurnState, error) {
	startedAt := time.Now()
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := responsesWebSocketSourceEndpoint
	modelName := responsesWebSocketPayloadModel(payload, r.URL.Query().Get("model"))
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelName)
	if err != nil {
		return false, responsesWebSocketTurnState{}, err
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		return false, responsesWebSocketTurnState{}, errors.New("model not allowed for this api key")
	}
	body, err := responsesWebSocketOpenAPIRequest(payload)
	if err != nil {
		return false, responsesWebSocketTurnState{}, err
	}
	if strings.TrimSpace(body.Model) == "" {
		body.Model = modelName
	}
	canonical := s.runtime.gateway.NormalizeResponses(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), &canonical, modelResolution, model.ID)
	if err != nil {
		return false, responsesWebSocketTurnState{}, err
	}
	if !admission.Entitlement.Allowed {
		return false, responsesWebSocketTurnState{}, errors.New(gatewayEntitlementMessage(gatewayEntitlementErrorClass(admission.Entitlement)))
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	result, err := s.runtime.scheduleGatewayRequest(withGatewayInboundClient(r.Context(), r), scheduleReq, model.ID, gatewayForcedProviderKey(r.Context()), authed.Key)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, nil, false, "no_available_account", http.StatusServiceUnavailable, elapsedMillis(startedAt), admission, nil, ""))
		return false, responsesWebSocketTurnState{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(result.Candidate.Provider.AdapterType), "reverse-proxy-codex-cli") || !accountCodexWebSocketEnabled(result.Candidate.Account.Metadata) {
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, false, "invalid_request", http.StatusBadRequest, elapsedMillis(startedAt), admission, nil, ""))
		return true, responsesWebSocketTurnState{}, errors.New("selected account does not support Codex Responses WebSocket reverse proxy")
	}
	if err := s.reserveGatewayAccountQuotaForScheduledRequest(r.Context(), r, authed, canonical, result, admission, startedAt); err != nil {
		return true, responsesWebSocketTurnState{}, err
	}
	s.runtime.bindGatewaySessionAffinity(r.Context(), scheduleReq.APIKeyID, scheduleReq.SessionAffinityKey, result.Candidate.Account.ID)
	session, credential, err := s.runtime.prepareProviderRealtime(r.Context(), providerRealtimeRequest(canonical, result.Candidate, payload, r))
	if err != nil {
		errorClass, upstreamStatus, _ := providerGatewayError(err)
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, false, errorClass, upstreamStatus, elapsedMillis(startedAt), admission, nil, providerErrorMessage(err)))
		return true, responsesWebSocketTurnState{}, err
	}
	session.Account.Credential = credential
	clientToUpstream := make(chan reverseproxycontract.WebSocketMessage, 32)
	upstreamToClient := make(chan reverseproxycontract.WebSocketMessage, 32)
	relayCtx, cancelRelay := context.WithCancel(r.Context())
	defer cancelRelay()
	relayDone := make(chan responsesWebSocketRelayResult, 1)
	go func() {
		result, err := s.runtime.reverseProxy.RelayWebSocket(relayCtx, reverseproxycontract.WebSocketRelayRequest{
			Account:          session.Account,
			URL:              session.URL,
			Headers:          session.Headers,
			ClientToUpstream: clientToUpstream,
			UpstreamToClient: upstreamToClient,
		})
		relayDone <- responsesWebSocketRelayResult{result: result, err: err}
	}()
	clientToUpstream <- reverseproxycontract.WebSocketMessage{Type: reverseproxycontract.WebSocketMessageText, Data: session.InitialFrame}
	close(clientToUpstream)
	success, errorClass, statusCode, usage, terminalForwarded, turnState := s.bridgeResponsesWebSocketRelay(r.Context(), conn, upstreamToClient, relayDone, idleSession)
	s.recordResponsesWebSocketUsage(r.Context(), authed, canonical, model.ID, result, success, errorClass, statusCode, elapsedMillis(startedAt), admission, usage, "")
	if !success && errorClass != "client_closed" && !terminalForwarded {
		return true, turnState, provideradaptercontract.ProviderError{Class: errorClass, StatusCode: statusCode, Message: providerGatewayMessage(errorClass)}
	}
	return true, turnState, nil
}

func responsesWebSocketRelayError(err error) (int, string, string) {
	var providerErr provideradaptercontract.ProviderError
	if errors.As(err, &providerErr) {
		errorClass, upstreamStatus, _ := providerGatewayError(err)
		return providerGatewayHTTPStatus(upstreamStatus), errorClass, providerGatewayMessage(errorClass)
	}
	return http.StatusBadGateway, "upstream_error", err.Error()
}

type responsesWebSocketRelayResult struct {
	result reverseproxycontract.WebSocketRelayResult
	err    error
}

type providerRealtimeSession struct {
	URL          string
	Headers      http.Header
	InitialFrame []byte
	Account      reverseproxycontract.AccountRuntime
}

func (s *Server) bridgeResponsesWebSocketRelay(ctx context.Context, conn *websocket.Conn, upstreamToClient <-chan reverseproxycontract.WebSocketMessage, relayDone <-chan responsesWebSocketRelayResult, idleSession *wsIdleSession) (bool, string, int, *gatewaycontract.Usage, bool, responsesWebSocketTurnState) {
	var usage *gatewaycontract.Usage
	var turnState responsesWebSocketTurnState
	relayDoneCh := relayDone
	var relayResult *responsesWebSocketRelayResult
	for {
		select {
		case msg, ok := <-upstreamToClient:
			if !ok {
				if relayResult == nil && relayDoneCh != nil {
					select {
					case result := <-relayDoneCh:
						relayResult = &result
					case <-ctx.Done():
						return false, "client_closed", statusClientClosedRequest, usage, false, turnState
					}
				}
				if relayResult != nil && relayResult.err != nil {
					return false, errorClassName(relayResult.err), providerStatusFromError(relayResult.err), usage, false, turnState
				}
				return true, "", http.StatusOK, usage, false, turnState
			}
			if msg.Type != reverseproxycontract.WebSocketMessageText {
				continue
			}
			turnState.merge(responsesWebSocketTurnStateFromEvent(msg.Data))
			if eventUsage, ok := responsesWebSocketUsage(msg.Data); ok {
				usage = &eventUsage
			}
			if err := conn.Write(ctx, websocket.MessageText, msg.Data); err != nil {
				return false, "client_closed", statusClientClosedRequest, usage, false, turnState
			}
			idleSession.SetActive()
			if terminal, success, errorClass, statusCode := responsesWebSocketTerminal(msg.Data); terminal {
				return success, errorClass, statusCode, usage, true, turnState
			}
		case result := <-relayDoneCh:
			relayResult = &result
			relayDoneCh = nil
		case <-ctx.Done():
			return false, "client_closed", statusClientClosedRequest, usage, false, turnState
		}
	}
}

func responsesWebSocketUsageRecord(authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, candidate *schedulercontract.Candidate, success bool, errorClass string, statusCode int, latencyMS int, admission gatewayAdmission, usage *gatewaycontract.Usage, providerMessage string) gatewayUsageRecord {
	recordUsage := admission.EstimatedUsage
	estimated := true
	if usage != nil {
		recordUsage = *usage
		estimated = usage.Estimated
	}
	rec := gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		Model:                 canonical.CanonicalModel,
		RequestedModel:        gatewayRequestedModel(canonical),
		Success:               success,
		StatusCode:            ptrInt(statusCode),
		LatencyMS:             latencyMS,
		InputTokens:           recordUsage.InputTokens,
		OutputTokens:          recordUsage.OutputTokens,
		CachedTokens:          recordUsage.CachedTokens,
		CacheCreationTokens:   recordUsage.CacheCreationTokens,
		UsageEstimated:        estimated,
		Pricing:               admission.Pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
		ProviderErrorMessage:  providerMessage,
	}
	if errorClass != "" {
		rec.ErrorClass = ptrStringValue(errorClass)
	}
	if candidate != nil {
		rec.ProviderID = ptrInt(candidate.Provider.ID)
		rec.AccountID = ptrInt(candidate.Account.ID)
		rec.TargetProtocol = candidate.Provider.Protocol
		rec.RequestedModel, rec.UpstreamModel = gatewayUsageModelSnapshot(canonical, *candidate)
	}
	return rec
}

func (s *Server) recordResponsesWebSocketUsage(ctx context.Context, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, modelID int, result schedulercontract.ScheduleResult, success bool, errorClass string, statusCode int, latencyMS int, admission gatewayAdmission, usage *gatewaycontract.Usage, providerMessage string) {
	rec := responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, success, errorClass, statusCode, latencyMS, admission, usage, providerMessage)
	if usage != nil {
		rec.Pricing = s.runtime.gatewayPricing(ctx, gatewayPricingRequestForCanonical(modelID, result.Candidate, canonical, *usage), ptrInt(result.Candidate.Account.ID), usage.Estimated)
	}
	s.runtime.recordGatewayUsage(ctx, rec)
}

func cloneHTTPHeader(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	out := make(http.Header, len(headers))
	for key, values := range headers {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func (rt *runtimeState) prepareProviderRealtime(ctx context.Context, req provideradaptercontract.RealtimeRequest) (providerRealtimeSession, map[string]any, error) {
	if req.Account.ID <= 0 {
		return providerRealtimeSession{}, nil, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	if err := rt.materializeProviderProxy(ctx, &req.Account); err != nil {
		return providerRealtimeSession{}, nil, err
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return providerRealtimeSession{}, nil, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return providerRealtimeSession{}, nil, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	session, err := rt.adapters.PrepareRealtime(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return providerRealtimeSession{}, nil, err
	}
	return providerRealtimeSession{
		URL:          session.URL,
		Headers:      session.Headers,
		InitialFrame: session.InitialFrame,
		Account:      reverseProxyAccountRuntime(req.Account, credential),
	}, credential, nil
}

func providerRealtimeRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate, payload []byte, source *http.Request, headers ...http.Header) provideradaptercontract.RealtimeRequest {
	var clonedHeaders http.Header
	if len(headers) > 0 {
		clonedHeaders = cloneHTTPHeader(headers[0])
	}
	return provideradaptercontract.RealtimeRequest{
		RequestID:       req.RequestID,
		SourceProtocol:  string(req.SourceProtocol),
		SourceEndpoint:  req.SourceEndpoint,
		Model:           req.CanonicalModel,
		RequestPayload:  append([]byte(nil), payload...),
		Headers:         clonedHeaders,
		Provider:        candidate.Provider,
		Account:         candidate.Account,
		Mapping:         candidate.Mapping,
		RequestSettings: gatewayProviderRequestSettings(source, req),
	}
}

func responsesWebSocketPayloadModel(payload []byte, fallback string) string {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(payload), &object); err != nil {
		return strings.TrimSpace(fallback)
	}
	if model := rawString(object["model"]); model != "" {
		return model
	}
	return strings.TrimSpace(fallback)
}

func responsesWebSocketOpenAPIRequest(payload []byte) (apiopenapi.ResponsesRequest, error) {
	var body apiopenapi.ResponsesRequest
	if err := json.Unmarshal(bytes.TrimSpace(payload), &body); err != nil {
		return apiopenapi.ResponsesRequest{}, err
	}
	return body, nil
}

func responsesWebSocketUsage(payload []byte) (gatewaycontract.Usage, bool) {
	type rawResponsesUsage struct {
		InputTokens         *int `json:"input_tokens"`
		OutputTokens        *int `json:"output_tokens"`
		PromptTokens        *int `json:"prompt_tokens"`
		CompletionTokens    *int `json:"completion_tokens"`
		CachedTokens        int  `json:"cached_tokens"`
		CacheCreationTokens int  `json:"cache_creation_input_tokens"`
		CacheCreation5m     int  `json:"cache_creation_ephemeral_5m_input_tokens"`
		CacheCreation1h     int  `json:"cache_creation_ephemeral_1h_input_tokens"`
		ImageOutputTokens   int  `json:"image_output_tokens"`
		InputTokensDetails  *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails *struct {
			ImageTokens     int `json:"image_tokens"`
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails *struct {
			ImageTokens     int `json:"image_tokens"`
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	}
	var event struct {
		Response *struct {
			Usage *rawResponsesUsage `json:"usage"`
		} `json:"response"`
		Usage *rawResponsesUsage `json:"usage"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(payload), &event); err != nil {
		return gatewaycontract.Usage{}, false
	}
	rawUsage := event.Usage
	if rawUsage == nil && event.Response != nil {
		rawUsage = event.Response.Usage
	}
	if rawUsage == nil {
		return gatewaycontract.Usage{}, false
	}
	inputTokens, inputOK := responsesWebSocketUsageToken(rawUsage.InputTokens, rawUsage.PromptTokens)
	outputTokens, outputOK := responsesWebSocketUsageToken(rawUsage.OutputTokens, rawUsage.CompletionTokens)
	cachedTokens := rawUsage.CachedTokens
	if cachedTokens == 0 && rawUsage.InputTokensDetails != nil {
		cachedTokens = rawUsage.InputTokensDetails.CachedTokens
	}
	if cachedTokens == 0 && rawUsage.PromptTokensDetails != nil {
		cachedTokens = rawUsage.PromptTokensDetails.CachedTokens
	}
	imageOutputTokens := rawUsage.ImageOutputTokens
	if imageOutputTokens == 0 && rawUsage.OutputTokensDetails != nil {
		imageOutputTokens = rawUsage.OutputTokensDetails.ImageTokens
	}
	if imageOutputTokens == 0 && rawUsage.CompletionTokensDetails != nil {
		imageOutputTokens = rawUsage.CompletionTokensDetails.ImageTokens
	}
	reasoningOutputTokens := 0
	if rawUsage.OutputTokensDetails != nil {
		reasoningOutputTokens = rawUsage.OutputTokensDetails.ReasoningTokens
	}
	if reasoningOutputTokens == 0 && rawUsage.CompletionTokensDetails != nil {
		reasoningOutputTokens = rawUsage.CompletionTokensDetails.ReasoningTokens
	}
	if outputTokens < imageOutputTokens+reasoningOutputTokens {
		outputTokens = imageOutputTokens + reasoningOutputTokens
	}
	cacheCreationTokens := rawUsage.CacheCreationTokens
	if cacheCreationTokens == 0 {
		cacheCreationTokens = rawUsage.CacheCreation5m + rawUsage.CacheCreation1h
	}
	cacheCreation5m, cacheCreation1h := responsesWebSocketCacheCreationBuckets(cacheCreationTokens, rawUsage.CacheCreation5m, rawUsage.CacheCreation1h)
	if !inputOK && !outputOK && cacheCreationTokens == 0 {
		return gatewaycontract.Usage{}, false
	}
	if inputOK != outputOK {
		return gatewaycontract.Usage{}, false
	}
	return gatewaycontract.Usage{
		InputTokens:           max(0, inputTokens-cachedTokens),
		OutputTokens:          outputTokens,
		ImageOutputTokens:     imageOutputTokens,
		CachedTokens:          cachedTokens,
		CacheCreationTokens:   cacheCreationTokens,
		CacheCreation5mTokens: cacheCreation5m,
		CacheCreation1hTokens: cacheCreation1h,
	}, true
}

func responsesWebSocketCacheCreationBuckets(total int, fiveMinutes int, oneHour int) (int, int) {
	if total > 0 && fiveMinutes == 0 && oneHour == 0 {
		return total, 0
	}
	if total == 0 {
		return fiveMinutes, oneHour
	}
	if fiveMinutes+oneHour == total {
		return fiveMinutes, oneHour
	}
	return total, 0
}

func responsesWebSocketUsageToken(primary *int, fallback *int) (int, bool) {
	if primary != nil {
		return *primary, true
	}
	if fallback != nil {
		return *fallback, true
	}
	return 0, false
}

func responsesWebSocketTerminal(payload []byte) (bool, bool, string, int) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(payload), &event); err != nil {
		return false, false, "", 0
	}
	switch rawString(event["type"]) {
	case "response.completed":
		return true, true, "", http.StatusOK
	case "response.done":
		status := responsesWebSocketResponseStatus(event["response"])
		if status == "failed" {
			return true, false, "provider_5xx", http.StatusBadGateway
		}
		if status == "incomplete" || status == "cancelled" || status == "canceled" {
			return true, true, "", http.StatusOK
		}
		return true, true, "", http.StatusOK
	case "response.incomplete", "response.cancelled", "response.canceled":
		return true, true, "", http.StatusOK
	case "response.failed", "error":
		return true, false, "provider_5xx", http.StatusBadGateway
	default:
		return false, false, "", 0
	}
}

func responsesWebSocketResponseStatus(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var response struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(response.Status))
}

func providerStatusFromError(err error) int {
	var runtimeErr reverseproxycontract.RuntimeError
	if errors.As(err, &runtimeErr) && runtimeErr.StatusCode > 0 {
		return runtimeErr.StatusCode
	}
	var providerErr provideradaptercontract.ProviderError
	if errors.As(err, &providerErr) && providerErr.StatusCode > 0 {
		return providerErr.StatusCode
	}
	return http.StatusBadGateway
}

func accountCodexWebSocketEnabled(metadata map[string]any) bool {
	for _, key := range []string{
		"codex_responses_websocket",
		"codex_responses_websockets",
		"responses_websockets_v2_enabled",
		"openai_oauth_responses_websockets_v2_enabled",
	} {
		if metadataBool(metadata, key) {
			return true
		}
	}
	return false
}

func handleResponsesWebSocketControl(ctx context.Context, conn *websocket.Conn, payload []byte) (bool, error) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(payload), &event); err != nil {
		return false, nil
	}
	eventType := rawString(event["type"])
	switch eventType {
	case "":
		return false, nil
	case "ping":
		return true, writeResponsesWebSocketJSON(ctx, conn, map[string]any{"type": "pong"})
	default:
		return false, nil
	}
}

func (s *Server) captureResponsesRequest(original *http.Request, payload []byte) (*gatewayCaptureResponse, error) {
	internal, err := http.NewRequestWithContext(original.Context(), http.MethodPost, "/v1/responses", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	internal.Header = original.Header.Clone()
	internal.Header.Set("Content-Type", "application/json")
	internal.URL.RawQuery = original.URL.RawQuery
	clearWebSocketUpgradeHeaders(internal.Header)

	route := gatewayRouteContext{SourceEndpoint: responsesWebSocketSourceEndpoint}
	internal = internal.WithContext(context.WithValue(internal.Context(), gatewayRouteContextKey{}, route))
	captured := newGatewayCaptureResponse()
	s.handleCreateResponse(captured, internal)
	return captured, nil
}

func clearWebSocketUpgradeHeaders(headers http.Header) {
	for _, key := range []string{
		"Connection",
		"Upgrade",
		"Sec-WebSocket-Accept",
		"Sec-WebSocket-Extensions",
		"Sec-WebSocket-Key",
		"Sec-WebSocket-Protocol",
		"Sec-WebSocket-Version",
	} {
		headers.Del(key)
	}
}

func responsesWebSocketRequestPayload(payload []byte, fallbackModel string) ([]byte, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil, errors.New("empty responses websocket message")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, errors.New("invalid JSON responses websocket message")
	}

	requestPayload := payload
	if eventType := rawString(object["type"]); eventType != "" {
		if eventType != "response.create" {
			return nil, errors.New("unsupported responses websocket event type")
		}
		raw := object["response"]
		if len(bytes.TrimSpace(raw)) == 0 {
			raw = object["request"]
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			requestPayload = raw
		} else {
			delete(object, "type")
			delete(object, "event_id")
			flatPayload, err := json.Marshal(object)
			if err != nil {
				return nil, err
			}
			requestPayload = flatPayload
		}
	}
	return injectResponsesWebSocketModel(requestPayload, fallbackModel)
}

func injectResponsesWebSocketModel(payload []byte, fallbackModel string) ([]byte, error) {
	fallbackModel = strings.TrimSpace(fallbackModel)
	if fallbackModel == "" {
		return payload, nil
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(payload), &request); err != nil {
		return nil, errors.New("response.create payload must be a JSON object")
	}
	if model := rawString(request["model"]); model != "" {
		return payload, nil
	}
	encodedModel, err := json.Marshal(fallbackModel)
	if err != nil {
		return nil, err
	}
	request["model"] = encodedModel
	return json.Marshal(request)
}

func responsesWebSocketInferPreviousResponseID(payload []byte, lastTurn responsesWebSocketTurnState) ([]byte, error) {
	if strings.TrimSpace(lastTurn.ResponseID) == "" || len(lastTurn.ToolCallIDs) == 0 {
		return payload, nil
	}
	var request map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(payload), &request); err != nil {
		return payload, nil
	}
	if strings.TrimSpace(anyString(request["previous_response_id"])) != "" {
		return payload, nil
	}
	callIDs := responsesWebSocketToolOutputCallIDs(request["input"])
	if len(callIDs) == 0 {
		return payload, nil
	}
	for _, callID := range callIDs {
		if _, ok := lastTurn.ToolCallIDs[callID]; !ok {
			return payload, nil
		}
	}
	request["previous_response_id"] = lastTurn.ResponseID
	return json.Marshal(request)
}

func responsesWebSocketToolOutputCallIDs(value any) []string {
	switch typed := value.(type) {
	case []any:
		var out []string
		for _, item := range typed {
			out = append(out, responsesWebSocketToolOutputCallIDs(item)...)
		}
		return out
	case map[string]any:
		itemType := strings.TrimSpace(anyString(typed["type"]))
		if responsesWebSocketToolOutputType(itemType) {
			if callID := strings.TrimSpace(anyString(typed["call_id"])); callID != "" {
				return []string{callID}
			}
		}
		return responsesWebSocketToolOutputCallIDs(typed["content"])
	default:
		return nil
	}
}

func responsesWebSocketToolOutputType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "function_call_output", "tool_search_output", "custom_tool_call_output", "mcp_tool_call_output":
		return true
	default:
		return false
	}
}

type responsesWebSocketTurnState struct {
	ResponseID  string
	ToolCallIDs map[string]struct{}
}

func (state *responsesWebSocketTurnState) merge(next responsesWebSocketTurnState) {
	if next.ResponseID != "" {
		state.ResponseID = next.ResponseID
	}
	if len(next.ToolCallIDs) == 0 {
		return
	}
	if state.ToolCallIDs == nil {
		state.ToolCallIDs = map[string]struct{}{}
	}
	for callID := range next.ToolCallIDs {
		state.ToolCallIDs[callID] = struct{}{}
	}
}

func responsesWebSocketTurnStateFromEvent(payload []byte) responsesWebSocketTurnState {
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(payload), &event); err != nil {
		return responsesWebSocketTurnState{}
	}
	state := responsesWebSocketTurnState{
		ResponseID:  responsesWebSocketEventResponseID(event),
		ToolCallIDs: map[string]struct{}{},
	}
	responsesWebSocketCollectToolCallIDs(event["response"], state.ToolCallIDs)
	responsesWebSocketCollectToolCallIDs(event["item"], state.ToolCallIDs)
	responsesWebSocketCollectToolCallIDs(event["output_item"], state.ToolCallIDs)
	responsesWebSocketCollectToolCallIDs(event["output"], state.ToolCallIDs)
	if len(state.ToolCallIDs) == 0 {
		state.ToolCallIDs = nil
	}
	return state
}

func responsesWebSocketEventResponseID(event map[string]any) string {
	if response, ok := event["response"].(map[string]any); ok {
		if id := strings.TrimSpace(anyString(response["id"])); id != "" {
			return id
		}
	}
	return strings.TrimSpace(firstNonEmpty(anyString(event["response_id"]), anyString(event["id"])))
}

func responsesWebSocketCollectToolCallIDs(value any, out map[string]struct{}) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			responsesWebSocketCollectToolCallIDs(item, out)
		}
	case map[string]any:
		itemType := strings.TrimSpace(anyString(typed["type"]))
		if responsesWebSocketToolCallType(itemType) {
			if callID := strings.TrimSpace(firstNonEmpty(anyString(typed["call_id"]), anyString(typed["id"]))); callID != "" {
				out[callID] = struct{}{}
			}
		}
		responsesWebSocketCollectToolCallIDs(typed["output"], out)
		responsesWebSocketCollectToolCallIDs(typed["content"], out)
	}
}

func responsesWebSocketToolCallType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "function_call", "tool_call", "local_shell_call", "tool_search_call", "custom_tool_call", "mcp_tool_call":
		return true
	default:
		return false
	}
}

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func writeCapturedResponsesWebSocket(ctx context.Context, conn *websocket.Conn, captured *gatewayCaptureResponse, idleSession *wsIdleSession) (responsesWebSocketTurnState, error) {
	body := bytes.TrimSpace(captured.body.Bytes())
	if captured.Status() >= http.StatusBadRequest {
		return responsesWebSocketTurnState{}, writeResponsesWebSocketError(ctx, conn, captured.Status(), "", "", body)
	}
	var turnState responsesWebSocketTurnState
	if strings.Contains(captured.Header().Get("Content-Type"), "text/event-stream") {
		events := parseResponsesServerSentEvents(body)
		for _, event := range events {
			data := bytes.TrimSpace(event.Data)
			if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
				continue
			}
			if jsonObjectHasType(data) {
				turnState.merge(responsesWebSocketTurnStateFromEvent(data))
				if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
					return turnState, err
				}
				idleSession.SetActive()
				continue
			}
			wrapped := map[string]any{
				"type": "response.stream_event",
				"data": string(data),
			}
			if event.Event != "" {
				wrapped["event"] = event.Event
			}
			if err := writeResponsesWebSocketJSON(ctx, conn, wrapped); err != nil {
				return turnState, err
			}
			idleSession.SetActive()
		}
		return turnState, nil
	}
	if len(body) == 0 {
		return responsesWebSocketTurnState{}, writeResponsesWebSocketError(ctx, conn, http.StatusInternalServerError, "internal_error", "empty responses gateway result", nil)
	}
	turnState = responsesWebSocketTurnStateFromEvent(body)
	err := writeResponsesWebSocketJSON(ctx, conn, map[string]any{
		"type":     "response.completed",
		"response": json.RawMessage(body),
	})
	if err == nil {
		idleSession.SetActive()
	}
	return turnState, err
}

type responsesServerSentEvent struct {
	Event string
	Data  []byte
}

func parseResponsesServerSentEvents(payload []byte) []responsesServerSentEvent {
	blocks := bytes.Split(payload, []byte("\n\n"))
	events := make([]responsesServerSentEvent, 0, len(blocks))
	for _, block := range blocks {
		block = bytes.TrimSpace(block)
		if len(block) == 0 {
			continue
		}
		var event responsesServerSentEvent
		for _, line := range bytes.Split(block, []byte("\n")) {
			line = bytes.TrimSpace(line)
			switch {
			case bytes.HasPrefix(line, []byte("event:")):
				event.Event = strings.TrimSpace(string(bytes.TrimPrefix(line, []byte("event:"))))
			case bytes.HasPrefix(line, []byte("data:")):
				data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
				if len(event.Data) > 0 {
					event.Data = append(event.Data, '\n')
				}
				event.Data = append(event.Data, data...)
			}
		}
		if len(event.Data) > 0 || event.Event != "" {
			events = append(events, event)
		}
	}
	return events
}

func writeResponsesWebSocketError(ctx context.Context, conn *websocket.Conn, status int, code string, message string, rawBody []byte) error {
	event := map[string]any{
		"type":   "error",
		"status": status,
	}
	if raw := gatewayErrorRawMessage(rawBody); len(raw) > 0 {
		event["error"] = raw
	} else {
		if message == "" {
			message = http.StatusText(status)
		}
		errorBody := map[string]any{"message": message}
		if code != "" {
			errorBody["code"] = code
		}
		event["error"] = errorBody
	}
	return writeResponsesWebSocketJSON(ctx, conn, event)
}

func gatewayErrorRawMessage(rawBody []byte) json.RawMessage {
	rawBody = bytes.TrimSpace(rawBody)
	if len(rawBody) == 0 || !json.Valid(rawBody) {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &object); err == nil {
		if raw := bytes.TrimSpace(object["error"]); len(raw) > 0 && json.Valid(raw) {
			return json.RawMessage(raw)
		}
	}
	return json.RawMessage(rawBody)
}

func writeResponsesWebSocketJSON(ctx context.Context, conn *websocket.Conn, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, encoded)
}

func rawString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func jsonObjectHasType(raw []byte) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return false
	}
	return rawString(object["type"]) != ""
}

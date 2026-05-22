package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"nhooyr.io/websocket"
)

const responsesWebSocketSourceEndpoint = "/v1/responses/ws"
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

	for {
		messageType, payload, err := conn.Read(r.Context())
		if err != nil {
			if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				return
			}
			if !errors.Is(err, context.Canceled) {
				s.logger.Debug("responses websocket closed", "error", err, "request_id", requestID)
			}
			return
		}
		if messageType != websocket.MessageText {
			if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusBadRequest, "invalid_request", "responses websocket only accepts JSON text frames", nil); err != nil {
				return
			}
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
		if s.shouldUseCodexWebSocketRelay(r, requestPayload) {
			relayed, err := s.relayCodexResponsesWebSocket(r, conn, requestPayload, authed)
			if err != nil {
				if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusBadGateway, "upstream_error", err.Error(), nil); err != nil {
					return
				}
				continue
			}
			if relayed {
				return
			}
		}

		captured, err := s.captureResponsesRequest(r, requestPayload)
		if err != nil {
			if err := writeResponsesWebSocketError(r.Context(), conn, http.StatusInternalServerError, "internal_error", "failed to execute responses request", nil); err != nil {
				return
			}
			continue
		}
		if err := writeCapturedResponsesWebSocket(r.Context(), conn, captured); err != nil {
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

func (s *Server) relayCodexResponsesWebSocket(r *http.Request, conn *websocket.Conn, payload []byte, authed apikeycontract.AuthResult) (bool, error) {
	startedAt := time.Now()
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := responsesWebSocketSourceEndpoint
	modelName := responsesWebSocketPayloadModel(payload, r.URL.Query().Get("model"))
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), modelName)
	if err != nil {
		return false, err
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		return false, errors.New("model not allowed for this api key")
	}
	body, err := responsesWebSocketOpenAPIRequest(payload)
	if err != nil {
		return false, err
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
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), canonical, modelResolution, model.ID)
	if err != nil {
		return false, err
	}
	if !admission.Entitlement.Allowed {
		return false, errors.New(gatewayEntitlementMessage(gatewayEntitlementErrorClass(admission.Entitlement)))
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	result, err := s.runtime.scheduleGatewayRequest(r.Context(), scheduleReq, model.ID, gatewayForcedProviderKey(r.Context()), authed.Key)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, nil, false, "no_available_account", http.StatusServiceUnavailable, elapsedMillis(startedAt), admission, nil))
		return false, err
	}
	if !strings.EqualFold(strings.TrimSpace(result.Candidate.Provider.AdapterType), "reverse-proxy-codex-cli") || !accountCodexWebSocketEnabled(result.Candidate.Account.Metadata) {
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, false, "invalid_request", http.StatusBadRequest, elapsedMillis(startedAt), admission, nil))
		return true, errors.New("selected account does not support Codex Responses WebSocket reverse proxy")
	}
	session, credential, err := s.runtime.prepareProviderRealtime(r.Context(), providerRealtimeRequest(canonical, result.Candidate, payload))
	if err != nil {
		errorClass, upstreamStatus, _ := providerGatewayError(err)
		s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, false, errorClass, upstreamStatus, elapsedMillis(startedAt), admission, nil))
		return true, err
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
	success, errorClass, statusCode, usage := s.bridgeResponsesWebSocketRelay(r.Context(), conn, upstreamToClient, relayDone)
	s.runtime.recordGatewayUsage(r.Context(), responsesWebSocketUsageRecord(authed, canonical, result, &result.Candidate, success, errorClass, statusCode, elapsedMillis(startedAt), admission, usage))
	if !success && errorClass != "client_closed" {
		return true, provideradaptercontract.ProviderError{Class: errorClass, StatusCode: statusCode, Message: providerGatewayMessage(errorClass)}
	}
	return true, nil
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

func (s *Server) bridgeResponsesWebSocketRelay(ctx context.Context, conn *websocket.Conn, upstreamToClient <-chan reverseproxycontract.WebSocketMessage, relayDone <-chan responsesWebSocketRelayResult) (bool, string, int, *gatewaycontract.Usage) {
	var usage *gatewaycontract.Usage
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
						return false, "client_closed", statusClientClosedRequest, usage
					}
				}
				if relayResult != nil && relayResult.err != nil {
					return false, errorClassName(relayResult.err), providerStatusFromError(relayResult.err), usage
				}
				return true, "", http.StatusOK, usage
			}
			if msg.Type != reverseproxycontract.WebSocketMessageText {
				continue
			}
			if eventUsage, ok := responsesWebSocketUsage(msg.Data); ok {
				usage = &eventUsage
			}
			if err := conn.Write(ctx, websocket.MessageText, msg.Data); err != nil {
				return false, "client_closed", statusClientClosedRequest, usage
			}
			if responsesWebSocketTerminal(msg.Data) {
				return true, "", http.StatusOK, usage
			}
		case result := <-relayDoneCh:
			relayResult = &result
			relayDoneCh = nil
		case <-ctx.Done():
			return false, "client_closed", statusClientClosedRequest, usage
		}
	}
}

func responsesWebSocketUsageRecord(authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, candidate *schedulercontract.Candidate, success bool, errorClass string, statusCode int, latencyMS int, admission gatewayAdmission, usage *gatewaycontract.Usage) gatewayUsageRecord {
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
		Success:               success,
		StatusCode:            ptrInt(statusCode),
		LatencyMS:             latencyMS,
		InputTokens:           recordUsage.InputTokens,
		OutputTokens:          recordUsage.OutputTokens,
		CachedTokens:          recordUsage.CachedTokens,
		UsageEstimated:        estimated,
		Pricing:               admission.Pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
	}
	if errorClass != "" {
		rec.ErrorClass = ptrStringValue(errorClass)
	}
	if candidate != nil {
		rec.ProviderID = ptrInt(candidate.Provider.ID)
		rec.AccountID = ptrInt(candidate.Account.ID)
		rec.TargetProtocol = candidate.Provider.Protocol
	}
	return rec
}

func (rt *runtimeState) prepareProviderRealtime(ctx context.Context, req provideradaptercontract.RealtimeRequest) (providerRealtimeSession, map[string]any, error) {
	if req.Account.ID <= 0 {
		return providerRealtimeSession{}, nil, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
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

func providerRealtimeRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate, payload []byte) provideradaptercontract.RealtimeRequest {
	return provideradaptercontract.RealtimeRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		RequestPayload: append([]byte(nil), payload...),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
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
	var event struct {
		Response *struct {
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
				CachedTokens int `json:"cached_tokens"`
			} `json:"usage"`
		} `json:"response"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			CachedTokens int `json:"cached_tokens"`
		} `json:"usage"`
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
	return gatewaycontract.Usage{InputTokens: rawUsage.InputTokens, OutputTokens: rawUsage.OutputTokens, CachedTokens: rawUsage.CachedTokens}, true
}

func responsesWebSocketTerminal(payload []byte) bool {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(payload), &event); err != nil {
		return false
	}
	switch rawString(event["type"]) {
	case "response.completed", "response.done", "error":
		return true
	default:
		return false
	}
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
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil, errors.New("response.create event must include a response request")
		}
		requestPayload = raw
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

func writeCapturedResponsesWebSocket(ctx context.Context, conn *websocket.Conn, captured *gatewayCaptureResponse) error {
	body := bytes.TrimSpace(captured.body.Bytes())
	if captured.Status() >= http.StatusBadRequest {
		return writeResponsesWebSocketError(ctx, conn, captured.Status(), "", "", body)
	}
	if strings.Contains(captured.Header().Get("Content-Type"), "text/event-stream") {
		events := parseResponsesServerSentEvents(body)
		for _, event := range events {
			data := bytes.TrimSpace(event.Data)
			if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
				continue
			}
			if jsonObjectHasType(data) {
				if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
					return err
				}
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
				return err
			}
		}
		return nil
	}
	if len(body) == 0 {
		return writeResponsesWebSocketError(ctx, conn, http.StatusInternalServerError, "internal_error", "empty responses gateway result", nil)
	}
	return writeResponsesWebSocketJSON(ctx, conn, map[string]any{
		"type":     "response.completed",
		"response": json.RawMessage(body),
	})
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

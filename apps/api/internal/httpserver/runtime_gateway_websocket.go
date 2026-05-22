package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"nhooyr.io/websocket"
)

const responsesWebSocketSourceEndpoint = "/v1/responses/ws"

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
	if _, err := s.requireGatewayKey(r); err != nil {
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

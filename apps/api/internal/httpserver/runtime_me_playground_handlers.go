package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// playgroundKeyName marks the per-user API key the 交界地 playground bills against.
const playgroundKeyName = "交界地 Playground"

// handleMePlaygroundModels lists the active models a user can pick in the
// playground. The gateway still enforces per-user entitlement at send time.
func (s *Server) handleMePlaygroundModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireConsoleSession(r); err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	out := make([]apiopenapi.PlaygroundModel, 0, len(models))
	for _, m := range models {
		if m.Status != modelcontract.StatusActive {
			continue
		}
		name := m.DisplayName
		if strings.TrimSpace(name) == "" {
			name = m.CanonicalName
		}
		out = append(out, apiopenapi.PlaygroundModel{Id: m.CanonicalName, Name: name})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PlaygroundModelsResponse{Data: out, RequestId: requestID})
}

// handleMePlaygroundChat streams a billed chat completion for the signed-in user.
// It builds a normal OpenAI gateway request (no tools) and runs the shared
// serveChatCompletion core, so balance, subscription, quota, RPM, entitlement,
// and metering all apply exactly like an API request — but session-authenticated.
func (s *Server) handleMePlaygroundChat(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	startedAt := time.Now()
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var req apiopenapi.PlaygroundChatRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid playground request", requestID)
		return
	}
	if strings.TrimSpace(req.Model) == "" || len(req.Messages) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "model and messages are required", requestID)
		return
	}
	authed, err := s.ensurePlaygroundAuth(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to prepare playground", requestID)
		return
	}
	rawBody, body, err := buildPlaygroundChatBody(req)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid playground request", requestID)
		return
	}
	s.serveChatCompletion(w, r, authed, body, rawBody, "/api/v1/me/playground", "", startedAt)
}

// ensurePlaygroundAuth resolves (find-or-create) the user's playground API key
// and returns an auth result the gateway core bills against. The key has no
// AllowedModels restriction so the user's subscription/group entitlements (which
// admission enforces) govern access; its plaintext is never exposed.
func (s *Server) ensurePlaygroundAuth(ctx context.Context, userID int) (apikeycontract.AuthResult, error) {
	keys, err := s.runtime.apiKeys.ListByUser(ctx, userID)
	if err != nil {
		return apikeycontract.AuthResult{}, err
	}
	for _, k := range keys {
		if k.Name == playgroundKeyName && k.Status == apikeycontract.StatusActive {
			return apikeycontract.AuthResult{Key: k, UserID: userID}, nil
		}
	}
	created, err := s.runtime.apiKeys.Create(ctx, apikeycontract.CreateRequest{
		UserID: userID,
		Name:   playgroundKeyName,
	})
	if err != nil {
		return apikeycontract.AuthResult{}, err
	}
	return apikeycontract.AuthResult{Key: created.Key, UserID: userID}, nil
}

// buildPlaygroundChatBody converts the playground request into a streaming
// OpenAI ChatCompletionRequest (raw bytes + decoded struct), mapping image
// attachments into multimodal content parts. No tools are ever included.
func buildPlaygroundChatBody(req apiopenapi.PlaygroundChatRequest) ([]byte, apiopenapi.ChatCompletionRequest, error) {
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		text := ""
		if m.Content != nil {
			text = *m.Content
		}
		if m.Images != nil && len(*m.Images) > 0 {
			parts := make([]map[string]any, 0, len(*m.Images)+1)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, map[string]any{"type": "text", "text": text})
			}
			for _, img := range *m.Images {
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "data:" + img.MimeType + ";base64," + img.Data},
				})
			}
			messages = append(messages, map[string]any{"role": string(m.Role), "content": parts})
			continue
		}
		messages = append(messages, map[string]any{"role": string(m.Role), "content": text})
	}
	payload := map[string]any{
		"model":    req.Model,
		"stream":   true,
		"messages": messages,
	}
	if req.ReasoningEffort != nil {
		if effort := string(*req.ReasoningEffort); effort != "" && effort != "off" {
			payload["reasoning_effort"] = effort
		}
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, apiopenapi.ChatCompletionRequest{}, err
	}
	var body apiopenapi.ChatCompletionRequest
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, apiopenapi.ChatCompletionRequest{}, err
	}
	return rawBody, body, nil
}

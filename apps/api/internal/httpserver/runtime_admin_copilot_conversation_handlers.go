package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	copilotconvcontract "github.com/srapi/srapi/apps/api/internal/modules/copilot/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// copilotConversationActor authorizes a copilot-history request and returns the
// signed-in admin's user id. Conversations are per-admin; the owner-only copilot
// gate is applied so a restricted copilot's history is restricted too.
func (s *Server) copilotConversationActor(w http.ResponseWriter, r *http.Request, requireCSRF bool) (int, bool) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return 0, false
	}
	if requireCSRF {
		if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
			return 0, false
		}
	}
	if settings, _, sErr := s.copilotSettings(r.Context()); sErr == nil && settings.OwnerOnly &&
		!sessionHasRole(session.User.Roles, userscontract.RoleOwner) {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "owner access required", requestID)
		return 0, false
	}
	if s.runtime.copilotConvsStore == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "conversation history unavailable", requestID)
		return 0, false
	}
	return session.User.ID, true
}

func (s *Server) handleListAdminCopilotConversations(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	actorID, ok := s.copilotConversationActor(w, r, false)
	if !ok {
		return
	}
	rows, err := s.runtime.copilotConvsStore.ListByAdmin(r.Context(), actorID, 200)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list conversations", requestID)
		return
	}
	summaries := make([]apiopenapi.CopilotConversationSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, apiopenapi.CopilotConversationSummary{
			Id:        row.ID,
			Title:     row.Title,
			UpdatedAt: row.UpdatedAt,
		})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.CopilotConversationListResponse{Data: summaries, RequestId: requestID})
}

func (s *Server) handleGetAdminCopilotConversation(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	actorID, ok := s.copilotConversationActor(w, r, false)
	if !ok {
		return
	}
	id, ok := copilotConversationID(w, r, requestID)
	if !ok {
		return
	}
	conv, err := s.runtime.copilotConvsStore.Get(r.Context(), actorID, id)
	if err != nil {
		writeCopilotConversationError(w, err, requestID)
		return
	}
	s.writeCopilotConversation(w, http.StatusOK, conv, requestID)
}

func (s *Server) handleCreateAdminCopilotConversation(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	actorID, ok := s.copilotConversationActor(w, r, true)
	if !ok {
		return
	}
	var body apiopenapi.CopilotConversationCreateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid conversation request", requestID)
		return
	}
	conv, err := s.runtime.copilotConvsStore.Create(r.Context(), actorID, copilotConversationTitle(body.Title, body.Messages), copilotMessagesJSON(body.Messages))
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to save conversation", requestID)
		return
	}
	s.writeCopilotConversation(w, http.StatusCreated, conv, requestID)
}

func (s *Server) handleUpdateAdminCopilotConversation(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	actorID, ok := s.copilotConversationActor(w, r, true)
	if !ok {
		return
	}
	id, ok := copilotConversationID(w, r, requestID)
	if !ok {
		return
	}
	var body apiopenapi.CopilotConversationCreateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid conversation request", requestID)
		return
	}
	conv, err := s.runtime.copilotConvsStore.Update(r.Context(), actorID, id, copilotConversationTitle(body.Title, body.Messages), copilotMessagesJSON(body.Messages))
	if err != nil {
		writeCopilotConversationError(w, err, requestID)
		return
	}
	s.writeCopilotConversation(w, http.StatusOK, conv, requestID)
}

func (s *Server) handleRenameAdminCopilotConversation(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	actorID, ok := s.copilotConversationActor(w, r, true)
	if !ok {
		return
	}
	id, ok := copilotConversationID(w, r, requestID)
	if !ok {
		return
	}
	var body apiopenapi.CopilotConversationRenameRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid conversation request", requestID)
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "title is required", requestID)
		return
	}
	conv, err := s.runtime.copilotConvsStore.Rename(r.Context(), actorID, id, title)
	if err != nil {
		writeCopilotConversationError(w, err, requestID)
		return
	}
	s.writeCopilotConversation(w, http.StatusOK, conv, requestID)
}

func (s *Server) handleDeleteAdminCopilotConversation(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	actorID, ok := s.copilotConversationActor(w, r, true)
	if !ok {
		return
	}
	id, ok := copilotConversationID(w, r, requestID)
	if !ok {
		return
	}
	if err := s.runtime.copilotConvsStore.Delete(r.Context(), actorID, id); err != nil {
		writeCopilotConversationError(w, err, requestID)
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusNoContent)
}

func copilotConversationID(w http.ResponseWriter, r *http.Request, requestID string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid conversation id", requestID)
		return 0, false
	}
	return id, true
}

func (s *Server) writeCopilotConversation(w http.ResponseWriter, status int, conv copilotconvcontract.Conversation, requestID string) {
	var msgs []apiopenapi.AdminCopilotMessage
	if len(conv.Messages) > 0 {
		if err := json.Unmarshal(conv.Messages, &msgs); err != nil {
			msgs = []apiopenapi.AdminCopilotMessage{}
		}
	}
	if msgs == nil {
		msgs = []apiopenapi.AdminCopilotMessage{}
	}
	writeJSONAny(w, status, apiopenapi.CopilotConversationResponse{
		Data: apiopenapi.CopilotConversation{
			Id:        conv.ID,
			Title:     conv.Title,
			Messages:  msgs,
			CreatedAt: conv.CreatedAt,
			UpdatedAt: conv.UpdatedAt,
		},
		RequestId: requestID,
	})
}

func writeCopilotConversationError(w http.ResponseWriter, err error, requestID string) {
	if errors.Is(err, copilotconvcontract.ErrNotFound) {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "conversation not found", requestID)
		return
	}
	writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "conversation request failed", requestID)
}

// copilotMessagesJSON marshals the request messages to the opaque JSON array we
// persist (the same shape the chat endpoint round-trips).
func copilotMessagesJSON(messages []apiopenapi.AdminCopilotMessage) json.RawMessage {
	if len(messages) == 0 {
		return json.RawMessage("[]")
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		return json.RawMessage("[]")
	}
	return raw
}

// copilotConversationTitle uses the provided title, else derives one from the
// first user message, else a generic fallback.
func copilotConversationTitle(title *string, messages []apiopenapi.AdminCopilotMessage) string {
	if title != nil && strings.TrimSpace(*title) != "" {
		return truncateRunes(strings.TrimSpace(*title), 80)
	}
	for _, m := range messages {
		if m.Role == apiopenapi.AdminCopilotMessageRoleUser && m.Content != nil {
			if c := strings.TrimSpace(*m.Content); c != "" {
				return truncateRunes(c, 60)
			}
		}
	}
	return "New conversation"
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

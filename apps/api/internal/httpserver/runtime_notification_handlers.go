package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"
	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type notificationUnsubscribeRequest struct {
	Token string `json:"token"`
}

func (s *Server) handleCurrentUserNotificationContacts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	contacts, err := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	if err != nil {
		writeNotificationContactError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationContactListResponse{
		Data:      toAPINotificationContacts(contacts),
		RequestId: requestID,
	})
}

func (s *Server) handleRequestCurrentUserNotificationContactVerification(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.NotificationContactVerificationRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification contact request", requestID)
		return
	}
	before, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	result, err := s.runtime.notificationContacts.RequestVerification(r.Context(), notificationsservice.ContactVerificationRequest{
		UserID:       session.User.ID,
		UserName:     session.User.Name,
		UserEmail:    session.User.Email,
		ContactEmail: string(body.Email),
	}, &session.User.ID)
	if err != nil {
		writeNotificationContactError(w, err, requestID)
		return
	}
	after, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "notification_contact.request_verification", "user_notification_contacts", strconv.Itoa(session.User.ID), notificationContactAuditSnapshot(before), notificationContactAuditSnapshot(after)))
	writeJSONAny(w, http.StatusAccepted, apiopenapi.NotificationContactVerificationResponse{
		Data: apiopenapi.NotificationContactVerificationAccepted{
			Accepted:         true,
			Contact:          toAPINotificationContact(result.Contact),
			VerificationSent: result.VerificationSent,
			ExpiresAt:        result.ExpiresAt,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleConfirmCurrentUserNotificationContactVerification(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.NotificationContactConfirmRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification contact verification request", requestID)
		return
	}
	before, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	contact, err := s.runtime.notificationContacts.ConfirmVerification(r.Context(), session.User.ID, body.Token, &session.User.ID)
	if err != nil {
		writeNotificationContactError(w, err, requestID)
		return
	}
	after, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "notification_contact.verify", "user_notification_contacts", strconv.Itoa(session.User.ID), notificationContactAuditSnapshot(before), notificationContactAuditSnapshot(after)))
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationContactResponse{
		Data:      toAPINotificationContact(contact),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateCurrentUserNotificationContact(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.UpdateNotificationContactRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification contact request", requestID)
		return
	}
	before, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	contact, err := s.runtime.notificationContacts.SetContactDisabled(r.Context(), session.User.ID, r.PathValue("id"), body.Disabled, &session.User.ID)
	if err != nil {
		writeNotificationContactError(w, err, requestID)
		return
	}
	after, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "notification_contact.update", "user_notification_contacts", strconv.Itoa(session.User.ID), notificationContactAuditSnapshot(before), notificationContactAuditSnapshot(after)))
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationContactResponse{
		Data:      toAPINotificationContact(contact),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteCurrentUserNotificationContact(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	before, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	if err := s.runtime.notificationContacts.DeleteContact(r.Context(), session.User.ID, r.PathValue("id"), &session.User.ID); err != nil {
		writeNotificationContactError(w, err, requestID)
		return
	}
	after, _ := s.runtime.notificationContacts.ListContacts(r.Context(), session.User.ID)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "notification_contact.delete", "user_notification_contacts", strconv.Itoa(session.User.ID), notificationContactAuditSnapshot(before), notificationContactAuditSnapshot(after)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePreviewNotificationUnsubscribe(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	svc, err := s.notificationPreferenceService()
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "notification preferences unavailable", requestID)
		return
	}
	preview, err := svc.PreviewUnsubscribe(r.URL.Query().Get("token"))
	if err != nil {
		writeNotificationPreferenceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationUnsubscribeResponse{
		Data: apiopenapi.NotificationUnsubscribe{
			Done:  preview.Done,
			Event: apiopenapi.NotificationUnsubscribeEvent(preview.Event),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleNotificationUnsubscribe(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	token, err := s.notificationUnsubscribeToken(w, r)
	if err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid unsubscribe request", requestID)
		return
	}
	svc, err := s.notificationPreferenceService()
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "notification preferences unavailable", requestID)
		return
	}
	result, err := svc.Unsubscribe(r.Context(), token)
	if err != nil {
		writeNotificationPreferenceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationUnsubscribeResponse{
		Data: apiopenapi.NotificationUnsubscribe{
			Done:  result.Done,
			Event: apiopenapi.NotificationUnsubscribeEvent(result.Event),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	svc, err := s.notificationPreferenceService()
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "notification preferences unavailable", requestID)
		return
	}
	preferences, err := svc.ListPreferences(r.Context(), session.User.Email)
	if err != nil {
		writeNotificationPreferenceManagementError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationPreferenceListResponse{
		Data:      toAPINotificationPreferences(preferences),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateCurrentUserNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.UpdateNotificationPreferencesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification preferences request", requestID)
		return
	}
	if len(body.Preferences) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification preferences request", requestID)
		return
	}
	seen := map[string]struct{}{}
	svc, err := s.notificationPreferenceService()
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "notification preferences unavailable", requestID)
		return
	}
	before, err := svc.ListPreferences(r.Context(), session.User.Email)
	if err != nil {
		writeNotificationPreferenceManagementError(w, err, requestID)
		return
	}
	for _, preference := range body.Preferences {
		event := strings.TrimSpace(string(preference.Event))
		if _, ok := seen[event]; ok {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification preferences request", requestID)
			return
		}
		seen[event] = struct{}{}
		if _, err := svc.SetPreference(r.Context(), session.User.Email, event, preference.Subscribed, "current_user", &session.User.ID); err != nil {
			writeNotificationPreferenceManagementError(w, err, requestID)
			return
		}
	}
	after, err := svc.ListPreferences(r.Context(), session.User.Email)
	if err != nil {
		writeNotificationPreferenceManagementError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "notification_preferences.update", "user_notification_preferences", strconv.Itoa(session.User.ID), notificationPreferenceAuditSnapshot(before), notificationPreferenceAuditSnapshot(after)))
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationPreferenceListResponse{
		Data:      toAPINotificationPreferences(after),
		RequestId: requestID,
	})
}

func (s *Server) notificationPreferenceService() (*notificationsservice.PreferenceService, error) {
	return notificationsservice.NewPreferenceService(s.runtime.adminControlStore, s.cfg.Security.MasterKey, s.cfg.Email.PublicBaseURL)
}

func (s *Server) notificationUnsubscribeToken(w http.ResponseWriter, r *http.Request) (string, error) {
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token, nil
	}
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "application/json") {
		var body notificationUnsubscribeRequest
		if err := s.decodeJSONBody(w, r, &body); err != nil {
			return "", err
		}
		if token := strings.TrimSpace(body.Token); token != "" {
			return token, nil
		}
		return "", notificationsservice.ErrInvalidInput
	}
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") || strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseForm(); err != nil {
			return "", err
		}
		if token := strings.TrimSpace(r.Form.Get("token")); token != "" {
			return token, nil
		}
	}
	if r.Body != nil {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize))
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(string(raw)) != "" {
			var body notificationUnsubscribeRequest
			if err := json.Unmarshal(raw, &body); err != nil {
				return "", err
			}
			if token := strings.TrimSpace(body.Token); token != "" {
				return token, nil
			}
		}
	}
	return "", notificationsservice.ErrInvalidInput
}

func writeNotificationPreferenceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, notificationsservice.ErrInvalidInput), errors.Is(err, notificationsservice.ErrUnsupportedNotificationEvent):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid unsubscribe token", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update notification preference", requestID)
	}
}

func writeNotificationPreferenceManagementError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, notificationsservice.ErrInvalidInput), errors.Is(err, notificationsservice.ErrUnsupportedNotificationEvent):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification preferences request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update notification preferences", requestID)
	}
}

func writeNotificationContactError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, notificationsservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification contact request", requestID)
	case errors.Is(err, notificationsservice.ErrNotificationContactLimit), errors.Is(err, notificationsservice.ErrNotificationContactConflict):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "notification contact conflict", requestID)
	case errors.Is(err, notificationsservice.ErrNotificationContactNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "notification contact not found", requestID)
	case errors.Is(err, notificationsservice.ErrNotConfigured):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "notification contact verification unavailable", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update notification contact", requestID)
	}
}

func toAPINotificationContacts(items []notificationsservice.NotificationContact) []apiopenapi.NotificationContact {
	out := make([]apiopenapi.NotificationContact, 0, len(items))
	for _, item := range items {
		out = append(out, toAPINotificationContact(item))
	}
	return out
}

func toAPINotificationContact(item notificationsservice.NotificationContact) apiopenapi.NotificationContact {
	return apiopenapi.NotificationContact{
		Id:         item.ID,
		Email:      openapi_types.Email(item.Email),
		EmailHash:  item.EmailHash,
		Verified:   item.Verified,
		Disabled:   item.Disabled,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
		VerifiedAt: item.VerifiedAt,
	}
}

func toAPINotificationPreferences(items []notificationsservice.EmailPreference) []apiopenapi.NotificationPreference {
	out := make([]apiopenapi.NotificationPreference, 0, len(items))
	for _, item := range items {
		out = append(out, apiopenapi.NotificationPreference{
			Category:    item.Category,
			Description: item.Description,
			Event:       apiopenapi.NotificationPreferenceEventName(item.Event),
			Label:       item.Label,
			Subscribed:  item.Subscribed,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	return out
}

func notificationPreferenceAuditSnapshot(items []notificationsservice.EmailPreference) map[string]any {
	out := map[string]any{"preferences": make([]map[string]any, 0, len(items))}
	preferences := out["preferences"].([]map[string]any)
	for _, item := range items {
		entry := map[string]any{
			"event":      item.Event,
			"subscribed": item.Subscribed,
		}
		if item.UpdatedAt != nil {
			entry["updated_at"] = item.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
		}
		preferences = append(preferences, entry)
	}
	out["preferences"] = preferences
	return out
}

func notificationContactAuditSnapshot(items []notificationsservice.NotificationContact) map[string]any {
	out := map[string]any{"contacts": make([]map[string]any, 0, len(items))}
	contacts := out["contacts"].([]map[string]any)
	for _, item := range items {
		entry := map[string]any{
			"id":         item.ID,
			"email_hash": item.EmailHash,
			"verified":   item.Verified,
			"disabled":   item.Disabled,
		}
		if item.VerifiedAt != nil {
			entry["verified_at"] = item.VerifiedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
		}
		contacts = append(contacts, entry)
	}
	out["contacts"] = contacts
	return out
}

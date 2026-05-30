package httpserver

import (
	"errors"
	"net/http"
	"strings"

	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminNotificationEmailTemplates(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	list := notificationsservice.ListEmailTemplates(settings.Email.Templates)
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationEmailTemplateListResponse{
		Data:      toAPINotificationEmailTemplateList(list),
		RequestId: requestID,
	})
}

func (s *Server) handleGetAdminNotificationEmailTemplate(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	detail, err := notificationsservice.GetEmailTemplate(settings.Email.Templates, notificationTemplateEventFromRequest(r))
	if err != nil {
		writeNotificationTemplateError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationEmailTemplateResponse{
		Data:      toAPINotificationEmailTemplate(detail),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminNotificationEmailTemplate(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.UpdateNotificationEmailTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification email template request", requestID)
		return
	}
	event := notificationTemplateEventFromRequest(r)
	before, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	beforeDetail, _ := notificationsservice.GetEmailTemplate(before.Email.Templates, event)
	updatedTemplates, _, err := notificationsservice.UpdateEmailTemplate(before.Email.Templates, event, body.Subject, body.Html)
	if err != nil {
		writeNotificationTemplateError(w, err, requestID)
		return
	}
	before.Email.Templates = updatedTemplates
	updatedSettings, err := s.runtime.adminControl.UpdateAdminSettings(r.Context(), before, session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	detail, err := notificationsservice.GetEmailTemplate(updatedSettings.Email.Templates, event)
	if err != nil {
		writeNotificationTemplateError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "notification_email_template.update", "notification_email_template", event, notificationTemplateAuditSnapshot(beforeDetail), notificationTemplateAuditSnapshot(detail)))
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationEmailTemplateResponse{
		Data:      toAPINotificationEmailTemplate(detail),
		RequestId: requestID,
	})
}

func (s *Server) handleRestoreAdminNotificationEmailTemplate(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	event := notificationTemplateEventFromRequest(r)
	before, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	beforeDetail, _ := notificationsservice.GetEmailTemplate(before.Email.Templates, event)
	updatedTemplates, _, err := notificationsservice.RestoreEmailTemplate(before.Email.Templates, event)
	if err != nil {
		writeNotificationTemplateError(w, err, requestID)
		return
	}
	before.Email.Templates = updatedTemplates
	updatedSettings, err := s.runtime.adminControl.UpdateAdminSettings(r.Context(), before, session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	detail, err := notificationsservice.GetEmailTemplate(updatedSettings.Email.Templates, event)
	if err != nil {
		writeNotificationTemplateError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "notification_email_template.restore", "notification_email_template", event, notificationTemplateAuditSnapshot(beforeDetail), notificationTemplateAuditSnapshot(detail)))
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationEmailTemplateResponse{
		Data:      toAPINotificationEmailTemplate(detail),
		RequestId: requestID,
	})
}

func (s *Server) handlePreviewAdminNotificationEmailTemplate(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.PreviewNotificationEmailTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification email template preview request", requestID)
		return
	}
	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	preview, err := notificationsservice.PreviewEmailTemplate(settings.Email.Templates, notificationsservice.EmailTemplatePreviewInput{
		Event:     string(body.Event),
		Subject:   body.Subject,
		HTML:      body.Html,
		Variables: notificationTemplateVariablesFromAPI(body.Variables),
	})
	if err != nil {
		writeNotificationTemplateError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.NotificationEmailTemplatePreviewResponse{
		Data: apiopenapi.NotificationEmailTemplatePreview{
			Html:    preview.HTML,
			Subject: preview.Subject,
		},
		RequestId: requestID,
	})
}

func toAPINotificationEmailTemplateList(list notificationsservice.EmailTemplateList) apiopenapi.NotificationEmailTemplateList {
	events := make([]apiopenapi.NotificationEmailTemplateEvent, 0, len(list.Events))
	for _, event := range list.Events {
		events = append(events, apiopenapi.NotificationEmailTemplateEvent{
			Category:     event.Category,
			Description:  event.Description,
			Event:        apiopenapi.NotificationEmailTemplateEventName(event.Event),
			Label:        event.Label,
			Optional:     event.Optional,
			Placeholders: append([]string(nil), event.Placeholders...),
		})
	}
	templates := make([]apiopenapi.NotificationEmailTemplate, 0, len(list.Templates))
	for _, detail := range list.Templates {
		templates = append(templates, toAPINotificationEmailTemplate(detail))
	}
	return apiopenapi.NotificationEmailTemplateList{
		Events:       events,
		Placeholders: append([]string(nil), list.Placeholders...),
		Templates:    templates,
	}
}

func toAPINotificationEmailTemplate(detail notificationsservice.EmailTemplateDetail) apiopenapi.NotificationEmailTemplate {
	return apiopenapi.NotificationEmailTemplate{
		Event:        apiopenapi.NotificationEmailTemplateEventName(detail.Event),
		Html:         detail.HTML,
		IsCustom:     detail.IsCustom,
		Placeholders: append([]string(nil), detail.Placeholders...),
		Subject:      detail.Subject,
	}
}

func notificationTemplateVariablesFromAPI(values *map[string]string) map[string]string {
	out := map[string]string{}
	if values == nil {
		return out
	}
	for key, value := range *values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func notificationTemplateEventFromRequest(r *http.Request) string {
	return strings.TrimSpace(r.PathValue("event"))
}

func notificationTemplateAuditSnapshot(detail notificationsservice.EmailTemplateDetail) map[string]any {
	if detail.Event == "" {
		return nil
	}
	return map[string]any{
		"event":        detail.Event,
		"subject":      detail.Subject,
		"html":         detail.HTML,
		"is_custom":    detail.IsCustom,
		"placeholders": append([]string(nil), detail.Placeholders...),
	}
}

func writeNotificationTemplateError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, notificationsservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification email template request", requestID)
	case errors.Is(err, notificationsservice.ErrUnsupportedNotificationEvent):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "notification email template event not found", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "notification email template service error", requestID)
	}
}

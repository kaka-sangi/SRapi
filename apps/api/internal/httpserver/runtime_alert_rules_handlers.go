package httpserver

import (
	"net/http"
	"strconv"

	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// registerAlertRulesRoutes wires the operational SLO definitions, alert events,
// configurable generic-metric alert rules, and alert silences endpoints onto the
// admin mux.
func (s *Server) registerAlertRulesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/ops/alert-rules", s.handleListAdminOpsAlertRules)
	mux.HandleFunc("POST /api/v1/admin/ops/alert-rules", s.handleCreateAdminOpsAlertRule)
	mux.HandleFunc("PATCH /api/v1/admin/ops/alert-rules/{id}", s.handleUpdateAdminOpsAlertRule)
	mux.HandleFunc("DELETE /api/v1/admin/ops/alert-rules/{id}", s.handleDeleteAdminOpsAlertRule)
	mux.HandleFunc("GET /api/v1/admin/ops/alert-silences", s.handleListAdminOpsAlertSilences)
	mux.HandleFunc("POST /api/v1/admin/ops/alert-silences", s.handleCreateAdminOpsAlertSilence)
	mux.HandleFunc("DELETE /api/v1/admin/ops/alert-silences/{id}", s.handleDeleteAdminOpsAlertSilence)
	mux.HandleFunc("GET /api/v1/admin/ops/notification-channels", s.handleListAdminOpsNotificationChannels)
	mux.HandleFunc("POST /api/v1/admin/ops/notification-channels", s.handleCreateAdminOpsNotificationChannel)
	mux.HandleFunc("PATCH /api/v1/admin/ops/notification-channels/{id}", s.handleUpdateAdminOpsNotificationChannel)
	mux.HandleFunc("DELETE /api/v1/admin/ops/notification-channels/{id}", s.handleDeleteAdminOpsNotificationChannel)
	mux.HandleFunc("GET /api/v1/admin/ops/notification-deliveries", s.handleListAdminOpsNotificationDeliveries)
}

func (s *Server) handleListAdminOpsAlertRules(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	result, err := s.runtime.operations.ListAlertRulesWithPosture(r.Context())
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.OpsAlertRule, 0, len(result.Rules))
	for _, item := range result.Rules {
		data = append(data, toAPIOpsAlertRule(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsAlertRuleListResponse{
		BaselinePosture: toAPIOpsAlertRuleBaselinePosture(result.BaselinePosture),
		Data:            data,
		Pagination:      pg,
		RequestId:       requestID,
	})
}

func (s *Server) handleCreateAdminOpsAlertRule(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateOpsAlertRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid alert rule request", requestID)
		return
	}
	createReq, err := toCreateAlertRuleRequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert rule request", requestID)
		return
	}
	created, err := s.runtime.operations.CreateAlertRule(r.Context(), createReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_alert_rule.create", "ops_alert_rule", strconv.Itoa(created.ID), nil, opsAlertRuleAuditSnapshot(created)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.OpsAlertRuleResponse{
		Data:      toAPIOpsAlertRule(created),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminOpsAlertRule(w http.ResponseWriter, r *http.Request) {
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
	ruleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ruleID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert rule id", requestID)
		return
	}
	var beforeSnapshot map[string]any
	if current, findErr := s.runtime.operations.ListAlertRules(r.Context()); findErr == nil {
		for _, item := range current {
			if item.ID == ruleID {
				beforeSnapshot = opsAlertRuleAuditSnapshot(item)
				break
			}
		}
	}
	var body apiopenapi.UpdateOpsAlertRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid alert rule request", requestID)
		return
	}
	updateReq, err := toUpdateAlertRuleRequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert rule request", requestID)
		return
	}
	updated, err := s.runtime.operations.UpdateAlertRule(r.Context(), ruleID, updateReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_alert_rule.update", "ops_alert_rule", strconv.Itoa(updated.ID), beforeSnapshot, opsAlertRuleAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsAlertRuleResponse{
		Data:      toAPIOpsAlertRule(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminOpsAlertRule(w http.ResponseWriter, r *http.Request) {
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
	ruleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ruleID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert rule id", requestID)
		return
	}
	if err := s.runtime.operations.DeleteAlertRule(r.Context(), ruleID); err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_alert_rule.delete", "ops_alert_rule", strconv.Itoa(ruleID), nil, nil))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAdminOpsAlertSilences(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.operations.ListAlertSilences(r.Context())
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.OpsAlertSilence, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIOpsAlertSilence(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsAlertSilenceListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminOpsAlertSilence(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateOpsAlertSilenceRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid alert silence request", requestID)
		return
	}
	createReq, err := toCreateAlertSilenceRequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert silence request", requestID)
		return
	}
	actorID := session.User.ID
	createReq.CreatedBy = &actorID
	created, err := s.runtime.operations.CreateAlertSilence(r.Context(), createReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_alert_silence.create", "ops_alert_silence", strconv.Itoa(created.ID), nil, opsAlertSilenceAuditSnapshot(created)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.OpsAlertSilenceResponse{
		Data:      toAPIOpsAlertSilence(created),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminOpsAlertSilence(w http.ResponseWriter, r *http.Request) {
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
	silenceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || silenceID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert silence id", requestID)
		return
	}
	if err := s.runtime.operations.DeleteAlertSilence(r.Context(), silenceID); err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_alert_silence.delete", "ops_alert_silence", strconv.Itoa(silenceID), nil, nil))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAdminOpsNotificationChannels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.operations.ListNotificationChannels(r.Context())
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.OpsNotificationChannel, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIOpsNotificationChannel(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsNotificationChannelListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminOpsNotificationChannel(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateOpsNotificationChannelRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification channel request", requestID)
		return
	}
	createReq := toCreateNotificationChannelRequest(body)
	created, err := s.runtime.operations.CreateNotificationChannel(r.Context(), createReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_notification_channel.create", "ops_notification_channel", strconv.Itoa(created.ID), nil, opsNotificationChannelAuditSnapshot(created)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.OpsNotificationChannelResponse{
		Data:      toAPIOpsNotificationChannel(created),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminOpsNotificationChannel(w http.ResponseWriter, r *http.Request) {
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
	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || channelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification channel id", requestID)
		return
	}
	var beforeSnapshot map[string]any
	if current, findErr := s.runtime.operations.ListNotificationChannels(r.Context()); findErr == nil {
		for _, item := range current {
			if item.ID == channelID {
				beforeSnapshot = opsNotificationChannelAuditSnapshot(item)
				break
			}
		}
	}
	var body apiopenapi.UpdateOpsNotificationChannelRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid notification channel request", requestID)
		return
	}
	updated, err := s.runtime.operations.UpdateNotificationChannel(r.Context(), channelID, toUpdateNotificationChannelRequest(body))
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_notification_channel.update", "ops_notification_channel", strconv.Itoa(updated.ID), beforeSnapshot, opsNotificationChannelAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsNotificationChannelResponse{
		Data:      toAPIOpsNotificationChannel(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminOpsNotificationChannel(w http.ResponseWriter, r *http.Request) {
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
	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || channelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification channel id", requestID)
		return
	}
	if err := s.runtime.operations.DeleteNotificationChannel(r.Context(), channelID); err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_notification_channel.delete", "ops_notification_channel", strconv.Itoa(channelID), nil, nil))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAdminOpsNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	opts, err := notificationDeliveryListOptionsFromRequest(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid notification delivery filters", requestID)
		return
	}
	items, err := s.runtime.operations.ListNotificationDeliveries(r.Context(), opts)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.OpsNotificationDelivery, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIOpsNotificationDelivery(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsNotificationDeliveryListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func opsAlertRuleAuditSnapshot(rule operationscontract.AlertRule) map[string]any {
	out := map[string]any{
		"name":              rule.Name,
		"metric_type":       rule.MetricType,
		"operator":          rule.Operator,
		"threshold":         rule.Threshold,
		"severity":          rule.Severity,
		"enabled":           rule.Enabled,
		"window_seconds":    rule.WindowSeconds,
		"cooldown_seconds":  rule.CooldownSeconds,
		"min_request_count": rule.MinRequestCount,
		"source_endpoint":   rule.Scope.SourceEndpoint,
		"model":             rule.Scope.Model,
	}
	if rule.Scope.ProviderID != nil {
		out["provider_id"] = *rule.Scope.ProviderID
	}
	return out
}

func opsNotificationChannelAuditSnapshot(channel operationscontract.NotificationChannel) map[string]any {
	return map[string]any{
		"name":             channel.Name,
		"type":             channel.Type,
		"status":           channel.Status,
		"min_severity":     channel.MinSeverity,
		"email_recipients": channel.EmailRecipients,
		"send_resolved":    channel.SendResolved,
	}
}

func opsAlertSilenceAuditSnapshot(silence operationscontract.AlertSilence) map[string]any {
	out := map[string]any{
		"comment":   silence.Comment,
		"starts_at": silence.StartsAt,
		"ends_at":   silence.EndsAt,
		"rule_id":   silence.Matcher.RuleID,
		"severity":  silence.Matcher.Severity,
	}
	if silence.Matcher.SourceEndpoint != "" {
		out["source_endpoint"] = silence.Matcher.SourceEndpoint
	}
	if silence.Matcher.Model != "" {
		out["model"] = silence.Matcher.Model
	}
	if silence.Matcher.ProviderID != nil {
		out["provider_id"] = *silence.Matcher.ProviderID
	}
	return out
}

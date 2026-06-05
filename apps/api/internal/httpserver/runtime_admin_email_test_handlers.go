package httpserver

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleSendAdminTestEmail delivers a probe email through the effective SMTP
// configuration so an admin can confirm the (write-only) SMTP password actually
// works. The effective config is assembled the same way the notification worker
// does (admin-settings SMTP fields layered over the static cfg.Email fallback),
// so a passing test exercises the real send path.
func (s *Server) handleSendAdminTestEmail(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.AdminSendTestEmailRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid test email request", requestID)
		return
	}
	recipient := strings.TrimSpace(session.User.Email)
	if body.Recipient != nil && strings.TrimSpace(string(*body.Recipient)) != "" {
		recipient = strings.TrimSpace(string(*body.Recipient))
	}

	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	emailConfig := effectiveTestEmailConfig(settings.Email, s.cfg.Email)

	startedAt := time.Now()
	result := sendTestEmailResult(r.Context(), emailConfig, recipient, startedAt)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "admin_settings.send_test_email", "admin_settings", "system", nil, map[string]any{
		"ok":        result.Ok,
		"status":    result.Status,
		"recipient": recipient,
		"checks":    result.Checks,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminTestResultResponse{
		Data:      result,
		RequestId: requestID,
	})
}

// effectiveTestEmailConfig layers the admin-settings SMTP fields over the static
// cfg.Email fallback, mirroring the outbox notificationEmailConfig precedence so
// the test path and the real send path resolve identical credentials. The SMTP
// password is sourced from the static config because it is write-only at the
// settings layer.
func effectiveTestEmailConfig(settings admincontrol.AdminSettingsEmail, fallback config.EmailConfig) notificationscontract.EmailConfig {
	cfg := notificationscontract.EmailConfig{
		PublicBaseURL: firstNonEmpty(settings.PublicBaseURL, fallback.PublicBaseURL),
		SMTPHost:      firstNonEmpty(settings.SMTPHost, fallback.SMTPHost),
		SMTPUsername:  firstNonEmpty(settings.SMTPUsername, fallback.SMTPUsername),
		SMTPPassword:  fallback.SMTPPassword,
		SMTPFrom:      firstNonEmpty(settings.SMTPFrom, fallback.SMTPFrom),
		SMTPFromName:  firstNonEmpty(settings.SMTPFromName, fallback.SMTPFromName),
		SMTPUseTLS:    settings.SMTPUseTLS || fallback.SMTPUseTLS,
	}
	cfg.SMTPPort = settings.SMTPPort
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = fallback.SMTPPort
	}
	return cfg
}

// sendTestEmailResult validates the effective config, attempts a single send and
// reports per-step checks (config-present, recipient, send) in the shared
// AdminTestResult shape used by the other admin "test" surfaces.
func sendTestEmailResult(ctx context.Context, cfg notificationscontract.EmailConfig, recipient string, startedAt time.Time) apiopenapi.AdminTestResult {
	checks := map[string]any{
		"smtp_host_present":     strings.TrimSpace(cfg.SMTPHost) != "",
		"smtp_from_present":     strings.TrimSpace(cfg.SMTPFrom) != "",
		"smtp_password_present": strings.TrimSpace(cfg.SMTPPassword) != "",
		"smtp_use_tls":          cfg.SMTPUseTLS,
		"recipient_present":     strings.TrimSpace(recipient) != "",
		"sent":                  false,
	}
	if strings.TrimSpace(cfg.SMTPHost) == "" || strings.TrimSpace(cfg.SMTPFrom) == "" {
		checks["error"] = "smtp_not_configured"
		return testEmailResult(false, "SMTP host and from address are required before a test can be sent", startedAt, checks)
	}
	if strings.TrimSpace(recipient) == "" {
		checks["error"] = "recipient_missing"
		return testEmailResult(false, "no recipient available for the test email", startedAt, checks)
	}
	sender := notificationsservice.NewSMTPSender(cfg)
	message := notificationscontract.EmailMessage{
		To:      recipient,
		Subject: "SRapi SMTP test email",
		HTML:    "<p>This is a test email from SRapi confirming your SMTP credentials work.</p>",
	}
	if err := sender.Send(ctx, message); err != nil {
		checks["error"] = "send_failed"
		checks["error_detail"] = err.Error()
		return testEmailResult(false, "test email delivery failed: "+err.Error(), startedAt, checks)
	}
	checks["sent"] = true
	return testEmailResult(true, "test email delivered to "+recipient, startedAt, checks)
}

func testEmailResult(ok bool, message string, startedAt time.Time, checks map[string]any) apiopenapi.AdminTestResult {
	status := "failed"
	if ok {
		status = "ok"
	}
	return apiopenapi.AdminTestResult{
		CheckedAt: time.Now().UTC(),
		Checks:    mapToJsonObjectPtr(checks),
		LatencyMs: ptrInt(elapsedMillis(startedAt)),
		Message:   ptrString(message),
		Ok:        ok,
		Status:    apiopenapi.AdminTestResultStatus(status),
	}
}

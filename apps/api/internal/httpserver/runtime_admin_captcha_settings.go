package httpserver

import (
	"errors"
	"net/http"
	"strings"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const captchaSecretVersion = "captchav1"

type captchaSettingsResponse struct {
	Data      captchaSettingsPayload `json:"data"`
	RequestID string                 `json:"request_id"`
}

type captchaSettingsPayload struct {
	Managed             bool   `json:"managed"`
	Enabled             bool   `json:"enabled"`
	Provider            string `json:"provider"`
	SiteKey             string `json:"site_key"`
	SecretKey           string `json:"secret_key,omitempty"`
	SecretKeyConfigured bool   `json:"secret_key_configured"`
	VerifyURL           string `json:"verify_url"`
}

func (s *Server) handleGetAdminCaptchaSettings(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	settings, err := s.runtime.adminControl.GetCaptchaSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, captchaSettingsResponse{
		Data:      captchaSettingsToPayload(settings),
		RequestID: requestID,
	})
}

func (s *Server) handleUpdateAdminCaptchaSettings(w http.ResponseWriter, r *http.Request) {
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
	before, err := s.runtime.adminControl.GetCaptchaSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	var body captchaSettingsPayload
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid captcha settings request", requestID)
		return
	}
	secretCiphertext := before.SecretKeyCiphertext
	if strings.TrimSpace(body.SecretKey) != "" {
		secretCiphertext, err = s.encryptMasterSecret(strings.TrimSpace(body.SecretKey), captchaSecretVersion)
		if err != nil {
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to secure captcha secret", requestID)
			return
		}
	}
	updated, err := s.runtime.adminControl.UpdateCaptchaSettings(r.Context(), admincontrol.CaptchaSettings{
		Managed:             body.Managed,
		Enabled:             body.Enabled,
		Provider:            body.Provider,
		SiteKey:             body.SiteKey,
		SecretKeyCiphertext: secretCiphertext,
		VerifyURL:           body.VerifyURL,
	}, session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "captcha_settings.update", "captcha_settings", "system", captchaSettingsAuditSnapshot(before), captchaSettingsAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, captchaSettingsResponse{
		Data:      captchaSettingsToPayload(updated),
		RequestID: requestID,
	})
}

func (s *Server) currentCaptchaConfig(r *http.Request) (captchaSettingsPayload, error) {
	cfg := captchaSettingsPayload{
		Managed:             false,
		Enabled:             s.cfg.Captcha.Enabled,
		Provider:            s.cfg.Captcha.Provider,
		SiteKey:             s.cfg.Captcha.SiteKey,
		SecretKey:           s.cfg.Captcha.SecretKey,
		SecretKeyConfigured: strings.TrimSpace(s.cfg.Captcha.SecretKey) != "",
		VerifyURL:           s.cfg.Captcha.VerifyURL,
	}
	settings, err := s.runtime.adminControl.GetCaptchaSettings(r.Context())
	if err != nil {
		return cfg, err
	}
	if !settings.Managed {
		return cfg, nil
	}
	cfg.Managed = true
	cfg.Enabled = settings.Enabled
	cfg.Provider = settings.Provider
	cfg.SiteKey = settings.SiteKey
	cfg.VerifyURL = settings.VerifyURL
	cfg.SecretKeyConfigured = strings.TrimSpace(settings.SecretKeyCiphertext) != ""
	cfg.SecretKey = ""
	if settings.SecretKeyCiphertext != "" {
		secret, err := s.decryptMasterSecret(settings.SecretKeyCiphertext, captchaSecretVersion)
		if err != nil {
			return captchaSettingsPayload{}, errors.New("captcha secret unavailable")
		}
		cfg.SecretKey = secret
	}
	return cfg, nil
}

func captchaSettingsToPayload(settings admincontrol.CaptchaSettings) captchaSettingsPayload {
	return captchaSettingsPayload{
		Managed:             settings.Managed,
		Enabled:             settings.Enabled,
		Provider:            settings.Provider,
		SiteKey:             settings.SiteKey,
		SecretKeyConfigured: strings.TrimSpace(settings.SecretKeyCiphertext) != "",
		VerifyURL:           settings.VerifyURL,
	}
}

func captchaSettingsAuditSnapshot(settings admincontrol.CaptchaSettings) map[string]any {
	return map[string]any{
		"managed":                settings.Managed,
		"enabled":                settings.Enabled,
		"provider":               settings.Provider,
		"site_key_configured":    strings.TrimSpace(settings.SiteKey) != "",
		"secret_key_configured":  strings.TrimSpace(settings.SecretKeyCiphertext) != "",
		"custom_verify_url_used": strings.TrimSpace(settings.VerifyURL) != "",
	}
}

package service

import (
	"context"
	"strings"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

const settingsKeyCaptcha = "admin_control.captcha_settings"

func (s *Service) GetCaptchaSettings(ctx context.Context) (admincontrol.CaptchaSettings, error) {
	settings := defaultCaptchaSettings()
	if err := s.loadTyped(ctx, settingsKeyCaptcha, &settings); err != nil {
		return admincontrol.CaptchaSettings{}, err
	}
	return normalizeCaptchaSettings(settings)
}

func (s *Service) UpdateCaptchaSettings(ctx context.Context, settings admincontrol.CaptchaSettings, actorUserID int) (admincontrol.CaptchaSettings, error) {
	normalized, err := normalizeCaptchaSettings(settings)
	if err != nil {
		return admincontrol.CaptchaSettings{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyCaptcha, normalized, actorUserID); err != nil {
		return admincontrol.CaptchaSettings{}, err
	}
	return normalized, nil
}

func defaultCaptchaSettings() admincontrol.CaptchaSettings {
	return admincontrol.CaptchaSettings{
		Managed:  false,
		Enabled:  false,
		Provider: "turnstile",
	}
}

func normalizeCaptchaSettings(settings admincontrol.CaptchaSettings) (admincontrol.CaptchaSettings, error) {
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	settings.SiteKey = strings.TrimSpace(settings.SiteKey)
	settings.SecretKeyCiphertext = strings.TrimSpace(settings.SecretKeyCiphertext)
	settings.VerifyURL = strings.TrimSpace(settings.VerifyURL)
	if settings.Provider == "" {
		settings.Provider = "turnstile"
	}
	switch settings.Provider {
	case "turnstile", "hcaptcha", "recaptcha":
	default:
		return admincontrol.CaptchaSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.VerifyURL != "" && !validPublicHTTPBaseURL(settings.VerifyURL) {
		return admincontrol.CaptchaSettings{}, admincontrol.ErrInvalidInput
	}
	return settings, nil
}

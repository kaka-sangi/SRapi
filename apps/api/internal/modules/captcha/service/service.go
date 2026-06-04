package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/captcha/contract"
)

// Config configures the captcha verification service.
type Config struct {
	Enabled   bool
	Provider  string
	SecretKey string
	SiteKey   string
	VerifyURL string
}

// Service verifies captcha tokens on auth endpoints. When disabled, Verify is a
// no-op so callers can wire it unconditionally.
type Service struct {
	enabled   bool
	provider  string
	siteKey   string
	secret    string
	verifyURL string
	verifier  contract.Verifier
}

const (
	turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	hcaptchaVerifyURL  = "https://api.hcaptcha.com/siteverify"
	recaptchaVerifyURL = "https://www.google.com/recaptcha/api/siteverify"
)

// New builds a captcha service. A nil verifier defaults to an HTTP siteverify
// verifier targeting the configured provider endpoint.
func New(cfg Config, verifier contract.Verifier) *Service {
	verifyURL := strings.TrimSpace(cfg.VerifyURL)
	if verifyURL == "" {
		verifyURL = verifyURLForProvider(cfg.Provider)
	}
	if verifier == nil {
		verifier = &httpVerifier{
			endpoint: verifyURL,
			client:   &http.Client{Timeout: 10 * time.Second},
		}
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "turnstile"
	}
	return &Service{
		enabled:   cfg.Enabled,
		provider:  provider,
		siteKey:   strings.TrimSpace(cfg.SiteKey),
		secret:    strings.TrimSpace(cfg.SecretKey),
		verifyURL: verifyURL,
		verifier:  verifier,
	}
}

// Enabled reports whether verification is active.
func (s *Service) Enabled() bool { return s.enabled }

// Provider reports the configured captcha provider (turnstile | hcaptcha | recaptcha).
func (s *Service) Provider() string { return s.provider }

// SiteKey reports the public site key the frontend widget renders. Never secret.
func (s *Service) SiteKey() string { return s.siteKey }

// Verify checks the supplied token. It returns nil when verification is disabled,
// contract.ErrCaptchaRequired when the token is missing, and
// contract.ErrCaptchaFailed when the provider rejects it.
func (s *Service) Verify(ctx context.Context, token, remoteIP string) error {
	if !s.enabled {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return contract.ErrCaptchaRequired
	}
	ok, err := s.verifier.Verify(ctx, s.secret, token, remoteIP)
	if err != nil {
		return err
	}
	if !ok {
		return contract.ErrCaptchaFailed
	}
	return nil
}

func verifyURLForProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "hcaptcha":
		return hcaptchaVerifyURL
	case "recaptcha":
		return recaptchaVerifyURL
	default:
		return turnstileVerifyURL
	}
}

// httpVerifier posts a token to a provider siteverify endpoint. Turnstile,
// hCaptcha and reCAPTCHA all share the same form-encoded request and a JSON
// response with a "success" boolean.
type httpVerifier struct {
	endpoint string
	client   *http.Client
}

type siteVerifyResponse struct {
	Success bool `json:"success"`
}

func (v *httpVerifier) Verify(ctx context.Context, secret, token, remoteIP string) (bool, error) {
	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var decoded siteVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return false, err
	}
	return decoded.Success, nil
}

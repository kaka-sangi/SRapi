package service

import (
	"context"
	"errors"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
)

// ErrInvalidInput is returned for malformed profiles.
var ErrInvalidInput = errors.New("invalid tls fingerprint profile")

// supportedTLSTemplates mirrors clientHelloIDForTLSTemplate in the reverse_proxy
// egress resolver. Kept in sync so profiles validate before they reach egress.
var supportedTLSTemplates = map[string]struct{}{
	"": {}, "none": {}, "default": {},
	"chrome": {}, "chrome_auto": {}, "chrome_120": {}, "chrome_133": {},
	"firefox": {}, "firefox_auto": {}, "firefox_120": {},
	"safari": {}, "safari_auto": {}, "safari_16": {}, "safari_16_0": {},
	"ios": {}, "ios_auto": {}, "ios_14": {},
	"android_11_okhttp": {}, "android_okhttp_11": {},
	"randomized": {}, "randomized_alpn": {}, "randomized_no_alpn": {},
}

// supportedHTTPVersionPolicies mirrors validateHTTPVersionPolicy (excluding the
// require_h2 family the egress layer rejects).
var supportedHTTPVersionPolicies = map[string]struct{}{
	"": {}, "auto": {},
	"prefer_h2": {}, "prefer_http2": {}, "prefer_h1": {}, "prefer_http1": {},
	"require_h1": {}, "require_http1": {},
}

type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
}

func (s *Service) ListProfiles(ctx context.Context) ([]contract.Profile, error) {
	return s.store.ListProfiles(ctx)
}

func (s *Service) CreateProfile(ctx context.Context, input contract.CreateProfile) (contract.Profile, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return contract.Profile{}, ErrInvalidInput
	}
	input.TLSTemplate = normalizeToken(input.TLSTemplate)
	if !validTemplate(input.TLSTemplate) {
		return contract.Profile{}, ErrInvalidInput
	}
	input.HTTPVersionPolicy = normalizeToken(input.HTTPVersionPolicy)
	if !validHTTPVersionPolicy(input.HTTPVersionPolicy) {
		return contract.Profile{}, ErrInvalidInput
	}
	if input.HTTPVersionPolicy == "" {
		input.HTTPVersionPolicy = "prefer_h2"
	}
	input.UserAgent = strings.TrimSpace(input.UserAgent)
	input.ExtraHeaders = cleanHeaders(input.ExtraHeaders)
	return s.store.CreateProfile(ctx, input)
}

func (s *Service) UpdateProfile(ctx context.Context, id int, input contract.UpdateProfile) (contract.Profile, error) {
	if id <= 0 {
		return contract.Profile{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Profile{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.TLSTemplate != nil {
		template := normalizeToken(*input.TLSTemplate)
		if !validTemplate(template) {
			return contract.Profile{}, ErrInvalidInput
		}
		input.TLSTemplate = &template
	}
	if input.HTTPVersionPolicy != nil {
		policy := normalizeToken(*input.HTTPVersionPolicy)
		if !validHTTPVersionPolicy(policy) {
			return contract.Profile{}, ErrInvalidInput
		}
		if policy == "" {
			policy = "prefer_h2"
		}
		input.HTTPVersionPolicy = &policy
	}
	if input.UserAgent != nil {
		ua := strings.TrimSpace(*input.UserAgent)
		input.UserAgent = &ua
	}
	if input.ExtraHeaders != nil {
		headers := cleanHeaders(*input.ExtraHeaders)
		input.ExtraHeaders = &headers
	}
	return s.store.UpdateProfile(ctx, id, input)
}

func (s *Service) DeleteProfile(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteProfile(ctx, id)
}

// Snapshot returns enabled profiles keyed by lowercase name, for the egress
// resolver to expand a named reference into egress_profile metadata.
func (s *Service) Snapshot(ctx context.Context) map[string]contract.Profile {
	profiles, err := s.store.ListProfiles(ctx)
	if err != nil {
		return nil
	}
	out := make(map[string]contract.Profile, len(profiles))
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		out[strings.ToLower(profile.Name)] = profile
	}
	return out
}

func validTemplate(template string) bool {
	_, ok := supportedTLSTemplates[template]
	return ok
}

func validHTTPVersionPolicy(policy string) bool {
	_, ok := supportedHTTPVersionPolicies[policy]
	return ok
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cleanHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	admincontrolmemory "github.com/srapi/srapi/apps/api/internal/modules/admin_control/store/memory"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usermemory "github.com/srapi/srapi/apps/api/internal/modules/users/store/memory"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func TestUpdateAdminSettingsNormalizesRegistrationEmailSuffixAllowlist(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Security.RegistrationEmailSuffixAllowlist = []string{
		"Example.COM",
		" @company.test ",
		"@example.com",
		"",
	}

	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	want := []string{"@example.com", "@company.test"}
	if len(updated.Security.RegistrationEmailSuffixAllowlist) != len(want) {
		t.Fatalf("suffix allowlist len = %d, want %d: %+v", len(updated.Security.RegistrationEmailSuffixAllowlist), len(want), updated.Security.RegistrationEmailSuffixAllowlist)
	}
	for idx, value := range want {
		if updated.Security.RegistrationEmailSuffixAllowlist[idx] != value {
			t.Fatalf("suffix allowlist[%d] = %q, want %q: %+v", idx, updated.Security.RegistrationEmailSuffixAllowlist[idx], value, updated.Security.RegistrationEmailSuffixAllowlist)
		}
	}
}

func TestUpdateAdminSettingsNormalizesPassthroughHeaderAllowlist(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.June, 5, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	if settings.Gateway.PassthroughUpstreamHeaders {
		t.Fatalf("passthrough should default off")
	}
	settings.Gateway.PassthroughUpstreamHeaders = true
	settings.Gateway.PassthroughHeaderAllowlist = []string{
		"Retry-After",
		" X-Request-Id ",
		"retry-after", // case-insensitive duplicate
		"X-RateLimit-*",
		"",
		"*",
	}

	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	want := []string{"retry-after", "x-request-id", "x-ratelimit-*"}
	got := updated.Gateway.PassthroughHeaderAllowlist
	if len(got) != len(want) {
		t.Fatalf("allowlist = %v, want %v", got, want)
	}
	for idx, value := range want {
		if got[idx] != value {
			t.Fatalf("allowlist[%d] = %q, want %q (%v)", idx, got[idx], value, got)
		}
	}
}

func TestDefaultAdminSettingsIncludesSafePassthroughHeaderAllowlist(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.June, 12, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	if settings.Gateway.PassthroughUpstreamHeaders {
		t.Fatalf("passthrough should default off")
	}
	for _, want := range []string{
		"retry-after",
		"cache-control",
		"content-language",
		"date",
		"etag",
		"expires",
		"last-modified",
		"location",
		"vary",
		"www-authenticate",
		"x-ratelimit-*",
		"anthropic-ratelimit-*",
		"x-codex-primary-used-percent",
		"x-codex-secondary-used-percent",
		"x-codex-primary-over-secondary-limit-percent",
	} {
		if !stringSliceContains(settings.Gateway.PassthroughHeaderAllowlist, want) {
			t.Fatalf("default passthrough allowlist missing %q: %+v", want, settings.Gateway.PassthroughHeaderAllowlist)
		}
	}
}

func TestGetAdminSettingsCacheReturnsIndependentCopies(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.June, 11, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Features.EnabledChannels = append(settings.Features.EnabledChannels, "mutated")
	settings.Gateway.PassthroughHeaderAllowlist = append(settings.Gateway.PassthroughHeaderAllowlist, "x-mutated")
	settings.Email.Templates["welcome"] = "mutated"
	*settings.Email.BalanceLowNotifyEnabled = false

	reloaded, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("reload admin settings: %v", err)
	}
	if len(reloaded.Features.EnabledChannels) != 0 || stringSliceContains(reloaded.Gateway.PassthroughHeaderAllowlist, "x-mutated") {
		t.Fatalf("cached slices were mutated: %+v", reloaded)
	}
	if _, ok := reloaded.Email.Templates["welcome"]; ok {
		t.Fatalf("cached template map was mutated: %+v", reloaded.Email.Templates)
	}
	if reloaded.Email.BalanceLowNotifyEnabled == nil || !*reloaded.Email.BalanceLowNotifyEnabled {
		t.Fatalf("cached bool pointer was mutated: %+v", reloaded.Email.BalanceLowNotifyEnabled)
	}
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestAdminSettingsEmailNotificationSwitchesDefaultVisible(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.June, 10, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	if settings.Email.BalanceLowNotifyEnabled == nil || !*settings.Email.BalanceLowNotifyEnabled {
		t.Fatalf("expected visible enabled balance notification switch, got %+v", settings.Email.BalanceLowNotifyEnabled)
	}
	if settings.Email.SubscriptionExpiryNotifyEnabled == nil || !*settings.Email.SubscriptionExpiryNotifyEnabled {
		t.Fatalf("expected visible enabled subscription notification switch, got %+v", settings.Email.SubscriptionExpiryNotifyEnabled)
	}
	if settings.Email.AccountQuotaNotifyEnabled == nil || !*settings.Email.AccountQuotaNotifyEnabled {
		t.Fatalf("expected visible enabled account quota notification switch, got %+v", settings.Email.AccountQuotaNotifyEnabled)
	}

	settings.Email.BalanceLowNotifyEnabled = nil
	settings.Email.SubscriptionExpiryNotifyEnabled = nil
	settings.Email.AccountQuotaNotifyEnabled = nil
	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	if updated.Email.BalanceLowNotifyEnabled == nil || !*updated.Email.BalanceLowNotifyEnabled {
		t.Fatalf("expected normalized balance notification switch, got %+v", updated.Email.BalanceLowNotifyEnabled)
	}
	if updated.Email.SubscriptionExpiryNotifyEnabled == nil || !*updated.Email.SubscriptionExpiryNotifyEnabled {
		t.Fatalf("expected normalized subscription notification switch, got %+v", updated.Email.SubscriptionExpiryNotifyEnabled)
	}
	if updated.Email.AccountQuotaNotifyEnabled == nil || !*updated.Email.AccountQuotaNotifyEnabled {
		t.Fatalf("expected normalized account quota notification switch, got %+v", updated.Email.AccountQuotaNotifyEnabled)
	}
}

func TestUpdateAdminSettingsRejectsInvalidRegistrationEmailSuffixAllowlist(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Security.RegistrationEmailSuffixAllowlist = []string{"@invalid_domain"}

	_, err = svc.UpdateAdminSettings(context.Background(), settings, 1)
	if !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestContentSafetyConfigNormalizesListsAndRejectsInvalidMode(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.June, 9, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	updated, err := svc.UpdateContentSafetyConfig(context.Background(), admincontrol.ContentSafetyConfig{
		Enabled:              true,
		Mode:                 admincontrol.ContentSafetyModeEnforce,
		RedactPII:            false,
		BlockPII:             true,
		BlockPromptInjection: true,
		BlockCustomKeywords:  true,
		CustomKeywords:       []string{" Secret ", "secret", ""},
		ModelScopes:          []string{" GPT-4O* ", "gpt-4o*"},
	}, 1)
	if err != nil {
		t.Fatalf("update content safety config: %v", err)
	}
	if len(updated.CustomKeywords) != 1 || updated.CustomKeywords[0] != "secret" {
		t.Fatalf("unexpected custom keywords: %+v", updated.CustomKeywords)
	}
	if len(updated.ModelScopes) != 1 || updated.ModelScopes[0] != "gpt-4o*" {
		t.Fatalf("unexpected model scopes: %+v", updated.ModelScopes)
	}

	loaded, err := svc.GetContentSafetyConfig(context.Background())
	if err != nil {
		t.Fatalf("get content safety config: %v", err)
	}
	if loaded.Mode != admincontrol.ContentSafetyModeEnforce || loaded.RedactPII {
		t.Fatalf("unexpected loaded config: %+v", loaded)
	}

	_, err = svc.UpdateContentSafetyConfig(context.Background(), admincontrol.ContentSafetyConfig{
		Enabled: true,
		Mode:    admincontrol.ContentSafetyMode("block"),
	}, 1)
	if !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}

func TestPromoCodeSupportsPerUserLimitAndMinimumOrderAmount(t *testing.T) {
	now := time.Date(2026, time.June, 9, 9, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	promo, err := svc.CreatePromoCode(context.Background(), admincontrol.PromoCodeRequest{
		Code:           "SAVE10",
		DiscountType:   admincontrol.PromoDiscountTypeAmount,
		DiscountValue:  "10.00",
		Currency:       "usd",
		MaxUses:        5,
		PerUserLimit:   1,
		MinOrderAmount: "50.00",
	}, 1)
	if err != nil {
		t.Fatalf("create promo code: %v", err)
	}
	if promo.PerUserLimit != 1 || promo.MinOrderAmount != "50.00" || promo.Currency != "USD" {
		t.Fatalf("unexpected promo code fields: %+v", promo)
	}

	if _, err := svc.CreatePromoCode(context.Background(), admincontrol.PromoCodeRequest{
		Code:           "BADLIMIT",
		DiscountType:   admincontrol.PromoDiscountTypeAmount,
		DiscountValue:  "1.00",
		MaxUses:        1,
		PerUserLimit:   2,
		MinOrderAmount: "1.00",
	}, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid per-user limit, got %v", err)
	}
	if _, err := svc.CreatePromoCode(context.Background(), admincontrol.PromoCodeRequest{
		Code:           "BADMIN",
		DiscountType:   admincontrol.PromoDiscountTypeAmount,
		DiscountValue:  "1.00",
		MaxUses:        1,
		MinOrderAmount: "-1.00",
	}, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid min order amount, got %v", err)
	}
}

func TestUpdateAdminSettingsNormalizesOAuthProviderConfigs(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 30, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Security.OAuthEnabled = true
	settings.Security.OAuthProviders = []string{" OIDC ", "oidc", "github"}
	settings.Security.OAuthProviderConfigs = []admincontrol.OAuthProviderConfig{
		{
			Provider:        " OIDC ",
			ProviderKey:     " issuer-main ",
			ClientID:        " client-123 ",
			AuthorizeURL:    "https://idp.example/authorize",
			TokenURL:        "http://localhost:8081/token",
			UserInfoURL:     "http://localhost:8081/userinfo",
			TokenAuthMethod: " none ",
			RedirectURI:     "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
			Scopes:          []string{"openid email", "profile", "email"},
		},
	}

	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	if got := updated.Security.OAuthProviders; len(got) != 2 || got[0] != "OIDC" || got[1] != "github" {
		t.Fatalf("unexpected oauth providers: %+v", got)
	}
	if len(updated.Security.OAuthProviderConfigs) != 1 {
		t.Fatalf("expected one oauth provider config, got %+v", updated.Security.OAuthProviderConfigs)
	}
	config := updated.Security.OAuthProviderConfigs[0]
	if config.Provider != "oidc" || config.ProviderKey != "issuer-main" || config.DisplayName != "issuer-main" || config.ClientID != "client-123" {
		t.Fatalf("unexpected normalized config: %+v", config)
	}
	if config.TokenURL != "http://localhost:8081/token" || config.UserInfoURL != "http://localhost:8081/userinfo" || config.TokenAuthMethod != "none" {
		t.Fatalf("unexpected oauth callback config: %+v", config)
	}
	wantScopes := []string{"openid", "email", "profile"}
	for idx, want := range wantScopes {
		if config.Scopes[idx] != want {
			t.Fatalf("scope[%d] = %q, want %q in %+v", idx, config.Scopes[idx], want, config.Scopes)
		}
	}
}

func TestUpdateAdminSettingsRejectsInvalidOAuthProviderConfig(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 30, 12, 30, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Security.OAuthProviderConfigs = []admincontrol.OAuthProviderConfig{
		{
			Provider:     "oidc",
			ProviderKey:  "issuer-main",
			ClientID:     "client-123",
			AuthorizeURL: "http://idp.example/authorize",
			RedirectURI:  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
		},
	}

	_, err = svc.UpdateAdminSettings(context.Background(), settings, 1)
	if !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestUpdateAdminSettingsRejectsPartialOAuthCallbackConfig(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 30, 12, 45, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Security.OAuthProviderConfigs = []admincontrol.OAuthProviderConfig{
		{
			Provider:     "oidc",
			ProviderKey:  "issuer-main",
			ClientID:     "client-123",
			AuthorizeURL: "https://idp.example/authorize",
			TokenURL:     "https://idp.example/token",
			RedirectURI:  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
		},
	}

	_, err = svc.UpdateAdminSettings(context.Background(), settings, 1)
	if !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestUpdateAdminSettingsNormalizesEmailConfigWithoutSMTPSecret(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Email.SMTPHost = " smtp.example.com "
	settings.Email.SMTPPort = 2525
	settings.Email.SMTPUsername = " sender "
	settings.Email.SMTPFrom = " noreply@example.com "
	settings.Email.SMTPFromName = " SRapi "
	settings.Email.SMTPUseTLS = true
	settings.Email.PublicBaseURL = " https://console.example.com/ "

	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	if !updated.Email.SMTPConfigured {
		t.Fatalf("expected configured SMTP flag, got %+v", updated.Email)
	}
	if updated.Email.SMTPHost != "smtp.example.com" || updated.Email.SMTPUsername != "sender" || updated.Email.PublicBaseURL != "https://console.example.com" {
		t.Fatalf("email config was not normalized: %+v", updated.Email)
	}
}

func TestUpdateAdminSettingsRejectsInvalidEmailPublicBaseURL(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Email.PublicBaseURL = "javascript:alert(1)"

	_, err = svc.UpdateAdminSettings(context.Background(), settings, 1)
	if !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestUpdateAdminSettingsValidatesAccountQuotaNotifyRatio(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	if settings.Email.AccountQuotaNotifyRemainingRatio != "0.20000000" {
		t.Fatalf("default account quota ratio = %q", settings.Email.AccountQuotaNotifyRemainingRatio)
	}
	settings.Email.AccountQuotaNotifyRemainingRatio = " 0.15000000 "

	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	if updated.Email.AccountQuotaNotifyRemainingRatio != "0.15000000" {
		t.Fatalf("account quota ratio was not normalized: %+v", updated.Email)
	}

	settings.Email.AccountQuotaNotifyRemainingRatio = "1.5"
	_, err = svc.UpdateAdminSettings(context.Background(), settings, 1)
	if !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("expected invalid ratio input, got %v", err)
	}
}

func TestUpdateAdminSettingsRoundTripsGatewayRetryKnobs(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	if settings.Gateway.RetryCount != 3 || settings.Gateway.MaxRetryCredentials != 0 || settings.Gateway.MaxRetryIntervalMS != 2000 {
		t.Fatalf("unexpected default gateway retry knobs: %+v", settings.Gateway)
	}

	settings.Gateway.RetryCount = 5
	settings.Gateway.MaxRetryCredentials = 2
	settings.Gateway.MaxRetryIntervalMS = 750

	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	if updated.Gateway.RetryCount != 5 || updated.Gateway.MaxRetryCredentials != 2 || updated.Gateway.MaxRetryIntervalMS != 750 {
		t.Fatalf("gateway retry knobs not preserved on update: %+v", updated.Gateway)
	}

	reloaded, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("reload admin settings: %v", err)
	}
	if reloaded.Gateway.RetryCount != 5 || reloaded.Gateway.MaxRetryCredentials != 2 || reloaded.Gateway.MaxRetryIntervalMS != 750 {
		t.Fatalf("gateway retry knobs not persisted: %+v", reloaded.Gateway)
	}
}

func TestUpdateAdminSettingsNormalizesGatewayRetryKnobs(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	settings, err := svc.GetAdminSettings(context.Background())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	// Zero retry_count falls back to the default; out-of-range clamps; negative
	// credentials and interval reset to sane values.
	settings.Gateway.RetryCount = 0
	settings.Gateway.MaxRetryCredentials = -4
	settings.Gateway.MaxRetryIntervalMS = -10

	updated, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	if updated.Gateway.RetryCount != 3 {
		t.Fatalf("expected retry_count to default to 3, got %d", updated.Gateway.RetryCount)
	}
	if updated.Gateway.MaxRetryCredentials != 0 {
		t.Fatalf("expected negative max_retry_credentials to clamp to 0, got %d", updated.Gateway.MaxRetryCredentials)
	}
	if updated.Gateway.MaxRetryIntervalMS != 2000 {
		t.Fatalf("expected negative max_retry_interval_ms to default to 2000, got %d", updated.Gateway.MaxRetryIntervalMS)
	}

	settings.Gateway.RetryCount = 99
	clamped, err := svc.UpdateAdminSettings(context.Background(), settings, 1)
	if err != nil {
		t.Fatalf("update admin settings (clamp): %v", err)
	}
	if clamped.Gateway.RetryCount != 20 {
		t.Fatalf("expected retry_count to clamp to 20, got %d", clamped.Gateway.RetryCount)
	}
}

func TestBatchEnableRedeemCodesFlipsDisabledBackToActive(t *testing.T) {
	now := time.Date(2026, time.June, 9, 9, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	// Three codes, all created as active.
	a, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "ENABLE1", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	b, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "ENABLE2", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	stillActive, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "ENABLE3", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)

	// Disable A and B; leave the third active. Verifies the enable path only
	// touches rows whose stored status is DISABLED.
	if _, err := svc.BatchDisableRedeemCodes(context.Background(), []int{a.ID, b.ID}, "", 1); err != nil {
		t.Fatalf("disable seed: %v", err)
	}

	result, err := svc.BatchEnableRedeemCodes(context.Background(), []int{a.ID, b.ID, stillActive.ID, 99999}, 1)
	if err != nil {
		t.Fatalf("batch enable: %v", err)
	}
	if result.Requested != 4 {
		t.Fatalf("requested: want 4, got %d", result.Requested)
	}
	if result.Succeeded != 2 {
		t.Fatalf("succeeded: want 2 (A, B), got %d", result.Succeeded)
	}
	if result.Failed != 2 {
		t.Fatalf("failed: want 2 (the active one + unknown), got %d (ids=%v)", result.Failed, result.FailedIDs)
	}

	list, _ := svc.ListRedeemCodes(context.Background(), admincontrol.ListOptions{})
	byID := map[int]admincontrol.RedeemCode{}
	for _, c := range list.Items {
		byID[c.ID] = c
	}
	for _, id := range []int{a.ID, b.ID, stillActive.ID} {
		if got := byID[id]; got.Status != admincontrol.RedeemCodeStatusActive {
			t.Fatalf("code %d should be active after enable, got %s", id, got.Status)
		}
	}

	if _, err := svc.BatchEnableRedeemCodes(context.Background(), nil, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("empty ids should be invalid, got %v", err)
	}
}

func TestBatchExtendRedeemCodesSetsExpiryAndSkipsFullyConsumed(t *testing.T) {
	now := time.Date(2026, time.June, 9, 9, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	// Create two unredeemed codes plus one that we'll mark fully consumed.
	codeA, err := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "EXTEND1", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	codeB, err := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "EXTEND2", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	consumed, err := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "EXTEND3", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	if err != nil {
		t.Fatalf("create C: %v", err)
	}
	// Mark the third one fully consumed via the store (no public service hook
	// to do this since redemption is a user-facing flow).
	all, _ := store.ListRedeemCodes(context.Background())
	for i := range all {
		if all[i].ID == consumed.ID {
			all[i].RedeemedCount = all[i].MaxRedemptions
			if _, err := store.DeleteRedeemCode(context.Background(), all[i].ID); err != nil {
				t.Fatalf("temp delete to re-create consumed: %v", err)
			}
			if _, err := store.CreateRedeemCode(context.Background(), all[i]); err != nil {
				t.Fatalf("recreate consumed: %v", err)
			}
		}
	}

	newExpiry := now.AddDate(0, 0, 30)
	result, err := svc.BatchExtendRedeemCodes(context.Background(), []int{codeA.ID, codeB.ID, consumed.ID, 99999}, newExpiry, 1)
	if err != nil {
		t.Fatalf("batch extend: %v", err)
	}
	if result.Requested != 4 {
		t.Fatalf("requested: want 4, got %d", result.Requested)
	}
	if result.Succeeded != 2 {
		t.Fatalf("succeeded: want 2 (A and B), got %d", result.Succeeded)
	}
	if result.Failed != 2 {
		t.Fatalf("failed: want 2 (consumed + unknown), got %d (ids=%v)", result.Failed, result.FailedIDs)
	}

	// Verify A and B actually got the new expiry; consumed unchanged.
	list, _ := svc.ListRedeemCodes(context.Background(), admincontrol.ListOptions{})
	byID := map[int]admincontrol.RedeemCode{}
	for _, c := range list.Items {
		byID[c.ID] = c
	}
	for _, id := range []int{codeA.ID, codeB.ID} {
		got := byID[id]
		if got.ExpiresAt == nil || !got.ExpiresAt.Equal(newExpiry) {
			t.Fatalf("code %d expiry: want %v, got %v", id, newExpiry, got.ExpiresAt)
		}
	}

	// Zero expiresAt must be rejected as invalid input.
	if _, err := svc.BatchExtendRedeemCodes(context.Background(), []int{codeA.ID}, time.Time{}, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("zero expiry should be invalid, got %v", err)
	}
	// Empty ids must be rejected.
	if _, err := svc.BatchExtendRedeemCodes(context.Background(), nil, newExpiry, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("empty ids should be invalid, got %v", err)
	}
}

// TestBatchUpdateRedeemCodesAllSuccess pins the happy path: per-row partial
// updates land on the right rows.
func TestBatchUpdateRedeemCodesAllSuccess(t *testing.T) {
	now := time.Date(2026, time.June, 14, 9, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	a, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "UPD1", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	b, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "UPD2", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	newAmount := "12.34"
	newMax := 3
	newNote := "bumped"
	expiresAt := now.Add(48 * time.Hour)
	items := []admincontrol.BatchUpdateRedeemCodeItem{
		{ID: a.ID, Value: &newAmount, Note: &newNote},
		{ID: b.ID, MaxRedemptions: &newMax, ExpiresAtSet: true, ExpiresAt: &expiresAt},
	}
	results, err := svc.BatchUpdateRedeemCodes(context.Background(), items, 1)
	if err != nil {
		t.Fatalf("BatchUpdateRedeemCodes: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" {
			t.Fatalf("row %d failed: %+v", i, row)
		}
	}
	list, _ := svc.ListRedeemCodes(context.Background(), admincontrol.ListOptions{})
	byID := map[int]admincontrol.RedeemCode{}
	for _, c := range list.Items {
		byID[c.ID] = c
	}
	if byID[a.ID].Value != "12.34" || byID[a.ID].Note != "bumped" {
		t.Fatalf("code A not updated: %+v", byID[a.ID])
	}
	if byID[b.ID].MaxRedemptions != 3 || byID[b.ID].ExpiresAt == nil {
		t.Fatalf("code B not updated: %+v", byID[b.ID])
	}
}

// TestBatchUpdateRedeemCodesPerRowFailureSurfaces pins per-row failure modes:
// invalid id, invalid value, missing row (idempotent), no-fields-set.
func TestBatchUpdateRedeemCodesPerRowFailureSurfaces(t *testing.T) {
	now := time.Date(2026, time.June, 14, 9, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	good, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "G", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	bad := "-5"
	ok := "9"
	items := []admincontrol.BatchUpdateRedeemCodeItem{
		{ID: good.ID, Value: &ok},
		{ID: 0, Value: &ok},          // invalid id
		{ID: good.ID + 999, Value: &ok}, // missing → idempotent
		{ID: 8888, Value: &bad},      // invalid value
		{ID: 7777},                   // no fields
	}
	results, err := svc.BatchUpdateRedeemCodes(context.Background(), items, 1)
	if err != nil {
		t.Fatalf("BatchUpdateRedeemCodes: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Fatalf("row 0 should succeed: %+v", results[0])
	}
	if results[1].Error == "" {
		t.Fatalf("row 1 invalid id should fail: %+v", results[1])
	}
	if results[2].Error != "" {
		t.Fatalf("row 2 missing should be idempotent: %+v", results[2])
	}
	if results[3].Error == "" {
		t.Fatalf("row 3 invalid value should fail: %+v", results[3])
	}
	if results[4].Error == "" {
		t.Fatalf("row 4 no-fields should fail: %+v", results[4])
	}
}

// TestBatchUpdateRedeemCodesDedupesWithinBatch: doubled id surfaces as
// duplicate on the second occurrence.
func TestBatchUpdateRedeemCodesDedupesWithinBatch(t *testing.T) {
	now := time.Date(2026, time.June, 14, 9, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	a, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "DUP", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	v1, v2 := "9", "11"
	items := []admincontrol.BatchUpdateRedeemCodeItem{
		{ID: a.ID, Value: &v1},
		{ID: a.ID, Value: &v2},
	}
	results, err := svc.BatchUpdateRedeemCodes(context.Background(), items, 1)
	if err != nil {
		t.Fatalf("BatchUpdateRedeemCodes: %v", err)
	}
	if results[0].Error != "" {
		t.Fatalf("first should succeed: %+v", results[0])
	}
	if results[1].Error != "duplicate id in batch" {
		t.Fatalf("second should report duplicate, got %+v", results[1])
	}
}

// TestBatchUpdateRedeemCodesRejectsEmptyAndOversize: outer guards.
func TestBatchUpdateRedeemCodesRejectsEmptyAndOversize(t *testing.T) {
	now := time.Date(2026, time.June, 14, 9, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, _ := admincontrolservice.New(store, fixedClock{now: now})
	if _, err := svc.BatchUpdateRedeemCodes(context.Background(), nil, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("empty should ErrInvalidInput, got %v", err)
	}
	oversize := make([]admincontrol.BatchUpdateRedeemCodeItem, 1001)
	v := "1"
	for i := range oversize {
		oversize[i] = admincontrol.BatchUpdateRedeemCodeItem{ID: i + 1, Value: &v}
	}
	if _, err := svc.BatchUpdateRedeemCodes(context.Background(), oversize, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf(">MaxItems should ErrInvalidInput, got %v", err)
	}
}

func TestDeleteRedeemCode(t *testing.T) {
	now := time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	code, err := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code:           "CLEANUP1",
		Type:           admincontrol.RedeemCodeTypeBalance,
		Value:          "5",
		Currency:       "USD",
		MaxRedemptions: 1,
	}, 1)
	if err != nil {
		t.Fatalf("create redeem code: %v", err)
	}

	deleted, err := svc.DeleteRedeemCode(context.Background(), code.ID, 1)
	if err != nil {
		t.Fatalf("delete redeem code: %v", err)
	}
	if deleted.ID != code.ID {
		t.Fatalf("unexpected deleted code: %+v", deleted)
	}

	list, err := svc.ListRedeemCodes(context.Background(), admincontrol.ListOptions{})
	if err != nil {
		t.Fatalf("list redeem codes: %v", err)
	}
	if list.Total != 0 {
		t.Fatalf("expected no redeem codes after delete, got %d", list.Total)
	}

	// Re-deleting an already-removed code is a not-found, not a crash.
	if _, err := svc.DeleteRedeemCode(context.Background(), code.ID, 1); !errors.Is(err, admincontrol.ErrNotFound) {
		t.Fatalf("expected ErrNotFound re-deleting, got %v", err)
	}
}

func TestRedeemCodeCreditsBalanceOnce(t *testing.T) {
	now := time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)
	users := usermemory.New()
	billing := billingmemory.New()
	store := admincontrolmemory.NewWithFulfillment(users, billing, nil)
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "redeem@example.com",
		Name:         "Redeem User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "1.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	code, err := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code:           "WELCOME10",
		Type:           admincontrol.RedeemCodeTypeBalance,
		Value:          "10",
		Currency:       "USD",
		MaxRedemptions: 1,
	}, 1)
	if err != nil {
		t.Fatalf("create redeem code: %v", err)
	}

	result, err := svc.RedeemCode(context.Background(), user.User, admincontrol.RedeemCodeRedemptionRequest{Code: " welcome10 "})
	if err != nil {
		t.Fatalf("redeem code: %v", err)
	}
	if result.AlreadyRedeemed || result.RedeemCode.ID != code.ID || result.Redemption.Amount != "10.00000000" || result.Redemption.BalanceAfter != "11.00000000" {
		t.Fatalf("unexpected redemption result: %+v", result)
	}
	updated, err := users.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("find updated user: %v", err)
	}
	if updated.Balance != "11.00000000" {
		t.Fatalf("balance = %s, want 11.00000000", updated.Balance)
	}
	ledger, err := billing.List(context.Background())
	if err != nil {
		t.Fatalf("list billing ledger: %v", err)
	}
	if len(ledger) != 1 || ledger[0].ReferenceType != "redeem_code" || ledger[0].Amount != "10.00000000" {
		t.Fatalf("unexpected billing ledger: %+v", ledger)
	}

	repeated, err := svc.RedeemCode(context.Background(), user.User, admincontrol.RedeemCodeRedemptionRequest{Code: "WELCOME10"})
	if err != nil {
		t.Fatalf("redeem same code again: %v", err)
	}
	if !repeated.AlreadyRedeemed || repeated.Redemption.ID != result.Redemption.ID {
		t.Fatalf("expected idempotent already redeemed result, got %+v", repeated)
	}
	ledger, _ = billing.List(context.Background())
	updated, _ = users.FindByID(context.Background(), user.ID)
	if len(ledger) != 1 || updated.Balance != "11.00000000" {
		t.Fatalf("repeat redemption changed side effects: ledger=%+v user=%+v", ledger, updated)
	}
}

func TestUserAnnouncementsFilterVisibleAndTrackReadState(t *testing.T) {
	now := time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	adminUser := userscontract.User{ID: 1, Roles: []userscontract.Role{userscontract.RoleAdmin}}
	regularUser := userscontract.User{ID: 2, Roles: []userscontract.Role{userscontract.RoleUser}}
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	if _, err := svc.CreateAnnouncement(context.Background(), admincontrol.AnnouncementRequest{
		Title:    "all",
		Content:  "visible to all",
		Status:   admincontrol.AnnouncementStatusPublished,
		Severity: admincontrol.AnnouncementSeverityInfo,
		Audience: admincontrol.AnnouncementAudienceAll,
	}, adminUser.ID); err != nil {
		t.Fatalf("create all announcement: %v", err)
	}
	usersOnly, err := svc.CreateAnnouncement(context.Background(), admincontrol.AnnouncementRequest{
		Title:    "users",
		Content:  "visible to users",
		Status:   admincontrol.AnnouncementStatusPublished,
		Severity: admincontrol.AnnouncementSeverityWarning,
		Audience: admincontrol.AnnouncementAudienceUsers,
	}, adminUser.ID)
	if err != nil {
		t.Fatalf("create users announcement: %v", err)
	}
	if _, err := svc.CreateAnnouncement(context.Background(), admincontrol.AnnouncementRequest{
		Title:    "admins",
		Content:  "visible to admins",
		Status:   admincontrol.AnnouncementStatusPublished,
		Severity: admincontrol.AnnouncementSeverityCritical,
		Audience: admincontrol.AnnouncementAudienceAdmins,
	}, adminUser.ID); err != nil {
		t.Fatalf("create admins announcement: %v", err)
	}
	if _, err := svc.CreateAnnouncement(context.Background(), admincontrol.AnnouncementRequest{
		Title:    "draft",
		Content:  "hidden draft",
		Status:   admincontrol.AnnouncementStatusDraft,
		Severity: admincontrol.AnnouncementSeverityInfo,
		Audience: admincontrol.AnnouncementAudienceAll,
	}, adminUser.ID); err != nil {
		t.Fatalf("create draft announcement: %v", err)
	}
	if _, err := svc.CreateAnnouncement(context.Background(), admincontrol.AnnouncementRequest{
		Title:    "future",
		Content:  "hidden future",
		Status:   admincontrol.AnnouncementStatusPublished,
		Severity: admincontrol.AnnouncementSeverityInfo,
		Audience: admincontrol.AnnouncementAudienceAll,
		StartsAt: &future,
	}, adminUser.ID); err != nil {
		t.Fatalf("create future announcement: %v", err)
	}
	if _, err := svc.CreateAnnouncement(context.Background(), admincontrol.AnnouncementRequest{
		Title:    "expired",
		Content:  "hidden expired",
		Status:   admincontrol.AnnouncementStatusPublished,
		Severity: admincontrol.AnnouncementSeverityInfo,
		Audience: admincontrol.AnnouncementAudienceAll,
		EndsAt:   &past,
	}, adminUser.ID); err != nil {
		t.Fatalf("create expired announcement: %v", err)
	}

	userList, err := svc.ListUserAnnouncements(context.Background(), regularUser, admincontrol.ListOptions{})
	if err != nil {
		t.Fatalf("list user announcements: %v", err)
	}
	if userList.Total != 2 || userList.Unread != 2 {
		t.Fatalf("expected regular user to see two unread announcements, got %+v", userList)
	}
	for _, item := range userList.Items {
		if item.Audience == admincontrol.AnnouncementAudienceAdmins || item.Status != admincontrol.AnnouncementStatusPublished {
			t.Fatalf("regular user saw hidden announcement: %+v", item)
		}
	}

	read, err := svc.MarkUserAnnouncementRead(context.Background(), regularUser, usersOnly.ID)
	if err != nil {
		t.Fatalf("mark announcement read: %v", err)
	}
	if !read.Read || read.ReadAt == nil {
		t.Fatalf("expected read state, got %+v", read)
	}
	readAgain, err := svc.MarkUserAnnouncementRead(context.Background(), regularUser, usersOnly.ID)
	if err != nil {
		t.Fatalf("mark announcement read again: %v", err)
	}
	if !readAgain.ReadAt.Equal(*read.ReadAt) {
		t.Fatalf("mark read should be idempotent, first=%v second=%v", read.ReadAt, readAgain.ReadAt)
	}

	userList, err = svc.ListUserAnnouncements(context.Background(), regularUser, admincontrol.ListOptions{})
	if err != nil {
		t.Fatalf("list user announcements after read: %v", err)
	}
	if userList.Unread != 1 {
		t.Fatalf("expected one unread announcement, got %+v", userList)
	}
	laterSvc, err := admincontrolservice.New(store, fixedClock{now: now.Add(time.Minute)})
	if err != nil {
		t.Fatalf("new later service: %v", err)
	}
	if _, err := laterSvc.UpdateAnnouncement(context.Background(), usersOnly.ID, admincontrol.AnnouncementRequest{
		Title:    "users updated",
		Content:  "new content",
		Status:   admincontrol.AnnouncementStatusPublished,
		Severity: admincontrol.AnnouncementSeverityWarning,
		Audience: admincontrol.AnnouncementAudienceUsers,
	}, adminUser.ID); err != nil {
		t.Fatalf("update users announcement: %v", err)
	}
	userList, err = laterSvc.ListUserAnnouncements(context.Background(), regularUser, admincontrol.ListOptions{})
	if err != nil {
		t.Fatalf("list user announcements after update: %v", err)
	}
	if userList.Unread != 2 {
		t.Fatalf("updated announcement should become unread again, got %+v", userList)
	}

	adminList, err := svc.ListUserAnnouncements(context.Background(), adminUser, admincontrol.ListOptions{})
	if err != nil {
		t.Fatalf("list admin announcements: %v", err)
	}
	if adminList.Total != 2 {
		t.Fatalf("expected admin to see all+admin announcements, got %+v", adminList)
	}
	if _, err := svc.MarkUserAnnouncementRead(context.Background(), regularUser, 3); err != admincontrol.ErrNotFound {
		t.Fatalf("expected user-only visibility to hide admin announcement, got %v", err)
	}
}

func TestAnnouncementSegmentTargetingAndReadStatus(t *testing.T) {
	now := time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	acme := userscontract.User{ID: 10, Email: "ops@acme.io", Roles: []userscontract.Role{userscontract.RoleUser}}
	other := userscontract.User{ID: 11, Email: "ops@other.io", Roles: []userscontract.Role{userscontract.RoleUser}}

	created, err := svc.CreateAnnouncement(context.Background(), admincontrol.AnnouncementRequest{
		Title:    "acme-only",
		Content:  "targeted by email domain",
		Status:   admincontrol.AnnouncementStatusPublished,
		Severity: admincontrol.AnnouncementSeverityInfo,
		Audience: admincontrol.AnnouncementAudienceAll,
		Segments: []admincontrol.AnnouncementSegment{{EmailDomains: []string{"ACME.io"}}},
	}, 1)
	if err != nil {
		t.Fatalf("create targeted announcement: %v", err)
	}

	acmeList, err := svc.ListUserAnnouncements(context.Background(), acme, admincontrol.ListOptions{})
	if err != nil {
		t.Fatalf("list for acme: %v", err)
	}
	if !announcementListContains(acmeList.Items, created.ID) {
		t.Fatal("a user in the targeted email domain must see the announcement")
	}

	otherList, err := svc.ListUserAnnouncements(context.Background(), other, admincontrol.ListOptions{})
	if err != nil {
		t.Fatalf("list for other: %v", err)
	}
	if announcementListContains(otherList.Items, created.ID) {
		t.Fatal("a user outside the targeted email domain must NOT see the announcement")
	}

	if _, err := svc.MarkUserAnnouncementRead(context.Background(), acme, created.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	status, err := svc.AnnouncementReadStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status.Total != 1 || len(status.Readers) != 1 || status.Readers[0].UserID != acme.ID {
		t.Fatalf("unexpected read status: %+v", status)
	}
}

func announcementListContains(items []admincontrol.UserAnnouncement, id int) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestSystemLogsRecordListAndCleanup(t *testing.T) {
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	first, err := svc.RecordSystemLog(context.Background(), admincontrol.RecordSystemLogRequest{
		Level:     admincontrol.OpsSystemLogLevelWarn,
		Source:    "ops.dashboard",
		Message:   "rotate log volume",
		RequestID: "req_1",
		TraceID:   "trace_1",
		Metadata:  map[string]any{"safe": true},
	})
	if err != nil {
		t.Fatalf("record first system log: %v", err)
	}
	second, err := svc.RecordSystemLog(context.Background(), admincontrol.RecordSystemLogRequest{
		Level:   admincontrol.OpsSystemLogLevelError,
		Source:  "ops.worker",
		Message: "worker failed",
	})
	if err != nil {
		t.Fatalf("record second system log: %v", err)
	}

	list, err := svc.ListSystemLogs(context.Background(), admincontrol.SystemLogListOptions{Level: admincontrol.OpsSystemLogLevelWarn, Query: "rotate"})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("expected one matching log, got total=%d items=%d", list.Total, len(list.Items))
	}
	if list.Items[0].RequestID != first.RequestID || list.Items[0].TraceID != first.TraceID {
		t.Fatalf("expected request/trace ids in list item, got %+v", list.Items[0])
	}

	dryRun, err := svc.CleanupSystemLogs(context.Background(), admincontrol.SystemLogCleanupFilter{
		Source:    "ops.dashboard",
		DryRun:    true,
		MaxDelete: 1,
	})
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if dryRun.Matched != 1 || dryRun.Deleted != 0 || !dryRun.DryRun {
		t.Fatalf("unexpected dry-run cleanup result: %+v", dryRun)
	}

	afterDryRun, err := svc.ListSystemLogs(context.Background(), admincontrol.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list after dry-run: %v", err)
	}
	if afterDryRun.Total != 2 {
		t.Fatalf("dry-run should not delete rows, got total=%d", afterDryRun.Total)
	}

	cleanup, err := svc.CleanupSystemLogs(context.Background(), admincontrol.SystemLogCleanupFilter{
		Source:    "ops.dashboard",
		MaxDelete: 10,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleanup.Matched != 1 || cleanup.Deleted != 1 || cleanup.Limited {
		t.Fatalf("unexpected cleanup result: %+v", cleanup)
	}

	remaining, err := svc.ListSystemLogs(context.Background(), admincontrol.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list after cleanup: %v", err)
	}
	if remaining.Total != 1 || len(remaining.Items) != 1 || remaining.Items[0].ID != second.ID {
		t.Fatalf("expected only second log to remain, got %+v", remaining)
	}

	if _, err := svc.CleanupSystemLogs(context.Background(), admincontrol.SystemLogCleanupFilter{}); err == nil {
		t.Fatal("expected cleanup without filters to fail")
	}
}

// TestBatchDisableRedeemCodesClassifiesPerItemReasons covers the four reason
// branches the bulk-disable now surfaces, plus note validation. Codes are
// seeded directly through the service so the test exercises both the service
// classification and the memory-store reason map.
func TestBatchDisableRedeemCodesClassifiesPerItemReasons(t *testing.T) {
	now := time.Date(2026, time.June, 12, 10, 0, 0, 0, time.UTC)
	store := admincontrolmemory.New()
	svc, err := admincontrolservice.New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	// Four codes covering each reason branch.
	expiredAt := now.Add(-1 * time.Hour)
	future := now.Add(24 * time.Hour)
	adminCode, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "DISABLE-ADMIN", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1, ExpiresAt: &future,
	}, 1)
	expiredCode, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "DISABLE-EXPIRED", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1, ExpiresAt: &expiredAt,
	}, 1)
	alreadyCode, _ := svc.CreateRedeemCode(context.Background(), admincontrol.CreateRedeemCodeRequest{
		Code: "DISABLE-AGAIN", Type: admincontrol.RedeemCodeTypeBalance, Value: "5", Currency: "USD", MaxRedemptions: 1,
	}, 1)
	// Pre-disable the "already" code so a second call classifies it as already_disabled.
	if _, err := svc.BatchDisableRedeemCodes(context.Background(), []int{alreadyCode.ID}, "seed", 1); err != nil {
		t.Fatalf("seed disable: %v", err)
	}

	const missingID = 9999999
	result, err := svc.BatchDisableRedeemCodes(
		context.Background(),
		[]int{adminCode.ID, expiredCode.ID, alreadyCode.ID, missingID},
		"campaign rollback",
		1,
	)
	if err != nil {
		t.Fatalf("batch disable: %v", err)
	}
	if result.Requested != 4 {
		t.Fatalf("requested: want 4, got %d", result.Requested)
	}
	// Succeeded = admin_action + expired (both ended up disabled this call).
	if result.Succeeded != 2 {
		t.Fatalf("succeeded: want 2, got %d (reasons=%v)", result.Succeeded, result.PerItemReasons)
	}
	// Failed = already_disabled + not_found.
	if result.Failed != 2 {
		t.Fatalf("failed: want 2, got %d (ids=%v)", result.Failed, result.FailedIDs)
	}
	wantReasons := map[int]string{
		adminCode.ID:   admincontrol.RedeemDisabledReasonAdminAction,
		expiredCode.ID: admincontrol.RedeemDisabledReasonExpired,
		alreadyCode.ID: admincontrol.RedeemDisabledReasonAlreadyDisabled,
		missingID:      admincontrol.RedeemDisabledReasonNotFound,
	}
	for id, want := range wantReasons {
		if got := result.PerItemReasons[id]; got != want {
			t.Fatalf("reason for %d: want %q, got %q", id, want, got)
		}
	}
	wantBreakdown := map[string]int{
		admincontrol.RedeemDisabledReasonAdminAction:     1,
		admincontrol.RedeemDisabledReasonExpired:         1,
		admincontrol.RedeemDisabledReasonAlreadyDisabled: 1,
		admincontrol.RedeemDisabledReasonNotFound:        1,
	}
	for reason, want := range wantBreakdown {
		if got := result.DisabledReasonBreakdown[reason]; got != want {
			t.Fatalf("breakdown[%s]: want %d, got %d (full=%v)", reason, want, got, result.DisabledReasonBreakdown)
		}
	}

	// Confirm the note + disabled_reason actually persisted on the row we
	// flipped (admin_action branch). This is the audit-trail guarantee.
	list, _ := svc.ListRedeemCodes(context.Background(), admincontrol.ListOptions{})
	byID := map[int]admincontrol.RedeemCode{}
	for _, c := range list.Items {
		byID[c.ID] = c
	}
	flipped := byID[adminCode.ID]
	if flipped.Note != "campaign rollback" {
		t.Fatalf("admin_action row note: want %q, got %q", "campaign rollback", flipped.Note)
	}
	if flipped.DisabledReason != admincontrol.RedeemDisabledReasonAdminAction {
		t.Fatalf("admin_action row disabled_reason: want %q, got %q", admincontrol.RedeemDisabledReasonAdminAction, flipped.DisabledReason)
	}
	expiredRow := byID[expiredCode.ID]
	if expiredRow.DisabledReason != admincontrol.RedeemDisabledReasonExpired {
		t.Fatalf("expired row disabled_reason: want %q, got %q", admincontrol.RedeemDisabledReasonExpired, expiredRow.DisabledReason)
	}

	// Note validation: anything over 500 chars is rejected up-front.
	longNote := strings.Repeat("x", 501)
	if _, err := svc.BatchDisableRedeemCodes(context.Background(), []int{adminCode.ID}, longNote, 1); !errors.Is(err, admincontrol.ErrInvalidInput) {
		t.Fatalf("long note should be rejected, got %v", err)
	}
}

package service

import (
	"context"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const (
	settingsKeyAdminSettings = "admin_control.admin_settings"

	oauthTokenAuthMethodNone              = "none"
	oauthTokenAuthMethodClientSecretPost  = "client_secret_post"
	oauthTokenAuthMethodClientSecretBasic = "client_secret_basic"

	// adminSettingsCacheTTL bounds how long a cached admin-settings read may be
	// served. The gateway consults these settings several times per request
	// (retry policy, channel filter, request shaper, passthrough headers), so
	// reads come from this cache instead of the settings store. Same-instance
	// updates invalidate immediately; cross-instance updates converge within
	// the TTL.
	adminSettingsCacheTTL = 3 * time.Second

	customMenuVisibilityUser  = "user"
	customMenuVisibilityAdmin = "admin"
)

var emailSuffixDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

var defaultGatewayPassthroughHeaderAllowlist = []string{
	"retry-after",
	"request-id",
	"x-request-id",
	"x-upstream-request-id",
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
	"ratelimit-*",
	"anthropic-ratelimit-*",
	"x-codex-primary-used-percent",
	"x-codex-primary-reset-after-seconds",
	"x-codex-primary-window-minutes",
	"x-codex-secondary-used-percent",
	"x-codex-secondary-reset-after-seconds",
	"x-codex-secondary-window-minutes",
	"x-codex-primary-over-secondary-limit-percent",
}

var defaultGatewayProtocolConversionRoutes = []string{
	"chat_completions_to_responses",
	"chat_completions_to_messages",
	"responses_to_chat_completions",
	"responses_to_messages",
	"messages_to_chat_completions",
	"messages_to_responses",
}

func (s *Service) GetAdminSettings(ctx context.Context) (admincontrol.AdminSettings, error) {
	settings, err := s.settingsCache.Get(ctx, s.loadAdminSettings)
	if err != nil {
		return admincontrol.AdminSettings{}, err
	}
	return cloneAdminSettings(settings), nil
}

func (s *Service) loadAdminSettings(ctx context.Context) (admincontrol.AdminSettings, error) {
	settings := defaultAdminSettings(s.clock.Now())
	if err := s.loadTyped(ctx, settingsKeyAdminSettings, &settings); err != nil {
		return admincontrol.AdminSettings{}, err
	}
	return settings, nil
}

func (s *Service) NotificationEmailTemplates(ctx context.Context) map[string]string {
	settings, err := s.GetAdminSettings(ctx)
	if err != nil {
		return map[string]string{}
	}
	return cloneStringMap(settings.Email.Templates)
}

func (s *Service) UpdateAdminSettings(ctx context.Context, settings admincontrol.AdminSettings, actorUserID int) (admincontrol.AdminSettings, error) {
	normalized, err := normalizeAdminSettings(settings)
	if err != nil {
		return admincontrol.AdminSettings{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyAdminSettings, normalized, actorUserID); err != nil {
		return admincontrol.AdminSettings{}, err
	}
	s.settingsCache.Invalidate()
	return cloneAdminSettings(normalized), nil
}

func defaultAdminSettings(now time.Time) admincontrol.AdminSettings {
	balanceLowNotifyEnabled := true
	subscriptionExpiryNotifyEnabled := true
	accountQuotaNotifyEnabled := true
	return admincontrol.AdminSettings{
		General: admincontrol.AdminSettingsGeneral{
			SiteName:     "SRapi",
			SiteSubtitle: "AI gateway control plane",
			LogoURL:      "",
			VersionLabel: "",
			ContactInfo:  "",
			DocURL:       "",
			CustomMenus:  []admincontrol.CustomMenuItem{},
		},
		Agreement: admincontrol.AdminSettingsAgreement{},
		Features: admincontrol.AdminSettingsFeatures{
			EnabledChannels:          []string{},
			ChannelMonitoringEnabled: true,
			InvitationRebateEnabled:  false,
			PaymentsEnabled:          false,
		},
		Security: admincontrol.AdminSettingsSecurity{
			AdminAPIKey:                      admincontrol.SecretConfigured{Configured: false},
			RegistrationEnabled:              true,
			RegistrationEmailSuffixAllowlist: []string{},
			OAuthEnabled:                     false,
			OAuthProviders:                   []string{},
			OAuthProviderConfigs:             []admincontrol.OAuthProviderConfig{},
		},
		Users: admincontrol.AdminSettingsUsers{
			DefaultBalance:        "0",
			DefaultGroup:          "default",
			UserSelfDeleteEnabled: false,
			RPMLimitDefault:       0,
		},
		Gateway: admincontrol.AdminSettingsGateway{
			OverloadCooldownSeconds:              30,
			RateLimitCooldownSeconds:             30,
			StreamTimeoutSeconds:                 600,
			RequestShaperEnabled:                 true,
			ProtocolConversionRoutes:             cloneStringSlice(defaultGatewayProtocolConversionRoutes),
			RetryCount:                           gatewayRetryCountDefault,
			MaxRetryCredentials:                  0,
			MaxRetryIntervalMS:                   2000,
			SchedulerStrategyRolloutEnabled:      false,
			SchedulerStrategyShadowStrategy:      "",
			SchedulerStrategyRolloutPercent:      0,
			SchedulerStrategyRolloutModels:       []string{},
			SchedulerStrategyRolloutAPIKeyHashes: []string{},
			PassthroughUpstreamHeaders:           false,
			PassthroughHeaderAllowlist:           cloneStringSlice(defaultGatewayPassthroughHeaderAllowlist),
		},
		Payment: admincontrol.AdminSettingsPayment{
			Enabled:                  false,
			Providers:                []string{},
			SubscriptionPlansEnabled: false,
		},
		Email: admincontrol.AdminSettingsEmail{
			SMTPConfigured:                   false,
			SMTPHost:                         "",
			SMTPPort:                         587,
			SMTPUsername:                     "",
			SMTPFrom:                         "",
			SMTPFromName:                     "",
			SMTPUseTLS:                       false,
			PublicBaseURL:                    "",
			Templates:                        map[string]string{},
			BalanceLowNotifyEnabled:          &balanceLowNotifyEnabled,
			BalanceLowNotifyThreshold:        "5.00000000",
			BalanceLowNotifyRechargeURL:      "",
			SubscriptionExpiryNotifyEnabled:  &subscriptionExpiryNotifyEnabled,
			AccountQuotaNotifyEnabled:        &accountQuotaNotifyEnabled,
			AccountQuotaNotifyRemainingRatio: "0.20000000",
		},
		Backup: admincontrol.AdminSettingsBackup{
			Enabled:       false,
			LastBackupAt:  &now,
			RetentionDays: 30,
		},
		Copilot: admincontrol.AdminSettingsCopilot{
			Enabled:           false,
			Source:            "account",
			Models:            []string{},
			DedicatedProtocol: "openai-compatible",
			OwnerOnly:         false,
			AutoRunReads:      true,
		},
		Maintenance: admincontrol.AdminSettingsMaintenance{
			Enabled: false,
			Message: "",
		},
	}
}

// maintenanceMessageMaxLen caps how long an operator-supplied maintenance
// message can be. 1KiB is enough for a sentence + a status-page link without
// risking gateway 503 payloads ballooning.
const maintenanceMessageMaxLen = 1024

func normalizeMaintenance(settings admincontrol.AdminSettingsMaintenance) admincontrol.AdminSettingsMaintenance {
	settings.Message = strings.TrimSpace(settings.Message)
	if len(settings.Message) > maintenanceMessageMaxLen {
		settings.Message = settings.Message[:maintenanceMessageMaxLen]
	}
	// A past recovery time is meaningless to surface — drop it rather than
	// persisting stale promises that confuse the banner.
	if settings.ExpectedRecoveryAt != nil && !settings.ExpectedRecoveryAt.After(time.Now()) {
		settings.ExpectedRecoveryAt = nil
	}
	return settings
}

func normalizeAdminSettings(settings admincontrol.AdminSettings) (admincontrol.AdminSettings, error) {
	settings.General.SiteName = strings.TrimSpace(settings.General.SiteName)
	settings.General.SiteSubtitle = strings.TrimSpace(settings.General.SiteSubtitle)
	settings.General.LogoURL = strings.TrimSpace(settings.General.LogoURL)
	settings.General.VersionLabel = strings.TrimSpace(settings.General.VersionLabel)
	settings.General.ContactInfo = strings.TrimSpace(settings.General.ContactInfo)
	settings.General.DocURL = strings.TrimSpace(settings.General.DocURL)
	settings.Users.DefaultBalance = strings.TrimSpace(settings.Users.DefaultBalance)
	settings.Users.DefaultGroup = strings.TrimSpace(settings.Users.DefaultGroup)
	settings.Gateway.SchedulerStrategyShadowStrategy = strings.TrimSpace(settings.Gateway.SchedulerStrategyShadowStrategy)
	settings.Email.SMTPHost = strings.TrimSpace(settings.Email.SMTPHost)
	settings.Email.SMTPUsername = strings.TrimSpace(settings.Email.SMTPUsername)
	settings.Email.SMTPFrom = strings.TrimSpace(settings.Email.SMTPFrom)
	settings.Email.SMTPFromName = strings.TrimSpace(settings.Email.SMTPFromName)
	settings.Email.PublicBaseURL = strings.TrimRight(strings.TrimSpace(settings.Email.PublicBaseURL), "/")
	settings.Email.BalanceLowNotifyThreshold = strings.TrimSpace(settings.Email.BalanceLowNotifyThreshold)
	settings.Email.BalanceLowNotifyRechargeURL = strings.TrimSpace(settings.Email.BalanceLowNotifyRechargeURL)
	settings.Email.AccountQuotaNotifyRemainingRatio = strings.TrimSpace(settings.Email.AccountQuotaNotifyRemainingRatio)
	registrationEmailSuffixAllowlist, err := normalizeRegistrationEmailSuffixAllowlist(settings.Security.RegistrationEmailSuffixAllowlist)
	if err != nil {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	customMenus, err := normalizeCustomMenus(settings.General.CustomMenus)
	if err != nil {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	settings.General.CustomMenus = customMenus
	settings.Security.RegistrationEmailSuffixAllowlist = registrationEmailSuffixAllowlist
	settings.Security.OAuthProviders = uniqueTrimmedStrings(settings.Security.OAuthProviders)
	oauthProviderConfigs, err := normalizeOAuthProviderConfigs(settings.Security.OAuthProviderConfigs)
	if err != nil {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	settings.Security.OAuthProviderConfigs = oauthProviderConfigs
	settings.Gateway.SchedulerStrategyRolloutModels = uniqueTrimmedStrings(settings.Gateway.SchedulerStrategyRolloutModels)
	settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = uniqueTrimmedStrings(settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes)
	settings.Gateway.ProtocolConversionRoutes = normalizeGatewayProtocolConversionRoutes(settings.Gateway.ProtocolConversionRoutes)
	settings.Gateway.RetryCount = normalizeGatewayRetryCount(settings.Gateway.RetryCount)
	settings.Gateway.MaxRetryCredentials = normalizeGatewayMaxRetryCredentials(settings.Gateway.MaxRetryCredentials)
	settings.Gateway.MaxRetryIntervalMS = normalizeGatewayMaxRetryIntervalMS(settings.Gateway.MaxRetryIntervalMS)
	settings.Gateway.PassthroughHeaderAllowlist = normalizePassthroughHeaderAllowlist(settings.Gateway.PassthroughHeaderAllowlist)
	if !validGeneralSettings(settings.General) || !validDecimal(settings.Users.DefaultBalance) || settings.Users.RPMLimitDefault < 0 || settings.Gateway.StreamTimeoutSeconds <= 0 || settings.Backup.RetentionDays <= 0 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Gateway.SchedulerStrategyRolloutPercent < 0 || settings.Gateway.SchedulerStrategyRolloutPercent > 100 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Gateway.SchedulerStrategyRolloutEnabled && (settings.Gateway.SchedulerStrategyShadowStrategy == "" || settings.Gateway.SchedulerStrategyRolloutPercent <= 0) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.SMTPPort == 0 {
		settings.Email.SMTPPort = 587
	}
	if settings.Email.SMTPPort < 0 || settings.Email.SMTPPort > 65535 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.PublicBaseURL != "" && !validPublicHTTPBaseURL(settings.Email.PublicBaseURL) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.BalanceLowNotifyThreshold == "" {
		settings.Email.BalanceLowNotifyThreshold = "5.00000000"
	}
	if !validPositiveDecimal(settings.Email.BalanceLowNotifyThreshold) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.BalanceLowNotifyRechargeURL != "" && !validPublicHTTPBaseURL(settings.Email.BalanceLowNotifyRechargeURL) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.AccountQuotaNotifyRemainingRatio == "" {
		settings.Email.AccountQuotaNotifyRemainingRatio = "0.20000000"
	}
	if !validPercentDecimal(settings.Email.AccountQuotaNotifyRemainingRatio) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	settings.Email.SMTPConfigured = settings.Email.SMTPHost != "" && settings.Email.SMTPFrom != ""
	if settings.General.CustomMenus == nil {
		settings.General.CustomMenus = []admincontrol.CustomMenuItem{}
	}
	if settings.Features.EnabledChannels == nil {
		settings.Features.EnabledChannels = []string{}
	}
	if settings.Security.OAuthProviders == nil {
		settings.Security.OAuthProviders = []string{}
	}
	if settings.Security.OAuthProviderConfigs == nil {
		settings.Security.OAuthProviderConfigs = []admincontrol.OAuthProviderConfig{}
	}
	if settings.Security.RegistrationEmailSuffixAllowlist == nil {
		settings.Security.RegistrationEmailSuffixAllowlist = []string{}
	}
	if settings.Payment.Providers == nil {
		settings.Payment.Providers = []string{}
	}
	if settings.Email.Templates == nil {
		settings.Email.Templates = map[string]string{}
	}
	if settings.Email.BalanceLowNotifyEnabled == nil {
		enabled := true
		settings.Email.BalanceLowNotifyEnabled = &enabled
	}
	if settings.Email.SubscriptionExpiryNotifyEnabled == nil {
		enabled := true
		settings.Email.SubscriptionExpiryNotifyEnabled = &enabled
	}
	if settings.Email.AccountQuotaNotifyEnabled == nil {
		enabled := true
		settings.Email.AccountQuotaNotifyEnabled = &enabled
	}
	if settings.Gateway.SchedulerStrategyRolloutModels == nil {
		settings.Gateway.SchedulerStrategyRolloutModels = []string{}
	}
	if settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes == nil {
		settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = []string{}
	}
	if settings.Gateway.ProtocolConversionRoutes == nil {
		settings.Gateway.ProtocolConversionRoutes = cloneStringSlice(defaultGatewayProtocolConversionRoutes)
	}
	settings.Copilot.Source = strings.TrimSpace(strings.ToLower(settings.Copilot.Source))
	if settings.Copilot.Source != "dedicated" {
		settings.Copilot.Source = "account"
	}
	settings.Copilot.Model = strings.TrimSpace(settings.Copilot.Model)
	settings.Copilot.Models = uniqueTrimmedStrings(settings.Copilot.Models)
	if settings.Copilot.Models == nil {
		settings.Copilot.Models = []string{}
	}
	settings.Copilot.DedicatedProtocol = strings.TrimSpace(strings.ToLower(settings.Copilot.DedicatedProtocol))
	if settings.Copilot.DedicatedProtocol == "" {
		settings.Copilot.DedicatedProtocol = "openai-compatible"
	}
	settings.Copilot.DedicatedBaseURL = strings.TrimRight(strings.TrimSpace(settings.Copilot.DedicatedBaseURL), "/")
	if settings.Copilot.ProviderAccountID < 0 {
		settings.Copilot.ProviderAccountID = 0
	}
	if settings.Copilot.ProviderAccountGroupID < 0 {
		settings.Copilot.ProviderAccountGroupID = 0
	}
	settings.Maintenance = normalizeMaintenance(settings.Maintenance)
	return settings, nil
}

func validGeneralSettings(settings admincontrol.AdminSettingsGeneral) bool {
	if settings.SiteName == "" || len(settings.SiteName) > 80 {
		return false
	}
	if len(settings.SiteSubtitle) > 160 || len(settings.VersionLabel) > 80 || len(settings.ContactInfo) > 240 {
		return false
	}
	if settings.LogoURL != "" && !validPublicHTTPBaseURL(settings.LogoURL) {
		return false
	}
	return settings.DocURL == "" || validPublicHTTPBaseURL(settings.DocURL)
}

// gatewayRetryCountDefault / gatewayRetryCountMax bound the operator-tunable
// cross-candidate failover cap. They mirror the OpenAPI schema bounds for
// AdminSettingsGateway.retry_count and keep the failover hot path within a sane
// envelope even if persisted settings predate the field.
const (
	gatewayRetryCountDefault       = 20
	gatewayRetryCountMax           = 20
	gatewayMaxRetryIntervalDefault = 2000
)

func normalizeGatewayRetryCount(count int) int {
	if count <= 0 {
		return gatewayRetryCountDefault
	}
	if count > gatewayRetryCountMax {
		return gatewayRetryCountMax
	}
	return count
}

func normalizeGatewayMaxRetryCredentials(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeGatewayMaxRetryIntervalMS(value int) int {
	if value < 0 {
		return gatewayMaxRetryIntervalDefault
	}
	return value
}

func normalizeGatewayProtocolConversionRoutes(values []string) []string {
	if values == nil {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, route := range defaultGatewayProtocolConversionRoutes {
		allowed[route] = struct{}{}
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := allowed[normalized]; !ok {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeOAuthProviderConfigs(values []admincontrol.OAuthProviderConfig) ([]admincontrol.OAuthProviderConfig, error) {
	if len(values) == 0 {
		return []admincontrol.OAuthProviderConfig{}, nil
	}
	out := make([]admincontrol.OAuthProviderConfig, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		provider := normalizeOAuthProvider(value.Provider)
		providerKey := strings.TrimSpace(value.ProviderKey)
		if providerKey == "" {
			providerKey = provider
		}
		displayName := strings.TrimSpace(value.DisplayName)
		if displayName == "" {
			displayName = providerKey
		}
		clientID := strings.TrimSpace(value.ClientID)
		authorizeURL := strings.TrimSpace(value.AuthorizeURL)
		tokenURL := strings.TrimSpace(value.TokenURL)
		userInfoURL := strings.TrimSpace(value.UserInfoURL)
		tokenAuthMethod := normalizeOAuthTokenAuthMethod(value.TokenAuthMethod)
		redirectURI := strings.TrimSpace(value.RedirectURI)
		if provider == "" || providerKey == "" || clientID == "" || !validOAuthAuthorizeURL(authorizeURL) || !validOAuthRedirectURI(redirectURI) {
			return nil, admincontrol.ErrInvalidInput
		}
		if tokenAuthMethod == "" || (tokenURL == "") != (userInfoURL == "") {
			return nil, admincontrol.ErrInvalidInput
		}
		if tokenURL != "" && (!validOAuthBackchannelURL(tokenURL) || !validOAuthBackchannelURL(userInfoURL)) {
			return nil, admincontrol.ErrInvalidInput
		}
		key := strings.ToLower(provider + "\x00" + providerKey)
		if _, ok := seen[key]; ok {
			return nil, admincontrol.ErrConflict
		}
		seen[key] = struct{}{}
		out = append(out, admincontrol.OAuthProviderConfig{
			Provider:        provider,
			ProviderKey:     providerKey,
			DisplayName:     displayName,
			ClientID:        clientID,
			AuthorizeURL:    authorizeURL,
			TokenURL:        tokenURL,
			UserInfoURL:     userInfoURL,
			TokenAuthMethod: tokenAuthMethod,
			RedirectURI:     redirectURI,
			Scopes:          normalizeOAuthScopes(value.Scopes),
		})
	}
	return out, nil
}

func normalizeCustomMenus(values []admincontrol.CustomMenuItem) ([]admincontrol.CustomMenuItem, error) {
	if len(values) == 0 {
		return []admincontrol.CustomMenuItem{}, nil
	}
	out := make([]admincontrol.CustomMenuItem, 0, len(values))
	for _, value := range values {
		label := strings.TrimSpace(value.Label)
		menuURL := strings.TrimSpace(value.URL)
		if label == "" && menuURL == "" {
			continue
		}
		if label == "" || !validCustomMenuURL(menuURL) {
			return nil, admincontrol.ErrInvalidInput
		}
		visibility := strings.ToLower(strings.TrimSpace(value.Visibility))
		switch visibility {
		case "":
			visibility = customMenuVisibilityUser
		case customMenuVisibilityUser, customMenuVisibilityAdmin:
		default:
			return nil, admincontrol.ErrInvalidInput
		}
		sortOrder := value.SortOrder
		if sortOrder < 0 {
			sortOrder = 0
		}
		id := normalizeCustomMenuID(value.ID)
		if id == "" {
			id = normalizeCustomMenuID(label)
		}
		if id == "" {
			return nil, admincontrol.ErrInvalidInput
		}
		out = append(out, admincontrol.CustomMenuItem{
			ID:         id,
			Label:      label,
			URL:        menuURL,
			Visibility: visibility,
			SortOrder:  sortOrder,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].ID < out[j].ID
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	for idx := range out {
		out[idx].SortOrder = idx
	}
	return out, nil
}

func normalizeCustomMenuID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 80 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func validCustomMenuURL(value string) bool {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return false
	}
	if strings.HasPrefix(value, "/") {
		return !strings.HasPrefix(value, "//")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func normalizeOAuthScopes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, scope := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			key := strings.ToLower(scope)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, scope)
		}
	}
	return out
}

func normalizeOAuthProvider(provider string) string {
	switch userscontract.AuthIdentityProvider(strings.ToLower(strings.TrimSpace(provider))) {
	case userscontract.AuthIdentityProviderOIDC:
		return string(userscontract.AuthIdentityProviderOIDC)
	case userscontract.AuthIdentityProviderGitHub:
		return string(userscontract.AuthIdentityProviderGitHub)
	case userscontract.AuthIdentityProviderGoogle:
		return string(userscontract.AuthIdentityProviderGoogle)
	case userscontract.AuthIdentityProviderLinuxDo:
		return string(userscontract.AuthIdentityProviderLinuxDo)
	case userscontract.AuthIdentityProviderWeChat:
		return string(userscontract.AuthIdentityProviderWeChat)
	case userscontract.AuthIdentityProviderDingTalk:
		return string(userscontract.AuthIdentityProviderDingTalk)
	default:
		return ""
	}
}

func normalizeOAuthTokenAuthMethod(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", oauthTokenAuthMethodNone:
		return oauthTokenAuthMethodNone
	case oauthTokenAuthMethodClientSecretPost, oauthTokenAuthMethodClientSecretBasic:
		return value
	default:
		return ""
	}
}

func validOAuthAuthorizeURL(value string) bool {
	parsed, ok := parseOAuthURL(value)
	return ok && parsed.Scheme == "https"
}

func validOAuthBackchannelURL(value string) bool {
	parsed, ok := parseOAuthURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localOAuthHost(parsed.Hostname())
}

func validOAuthRedirectURI(value string) bool {
	parsed, ok := parseOAuthURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localOAuthHost(parsed.Hostname())
}

func parseOAuthURL(value string) (*url.URL, bool) {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return nil, false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, false
	}
	return parsed, true
}

func localOAuthHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func normalizeRegistrationEmailSuffixAllowlist(values []string) ([]string, error) {
	if len(values) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		suffix, err := normalizeRegistrationEmailSuffix(value)
		if err != nil {
			return nil, err
		}
		if suffix == "" {
			continue
		}
		if _, ok := seen[suffix]; ok {
			continue
		}
		seen[suffix] = struct{}{}
		out = append(out, suffix)
	}
	return out, nil
}

func normalizeRegistrationEmailSuffix(value string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", nil
	}
	domain := trimmed
	if strings.Contains(trimmed, "@") {
		if !strings.HasPrefix(trimmed, "@") || strings.Count(trimmed, "@") != 1 {
			return "", admincontrol.ErrInvalidInput
		}
		domain = strings.TrimPrefix(trimmed, "@")
	}
	if domain == "" || strings.Contains(domain, "@") || !emailSuffixDomainPattern.MatchString(domain) {
		return "", admincontrol.ErrInvalidInput
	}
	return "@" + domain, nil
}

func validPublicHTTPBaseURL(value string) bool {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") || strings.Contains(value, "?") || strings.Contains(value, "#") {
		return false
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}

// normalizePassthroughHeaderAllowlist canonicalizes upstream response header
// allowlist entries to lowercase (HTTP header names are case-insensitive),
// trims whitespace, drops blanks, and dedupes. It always returns a non-nil
// slice so the persisted settings round-trip cleanly. A trailing "*" wildcard
// is preserved for prefix matching (e.g. "x-ratelimit-*").
func normalizePassthroughHeaderAllowlist(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" || trimmed == "*" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func cloneAdminSettings(settings admincontrol.AdminSettings) admincontrol.AdminSettings {
	settings.General.CustomMenus = cloneCustomMenus(settings.General.CustomMenus)
	settings.Features.EnabledChannels = cloneStringSlice(settings.Features.EnabledChannels)
	settings.Security.RegistrationEmailSuffixAllowlist = cloneStringSlice(settings.Security.RegistrationEmailSuffixAllowlist)
	settings.Security.OAuthProviders = cloneStringSlice(settings.Security.OAuthProviders)
	settings.Security.OAuthProviderConfigs = cloneOAuthProviderConfigs(settings.Security.OAuthProviderConfigs)
	settings.Gateway.ProtocolConversionRoutes = cloneStringSlice(settings.Gateway.ProtocolConversionRoutes)
	settings.Gateway.SchedulerStrategyRolloutModels = cloneStringSlice(settings.Gateway.SchedulerStrategyRolloutModels)
	settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = cloneStringSlice(settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes)
	settings.Gateway.PassthroughHeaderAllowlist = cloneStringSlice(settings.Gateway.PassthroughHeaderAllowlist)
	settings.Payment.Providers = cloneStringSlice(settings.Payment.Providers)
	settings.Email.Templates = cloneStringMap(settings.Email.Templates)
	settings.Email.BalanceLowNotifyEnabled = cloneBoolPtr(settings.Email.BalanceLowNotifyEnabled)
	settings.Email.SubscriptionExpiryNotifyEnabled = cloneBoolPtr(settings.Email.SubscriptionExpiryNotifyEnabled)
	settings.Email.AccountQuotaNotifyEnabled = cloneBoolPtr(settings.Email.AccountQuotaNotifyEnabled)
	settings.Backup.LastBackupAt = cloneTimePtr(settings.Backup.LastBackupAt)
	settings.Copilot.Models = cloneStringSlice(settings.Copilot.Models)
	settings.Maintenance.ExpectedRecoveryAt = cloneTimePtr(settings.Maintenance.ExpectedRecoveryAt)
	return settings
}

func cloneCustomMenus(values []admincontrol.CustomMenuItem) []admincontrol.CustomMenuItem {
	if values == nil {
		return nil
	}
	out := make([]admincontrol.CustomMenuItem, len(values))
	copy(out, values)
	return out
}

func cloneOAuthProviderConfigs(values []admincontrol.OAuthProviderConfig) []admincontrol.OAuthProviderConfig {
	if values == nil {
		return nil
	}
	out := make([]admincontrol.OAuthProviderConfig, 0, len(values))
	for _, value := range values {
		value.Scopes = cloneStringSlice(value.Scopes)
		out = append(out, value)
	}
	return out
}

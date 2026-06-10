package contract

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("user not found")
	ErrAlreadyExists = errors.New("user already exists")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusPending  Status = "pending"
)

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleUser     Role = "user"
)

const (
	PermissionDashboardRead             = "dashboard:read"
	PermissionRoleRead                  = "role:read"
	PermissionRoleWrite                 = "role:write"
	PermissionUserRead                  = "user:read"
	PermissionUserWrite                 = "user:write"
	PermissionUserAttributeRead         = "user_attribute:read"
	PermissionUserAttributeWrite        = "user_attribute:write"
	PermissionAPIKeyRead                = "api_key:read"
	PermissionAPIKeyWrite               = "api_key:write"
	PermissionProviderRead              = "provider:read"
	PermissionProviderWrite             = "provider:write"
	PermissionModelRead                 = "model:read"
	PermissionModelWrite                = "model:write"
	PermissionAccountRead               = "account:read"
	PermissionAccountWrite              = "account:write"
	PermissionAccountGroupRead          = "account_group:read"
	PermissionAccountGroupWrite         = "account_group:write"
	PermissionProxyRead                 = "proxy:read"
	PermissionProxyWrite                = "proxy:write"
	PermissionUsageRead                 = "usage:read"
	PermissionUsageWrite                = "usage:write"
	PermissionAuditLogRead              = "audit_log:read"
	PermissionBillingLedgerRead         = "billing_ledger:read"
	PermissionAffiliateRead             = "affiliate:read"
	PermissionAffiliateWrite            = "affiliate:write"
	PermissionPaymentRead               = "payment:read"
	PermissionPaymentWrite              = "payment:write"
	PermissionPaymentProviderRead       = "payment_provider:read"
	PermissionPaymentProviderWrite      = "payment_provider:write"
	PermissionSubscriptionRead          = "subscription:read"
	PermissionSubscriptionWrite         = "subscription:write"
	PermissionPricingRuleRead           = "pricing_rule:read"
	PermissionPricingRuleWrite          = "pricing_rule:write"
	PermissionSettingsRead              = "settings:read"
	PermissionSettingsWrite             = "settings:write"
	PermissionNotificationTemplateRead  = "notification_template:read"
	PermissionNotificationTemplateWrite = "notification_template:write"
	PermissionRiskControlRead           = "risk_control:read"
	PermissionRiskControlWrite          = "risk_control:write"
	PermissionCapabilityRead            = "capability:read"
	PermissionSchedulerRead             = "scheduler:read"
	PermissionSchedulerWrite            = "scheduler:write"
	PermissionOpsRead                   = "ops:read"
	PermissionOpsWrite                  = "ops:write"
	PermissionAlertRuleRead             = "alert_rule:read"
	PermissionAlertRuleWrite            = "alert_rule:write"
	PermissionTLSProfileRead            = "tls_profile:read"
	PermissionTLSProfileWrite           = "tls_profile:write"
	PermissionPayloadRuleRead           = "payload_rule:read"
	PermissionPayloadRuleWrite          = "payload_rule:write"
	PermissionErrorPassthroughRead      = "error_passthrough:read"
	PermissionErrorPassthroughWrite     = "error_passthrough:write"
	PermissionModelRateLimitRead        = "model_rate_limit:read"
	PermissionModelRateLimitWrite       = "model_rate_limit:write"
	PermissionGroupRateLimitRead        = "group_rate_limit:read"
	PermissionGroupRateLimitWrite       = "group_rate_limit:write"
	PermissionUserPlatformQuotaRead     = "user_platform_quota:read"
	PermissionUserPlatformQuotaWrite    = "user_platform_quota:write"
	PermissionChannelMonitorRead        = "channel_monitor:read"
	PermissionChannelMonitorWrite       = "channel_monitor:write"
	PermissionScheduledTestRead         = "scheduled_test:read"
	PermissionScheduledTestWrite        = "scheduled_test:write"
	PermissionCopilotRead               = "copilot:read"
	PermissionCopilotWrite              = "copilot:write"
	PermissionAnnouncementRead          = "announcement:read"
	PermissionAnnouncementWrite         = "announcement:write"
	PermissionRedeemCodeRead            = "redeem_code:read"
	PermissionRedeemCodeWrite           = "redeem_code:write"
	PermissionPromoCodeRead             = "promo_code:read"
	PermissionPromoCodeWrite            = "promo_code:write"
	PermissionContentSafetyRead         = "content_safety:read"
	PermissionContentSafetyWrite        = "content_safety:write"
)

type BalanceOperation string

const (
	BalanceOperationSet       BalanceOperation = "set"
	BalanceOperationIncrement BalanceOperation = "increment"
	BalanceOperationDecrement BalanceOperation = "decrement"
)

type BalanceUpdateRequest struct {
	Operation BalanceOperation
	Amount    string
	Currency  string
}

type AuthIdentityProvider string

const (
	AuthIdentityProviderEmail    AuthIdentityProvider = "email"
	AuthIdentityProviderOIDC     AuthIdentityProvider = "oidc"
	AuthIdentityProviderGitHub   AuthIdentityProvider = "github"
	AuthIdentityProviderGoogle   AuthIdentityProvider = "google"
	AuthIdentityProviderLinuxDo  AuthIdentityProvider = "linuxdo"
	AuthIdentityProviderWeChat   AuthIdentityProvider = "wechat"
	AuthIdentityProviderDingTalk AuthIdentityProvider = "dingtalk"
)

// UserAuthIdentity describes a console sign-in identity visible to a user.
type UserAuthIdentity struct {
	ID              int
	UserID          int
	Provider        AuthIdentityProvider
	ProviderKey     string
	SubjectHint     string
	DisplayName     string
	Email           string
	EmailVerified   bool
	AvatarURL       string
	External        bool
	VerifiedAt      *time.Time
	LastUsedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CanUnbind       bool
	UnbindBlockedBy string
}

// CreateUserAuthIdentity is the persistence payload for a verified external sign-in identity.
type CreateUserAuthIdentity struct {
	UserID              int
	Provider            AuthIdentityProvider
	ProviderKey         string
	ProviderSubjectHash string
	SubjectHint         string
	DisplayName         string
	Email               string
	EmailVerified       bool
	AvatarURL           string
	VerifiedAt          *time.Time
	LastUsedAt          *time.Time
}

type User struct {
	ID              int
	Email           string
	Name            string
	Status          Status
	WorkspaceID     *int
	Roles           []Role
	Permissions     []string
	Balance         string
	Currency        string
	RPMLimit        *int
	CreatedAt       time.Time
	LastLoginAt     *time.Time
	EmailVerifiedAt *time.Time
	AvatarURL       string
	AvatarMIME      string
	AvatarByteSize  int
	AvatarSHA256    string
	AvatarUpdatedAt *time.Time
}

// RoleDefinition is a persisted role catalog entry used to resolve user permissions.
type RoleDefinition struct {
	ID          int
	Name        Role
	Description string
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PermissionDefinition struct {
	Permission  string
	Resource    string
	Action      string
	Description string
}

// CreateStoredRole is the store-level payload for creating a role definition.
type CreateStoredRole struct {
	Name        Role
	Description string
	Permissions []string
}

// UpdateStoredRole is the store-level payload for updating a role definition.
// The role name (identity) is immutable; only description and permissions change.
type UpdateStoredRole struct {
	Description *string
	Permissions *[]string
}

type StoredUser struct {
	User
	PasswordHash    string
	EmailVerifiedAt *time.Time
}

type CreateStoredUser struct {
	Email           string
	Name            string
	PasswordHash    string
	Status          Status
	WorkspaceID     *int
	Roles           []Role
	EmailVerifiedAt *time.Time
	Balance         string
	Currency        string
	RPMLimit        *int
}

type UpdateStoredUser struct {
	Email           *string
	Name            *string
	PasswordHash    *string
	Status          *Status
	WorkspaceID     **int
	Roles           *[]Role
	Balance         *string
	Currency        *string
	RPMLimit        **int
	EmailVerifiedAt **time.Time
}

type ListUsersFilter struct {
	Status *Status
	Query  string
}

type Store interface {
	Create(ctx context.Context, input CreateStoredUser) (StoredUser, error)
	FindByID(ctx context.Context, id int) (StoredUser, error)
	FindByEmail(ctx context.Context, email string) (StoredUser, error)
	List(ctx context.Context, filter ListUsersFilter) ([]StoredUser, error)
	ListByIDs(ctx context.Context, ids []int) ([]StoredUser, error)
	Update(ctx context.Context, id int, input UpdateStoredUser) (StoredUser, error)
	Delete(ctx context.Context, id int) error
	UpdateLastLogin(ctx context.Context, id int, at time.Time) error
	CreateRole(ctx context.Context, input CreateStoredRole) (RoleDefinition, error)
	ListRoles(ctx context.Context) ([]RoleDefinition, error)
	UpdateRole(ctx context.Context, id int, input UpdateStoredRole) (RoleDefinition, error)
	DeleteRole(ctx context.Context, id int) error
	ListAuthIdentities(ctx context.Context, userID int) ([]UserAuthIdentity, error)
	FindAuthIdentityByProviderSubject(ctx context.Context, provider AuthIdentityProvider, providerKey string, providerSubjectHash string) (UserAuthIdentity, error)
	UpsertAuthIdentity(ctx context.Context, input CreateUserAuthIdentity) (UserAuthIdentity, error)
	DeleteAuthIdentity(ctx context.Context, userID int, identityID int) error
}

// IsBuiltInRole reports whether a role is one of SRapi's bootstrap roles.
func IsBuiltInRole(role Role) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleOperator, RoleUser:
		return true
	default:
		return false
	}
}

func PermissionCatalog() []PermissionDefinition {
	out := make([]PermissionDefinition, 0, len(permissionCatalog))
	for _, item := range permissionCatalog {
		out = append(out, item)
	}
	return out
}

func AllPermissions() []string {
	out := make([]string, 0, len(permissionCatalog))
	for _, item := range permissionCatalog {
		out = append(out, item.Permission)
	}
	return out
}

func ReadOnlyPermissions() []string {
	out := make([]string, 0, len(permissionCatalog))
	for _, item := range permissionCatalog {
		if item.Action == "read" {
			out = append(out, item.Permission)
		}
	}
	return out
}

func BuiltInRoleDefinition(role Role) RoleDefinition {
	def := RoleDefinition{Name: role}
	switch role {
	case RoleOwner:
		def.Description = "Full owner access."
		def.Permissions = AllPermissions()
	case RoleAdmin:
		def.Description = "Administrative access across the SRapi console."
		def.Permissions = AllPermissions()
	case RoleOperator:
		def.Description = "Operational read access plus limited maintenance actions."
		def.Permissions = append(ReadOnlyPermissions(), PermissionOpsWrite, PermissionAccountWrite, PermissionRiskControlWrite)
	case RoleUser:
		def.Description = "Workspace user access without admin console permissions."
	default:
		def.Description = string(role)
	}
	return def
}

func IsKnownPermission(permission string) bool {
	for _, item := range permissionCatalog {
		if item.Permission == permission {
			return true
		}
	}
	return false
}

var permissionCatalog = []PermissionDefinition{
	{Permission: PermissionDashboardRead, Resource: "dashboard", Action: "read", Description: "View admin dashboards and overview metrics."},
	{Permission: PermissionRoleRead, Resource: "role", Action: "read", Description: "View role definitions and permission grants."},
	{Permission: PermissionRoleWrite, Resource: "role", Action: "write", Description: "Create, update, and delete custom roles."},
	{Permission: PermissionUserRead, Resource: "user", Action: "read", Description: "View users and user details."},
	{Permission: PermissionUserWrite, Resource: "user", Action: "write", Description: "Create and update users."},
	{Permission: PermissionUserAttributeRead, Resource: "user_attribute", Action: "read", Description: "View user attribute definitions and values."},
	{Permission: PermissionUserAttributeWrite, Resource: "user_attribute", Action: "write", Description: "Manage user attribute definitions and values."},
	{Permission: PermissionAPIKeyRead, Resource: "api_key", Action: "read", Description: "View API keys and key usage."},
	{Permission: PermissionAPIKeyWrite, Resource: "api_key", Action: "write", Description: "Update admin-managed API keys."},
	{Permission: PermissionProviderRead, Resource: "provider", Action: "read", Description: "View providers and provider metadata."},
	{Permission: PermissionProviderWrite, Resource: "provider", Action: "write", Description: "Create and update providers."},
	{Permission: PermissionModelRead, Resource: "model", Action: "read", Description: "View models, mappings, aliases, and limits."},
	{Permission: PermissionModelWrite, Resource: "model", Action: "write", Description: "Create and update models, mappings, and aliases."},
	{Permission: PermissionAccountRead, Resource: "account", Action: "read", Description: "View provider accounts, health, quotas, and RPM state."},
	{Permission: PermissionAccountWrite, Resource: "account", Action: "write", Description: "Create, update, test, import, and recover provider accounts."},
	{Permission: PermissionAccountGroupRead, Resource: "account_group", Action: "read", Description: "View account groups and group members."},
	{Permission: PermissionAccountGroupWrite, Resource: "account_group", Action: "write", Description: "Manage account groups and group members."},
	{Permission: PermissionProxyRead, Resource: "proxy", Action: "read", Description: "View outbound proxy definitions."},
	{Permission: PermissionProxyWrite, Resource: "proxy", Action: "write", Description: "Create, update, and delete outbound proxies."},
	{Permission: PermissionUsageRead, Resource: "usage", Action: "read", Description: "View usage logs and usage aggregates."},
	{Permission: PermissionUsageWrite, Resource: "usage", Action: "write", Description: "Run usage cleanup actions."},
	{Permission: PermissionAuditLogRead, Resource: "audit_log", Action: "read", Description: "View admin audit logs."},
	{Permission: PermissionBillingLedgerRead, Resource: "billing_ledger", Action: "read", Description: "View billing ledger entries."},
	{Permission: PermissionAffiliateRead, Resource: "affiliate", Action: "read", Description: "View affiliate relationships and ledgers."},
	{Permission: PermissionAffiliateWrite, Resource: "affiliate", Action: "write", Description: "Manage affiliate rules, adjustments, and withdrawals."},
	{Permission: PermissionPaymentRead, Resource: "payment", Action: "read", Description: "View payment orders."},
	{Permission: PermissionPaymentWrite, Resource: "payment", Action: "write", Description: "Refund payment orders."},
	{Permission: PermissionPaymentProviderRead, Resource: "payment_provider", Action: "read", Description: "View payment provider instances."},
	{Permission: PermissionPaymentProviderWrite, Resource: "payment_provider", Action: "write", Description: "Manage payment provider instances."},
	{Permission: PermissionSubscriptionRead, Resource: "subscription", Action: "read", Description: "View subscription plans and user subscriptions."},
	{Permission: PermissionSubscriptionWrite, Resource: "subscription", Action: "write", Description: "Manage subscription plans and user subscriptions."},
	{Permission: PermissionPricingRuleRead, Resource: "pricing_rule", Action: "read", Description: "View pricing rules."},
	{Permission: PermissionPricingRuleWrite, Resource: "pricing_rule", Action: "write", Description: "Create, update, import, and delete pricing rules."},
	{Permission: PermissionSettingsRead, Resource: "settings", Action: "read", Description: "View admin settings."},
	{Permission: PermissionSettingsWrite, Resource: "settings", Action: "write", Description: "Update admin settings and send test email."},
	{Permission: PermissionNotificationTemplateRead, Resource: "notification_template", Action: "read", Description: "View notification templates."},
	{Permission: PermissionNotificationTemplateWrite, Resource: "notification_template", Action: "write", Description: "Update, preview, and restore notification templates."},
	{Permission: PermissionRiskControlRead, Resource: "risk_control", Action: "read", Description: "View risk-control config, status, and logs."},
	{Permission: PermissionRiskControlWrite, Resource: "risk_control", Action: "write", Description: "Update risk-control config."},
	{Permission: PermissionCapabilityRead, Resource: "capability", Action: "read", Description: "View capability definitions."},
	{Permission: PermissionSchedulerRead, Resource: "scheduler", Action: "read", Description: "View scheduler overview, strategies, and decisions."},
	{Permission: PermissionSchedulerWrite, Resource: "scheduler", Action: "write", Description: "Simulate and replay scheduler strategies."},
	{Permission: PermissionOpsRead, Resource: "ops", Action: "read", Description: "View operational telemetry, alerts, SLOs, and system logs."},
	{Permission: PermissionOpsWrite, Resource: "ops", Action: "write", Description: "Update operations settings, alerts, SLOs, and cleanup logs."},
	{Permission: PermissionAlertRuleRead, Resource: "alert_rule", Action: "read", Description: "View alert rules and silences."},
	{Permission: PermissionAlertRuleWrite, Resource: "alert_rule", Action: "write", Description: "Manage alert rules and silences."},
	{Permission: PermissionTLSProfileRead, Resource: "tls_profile", Action: "read", Description: "View TLS fingerprint profiles."},
	{Permission: PermissionTLSProfileWrite, Resource: "tls_profile", Action: "write", Description: "Manage TLS fingerprint profiles."},
	{Permission: PermissionPayloadRuleRead, Resource: "payload_rule", Action: "read", Description: "View payload transformation rules."},
	{Permission: PermissionPayloadRuleWrite, Resource: "payload_rule", Action: "write", Description: "Manage payload transformation rules."},
	{Permission: PermissionErrorPassthroughRead, Resource: "error_passthrough", Action: "read", Description: "View error passthrough rules."},
	{Permission: PermissionErrorPassthroughWrite, Resource: "error_passthrough", Action: "write", Description: "Manage error passthrough rules."},
	{Permission: PermissionModelRateLimitRead, Resource: "model_rate_limit", Action: "read", Description: "View model rate limits."},
	{Permission: PermissionModelRateLimitWrite, Resource: "model_rate_limit", Action: "write", Description: "Manage model rate limits."},
	{Permission: PermissionGroupRateLimitRead, Resource: "group_rate_limit", Action: "read", Description: "View group rate limits."},
	{Permission: PermissionGroupRateLimitWrite, Resource: "group_rate_limit", Action: "write", Description: "Manage group rate limits."},
	{Permission: PermissionUserPlatformQuotaRead, Resource: "user_platform_quota", Action: "read", Description: "View user platform quotas."},
	{Permission: PermissionUserPlatformQuotaWrite, Resource: "user_platform_quota", Action: "write", Description: "Manage user platform quotas."},
	{Permission: PermissionChannelMonitorRead, Resource: "channel_monitor", Action: "read", Description: "View channel monitors and runs."},
	{Permission: PermissionChannelMonitorWrite, Resource: "channel_monitor", Action: "write", Description: "Manage and run channel monitors."},
	{Permission: PermissionScheduledTestRead, Resource: "scheduled_test", Action: "read", Description: "View scheduled tests and runs."},
	{Permission: PermissionScheduledTestWrite, Resource: "scheduled_test", Action: "write", Description: "Manage and run scheduled tests."},
	{Permission: PermissionCopilotRead, Resource: "copilot", Action: "read", Description: "View copilot config and conversations."},
	{Permission: PermissionCopilotWrite, Resource: "copilot", Action: "write", Description: "Use and manage admin copilot conversations."},
	{Permission: PermissionAnnouncementRead, Resource: "announcement", Action: "read", Description: "View announcements and read status."},
	{Permission: PermissionAnnouncementWrite, Resource: "announcement", Action: "write", Description: "Manage announcements."},
	{Permission: PermissionRedeemCodeRead, Resource: "redeem_code", Action: "read", Description: "View redeem codes and stats."},
	{Permission: PermissionRedeemCodeWrite, Resource: "redeem_code", Action: "write", Description: "Create, batch, disable, and delete redeem codes."},
	{Permission: PermissionPromoCodeRead, Resource: "promo_code", Action: "read", Description: "View promo codes and usages."},
	{Permission: PermissionPromoCodeWrite, Resource: "promo_code", Action: "write", Description: "Manage promo codes."},
	{Permission: PermissionContentSafetyRead, Resource: "content_safety", Action: "read", Description: "View content-safety settings and audit findings."},
	{Permission: PermissionContentSafetyWrite, Resource: "content_safety", Action: "write", Description: "Update content-safety settings."},
}

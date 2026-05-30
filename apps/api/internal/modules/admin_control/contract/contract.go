package contract

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid admin control input")
	ErrNotFound     = errors.New("admin control resource not found")
	ErrConflict     = errors.New("admin control resource conflict")
)

type Store interface {
	Get(ctx context.Context, key string) (map[string]any, bool, error)
	Set(ctx context.Context, key string, value map[string]any, updatedBy *int) error
	ListAnnouncementReads(ctx context.Context, userID int, announcementIDs []int) ([]AnnouncementRead, error)
	MarkAnnouncementRead(ctx context.Context, userID int, announcementID int, at time.Time) (AnnouncementRead, error)
	RedeemCode(ctx context.Context, input RedeemCodeRedemptionInput) (RedeemCodeRedemptionResult, error)
	PreviewPromoCode(ctx context.Context, input PromoCodePreviewInput) (PromoCodeApplication, error)
	FinalizePromoCode(ctx context.Context, input PromoCodeFinalizeInput) (PromoCodeApplication, error)
}

type ListOptions struct {
	Page     int
	PageSize int
	Status   string
	Level    string
}

type SecretConfigured struct {
	Configured bool `json:"configured"`
}
type AdminSettings struct {
	General   AdminSettingsGeneral   `json:"general"`
	Agreement AdminSettingsAgreement `json:"agreement"`
	Features  AdminSettingsFeatures  `json:"features"`
	Security  AdminSettingsSecurity  `json:"security"`
	Users     AdminSettingsUsers     `json:"users"`
	Gateway   AdminSettingsGateway   `json:"gateway"`
	Payment   AdminSettingsPayment   `json:"payment"`
	Email     AdminSettingsEmail     `json:"email"`
	Backup    AdminSettingsBackup    `json:"backup"`
}
type AdminSettingsGeneral struct {
	SiteName     string           `json:"site_name"`
	LogoURL      string           `json:"logo_url"`
	VersionLabel string           `json:"version_label"`
	CustomMenus  []map[string]any `json:"custom_menus"`
}
type AdminSettingsAgreement struct {
	UserAgreement string `json:"user_agreement"`
	PrivacyPolicy string `json:"privacy_policy"`
}
type AdminSettingsFeatures struct {
	EnabledChannels          []string `json:"enabled_channels"`
	ChannelMonitoringEnabled bool     `json:"channel_monitoring_enabled"`
	InvitationRebateEnabled  bool     `json:"invitation_rebate_enabled"`
	PaymentsEnabled          bool     `json:"payments_enabled"`
}
type AdminSettingsSecurity struct {
	AdminAPIKey                      SecretConfigured      `json:"admin_api_key"`
	RegistrationEnabled              bool                  `json:"registration_enabled"`
	RegistrationEmailSuffixAllowlist []string              `json:"registration_email_suffix_allowlist"`
	OAuthEnabled                     bool                  `json:"oauth_enabled"`
	OAuthProviders                   []string              `json:"oauth_providers"`
	OAuthProviderConfigs             []OAuthProviderConfig `json:"oauth_provider_configs"`
}

type OAuthProviderConfig struct {
	Provider        string   `json:"provider"`
	ProviderKey     string   `json:"provider_key"`
	DisplayName     string   `json:"display_name"`
	ClientID        string   `json:"client_id"`
	AuthorizeURL    string   `json:"authorize_url"`
	TokenURL        string   `json:"token_url"`
	UserInfoURL     string   `json:"userinfo_url"`
	TokenAuthMethod string   `json:"token_auth_method"`
	RedirectURI     string   `json:"redirect_uri"`
	Scopes          []string `json:"scopes"`
}
type AdminSettingsUsers struct {
	DefaultBalance        string `json:"default_balance"`
	DefaultGroup          string `json:"default_group"`
	UserSelfDeleteEnabled bool   `json:"user_self_delete_enabled"`
	RPMLimitDefault       int    `json:"rpm_limit_default"`
}
type AdminSettingsGateway struct {
	OverloadCooldownSeconds              int      `json:"overload_cooldown_seconds"`
	RateLimitCooldownSeconds             int      `json:"rate_limit_cooldown_seconds"`
	StreamTimeoutSeconds                 int      `json:"stream_timeout_seconds"`
	RequestShaperEnabled                 bool     `json:"request_shaper_enabled"`
	BetaStrategy                         string   `json:"beta_strategy"`
	SchedulerStrategyRolloutEnabled      bool     `json:"scheduler_strategy_rollout_enabled"`
	SchedulerStrategyShadowStrategy      string   `json:"scheduler_strategy_shadow_strategy"`
	SchedulerStrategyRolloutPercent      float64  `json:"scheduler_strategy_rollout_percent"`
	SchedulerStrategyRolloutModels       []string `json:"scheduler_strategy_rollout_models"`
	SchedulerStrategyRolloutAPIKeyHashes []string `json:"scheduler_strategy_rollout_api_key_hashes"`
}
type AdminSettingsPayment struct {
	Enabled                  bool     `json:"enabled"`
	Providers                []string `json:"providers"`
	SubscriptionPlansEnabled bool     `json:"subscription_plans_enabled"`
}
type AdminSettingsEmail struct {
	SMTPConfigured                   bool              `json:"smtp_configured"`
	SMTPHost                         string            `json:"smtp_host"`
	SMTPPort                         int               `json:"smtp_port"`
	SMTPUsername                     string            `json:"smtp_username"`
	SMTPFrom                         string            `json:"smtp_from"`
	SMTPFromName                     string            `json:"smtp_from_name"`
	SMTPUseTLS                       bool              `json:"smtp_use_tls"`
	PublicBaseURL                    string            `json:"public_base_url"`
	Templates                        map[string]string `json:"templates"`
	BalanceLowNotifyEnabled          *bool             `json:"balance_low_notify_enabled,omitempty"`
	BalanceLowNotifyThreshold        string            `json:"balance_low_notify_threshold,omitempty"`
	BalanceLowNotifyRechargeURL      string            `json:"balance_low_notify_recharge_url,omitempty"`
	SubscriptionExpiryNotifyEnabled  *bool             `json:"subscription_expiry_notify_enabled,omitempty"`
	AccountQuotaNotifyEnabled        *bool             `json:"account_quota_notify_enabled,omitempty"`
	AccountQuotaNotifyRemainingRatio string            `json:"account_quota_notify_remaining_ratio,omitempty"`
}
type AdminSettingsBackup struct {
	Enabled       bool       `json:"enabled"`
	LastBackupAt  *time.Time `json:"last_backup_at,omitempty"`
	RetentionDays int        `json:"retention_days"`
}

type OpsSettings struct {
	AutoRefreshEnabled     bool    `json:"auto_refresh_enabled"`
	RefreshIntervalSeconds int     `json:"refresh_interval_seconds"`
	ErrorRateThreshold     float32 `json:"error_rate_threshold"`
	LatencyP95ThresholdMS  int     `json:"latency_p95_threshold_ms"`
	AlertRetentionDays     int     `json:"alert_retention_days"`
}

type AnnouncementStatus string

const (
	AnnouncementStatusDraft     AnnouncementStatus = "draft"
	AnnouncementStatusPublished AnnouncementStatus = "published"
	AnnouncementStatusArchived  AnnouncementStatus = "archived"
)

func (s AnnouncementStatus) Valid() bool {
	return s == AnnouncementStatusDraft || s == AnnouncementStatusPublished || s == AnnouncementStatusArchived
}

type AnnouncementSeverity string

const (
	AnnouncementSeverityInfo     AnnouncementSeverity = "info"
	AnnouncementSeverityWarning  AnnouncementSeverity = "warning"
	AnnouncementSeverityCritical AnnouncementSeverity = "critical"
)

func (s AnnouncementSeverity) Valid() bool {
	return s == AnnouncementSeverityInfo || s == AnnouncementSeverityWarning || s == AnnouncementSeverityCritical
}

type AnnouncementAudience string

const (
	AnnouncementAudienceAll    AnnouncementAudience = "all"
	AnnouncementAudienceUsers  AnnouncementAudience = "users"
	AnnouncementAudienceAdmins AnnouncementAudience = "admins"
)

func (a AnnouncementAudience) Valid() bool {
	return a == AnnouncementAudienceAll || a == AnnouncementAudienceUsers || a == AnnouncementAudienceAdmins
}

type Announcement struct {
	ID        int                  `json:"id"`
	Title     string               `json:"title"`
	Content   string               `json:"content"`
	Status    AnnouncementStatus   `json:"status"`
	Severity  AnnouncementSeverity `json:"severity"`
	Audience  AnnouncementAudience `json:"audience"`
	StartsAt  *time.Time           `json:"starts_at,omitempty"`
	EndsAt    *time.Time           `json:"ends_at,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}
type AnnouncementRequest struct {
	Title    string
	Content  string
	Status   AnnouncementStatus
	Severity AnnouncementSeverity
	Audience AnnouncementAudience
	StartsAt *time.Time
	EndsAt   *time.Time
}
type AnnouncementList struct {
	Items []Announcement
	Total int
}
type UserAnnouncement struct {
	Announcement
	Read   bool       `json:"read"`
	ReadAt *time.Time `json:"read_at,omitempty"`
}
type UserAnnouncementList struct {
	Items  []UserAnnouncement
	Total  int
	Unread int
}
type AnnouncementRead struct {
	ID             int
	UserID         int
	AnnouncementID int
	ReadAt         time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RedeemCodeStatus string

const (
	RedeemCodeStatusActive   RedeemCodeStatus = "active"
	RedeemCodeStatusRedeemed RedeemCodeStatus = "redeemed"
	RedeemCodeStatusDisabled RedeemCodeStatus = "disabled"
	RedeemCodeStatusExpired  RedeemCodeStatus = "expired"
)

type RedeemCodeType string

const (
	RedeemCodeTypeBalance      RedeemCodeType = "balance"
	RedeemCodeTypeSubscription RedeemCodeType = "subscription"
)

func (t RedeemCodeType) Valid() bool {
	return t == RedeemCodeTypeBalance || t == RedeemCodeTypeSubscription
}

type RedeemCode struct {
	ID             int              `json:"id"`
	Code           string           `json:"code"`
	Type           RedeemCodeType   `json:"type"`
	Status         RedeemCodeStatus `json:"status"`
	Value          string           `json:"value"`
	Currency       string           `json:"currency"`
	MaxRedemptions int              `json:"max_redemptions"`
	RedeemedCount  int              `json:"redeemed_count"`
	ExpiresAt      *time.Time       `json:"expires_at,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}
type RedeemCodeRedemptionRequest struct {
	Code string
}
type RedeemCodeRedemptionInput struct {
	UserID     int
	Code       string
	RedeemedAt time.Time
}
type RedeemCodeRedemption struct {
	ID                 int            `json:"id"`
	UserID             int            `json:"user_id"`
	RedeemCodeID       int            `json:"redeem_code_id"`
	Type               RedeemCodeType `json:"type"`
	Amount             string         `json:"amount"`
	Currency           string         `json:"currency"`
	BalanceBefore      string         `json:"balance_before"`
	BalanceAfter       string         `json:"balance_after"`
	BillingLedgerID    *int           `json:"billing_ledger_id,omitempty"`
	UserSubscriptionID *int           `json:"user_subscription_id,omitempty"`
	RedeemedAt         time.Time      `json:"redeemed_at"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}
type RedeemCodeRedemptionResult struct {
	Redemption      RedeemCodeRedemption `json:"redemption"`
	RedeemCode      RedeemCode           `json:"redeem_code"`
	AlreadyRedeemed bool                 `json:"already_redeemed"`
}
type CreateRedeemCodeRequest struct {
	Code           string
	Type           RedeemCodeType
	Value          string
	Currency       string
	MaxRedemptions int
	ExpiresAt      *time.Time
}
type BatchGenerateRedeemCodesRequest struct {
	Prefix         string
	Count          int
	Type           RedeemCodeType
	Value          string
	Currency       string
	MaxRedemptions int
	ExpiresAt      *time.Time
}
type RedeemCodeList struct {
	Items []RedeemCode
	Total int
}
type RedeemCodeStats struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Redeemed int `json:"redeemed"`
	Disabled int `json:"disabled"`
	Expired  int `json:"expired"`
}
type BatchOperationResult struct {
	Requested int   `json:"requested"`
	Succeeded int   `json:"succeeded"`
	Failed    int   `json:"failed"`
	FailedIDs []int `json:"failed_ids"`
}

type PromoCodeStatus string

const (
	PromoCodeStatusActive   PromoCodeStatus = "active"
	PromoCodeStatusDisabled PromoCodeStatus = "disabled"
	PromoCodeStatusExpired  PromoCodeStatus = "expired"
)

func (s PromoCodeStatus) Valid() bool {
	return s == PromoCodeStatusActive || s == PromoCodeStatusDisabled || s == PromoCodeStatusExpired
}

type PromoDiscountType string

const (
	PromoDiscountTypeAmount  PromoDiscountType = "amount"
	PromoDiscountTypePercent PromoDiscountType = "percent"
)

func (t PromoDiscountType) Valid() bool {
	return t == PromoDiscountTypeAmount || t == PromoDiscountTypePercent
}

type PromoCode struct {
	ID            int               `json:"id"`
	Code          string            `json:"code"`
	Status        PromoCodeStatus   `json:"status"`
	DiscountType  PromoDiscountType `json:"discount_type"`
	DiscountValue string            `json:"discount_value"`
	Currency      string            `json:"currency"`
	MaxUses       int               `json:"max_uses"`
	UsedCount     int               `json:"used_count"`
	StartsAt      *time.Time        `json:"starts_at,omitempty"`
	ExpiresAt     *time.Time        `json:"expires_at,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}
type PromoCodeRequest struct {
	Code          string
	Status        PromoCodeStatus
	DiscountType  PromoDiscountType
	DiscountValue string
	Currency      string
	MaxUses       int
	StartsAt      *time.Time
	ExpiresAt     *time.Time
}
type PromoCodeList struct {
	Items []PromoCode
	Total int
}
type PromoCodePreviewInput struct {
	UserID   int
	Code     string
	Amount   string
	Currency string
	Now      time.Time
}
type PromoCodeFinalizeInput struct {
	UserID         int
	Code           string
	PaymentOrderID int
	OrderNo        string
	OriginalAmount string
	FinalAmount    string
	Currency       string
	AppliedAt      time.Time
}
type PromoCodeApplication struct {
	ID             int               `json:"id"`
	UserID         int               `json:"user_id"`
	PromoCodeID    int               `json:"promo_code_id"`
	PaymentOrderID int               `json:"payment_order_id"`
	OrderNo        string            `json:"order_no"`
	OriginalAmount string            `json:"original_amount"`
	DiscountAmount string            `json:"discount_amount"`
	FinalAmount    string            `json:"final_amount"`
	Currency       string            `json:"currency"`
	DiscountType   PromoDiscountType `json:"discount_type"`
	AppliedAt      time.Time         `json:"applied_at"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type RiskControlMode string

const (
	RiskControlModeMonitor RiskControlMode = "monitor"
	RiskControlModeEnforce RiskControlMode = "enforce"
)

func (m RiskControlMode) Valid() bool {
	return m == RiskControlModeMonitor || m == RiskControlModeEnforce
}

type RiskControlConfig struct {
	Enabled                    bool            `json:"enabled"`
	Mode                       RiskControlMode `json:"mode"`
	MaxFailedRequestsPerMinute int             `json:"max_failed_requests_per_minute"`
	MaxCostPerDay              string          `json:"max_cost_per_day"`
	CooldownSeconds            int             `json:"cooldown_seconds"`
	BlockedCountries           []string        `json:"blocked_countries"`
	BlockedIPs                 []string        `json:"blocked_ips"`
}
type RiskControlStatus struct {
	Enabled      bool            `json:"enabled"`
	Mode         RiskControlMode `json:"mode"`
	ActiveBlocks int             `json:"active_blocks"`
	RecentEvents int             `json:"recent_events"`
	EvaluatedAt  time.Time       `json:"evaluated_at"`
}
type RiskControlLogLevel string

const (
	RiskControlLogLevelInfo  RiskControlLogLevel = "info"
	RiskControlLogLevelWarn  RiskControlLogLevel = "warn"
	RiskControlLogLevelBlock RiskControlLogLevel = "block"
)

type RiskControlLog struct {
	ID        int                 `json:"id"`
	Level     RiskControlLogLevel `json:"level"`
	Action    string              `json:"action"`
	Reason    string              `json:"reason"`
	Subject   *string             `json:"subject,omitempty"`
	Metadata  map[string]any      `json:"metadata,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
}
type RiskLogList struct {
	Items []RiskControlLog
	Total int
}

type OpsSystemLogLevel string

const (
	OpsSystemLogLevelDebug OpsSystemLogLevel = "debug"
	OpsSystemLogLevelInfo  OpsSystemLogLevel = "info"
	OpsSystemLogLevelWarn  OpsSystemLogLevel = "warn"
	OpsSystemLogLevelError OpsSystemLogLevel = "error"
)

func (l OpsSystemLogLevel) Valid() bool {
	return l == OpsSystemLogLevelDebug || l == OpsSystemLogLevelInfo || l == OpsSystemLogLevelWarn || l == OpsSystemLogLevelError
}

type OpsSystemLog struct {
	ID        int               `json:"id"`
	Level     OpsSystemLogLevel `json:"level"`
	Message   string            `json:"message"`
	Source    string            `json:"source"`
	RequestID string            `json:"request_id,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
type SystemLogList struct {
	Items []OpsSystemLog
	Total int
}

type SystemLogListOptions struct {
	Page     int
	PageSize int
	Level    OpsSystemLogLevel
	Source   string
	Query    string
	Start    *time.Time
	End      *time.Time
}

type RecordSystemLogRequest struct {
	Level     OpsSystemLogLevel
	Message   string
	Source    string
	RequestID string
	TraceID   string
	Metadata  map[string]any
	CreatedAt time.Time
}

type SystemLogCleanupFilter struct {
	Level     OpsSystemLogLevel
	Source    string
	Query     string
	Start     *time.Time
	End       *time.Time
	DryRun    bool
	MaxDelete int
}

type SystemLogCleanupResult struct {
	Matched   int  `json:"matched"`
	Deleted   int  `json:"deleted"`
	DryRun    bool `json:"dry_run"`
	MaxDelete int  `json:"max_delete"`
	Limited   bool `json:"limited"`
}

type SystemLogStore interface {
	CreateSystemLog(ctx context.Context, input OpsSystemLog) (OpsSystemLog, error)
	ListSystemLogs(ctx context.Context, opts SystemLogListOptions) (SystemLogList, error)
	CleanupSystemLogs(ctx context.Context, filter SystemLogCleanupFilter) (SystemLogCleanupResult, error)
}

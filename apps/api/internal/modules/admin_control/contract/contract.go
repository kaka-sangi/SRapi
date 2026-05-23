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
	AdminAPIKey         SecretConfigured `json:"admin_api_key"`
	RegistrationEnabled bool             `json:"registration_enabled"`
	OAuthEnabled        bool             `json:"oauth_enabled"`
	OAuthProviders      []string         `json:"oauth_providers"`
}
type AdminSettingsUsers struct {
	DefaultBalance        string `json:"default_balance"`
	DefaultGroup          string `json:"default_group"`
	UserSelfDeleteEnabled bool   `json:"user_self_delete_enabled"`
	RPMLimitDefault       int    `json:"rpm_limit_default"`
}
type AdminSettingsGateway struct {
	OverloadCooldownSeconds  int    `json:"overload_cooldown_seconds"`
	RateLimitCooldownSeconds int    `json:"rate_limit_cooldown_seconds"`
	StreamTimeoutSeconds     int    `json:"stream_timeout_seconds"`
	RequestShaperEnabled     bool   `json:"request_shaper_enabled"`
	BetaStrategy             string `json:"beta_strategy"`
}
type AdminSettingsPayment struct {
	Enabled                  bool     `json:"enabled"`
	Providers                []string `json:"providers"`
	SubscriptionPlansEnabled bool     `json:"subscription_plans_enabled"`
}
type AdminSettingsEmail struct {
	SMTPConfigured bool              `json:"smtp_configured"`
	Templates      map[string]string `json:"templates"`
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

type OpsSystemLog struct {
	ID        int               `json:"id"`
	Level     OpsSystemLogLevel `json:"level"`
	Message   string            `json:"message"`
	Source    string            `json:"source"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
type SystemLogList struct {
	Items []OpsSystemLog
	Total int
}

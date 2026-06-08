package contract

import (
	"context"
	"time"
)

type RuntimeClass string

const (
	RuntimeClassAPIKey             RuntimeClass = "api_key"
	RuntimeClassOauthRefresh       RuntimeClass = "oauth_refresh"
	RuntimeClassOauthDeviceCode    RuntimeClass = "oauth_device_code"
	RuntimeClassWebSessionCookie   RuntimeClass = "web_session_cookie"
	RuntimeClassDesktopClientToken RuntimeClass = "desktop_client_token"
	RuntimeClassCliClientToken     RuntimeClass = "cli_client_token"
	RuntimeClassIdePluginToken     RuntimeClass = "ide_plugin_token"
	RuntimeClassServiceAccountJSON RuntimeClass = "service_account_json"
	RuntimeClassCustomReverseProxy RuntimeClass = "custom_reverse_proxy"
)

type Status string

const (
	StatusActive      Status = "active"
	StatusDisabled    Status = "disabled"
	StatusNeedsReauth Status = "needs_reauth"
	StatusSuspended   Status = "suspended"
	StatusDead        Status = "dead"
	// StatusArchived is an operator soft-delete: the account is hidden from the
	// default admin list and never scheduled (only "active" is a candidate), but
	// the row is kept so historical usage/audit references stay intact.
	StatusArchived Status = "archived"
)

type GroupStatus string

const (
	GroupStatusActive   GroupStatus = "active"
	GroupStatusDisabled GroupStatus = "disabled"
)

type ProxyType string

const (
	ProxyTypeHTTP   ProxyType = "http"
	ProxyTypeHTTPS  ProxyType = "https"
	ProxyTypeSOCKS5 ProxyType = "socks5"
)

type ProxyStatus string

const (
	ProxyStatusActive   ProxyStatus = "active"
	ProxyStatusDisabled ProxyStatus = "disabled"
)

type ProviderAccount struct {
	ID                   int
	ProviderID           int
	Name                 string
	RuntimeClass         RuntimeClass
	UpstreamClient       *string
	CredentialCiphertext string
	CredentialVersion    string
	ProxyID              *string
	Status               Status
	Priority             int
	Weight               float32
	RiskLevel            *string
	Metadata             map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
}

type ProxyDefinition struct {
	ID            int
	Name          string
	Type          ProxyType
	URLCiphertext string
	URLVersion    string
	Status        ProxyStatus
	Metadata      map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

type AccountGroup struct {
	ID            int
	Name          string
	Description   string
	ProviderScope map[string]any
	ModelScope    map[string]any
	StrategyHint  string
	RateMultiplier string
	Status        GroupStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AccountGroupMember struct {
	ID             int
	AccountID      int
	AccountGroupID int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AccountHealthSnapshot struct {
	ID             int
	AccountID      int
	ProviderID     int
	Status         string
	SuccessRate    float32
	ErrorRate      float32
	LatencyP50MS   int
	LatencyP95MS   int
	RateLimitCount int
	TimeoutCount   int
	CooldownUntil  *time.Time
	CircuitState   string
	SnapshotAt     time.Time
}

// AccountProbeResult captures one active upstream health probe result.
type AccountProbeResult struct {
	OK         bool
	ErrorClass string
	StatusCode int
	LatencyMS  int
	CheckedAt  time.Time
	Metadata   map[string]any
}

// AccountProbePolicy controls how probe results are folded into health state.
type AccountProbePolicy struct {
	HistoryLimit           int
	FailureThreshold       int
	ErrorRateThreshold     float32
	MinSamplesForErrorRate int
	Cooldown               time.Duration
}

// AccountProber performs the provider-specific account health check.
type AccountProber interface {
	ProbeAccount(ctx context.Context, account ProviderAccount, credential map[string]any) (AccountProbeResult, error)
}

type AccountQuotaSnapshot struct {
	ID             int
	AccountID      int
	ProviderID     int
	QuotaType      string
	Remaining      string
	Used           string
	QuotaLimit     string
	RemainingRatio float32
	ResetAt        *time.Time
	SnapshotAt     time.Time
}

type BatchUpdateResult struct {
	Updated []ProviderAccount
	Errors  []string
}

type RPMStatus struct {
	AccountID     int
	RPMUsed       int
	RPMLimit      *int
	WindowSeconds int
	ResetAt       *time.Time
}

type ProxyQuality struct {
	AccountID     int
	ProxyID       *string
	SuccessRate   float32
	ErrorRate     float32
	LatencyP95MS  int
	SampleCount   int
	LastCheckedAt *time.Time
	Metadata      map[string]any
}

type CreateRequest struct {
	ProviderID     int
	Name           string
	RuntimeClass   RuntimeClass
	Credential     map[string]any
	Metadata       map[string]any
	ProxyID        *string
	Status         *Status
	Priority       *int
	Weight         *float32
	UpstreamClient *string
}

type UpdateRequest struct {
	Name           *string
	RuntimeClass   *RuntimeClass
	Credential     *map[string]any
	Metadata       *map[string]any
	ProxyID        **string
	Status         *Status
	Priority       *int
	Weight         *float32
	UpstreamClient **string
}

type CreateGroupRequest struct {
	Name          string
	Description   string
	ProviderScope map[string]any
	ModelScope    map[string]any
	StrategyHint  *string
	RateMultiplier *string
	Status        *GroupStatus
}

type UpdateGroupRequest struct {
	Name          *string
	Description   *string
	ProviderScope *map[string]any
	ModelScope    *map[string]any
	StrategyHint  *string
	RateMultiplier *string
	Status        *GroupStatus
}

type CreateProxyRequest struct {
	Name     string
	Type     ProxyType
	URL      string
	Status   *ProxyStatus
	Metadata map[string]any
}

type UpdateProxyRequest struct {
	Name     *string
	Type     *ProxyType
	URL      *string
	Status   *ProxyStatus
	Metadata *map[string]any
}

type CreateStoredAccount struct {
	ProviderID           int
	Name                 string
	RuntimeClass         RuntimeClass
	CredentialCiphertext string
	CredentialVersion    string
	Metadata             map[string]any
	ProxyID              *string
	Status               Status
	Priority             int
	Weight               float32
	UpstreamClient       *string
}

type CreateStoredProxy struct {
	Name          string
	Type          ProxyType
	URLCiphertext string
	URLVersion    string
	Status        ProxyStatus
	Metadata      map[string]any
}

type CreateStoredAccountGroup struct {
	Name          string
	Description   string
	ProviderScope map[string]any
	ModelScope    map[string]any
	StrategyHint  string
	RateMultiplier string
	Status        GroupStatus
}

type Store interface {
	Create(ctx context.Context, input CreateStoredAccount) (ProviderAccount, error)
	Update(ctx context.Context, account ProviderAccount) (ProviderAccount, error)
	FindByID(ctx context.Context, id int) (ProviderAccount, error)
	List(ctx context.Context) ([]ProviderAccount, error)
	CreateProxy(ctx context.Context, input CreateStoredProxy) (ProxyDefinition, error)
	UpdateProxy(ctx context.Context, proxy ProxyDefinition) (ProxyDefinition, error)
	FindProxyByID(ctx context.Context, id int) (ProxyDefinition, error)
	ListProxies(ctx context.Context) ([]ProxyDefinition, error)
	SoftDeleteProxy(ctx context.Context, id int) error
	CreateGroup(ctx context.Context, input CreateStoredAccountGroup) (AccountGroup, error)
	UpdateGroup(ctx context.Context, group AccountGroup) (AccountGroup, error)
	FindGroupByID(ctx context.Context, id int) (AccountGroup, error)
	ListGroups(ctx context.Context) ([]AccountGroup, error)
	DeleteGroup(ctx context.Context, id int) error
	AddAccountToGroup(ctx context.Context, accountID int, groupID int) (AccountGroupMember, error)
	RemoveAccountFromGroup(ctx context.Context, accountID int, groupID int) error
	ListGroupMembers(ctx context.Context, groupID int) ([]AccountGroupMember, error)
	ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error)
	RecordHealthSnapshot(ctx context.Context, snapshot AccountHealthSnapshot) (AccountHealthSnapshot, error)
	LatestHealthSnapshotByAccount(ctx context.Context, accountID int) (AccountHealthSnapshot, error)
	ListHealthSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]AccountHealthSnapshot, error)
	RecordQuotaSnapshot(ctx context.Context, snapshot AccountQuotaSnapshot) (AccountQuotaSnapshot, error)
	ListQuotaSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]AccountQuotaSnapshot, error)
	Delete(ctx context.Context, id int) error
}

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
)

type GroupStatus string

const (
	GroupStatusActive   GroupStatus = "active"
	GroupStatusDisabled GroupStatus = "disabled"
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

type AccountGroup struct {
	ID            int
	Name          string
	Description   string
	ProviderScope map[string]any
	ModelScope    map[string]any
	StrategyHint  string
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
	Status        *GroupStatus
}

type UpdateGroupRequest struct {
	Name          *string
	Description   *string
	ProviderScope *map[string]any
	ModelScope    *map[string]any
	StrategyHint  *string
	Status        *GroupStatus
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

type CreateStoredAccountGroup struct {
	Name          string
	Description   string
	ProviderScope map[string]any
	ModelScope    map[string]any
	StrategyHint  string
	Status        GroupStatus
}

type Store interface {
	Create(ctx context.Context, input CreateStoredAccount) (ProviderAccount, error)
	Update(ctx context.Context, account ProviderAccount) (ProviderAccount, error)
	FindByID(ctx context.Context, id int) (ProviderAccount, error)
	List(ctx context.Context) ([]ProviderAccount, error)
	CreateGroup(ctx context.Context, input CreateStoredAccountGroup) (AccountGroup, error)
	UpdateGroup(ctx context.Context, group AccountGroup) (AccountGroup, error)
	FindGroupByID(ctx context.Context, id int) (AccountGroup, error)
	ListGroups(ctx context.Context) ([]AccountGroup, error)
	AddAccountToGroup(ctx context.Context, accountID int, groupID int) (AccountGroupMember, error)
	RemoveAccountFromGroup(ctx context.Context, accountID int, groupID int) error
	ListGroupMembers(ctx context.Context, groupID int) ([]AccountGroupMember, error)
	ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error)
	RecordHealthSnapshot(ctx context.Context, snapshot AccountHealthSnapshot) (AccountHealthSnapshot, error)
	LatestHealthSnapshotByAccount(ctx context.Context, accountID int) (AccountHealthSnapshot, error)
	ListHealthSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]AccountHealthSnapshot, error)
	RecordQuotaSnapshot(ctx context.Context, snapshot AccountQuotaSnapshot) (AccountQuotaSnapshot, error)
	ListQuotaSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]AccountQuotaSnapshot, error)
}

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

type Store interface {
	Create(ctx context.Context, input CreateStoredAccount) (ProviderAccount, error)
	Update(ctx context.Context, account ProviderAccount) (ProviderAccount, error)
	FindByID(ctx context.Context, id int) (ProviderAccount, error)
	List(ctx context.Context) ([]ProviderAccount, error)
	ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error)
}

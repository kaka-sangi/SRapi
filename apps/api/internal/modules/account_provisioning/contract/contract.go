// Package contract defines the value types for the upstream-account OAuth
// provisioning state machine. It carries no infrastructure dependencies so it
// can be shared by the service, its HTTP handlers, and tests.
package contract

import "time"

// Mode distinguishes the two provisioning flows a pending session can drive.
type Mode string

const (
	// ModeAuthorizationCode is the redirect/PKCE authorization-code flow.
	ModeAuthorizationCode Mode = "authorization_code"
	// ModeDeviceCode is the RFC 8628 device-authorization flow.
	ModeDeviceCode Mode = "device_code"
)

// Status reports where a pending provisioning session is in its lifecycle.
type Status string

const (
	// StatusPending means the session is awaiting the user completing the
	// upstream consent (authorization-code) or polling (device-code).
	StatusPending Status = "pending"
	// StatusCompleted means tokens were minted and the credential is ready to
	// attach to a new provider account.
	StatusCompleted Status = "completed"
	// StatusFailed means the exchange/poll terminally failed.
	StatusFailed Status = "failed"
	// StatusExpired means the session TTL elapsed before completion.
	StatusExpired Status = "expired"
)

// ProviderOAuthConfig is the config-driven description of an upstream provider's
// OAuth endpoints. It mirrors admin_control.OAuthProviderConfig but is scoped to
// what the provisioning flows need so the module stays infrastructure-free.
type ProviderOAuthConfig struct {
	ClientID             string
	ClientSecret         string
	AuthorizeURL         string
	TokenURL             string
	DeviceAuthorizeURL   string
	RedirectURI          string
	Scopes               []string
	TokenAuthMethod      string
	UsePKCE              bool
	ExtraAuthorizeParams map[string]string
}

// PendingSession is the short-lived, in-memory provisioning record.
type PendingSession struct {
	ID           string
	Mode         Mode
	Status       Status
	ClientID     string
	ClientSecret string
	TokenURL     string
	RedirectURI  string
	Scopes       []string
	State        string
	CodeVerifier string
	// DeviceCode is the opaque device_code returned by the provider for the
	// device-authorization flow; never surfaced to the browser after start.
	DeviceCode    string
	UserCode      string
	IntervalSecs  int
	UsePKCE       bool
	FailureReason string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	LastPolledAt  time.Time
	CompletedAt   time.Time
}

// MintedCredential is the access/refresh token bundle produced on completion.
// The map form is what the account credential store ultimately encrypts.
type MintedCredential struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresInSec int
	Scope        string
	IDToken      string
	Raw          map[string]any
}

// Credential renders the minted tokens into the credential map shape the
// accounts service encrypts. Empty fields are omitted so a refresh-only or
// access-only provider is represented faithfully.
func (c MintedCredential) Credential() map[string]any {
	out := map[string]any{}
	if c.AccessToken != "" {
		out["access_token"] = c.AccessToken
	}
	if c.RefreshToken != "" {
		out["refresh_token"] = c.RefreshToken
	}
	if c.TokenType != "" {
		out["token_type"] = c.TokenType
	}
	if c.Scope != "" {
		out["scope"] = c.Scope
	}
	if c.IDToken != "" {
		out["id_token"] = c.IDToken
	}
	if c.ExpiresInSec > 0 {
		out["expires_in"] = c.ExpiresInSec
	}
	return out
}

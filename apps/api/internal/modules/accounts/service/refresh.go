package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

// permanentRefreshErrorRegex matches OAuth refresh failures that signal the
// account will never refresh again without a human re-authorizing. Sourced
// from RFC 6749 §5.2 and OIDC core 1.0 §3.1.2.6:
//   - invalid_grant: refresh token revoked / expired / never valid
//   - invalid_client / unauthorized_client: client credential rotated or revoked
//   - invalid_token: ChatGPT-web style "session expired"
//   - access_denied: user revoked consent
//   - consent_required / login_required: interactive flow required
//
// On a permanent error the worker sets needs_reauth_at immediately so it
// stops calling the upstream until an operator re-binds the account.
var permanentRefreshErrorRegex = regexp.MustCompile(`(?i)invalid_grant|invalid_client|unauthorized_client|invalid_token|access_denied|consent_required|login_required`)

// refreshFailureThreshold is the consecutive transient-failure count at which
// a non-permanent error also flips the account into needs_reauth. 5 keeps
// the worker from hammering a flapping upstream while still tolerating brief
// outages at the default 5-minute cadence (≈ 25 minutes of failures).
const refreshFailureThreshold = 5

// maxRefreshErrorLength caps refresh_last_error so a verbose upstream body
// cannot bloat the row. Long enough for the typical OAuth JSON snippet but
// short enough to fit comfortably in the admin row.
const maxRefreshErrorLength = 500

// RefreshResult is the outcome of one OAuth refresh attempt. The Refresher
// interface mirrors reverse_proxy.contract.Refresher but is defined locally
// so the accounts module does not need to import the reverse-proxy contract
// (and so tests can fake it).
type RefreshResult struct {
	Credential map[string]any
}

// RefreshRequest carries the data the refresher needs to perform a refresh.
// Mirrors reverse_proxy.contract.AccountRuntime closely; callers in app.go
// adapt the reverse_proxy refresher into this interface so the cross-module
// import stays one-way.
type RefreshRequest struct {
	AccountID      int
	RuntimeClass   contract.RuntimeClass
	UpstreamClient *string
	ProxyID        *string
	Metadata       map[string]any
	Credential     map[string]any
}

// AccountRefresher performs one OAuth token refresh against an upstream and
// returns the new credential map (which must contain access_token and
// expires_at on success).
type AccountRefresher interface {
	RefreshAccount(ctx context.Context, req RefreshRequest) (RefreshResult, error)
}

// RefreshAccessToken runs one OAuth refresh against the upstream and
// persists the outcome on the account row. Used by both the proactive
// accounts_token_refresh worker and the admin "Refresh now" endpoint so
// the bookkeeping rules (refresh_attempts / needs_reauth_at) stay in one
// place.
func (s *Service) RefreshAccessToken(ctx context.Context, accountID int, refresher AccountRefresher) (contract.ProviderAccount, error) {
	if accountID <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	if refresher == nil {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, accountID)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	if account.RuntimeClass != contract.RuntimeClassOauthRefresh && account.RuntimeClass != contract.RuntimeClassOauthDeviceCode {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	credential, err := s.decryptCredential(account.CredentialCiphertext)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	resp, refreshErr := refresher.RefreshAccount(ctx, RefreshRequest{
		AccountID:      account.ID,
		RuntimeClass:   account.RuntimeClass,
		UpstreamClient: cloneString(account.UpstreamClient),
		ProxyID:        cloneString(account.ProxyID),
		Metadata:       cloneMap(account.Metadata),
		Credential:     credential,
	})
	now := s.clock.Now().UTC()
	if refreshErr != nil {
		return s.applyRefreshFailure(ctx, account, refreshErr, now)
	}
	return s.applyRefreshSuccess(ctx, account, resp.Credential, now)
}

func (s *Service) applyRefreshSuccess(ctx context.Context, account contract.ProviderAccount, credential map[string]any, now time.Time) (contract.ProviderAccount, error) {
	if len(credential) == 0 {
		return account, errors.New("oauth refresh returned empty credential")
	}
	ciphertext, err := s.encryptCredential(credential)
	if err != nil {
		return account, err
	}
	account.CredentialCiphertext = ciphertext
	account.CredentialVersion = credentialVersionV1
	account.UpdatedAt = now
	account.LastRefreshedAt = timePtr(now)
	account.NeedsReauthAt = nil
	account.RefreshAttempts = 0
	account.RefreshLastError = ""
	if expiresAt, ok := parseCredentialExpiresAt(credential); ok {
		account.TokenExpiresAt = timePtr(expiresAt)
	} else {
		account.TokenExpiresAt = nil
	}
	// Clearing the metadata "needs_reauth" / "access_token_expired" hints keeps
	// downstream readers (ShouldRefreshOAuthCredential) from flipping straight
	// back into "needs refresh" after a successful pull.
	if len(account.Metadata) > 0 {
		metadata := cloneMap(account.Metadata)
		delete(metadata, "force_refresh")
		delete(metadata, "access_token_expired")
		delete(metadata, "needs_reauth_reason")
		account.Metadata = metadata
	}
	updated, err := s.store.Update(ctx, account)
	if err != nil {
		return account, err
	}
	return updated, nil
}

func (s *Service) applyRefreshFailure(ctx context.Context, account contract.ProviderAccount, refreshErr error, now time.Time) (contract.ProviderAccount, error) {
	message := truncateRefreshError(refreshErr.Error())
	account.RefreshAttempts++
	account.RefreshLastError = message
	account.UpdatedAt = now
	if isPermanentRefreshError(refreshErr) || account.RefreshAttempts >= refreshFailureThreshold {
		account.NeedsReauthAt = timePtr(now)
	}
	if _, updateErr := s.store.Update(ctx, account); updateErr != nil {
		// Surface the original refresh error to the caller; the store error is
		// secondary and the worker will retry on the next pass.
		return account, refreshErr
	}
	return account, refreshErr
}

func isPermanentRefreshError(err error) bool {
	if err == nil {
		return false
	}
	return permanentRefreshErrorRegex.MatchString(err.Error())
}

func truncateRefreshError(message string) string {
	trimmed := strings.TrimSpace(message)
	if len(trimmed) <= maxRefreshErrorLength {
		return trimmed
	}
	return trimmed[:maxRefreshErrorLength]
}

func parseCredentialExpiresAt(credential map[string]any) (time.Time, bool) {
	if credential == nil {
		return time.Time{}, false
	}
	raw, ok := credential["expires_at"]
	if !ok || raw == nil {
		return time.Time{}, false
	}
	switch value := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return time.Time{}, false
		}
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return time.Time{}, false
		}
		return parsed.UTC(), true
	case time.Time:
		return value.UTC(), true
	default:
		return time.Time{}, false
	}
}

func timePtr(value time.Time) *time.Time {
	cloned := value
	return &cloned
}

package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
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

// RefreshOutcomeClass categorises one refresh attempt for audit replay and
// metrics. Decided inside the failure-application path rather than by
// regex-matching the raw error message at the call site, so an audit log
// answer to "why did needs_reauth fire?" doesn't depend on the truncated
// error string.
type RefreshOutcomeClass string

const (
	RefreshOutcomeSuccess           RefreshOutcomeClass = "success"
	RefreshOutcomePermanentError    RefreshOutcomeClass = "permanent_error"
	RefreshOutcomeTransientError    RefreshOutcomeClass = "transient_error"
	RefreshOutcomeThresholdExceeded RefreshOutcomeClass = "threshold_exceeded"
)

// RefreshOutcome is the structured result of one RefreshAccessToken call.
// Class is the categorical outcome (used by the audit snapshot + the
// accounts_token_refresh worker's Prometheus counters); Attempts is the
// post-call refresh_attempts value; NeedsReauthFlipped is true when this
// call moved needs_reauth_at from nil to non-nil; Account is the
// post-update row.
type RefreshOutcome struct {
	Class              RefreshOutcomeClass
	Attempts           int
	NeedsReauthFlipped bool
	Account            contract.ProviderAccount
}

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

// refreshLockFor returns a per-account mutex used to serialize the
// (FindByID → refresher.RefreshAccount → applyResult → Update) sequence so
// the worker + manual /refresh endpoint cannot both write back stale state
// from concurrent in-flight refreshes for the same row. Loaded lazily;
// stale entries are left in the map (low cardinality in practice — one per
// account that has ever refreshed).
func (s *Service) refreshLockFor(accountID int) *sync.Mutex {
	if existing, ok := s.refreshLocks.Load(accountID); ok {
		return existing.(*sync.Mutex)
	}
	created := &sync.Mutex{}
	actual, _ := s.refreshLocks.LoadOrStore(accountID, created)
	return actual.(*sync.Mutex)
}

// RefreshAccessToken runs one OAuth refresh against the upstream and
// persists the outcome on the account row. Used by both the proactive
// accounts_token_refresh worker and the admin "Refresh now" endpoint so
// the bookkeeping rules (refresh_attempts / needs_reauth_at) stay in one
// place.
//
// Returns the post-update account on both success and failure so the
// caller can read refresh_attempts / needs_reauth_at without a second
// store round-trip. On a refresh failure the returned error is the
// original refresher error (so the HTTP handler can surface it verbatim),
// but the row has already been persisted.
func (s *Service) RefreshAccessToken(ctx context.Context, accountID int, refresher AccountRefresher) (contract.ProviderAccount, error) {
	account, _, err := s.refreshAccessToken(ctx, accountID, refresher)
	return account, err
}

// RefreshAccessTokenWithOutcome is RefreshAccessToken with the structured
// outcome alongside the error, so callers (audit snapshot, worker metrics)
// can answer "why did needs_reauth fire?" without re-parsing the error.
func (s *Service) RefreshAccessTokenWithOutcome(ctx context.Context, accountID int, refresher AccountRefresher) (RefreshOutcome, error) {
	account, outcome, err := s.refreshAccessToken(ctx, accountID, refresher)
	outcome.Account = account
	return outcome, err
}

func (s *Service) refreshAccessToken(ctx context.Context, accountID int, refresher AccountRefresher) (contract.ProviderAccount, RefreshOutcome, error) {
	if accountID <= 0 {
		return contract.ProviderAccount{}, RefreshOutcome{}, ErrInvalidInput
	}
	if refresher == nil {
		return contract.ProviderAccount{}, RefreshOutcome{}, ErrInvalidInput
	}
	// Serialize per-account so two concurrent refreshes for the same row can't
	// both Find → refresh → Update with stale state — last-write would clobber
	// the other's refresh_attempts / needs_reauth_at bookkeeping. The memory
	// store happens to be locked too, but the SQL backend isn't, and the worker
	// + manual /refresh endpoint really can collide in production.
	lock := s.refreshLockFor(accountID)
	lock.Lock()
	defer lock.Unlock()

	account, err := s.store.FindByID(ctx, accountID)
	if err != nil {
		return contract.ProviderAccount{}, RefreshOutcome{}, err
	}
	if account.RuntimeClass != contract.RuntimeClassOauthRefresh && account.RuntimeClass != contract.RuntimeClassOauthDeviceCode {
		return contract.ProviderAccount{}, RefreshOutcome{}, ErrInvalidInput
	}
	credential, err := s.decryptCredential(account.CredentialCiphertext)
	if err != nil {
		return contract.ProviderAccount{}, RefreshOutcome{}, err
	}
	wasReauthFlagged := account.NeedsReauthAt != nil
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
		updated, outcome, applyErr := s.applyRefreshFailure(ctx, account, refreshErr, now)
		outcome.NeedsReauthFlipped = !wasReauthFlagged && updated.NeedsReauthAt != nil
		return updated, outcome, applyErr
	}
	updated, outcome, applyErr := s.applyRefreshSuccess(ctx, account, resp.Credential, now)
	return updated, outcome, applyErr
}

func (s *Service) applyRefreshSuccess(ctx context.Context, account contract.ProviderAccount, credential map[string]any, now time.Time) (contract.ProviderAccount, RefreshOutcome, error) {
	if len(credential) == 0 {
		return account, RefreshOutcome{Class: RefreshOutcomeTransientError, Attempts: account.RefreshAttempts}, errors.New("oauth refresh returned empty credential")
	}
	credential = credentialWithTokenVersion(credential, now)
	ciphertext, err := s.encryptCredential(credential)
	if err != nil {
		return account, RefreshOutcome{Class: RefreshOutcomeTransientError, Attempts: account.RefreshAttempts}, err
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
		return account, RefreshOutcome{Class: RefreshOutcomeTransientError, Attempts: account.RefreshAttempts}, err
	}
	return updated, RefreshOutcome{Class: RefreshOutcomeSuccess, Attempts: 0}, nil
}

func (s *Service) applyRefreshFailure(ctx context.Context, account contract.ProviderAccount, refreshErr error, now time.Time) (contract.ProviderAccount, RefreshOutcome, error) {
	message := truncateRefreshError(refreshErr.Error())
	account.RefreshAttempts++
	account.RefreshLastError = message
	account.UpdatedAt = now
	class := RefreshOutcomeTransientError
	switch {
	case isPermanentRefreshError(refreshErr):
		class = RefreshOutcomePermanentError
		account.NeedsReauthAt = timePtr(now)
	case account.RefreshAttempts >= refreshFailureThreshold:
		class = RefreshOutcomeThresholdExceeded
		account.NeedsReauthAt = timePtr(now)
	}
	if _, updateErr := s.store.Update(ctx, account); updateErr != nil {
		// Surface the original refresh error to the caller; the store error is
		// secondary and the worker will retry on the next pass.
		return account, RefreshOutcome{Class: class, Attempts: account.RefreshAttempts}, refreshErr
	}
	return account, RefreshOutcome{Class: class, Attempts: account.RefreshAttempts}, refreshErr
}

func credentialWithTokenVersion(credential map[string]any, now time.Time) map[string]any {
	out := cloneMap(credential)
	out["_token_version"] = now.UTC().UnixMilli()
	return out
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

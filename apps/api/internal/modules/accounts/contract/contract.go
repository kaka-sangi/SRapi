package contract

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type RuntimeClass string

const (
	RuntimeClassAPIKey             RuntimeClass = "api_key"
	RuntimeClassOauthRefresh       RuntimeClass = "oauth_refresh"
	RuntimeClassOauthDeviceCode    RuntimeClass = "oauth_device_code"
	RuntimeClassWebSessionCookie   RuntimeClass = "web_session_cookie"
	RuntimeClassCliClientToken     RuntimeClass = "cli_client_token"
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

// ShouldRefreshOAuthCredential reports whether an OAuth-style account should
// refresh its access token before an upstream request.
func ShouldRefreshOAuthCredential(account ProviderAccount, credential map[string]any, now time.Time) bool {
	if account.RuntimeClass != RuntimeClassOauthRefresh && account.RuntimeClass != RuntimeClassOauthDeviceCode {
		return false
	}
	if metadataBool(account.Metadata, "force_refresh") || metadataBool(account.Metadata, "access_token_expired") {
		return true
	}
	expiresAt := metadataString(credential, "expires_at")
	if expiresAt == "" {
		return metadataString(credential, "access_token") == ""
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.UTC().After(parsed.Add(-30 * time.Second))
}

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
	// TokenExpiresAt is snapshotted from the OAuth credential's "expires_at"
	// after a refresh. The admin list/worker use it to drive the proactive
	// refresh window without decrypting credential_ciphertext.
	TokenExpiresAt *time.Time
	// LastRefreshedAt is the wall-clock time of the most recent successful
	// OAuth refresh on this account.
	LastRefreshedAt *time.Time
	// NeedsReauthAt is set when refresh is hopeless (permanent OAuth error
	// such as invalid_grant, or refresh_attempts >= 5). While non-nil, the
	// proactive worker skips this account so the upstream is not hammered.
	NeedsReauthAt *time.Time
	// RefreshAttempts counts consecutive refresh failures. Zeroed on success.
	RefreshAttempts int
	// RefreshLastError is the most recent refresh error message, truncated
	// to 500 chars before persistence so operators can see WHY the account
	// flipped into needs_reauth.
	RefreshLastError string
}

type ProxyDefinition struct {
	ID            int
	Name          string
	Type          ProxyType
	URLCiphertext string
	URLVersion    string
	Status        ProxyStatus
	Metadata      map[string]any
	// CountryCode is the operator-supplied ISO-3166-1 alpha-2 code (e.g. "US",
	// "CN"). Empty when unset.
	CountryCode string
	// CountryName is the localized display name snapshotted at write time so
	// list views render a stable label even when the frontend locale changes.
	CountryName string
	// LastProbedAt is set by the proxy_probe worker after each pass; nil
	// before the proxy has ever been probed.
	LastProbedAt *time.Time
	// ProbeSuccessCount + ProbeFailureCount are cumulative counters that the
	// proxy_probe worker resets every ~7 days, giving a rolling-window
	// availability percentage without a separate snapshot table.
	ProbeSuccessCount int
	ProbeFailureCount int
	// LastProbeLatencyMs records the milliseconds the most recent successful
	// probe took. 0 when no probe has ever succeeded.
	LastProbeLatencyMs int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

// ProbeSuccessPct7d returns the rolling 7-day availability percentage rounded
// to the nearest integer when at least one probe has been recorded since the
// last reset; otherwise nil. Lives on the contract (not on ent) because it is
// purely derived — recomputing in the response mapper keeps the rule in one
// place across HTTP, audit, and worker call sites.
func (p ProxyDefinition) ProbeSuccessPct7d() *int {
	total := p.ProbeSuccessCount + p.ProbeFailureCount
	if total <= 0 {
		return nil
	}
	pct := int((float64(p.ProbeSuccessCount)*100.0)/float64(total) + 0.5)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return &pct
}

type AccountGroup struct {
	ID             int
	Name           string
	Description    string
	ProviderScope  map[string]any
	ModelScope     map[string]any
	StrategyHint   string
	RateMultiplier string
	Status         GroupStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
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

const QuotaTypeSyntheticMonthlyTokens = "synthetic_monthly_tokens"
const QuotaTypeProviderCredits = "provider_credits"

// IsSyntheticQuotaSnapshot reports whether a quota snapshot is locally derived
// by SRapi rather than read from an upstream provider quota signal.
func IsSyntheticQuotaSnapshot(snapshot AccountQuotaSnapshot) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(snapshot.QuotaType)), "synthetic_")
}

// QuotaCreditReport carries provider subscription/credits fields parsed from an
// active quota fetch. It is intentionally small so adapters can feed account
// metadata and quota history without importing provider-adapter contracts.
type QuotaCreditReport struct {
	Plan             string
	CreditsRemaining string
	CreditsUsed      string
	CreditsLimit     string
	Currency         string
	FetchedAt        time.Time
}

// QuotaMetadataFromReport persists subscription/credits standing on account
// metadata so admin views and operators can inspect the latest provider state.
func QuotaMetadataFromReport(current map[string]any, report QuotaCreditReport) map[string]any {
	metadata := cloneMetadata(current)
	if strings.TrimSpace(report.Plan) != "" {
		metadata["last_quota_plan"] = strings.TrimSpace(report.Plan)
		// Mirror sub2api's creds["plan_type"]: the report's plan should be
		// stored on the account credential. This function only reaches account
		// metadata, so surface plan_type here under the established convention.
		metadata["plan_type"] = strings.TrimSpace(report.Plan)
	}
	if strings.TrimSpace(report.CreditsRemaining) != "" {
		metadata["last_quota_credits_remaining"] = strings.TrimSpace(report.CreditsRemaining)
	}
	if strings.TrimSpace(report.CreditsUsed) != "" {
		metadata["last_quota_credits_used"] = strings.TrimSpace(report.CreditsUsed)
	}
	if strings.TrimSpace(report.CreditsLimit) != "" {
		metadata["last_quota_credits_limit"] = strings.TrimSpace(report.CreditsLimit)
	}
	if strings.TrimSpace(report.Currency) != "" {
		metadata["last_quota_currency"] = strings.TrimSpace(report.Currency)
	}
	fetchedAt := report.FetchedAt
	if fetchedAt.IsZero() {
		fetchedAt = time.Now().UTC()
	}
	metadata["last_quota_fetched_at"] = fetchedAt.UTC().Format(time.RFC3339)
	return metadata
}

// QuotaCreditSnapshotFromReport returns a real quota snapshot for provider
// credits, or false when the report has no credit fields to persist.
func QuotaCreditSnapshotFromReport(account ProviderAccount, report QuotaCreditReport) (AccountQuotaSnapshot, bool) {
	if account.ID <= 0 || account.ProviderID <= 0 {
		return AccountQuotaSnapshot{}, false
	}
	remaining := strings.TrimSpace(report.CreditsRemaining)
	used := strings.TrimSpace(report.CreditsUsed)
	limit := strings.TrimSpace(report.CreditsLimit)
	if remaining == "" && used == "" && limit == "" {
		return AccountQuotaSnapshot{}, false
	}
	if _, ok := creditRemainingRatio(remaining, used, limit); !ok {
		return AccountQuotaSnapshot{}, false
	}
	snapshotAt := report.FetchedAt
	if snapshotAt.IsZero() {
		snapshotAt = time.Now().UTC()
	}
	remainingRatio, _ := creditRemainingRatio(remaining, used, limit)
	return AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      QuotaTypeProviderCredits,
		Remaining:      remaining,
		Used:           used,
		QuotaLimit:     limit,
		RemainingRatio: remainingRatio,
		SnapshotAt:     snapshotAt.UTC(),
	}, true
}

func creditRemainingRatio(remaining string, used string, limit string) (float32, bool) {
	remainingValue, remainingOK := parseQuotaFloat(remaining)
	limitValue, limitOK := parseQuotaFloat(limit)
	if remainingOK && limitOK && limitValue > 0 {
		return clampQuotaRatio(float32(remainingValue / limitValue)), true
	}
	usedValue, usedOK := parseQuotaFloat(used)
	if remainingOK && usedOK && remainingValue+usedValue > 0 {
		return clampQuotaRatio(float32(remainingValue / (remainingValue + usedValue))), true
	}
	return 0, false
}

func parseQuotaFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func clampQuotaRatio(value float32) float32 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func cloneMetadata(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func metadataString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func metadataBool(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	switch value := value.(type) {
	case bool:
		return value
	case string:
		value = strings.TrimSpace(value)
		return strings.EqualFold(value, "true") || value == "1"
	default:
		return false
	}
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

// ProxyTestResult is the outcome of a one-shot HTTP probe sent through the
// proxy at a known-good target URL. Used by the per-row "Test" action so an
// operator can verify a proxy works without bouncing real traffic through it.
// OK is true only when the upstream returns 2xx within the probe timeout;
// ErrorClass categorizes the failure mode so the UI can render a useful hint.
type ProxyTestResult struct {
	OK         bool
	LatencyMS  int
	StatusCode int
	ErrorClass string
	TargetURL  string
}

// ProxyBatchTestRow is one row of a BatchTestProxies result. ProxyID identifies
// which input row the result applies to; Result is identical in shape to the
// single-row TestProxy result. A missing or invalid id surfaces as a row whose
// ErrorClass is "not_found" rather than as a hard error on the call.
type ProxyBatchTestRow struct {
	ProxyID int
	Result  ProxyTestResult
}

// BatchCreateAccountsDefaults is the shared set of fields applied to every
// row of a BatchCreateAccounts call unless the per-row item overrides them.
// Mirrors CreateRequest minus Name + Credential, which are per-row required.
type BatchCreateAccountsDefaults struct {
	ProviderID     int
	RuntimeClass   RuntimeClass
	UpstreamClient *string
	GroupID        *int
	ProxyID        *string
	Priority       *int
	Weight         *float32
	RiskLevel      *string
	Metadata       map[string]any
}

// BatchAccountItem is one row in a BatchCreateAccounts call. Name + Credential
// are mandatory per row; GroupID / Priority / Weight, when non-nil, override
// the defaults on this row only.
type BatchAccountItem struct {
	Name       string
	Credential map[string]any
	GroupID    *int
	Priority   *int
	Weight     *float32
}

// BatchCreateAccountResult is per-row outcome from BatchCreateAccounts. On
// success, AccountID is set and Error is empty; on validation/dedup/store
// failure, AccountID is nil and Error carries the message. Order matches
// the request.
type BatchCreateAccountResult struct {
	Index     int
	Name      string
	AccountID *int
	Error     string
}

// BatchDeleteAccountResult is per-row outcome from BatchDeleteAccounts.
// Order matches the request. Error is empty on a successful delete (or
// when the row was already gone — idempotent semantics: NotFound is NOT
// surfaced as a failure since the caller's intent is already achieved).
// Any other store/validation failure surfaces in Error without aborting
// the batch.
type BatchDeleteAccountResult struct {
	Index     int
	AccountID int
	Error     string
}

// BatchGroupMemberResult is per-row outcome from
// BatchAddAccountsToGroup / BatchRemoveAccountsFromGroup. Order matches
// the request. Error is empty on success OR on the idempotent case (already
// member / not member), so the caller can read "no errors" as "every
// requested membership is now in the desired state". Per-row store
// failures (account not found, group not found, store conflict) surface
// in Error without aborting the batch.
type BatchGroupMemberResult struct {
	Index     int
	AccountID int
	Error     string
}

// BatchUpdateConcurrencyItem is one row in a BatchUpdateConcurrency call:
// the per-account target max-concurrency ceiling (stored in account metadata
// under "max_concurrency", which the scheduler reads at admission). Mirrors
// sub2api's BatchUpdateConcurrency (admin_service.go), only there it lived on
// users — srapi's equivalent ceiling lives in provider account metadata, so
// the per-row identifier is account_id.
type BatchUpdateConcurrencyItem struct {
	AccountID      int
	MaxConcurrency int
}

// BatchUpdateConcurrencyResult is per-row outcome from BatchUpdateConcurrency.
// Order matches the request. Error is empty on a successful update (and on
// a missing-row idempotent skip, matching BatchDeleteAccountResult). Per-row
// failures (invalid id, store/validation error) surface in Error without
// aborting the batch.
type BatchUpdateConcurrencyResult struct {
	Index     int
	AccountID int
	Error     string
}

// BatchSetGroupRateMultiplierItem is one row in a BatchSetGroupRateMultipliers
// call: the per-account-group billing rate multiplier (e.g. 0.5 = 50% off,
// 1.5 = 50% surcharge). Verbatim port of sub2api's BatchSetGroupRateMultipliers
// (admin_service.go) — sub2api scoped this to user-groups since rate
// multipliers live on the user-group object there; srapi's AccountGroup
// carries the rate_multiplier field, so the per-row identifier is group_id.
type BatchSetGroupRateMultiplierItem struct {
	GroupID    int
	Multiplier string
}

// BatchSetGroupRateMultiplierResult is per-row outcome — same shape as the
// other batch-result structs so the admin UI can render mixed outcomes.
type BatchSetGroupRateMultiplierResult struct {
	Index   int
	GroupID int
	Error   string
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
	RiskLevel      *string
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
	RiskLevel      *string
	UpstreamClient **string
}

type CreateGroupRequest struct {
	Name           string
	Description    string
	ProviderScope  map[string]any
	ModelScope     map[string]any
	StrategyHint   *string
	RateMultiplier *string
	Status         *GroupStatus
}

type UpdateGroupRequest struct {
	Name           *string
	Description    *string
	ProviderScope  *map[string]any
	ModelScope     *map[string]any
	StrategyHint   *string
	RateMultiplier *string
	Status         *GroupStatus
}

type CreateProxyRequest struct {
	Name        string
	Type        ProxyType
	URL         string
	Status      *ProxyStatus
	Metadata    map[string]any
	CountryCode *string
	CountryName *string
}

type UpdateProxyRequest struct {
	Name        *string
	Type        *ProxyType
	URL         *string
	Status      *ProxyStatus
	Metadata    *map[string]any
	CountryCode *string
	CountryName *string
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
	RiskLevel            *string
	UpstreamClient       *string
}

type CreateStoredProxy struct {
	Name          string
	Type          ProxyType
	URLCiphertext string
	URLVersion    string
	Status        ProxyStatus
	Metadata      map[string]any
	CountryCode   string
	CountryName   string
}

type CreateStoredAccountGroup struct {
	Name           string
	Description    string
	ProviderScope  map[string]any
	ModelScope     map[string]any
	StrategyHint   string
	RateMultiplier string
	Status         GroupStatus
}

type Store interface {
	Create(ctx context.Context, input CreateStoredAccount) (ProviderAccount, error)
	Update(ctx context.Context, account ProviderAccount) (ProviderAccount, error)
	FindByID(ctx context.Context, id int) (ProviderAccount, error)
	List(ctx context.Context) ([]ProviderAccount, error)
	ListActiveByProviderIDs(ctx context.Context, providerIDs []int) ([]ProviderAccount, error)
	CreateProxy(ctx context.Context, input CreateStoredProxy) (ProxyDefinition, error)
	UpdateProxy(ctx context.Context, proxy ProxyDefinition) (ProxyDefinition, error)
	FindProxyByID(ctx context.Context, id int) (ProxyDefinition, error)
	ListProxies(ctx context.Context) ([]ProxyDefinition, error)
	SoftDeleteProxy(ctx context.Context, id int) error
	CreateGroup(ctx context.Context, input CreateStoredAccountGroup) (AccountGroup, error)
	UpdateGroup(ctx context.Context, group AccountGroup) (AccountGroup, error)
	FindGroupByID(ctx context.Context, id int) (AccountGroup, error)
	FindGroupsByID(ctx context.Context, ids []int) ([]AccountGroup, error)
	ListGroups(ctx context.Context) ([]AccountGroup, error)
	DeleteGroup(ctx context.Context, id int) error
	AddAccountToGroup(ctx context.Context, accountID int, groupID int) (AccountGroupMember, error)
	RemoveAccountFromGroup(ctx context.Context, accountID int, groupID int) error
	ListGroupMembers(ctx context.Context, groupID int) ([]AccountGroupMember, error)
	ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error)
	ListGroupIDsByAccounts(ctx context.Context, accountIDs []int) (map[int][]int, error)
	RecordHealthSnapshot(ctx context.Context, snapshot AccountHealthSnapshot) (AccountHealthSnapshot, error)
	LatestHealthSnapshotByAccount(ctx context.Context, accountID int) (AccountHealthSnapshot, error)
	ListHealthSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]AccountHealthSnapshot, error)
	RecordQuotaSnapshot(ctx context.Context, snapshot AccountQuotaSnapshot) (AccountQuotaSnapshot, error)
	ListQuotaSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]AccountQuotaSnapshot, error)
	Delete(ctx context.Context, id int) error
}

// BatchSnapshotReader is an optional Store capability that resolves the latest
// health and quota snapshots for many accounts in a constant number of
// queries. The gateway scheduling hot path uses it to assemble candidate
// runtime state without one query per candidate account; stores that do not
// implement it fall back to the per-account readers.
type BatchSnapshotReader interface {
	// LatestHealthSnapshotsByAccounts returns each account's most recent
	// health snapshot keyed by account ID; accounts without snapshots are
	// absent.
	LatestHealthSnapshotsByAccounts(ctx context.Context, accountIDs []int) (map[int]AccountHealthSnapshot, error)
	// LatestQuotaSnapshotsByAccounts returns, per account, the most recent
	// quota snapshot of each quota type (the batched equivalent of
	// ListQuotaSnapshotsByAccount with limit 1).
	LatestQuotaSnapshotsByAccounts(ctx context.Context, accountIDs []int) (map[int][]AccountQuotaSnapshot, error)
}

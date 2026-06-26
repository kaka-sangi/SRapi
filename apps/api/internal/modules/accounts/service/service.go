package service

import (
	"context"
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/singleflight"
)

const credentialVersionV1 = "v1"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store     contract.Store
	masterKey []byte
	clock     Clock
	// proxyLocks serializes the (FindProxyByID → mutate counters → UpdateProxy)
	// sequence inside RecordProxyProbe per proxy id so two concurrent probes
	// can't both observe the same "no reset needed" state and both write back,
	// dropping one increment on the SQL backend. Keyed by proxy id; values are
	// *sync.Mutex.
	proxyLocks sync.Map
	// refreshLocks does the same for RefreshAccessToken so the worker + the
	// manual /admin/accounts/{id}/refresh endpoint can't race on the same
	// account row. Keyed by account id; values are *sync.Mutex.
	refreshLocks sync.Map
	// decryptGroup coalesces concurrent DecryptCredential calls for the same
	// account ID so the DB lookup + AES-GCM decrypt runs at most once per
	// in-flight window. Ported from sub2api's singleflight pattern.
	decryptGroup singleflight.Group
}

func New(store contract.Store, masterKey string, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if len(masterKey) < 32 {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	derivedKey, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store, masterKey: derivedKey, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, req contract.CreateRequest) (contract.ProviderAccount, error) {
	name := strings.TrimSpace(req.Name)
	if req.ProviderID <= 0 || name == "" || !validRuntimeClass(req.RuntimeClass) {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	if len(req.Credential) == 0 {
		return contract.ProviderAccount{}, ErrCredentialMissing
	}
	credentialCiphertext, err := s.encryptCredential(req.Credential)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	status := contract.StatusActive
	if req.Status != nil {
		status = *req.Status
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	weight := float32(1.0)
	if req.Weight != nil {
		weight = *req.Weight
	}
	riskLevel := "normal"
	if req.RiskLevel != nil {
		normalized, ok := normalizeRiskLevel(*req.RiskLevel)
		if !ok {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		riskLevel = normalized
	}
	proxyID, err := s.normalizeAvailableProxyID(ctx, req.ProxyID)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	concurrency := 3
	if req.Concurrency != nil && *req.Concurrency > 0 {
		concurrency = *req.Concurrency
	}
	rateMultiplier := 1.0
	if req.RateMultiplier != nil && *req.RateMultiplier >= 0 {
		rateMultiplier = *req.RateMultiplier
	}
	autoPause := true
	if req.AutoPauseOnExpired != nil {
		autoPause = *req.AutoPauseOnExpired
	}
	notes := ""
	if req.Notes != nil {
		notes = strings.TrimSpace(*req.Notes)
	}

	stored, err := s.store.Create(ctx, contract.CreateStoredAccount{
		ProviderID:           req.ProviderID,
		Name:                 name,
		Platform:             strings.TrimSpace(req.Platform),
		AccountType:          contract.RuntimeClassToAccountType(req.RuntimeClass),
		RuntimeClass:         req.RuntimeClass,
		CredentialCiphertext: credentialCiphertext,
		CredentialVersion:    credentialVersionV1,
		Metadata:             CanonicalizeAccountMetadata(cloneMap(req.Metadata)),
		Extra:                cloneMap(req.Extra),
		ProxyID:              proxyID,
		Status:               status,
		Priority:             priority,
		Weight:               weight,
		RiskLevel:            &riskLevel,
		UpstreamClient:       req.UpstreamClient,
		Notes:                notes,
		Concurrency:          concurrency,
		RateMultiplier:       rateMultiplier,
		LoadFactor:           cloneIntPtr(req.LoadFactor),
		Schedulable:          true,
		ExpiresAt:            req.ExpiresAt,
		AutoPauseOnExpired:   autoPause,
	})
	if err != nil {
		return contract.ProviderAccount{}, err
	}

	// Mixed channel check + group binding.
	platform := strings.TrimSpace(req.Platform)
	for _, groupID := range req.GroupIDs {
		if groupID <= 0 {
			continue
		}
		if platform != "" && !req.SkipMixedChannelCheck {
			if conflict := s.checkMixedChannel(ctx, groupID, platform); conflict != "" {
				return contract.ProviderAccount{}, &MixedChannelError{
					GroupID:         groupID,
					AccountPlatform: platform,
					ExistingPlatform: conflict,
				}
			}
		}
		_, _ = s.store.AddAccountToGroup(ctx, stored.ID, groupID)
	}

	return stored, nil
}

func (s *Service) checkMixedChannel(ctx context.Context, groupID int, platform string) string {
	members, err := s.store.ListGroupMembers(ctx, groupID)
	if err != nil || len(members) == 0 {
		return ""
	}
	for _, member := range members {
		account, err := s.store.FindByID(ctx, member.AccountID)
		if err != nil {
			continue
		}
		if account.Platform != "" && account.Platform != platform {
			return account.Platform
		}
	}
	return ""
}

func cloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// BatchCreateAccountsMaxItems is the hard cap on the number of rows per
// BatchCreateAccounts call. Mirrors the OpenAPI maxItems on the request body.
const BatchCreateAccountsMaxItems = 1000

// BatchCreateAccounts inserts many provider accounts in one call against a
// shared `defaults` (provider, runtime_class, …) so a fleet rollout does not
// require N single-create requests. Dedupes by name within the batch (first
// occurrence wins) AND against existing non-deleted accounts. Per-row
// validation/dedup/store failures surface in result.Error without aborting
// the call — successful rows still apply.
//
// When defaults.GroupID is set (or a per-row override is set), the created
// account is also added to that group; a group-add failure flips the row
// into an Error outcome but does NOT roll back the account create — same
// best-effort semantics the bulk import uses.
//
// Returns an outer error only for catastrophic preconditions (empty items,
// too many items, defaults invalid before any item is touched).
func (s *Service) BatchCreateAccounts(ctx context.Context, defaults contract.BatchCreateAccountsDefaults, items []contract.BatchAccountItem) ([]contract.BatchCreateAccountResult, error) {
	if len(items) == 0 {
		return nil, ErrInvalidInput
	}
	if len(items) > BatchCreateAccountsMaxItems {
		return nil, ErrInvalidInput
	}
	if defaults.ProviderID <= 0 || !validRuntimeClass(defaults.RuntimeClass) {
		return nil, ErrInvalidInput
	}
	if defaults.RiskLevel != nil {
		if _, ok := normalizeRiskLevel(*defaults.RiskLevel); !ok {
			return nil, ErrInvalidInput
		}
	}
	if defaults.GroupID != nil && *defaults.GroupID <= 0 {
		return nil, ErrInvalidInput
	}

	existing, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]struct{}, len(existing))
	for _, account := range existing {
		taken[strings.ToLower(strings.TrimSpace(account.Name))] = struct{}{}
	}

	out := make([]contract.BatchCreateAccountResult, len(items))
	for i, item := range items {
		name := strings.TrimSpace(item.Name)
		row := contract.BatchCreateAccountResult{Index: i, Name: name}
		if name == "" {
			row.Error = "name required"
			out[i] = row
			continue
		}
		if len(item.Credential) == 0 {
			row.Error = "credential required"
			out[i] = row
			continue
		}
		key := strings.ToLower(name)
		if _, dup := taken[key]; dup {
			row.Error = "duplicate name"
			out[i] = row
			continue
		}
		// Per-row overrides win over defaults.
		priority := defaults.Priority
		if item.Priority != nil {
			priority = item.Priority
		}
		weight := defaults.Weight
		if item.Weight != nil {
			weight = item.Weight
		}
		groupID := defaults.GroupID
		if item.GroupID != nil {
			groupID = item.GroupID
		}
		created, createErr := s.Create(ctx, contract.CreateRequest{
			ProviderID:     defaults.ProviderID,
			Name:           name,
			RuntimeClass:   defaults.RuntimeClass,
			Credential:     item.Credential,
			Metadata:       cloneMap(defaults.Metadata),
			ProxyID:        defaults.ProxyID,
			Priority:       priority,
			Weight:         weight,
			RiskLevel:      defaults.RiskLevel,
			UpstreamClient: defaults.UpstreamClient,
		})
		if createErr != nil {
			row.Error = createErr.Error()
			out[i] = row
			continue
		}
		taken[key] = struct{}{}
		accountID := created.ID
		row.AccountID = &accountID
		if groupID != nil && *groupID > 0 {
			if _, addErr := s.AddAccountToGroup(ctx, created.ID, *groupID); addErr != nil {
				// Flip the row to an error so the operator sees why this row
				// did not end up in the requested group, but keep AccountID set
				// — the account itself was created and is usable on its own.
				row.Error = "added to provider but failed to add to group: " + addErr.Error()
			}
		}
		out[i] = row
	}
	return out, nil
}

// BatchDeleteAccountsMaxItems caps the number of ids per BatchDeleteAccounts
// call. Mirrors BatchCreateAccountsMaxItems so the two batch surfaces share
// the same operator-facing limit.
const BatchDeleteAccountsMaxItems = 1000

// BatchDeleteAccounts soft-deletes N accounts in one call, returning a
// per-row result. The operation is best-effort across the batch — a
// failure on any single row populates that row's Error without aborting
// the rest. NotFound is treated as success (idempotent: the caller's
// intent of "this id should not exist" is already true).
//
// The outer error is reserved for catastrophic precondition failures (zero
// ids, more than BatchDeleteAccountsMaxItems, duplicates within the batch).
// Per-row store/validation failures surface in the result slice.
//
// Dedups ids within the batch (first occurrence wins) so an accidental
// double-id doesn't surface as a second NotFound.
func (s *Service) BatchDeleteAccounts(ctx context.Context, ids []int) ([]contract.BatchDeleteAccountResult, error) {
	if len(ids) == 0 {
		return nil, ErrInvalidInput
	}
	if len(ids) > BatchDeleteAccountsMaxItems {
		return nil, ErrInvalidInput
	}
	results := make([]contract.BatchDeleteAccountResult, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for i, id := range ids {
		row := contract.BatchDeleteAccountResult{Index: i, AccountID: id}
		if id <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if _, dup := seen[id]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[id] = struct{}{}
		if err := s.Delete(ctx, id); err != nil {
			// Idempotent: a row that's already gone is not a failure
			// for the caller's intent. Match both the typed sentinel
			// AND the ad-hoc "account not found" string the memory
			// store returns today (entstore returns the typed error;
			// memory store still uses errors.New — coordinated
			// migration not worth the blast radius for this one path).
			if errors.Is(err, ErrAccountNotFound) || strings.Contains(err.Error(), "account not found") {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

// BatchUpdateConcurrencyMaxItems caps the number of items per
// BatchUpdateConcurrency call. Mirrors the other batch-account caps so the
// operator-facing surface is consistent.
const BatchUpdateConcurrencyMaxItems = 1000

// BatchUpdateConcurrency sets the per-account max_concurrency ceiling on N
// provider accounts in one call. Verbatim port of sub2api's
// BatchUpdateConcurrency (admin_service.go) — sub2api scoped this to users
// since rate-limit caps live on the user object there; srapi's equivalent
// per-account ceiling lives in the account's metadata blob (the scheduler
// reads metadata["max_concurrency"] at admission, see
// runtime_gateway_resolution.go), so the per-row identifier is account_id
// instead of user_id. Otherwise the loop shape, idempotent-NotFound +
// per-row failure surfacing matches batch-delete (0a3c2586).
//
// Best-effort across the batch: a single-row failure populates that row's
// Error and the rest of the batch continues. NotFound is treated as success
// (the caller's intent — "this id should have concurrency X" — is moot if
// the row does not exist). Dedups ids within the batch (first occurrence
// wins).
//
// Outer error is reserved for precondition failures (empty input, > max
// items). Per-row store / validation failures stay in the result slice.
func (s *Service) BatchUpdateConcurrency(ctx context.Context, items []contract.BatchUpdateConcurrencyItem) ([]contract.BatchUpdateConcurrencyResult, error) {
	if len(items) == 0 {
		return nil, ErrInvalidInput
	}
	if len(items) > BatchUpdateConcurrencyMaxItems {
		return nil, ErrInvalidInput
	}
	results := make([]contract.BatchUpdateConcurrencyResult, 0, len(items))
	seen := make(map[int]struct{}, len(items))
	for i, item := range items {
		row := contract.BatchUpdateConcurrencyResult{Index: i, AccountID: item.AccountID}
		if item.AccountID <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if item.MaxConcurrency < 0 {
			row.Error = "max_concurrency must be >= 0"
			results = append(results, row)
			continue
		}
		if _, dup := seen[item.AccountID]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[item.AccountID] = struct{}{}
		concurrency := item.MaxConcurrency
		if _, err := s.Update(ctx, item.AccountID, contract.UpdateRequest{Concurrency: &concurrency}); err != nil {
			if errors.Is(err, ErrAccountNotFound) || strings.Contains(err.Error(), "account not found") {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

// BatchRefreshAccountsMaxItems caps the number of accounts per BatchRefreshAccounts
// call. Mirrors the other batch-account caps so the operator-facing surface stays
// consistent (1000 rows / call).
const BatchRefreshAccountsMaxItems = 1000

// BatchRefreshAccounts triggers an OAuth refresh against N accounts in one
// call. Verbatim port of sub2api's AccountHandler.BatchRefresh
// (account_handler.go): sub2api fans out via errgroup with maxConcurrency=10
// and collects per-row outcomes. srapi reuses RefreshAccessTokenWithOutcome
// (the same path the single-account /admin/accounts/{id}/refresh endpoint
// uses) so the bookkeeping rules (refresh_attempts / needs_reauth_at /
// token_expires_at) stay in one place. Per-row failures surface in
// results[i].Error without aborting the batch. NotFound is idempotent
// (matching BatchDeleteAccountResult), and non-OAuth runtime classes are
// rejected per-row (the upstream handler-level gate is duplicated here so the
// caller does not pre-filter).
//
// Outer error is reserved for precondition failures (empty input, > max
// items, nil refresher).
func (s *Service) BatchRefreshAccounts(ctx context.Context, ids []int, refresher AccountRefresher) ([]contract.BatchRefreshAccountResult, error) {
	if len(ids) == 0 {
		return nil, ErrInvalidInput
	}
	if len(ids) > BatchRefreshAccountsMaxItems {
		return nil, ErrInvalidInput
	}
	if refresher == nil {
		return nil, ErrInvalidInput
	}
	results := make([]contract.BatchRefreshAccountResult, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for i, id := range ids {
		row := contract.BatchRefreshAccountResult{Index: i, AccountID: id}
		if id <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if _, dup := seen[id]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[id] = struct{}{}
		// Pre-flight the row so we can apply the idempotent-NotFound rule and
		// reject non-OAuth rows before paying the refresher round-trip.
		account, err := s.store.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrAccountNotFound) || strings.Contains(err.Error(), "account not found") {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		if account.RuntimeClass != contract.RuntimeClassOauthRefresh && account.RuntimeClass != contract.RuntimeClassOauthDeviceCode {
			row.Error = "account is not an oauth runtime class"
			results = append(results, row)
			continue
		}
		outcome, refreshErr := s.RefreshAccessTokenWithOutcome(ctx, id, refresher)
		row.OutcomeClass = string(outcome.Class)
		row.Attempts = outcome.Attempts
		row.NeedsReauthFlipped = outcome.NeedsReauthFlipped
		if refreshErr != nil {
			row.Error = refreshErr.Error()
		}
		results = append(results, row)
	}
	return results, nil
}

// BatchUpdateAccountCredentialsMaxItems caps the number of items per
// BatchUpdateAccountCredentials call. Mirrors the other batch-account caps so
// the operator-facing surface stays consistent (1000 rows / call).
const BatchUpdateAccountCredentialsMaxItems = 1000

// BatchUpdateAccountCredentials rotates the stored credential on N accounts in
// one call, each row carrying its own partial-credential patch. Verbatim port
// of sub2api's BatchUpdateCredentials (account_handler.go) — sub2api applied
// a single shared {field, value} across the whole batch; srapi accepts a
// per-row credential patch so a single call can rotate disjoint fields
// (refresh_token on one account, api_key on another) without N round trips.
//
// Per-row: load current credential → merge patch on top → persist via
// Service.Update with the merged Credential. Empty patches are rejected
// per-row (a no-op silent succeed would mask operator mistakes). NotFound is
// idempotent (matching BatchDeleteAccountResult), and per-row store /
// encryption failures surface in results[i].Error without aborting the
// batch.
//
// Outer error is reserved for precondition failures (empty input, > max
// items). The audit snapshot recorded by the HTTP handler covers
// requested/succeeded/failed counts only — credential bytes are NEVER
// included.
func (s *Service) BatchUpdateAccountCredentials(ctx context.Context, items []contract.BatchUpdateAccountCredentialItem) ([]contract.BatchUpdateAccountCredentialResult, error) {
	if len(items) == 0 {
		return nil, ErrInvalidInput
	}
	if len(items) > BatchUpdateAccountCredentialsMaxItems {
		return nil, ErrInvalidInput
	}
	results := make([]contract.BatchUpdateAccountCredentialResult, 0, len(items))
	seen := make(map[int]struct{}, len(items))
	for i, item := range items {
		row := contract.BatchUpdateAccountCredentialResult{Index: i, AccountID: item.AccountID}
		if item.AccountID <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if len(item.Credential) == 0 {
			row.Error = "credential patch is empty"
			results = append(results, row)
			continue
		}
		if _, dup := seen[item.AccountID]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[item.AccountID] = struct{}{}
		account, err := s.store.FindByID(ctx, item.AccountID)
		if err != nil {
			if errors.Is(err, ErrAccountNotFound) || strings.Contains(err.Error(), "account not found") {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		current, err := s.decryptCredential(account.CredentialCiphertext)
		if err != nil {
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		merged := cloneMap(current)
		if merged == nil {
			merged = map[string]any{}
		}
		for k, v := range item.Credential {
			merged[k] = v
		}
		if _, err := s.Update(ctx, item.AccountID, contract.UpdateRequest{Credential: &merged}); err != nil {
			if errors.Is(err, ErrAccountNotFound) || strings.Contains(err.Error(), "account not found") {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

func (s *Service) List(ctx context.Context) ([]contract.ProviderAccount, error) {
	accounts, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, account)
	}
	return out, nil
}

// ListPage delegates the admin-list read to the store's PageReader so the
// (filter, count, slice) trio executes against SQL instead of pulling every
// row into Go memory. Stores that omit PageReader (mostly test doubles) get a
// full-table fallback with the same predicates applied in-process, so the
// service contract stays uniform.
func (s *Service) ListPage(ctx context.Context, filter contract.ListFilter, limit, offset int) (contract.ListPageResult, error) {
	if offset < 0 {
		offset = 0
	}
	if reader, ok := s.store.(contract.PageReader); ok {
		return reader.ListPage(ctx, filter, limit, offset)
	}

	all, err := s.store.List(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}

	var inGroup map[int]struct{}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		members, mErr := s.store.ListGroupMembers(ctx, *filter.GroupID)
		if mErr != nil {
			return contract.ListPageResult{}, mErr
		}
		inGroup = make(map[int]struct{}, len(members))
		for _, m := range members {
			inGroup[m.AccountID] = struct{}{}
		}
	}

	matched := make([]contract.ProviderAccount, 0, len(all))
	for _, account := range all {
		if !accountFilterFallbackMatches(account, filter) {
			continue
		}
		if inGroup != nil {
			if _, ok := inGroup[account.ID]; !ok {
				continue
			}
		}
		matched = append(matched, account)
	}
	// Store.List returns ascending by id; flip to newest-first to mirror the
	// PageReader contract.
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
	total := len(matched)
	if offset >= total {
		return contract.ListPageResult{Items: []contract.ProviderAccount{}, Total: total}, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return contract.ListPageResult{Items: matched[offset:end], Total: total}, nil
}

// accountFilterFallbackMatches mirrors the SQL/memory predicates for stores
// that lack PageReader. Kept here rather than in the contract so it does not
// add a runtime-data dependency to the public interface.
func accountFilterFallbackMatches(account contract.ProviderAccount, filter contract.ListFilter) bool {
	if filter.Status == "" {
		if !filter.IncludeArchived && account.Status == contract.StatusArchived {
			return false
		}
	} else if account.Status != filter.Status {
		return false
	}
	if filter.ProviderID != nil && account.ProviderID != *filter.ProviderID {
		return false
	}
	if filter.RuntimeClass != "" && account.RuntimeClass != filter.RuntimeClass {
		return false
	}
	if search := strings.ToLower(strings.TrimSpace(filter.Search)); search != "" {
		if !accountFilterFallbackMatchesSearch(account, search) {
			return false
		}
	}
	return true
}

func accountFilterFallbackMatchesSearch(account contract.ProviderAccount, search string) bool {
	if strings.Contains(strings.ToLower(account.Name), search) {
		return true
	}
	if account.UpstreamClient != nil && strings.Contains(strings.ToLower(strings.TrimSpace(*account.UpstreamClient)), search) {
		return true
	}
	if isAllDigitsFallback(search) && search == strconv.Itoa(account.ID) {
		return true
	}
	return false
}

func isAllDigitsFallback(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Service) ListActiveByProviderIDs(ctx context.Context, providerIDs []int) ([]contract.ProviderAccount, error) {
	ids := normalizePositiveIDs(providerIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	accounts, err := s.store.ListActiveByProviderIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, account)
	}
	return out, nil
}

func (s *Service) ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error) {
	if accountID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListGroupIDsByAccount(ctx, accountID)
}

func (s *Service) ListGroupIDsByAccounts(ctx context.Context, accountIDs []int) (map[int][]int, error) {
	ids := normalizePositiveIDs(accountIDs)
	if len(ids) == 0 {
		return map[int][]int{}, nil
	}
	return s.store.ListGroupIDsByAccounts(ctx, ids)
}

func (s *Service) Delete(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}
func (s *Service) CreateGroup(ctx context.Context, req contract.CreateGroupRequest) (contract.AccountGroup, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return contract.AccountGroup{}, ErrInvalidInput
	}
	strategy := "balanced"
	if req.StrategyHint != nil {
		strategy = strings.TrimSpace(*req.StrategyHint)
		if strategy == "" {
			return contract.AccountGroup{}, ErrInvalidInput
		}
	}
	status := contract.GroupStatusActive
	if req.Status != nil {
		if !validGroupStatus(*req.Status) {
			return contract.AccountGroup{}, ErrInvalidInput
		}
		status = *req.Status
	}
	rateMultiplier, ok := normalizeRateMultiplier(req.RateMultiplier)
	if !ok {
		return contract.AccountGroup{}, ErrInvalidInput
	}
	return s.store.CreateGroup(ctx, contract.CreateStoredAccountGroup{
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		ProviderScope:  cloneMap(req.ProviderScope),
		ModelScope:     cloneMap(req.ModelScope),
		StrategyHint:   strategy,
		RateMultiplier: rateMultiplier,
		Status:         status,
	})
}

func (s *Service) UpdateGroup(ctx context.Context, id int, req contract.UpdateGroupRequest) (contract.AccountGroup, error) {
	if id <= 0 {
		return contract.AccountGroup{}, ErrInvalidInput
	}
	group, err := s.store.FindGroupByID(ctx, id)
	if err != nil {
		return contract.AccountGroup{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.AccountGroup{}, ErrInvalidInput
		}
		group.Name = name
	}
	if req.Description != nil {
		group.Description = strings.TrimSpace(*req.Description)
	}
	if req.ProviderScope != nil {
		group.ProviderScope = cloneMap(*req.ProviderScope)
	}
	if req.ModelScope != nil {
		group.ModelScope = cloneMap(*req.ModelScope)
	}
	if req.StrategyHint != nil {
		strategy := strings.TrimSpace(*req.StrategyHint)
		if strategy == "" {
			return contract.AccountGroup{}, ErrInvalidInput
		}
		group.StrategyHint = strategy
	}
	if req.RateMultiplier != nil {
		rateMultiplier, ok := normalizeRateMultiplier(req.RateMultiplier)
		if !ok {
			return contract.AccountGroup{}, ErrInvalidInput
		}
		group.RateMultiplier = rateMultiplier
	}
	if req.Status != nil {
		if !validGroupStatus(*req.Status) {
			return contract.AccountGroup{}, ErrInvalidInput
		}
		group.Status = *req.Status
	}
	group.UpdatedAt = s.clock.Now()
	return s.store.UpdateGroup(ctx, group)
}

// BatchSetGroupRateMultipliersMaxItems caps the number of items per
// BatchSetGroupRateMultipliers call. Mirrors the other batch-op caps for a
// consistent operator-facing surface.
const BatchSetGroupRateMultipliersMaxItems = 1000

// BatchSetGroupRateMultipliers sets `rate_multiplier` on N account groups in
// one call. Verbatim port of sub2api's BatchSetGroupRateMultipliers — sub2api
// scoped the multiplier to user-groups (the consumer side) but srapi stores
// the rate_multiplier on AccountGroup (the provider scheduling group), so
// the per-row identifier is account_group_id.
//
// Best-effort across the batch: a single-row failure populates that row's
// Error and the rest continues. NotFound is idempotent — a missing id counts
// as success since the caller's intent ("this group's multiplier should be X")
// is moot. Dedups within the batch.
//
// Per-row validation enforces sub2api's "multiplier must be > 0" check (the
// sub2api impl uses RateMultiplier <= 0; the float reject below covers the
// same surface for srapi's string-decimal representation).
func (s *Service) BatchSetGroupRateMultipliers(ctx context.Context, items []contract.BatchSetGroupRateMultiplierItem) ([]contract.BatchSetGroupRateMultiplierResult, error) {
	if len(items) == 0 {
		return nil, ErrInvalidInput
	}
	if len(items) > BatchSetGroupRateMultipliersMaxItems {
		return nil, ErrInvalidInput
	}
	results := make([]contract.BatchSetGroupRateMultiplierResult, 0, len(items))
	seen := make(map[int]struct{}, len(items))
	for i, item := range items {
		row := contract.BatchSetGroupRateMultiplierResult{Index: i, GroupID: item.GroupID}
		if item.GroupID <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if _, dup := seen[item.GroupID]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[item.GroupID] = struct{}{}
		multiplier := strings.TrimSpace(item.Multiplier)
		if multiplier == "" {
			row.Error = "rate_multiplier must be > 0"
			results = append(results, row)
			continue
		}
		// sub2api rejects RateMultiplier <= 0. Mirror that on the string-decimal
		// representation using the same normalizer the per-group UpdateGroup
		// uses, then reject zero.
		normalized, ok := normalizeRateMultiplier(&multiplier)
		if !ok {
			row.Error = "invalid rate_multiplier"
			results = append(results, row)
			continue
		}
		// Zero is forbidden by sub2api (multiplier > 0). normalizeRateMultiplier
		// permits zero (it allows non-negative), so re-check here.
		zero, ok := new(big.Rat).SetString(normalized)
		if !ok || zero.Sign() <= 0 {
			row.Error = "rate_multiplier must be > 0"
			results = append(results, row)
			continue
		}
		if _, err := s.UpdateGroup(ctx, item.GroupID, contract.UpdateGroupRequest{RateMultiplier: &normalized}); err != nil {
			// Idempotent: NotFound is not a failure. Memory store still uses
			// errors.New("account group not found") — match both the typed
			// sentinel (via store) and the string for safety.
			if strings.Contains(err.Error(), "account group not found") || strings.Contains(err.Error(), "group not found") {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

func (s *Service) FindGroupByID(ctx context.Context, id int) (contract.AccountGroup, error) {
	if id <= 0 {
		return contract.AccountGroup{}, ErrInvalidInput
	}
	return s.store.FindGroupByID(ctx, id)
}

func (s *Service) FindGroupsByID(ctx context.Context, ids []int) ([]contract.AccountGroup, error) {
	normalized := normalizePositiveIDs(ids)
	if len(normalized) == 0 {
		return nil, ErrInvalidInput
	}
	return s.store.FindGroupsByID(ctx, normalized)
}

func (s *Service) ListGroups(ctx context.Context) ([]contract.AccountGroup, error) {
	return s.store.ListGroups(ctx)
}

func (s *Service) DeleteGroup(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if _, err := s.store.FindGroupByID(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteGroup(ctx, id)
}

func (s *Service) AddAccountToGroup(ctx context.Context, accountID int, groupID int) (contract.AccountGroupMember, error) {
	if accountID <= 0 || groupID <= 0 {
		return contract.AccountGroupMember{}, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, accountID); err != nil {
		return contract.AccountGroupMember{}, err
	}
	if _, err := s.store.FindGroupByID(ctx, groupID); err != nil {
		return contract.AccountGroupMember{}, err
	}
	return s.store.AddAccountToGroup(ctx, accountID, groupID)
}

func (s *Service) RemoveAccountFromGroup(ctx context.Context, accountID int, groupID int) error {
	if accountID <= 0 || groupID <= 0 {
		return ErrInvalidInput
	}
	return s.store.RemoveAccountFromGroup(ctx, accountID, groupID)
}

// BatchGroupMembersMaxItems caps the number of account ids per
// BatchAddAccountsToGroup / BatchRemoveAccountsFromGroup call. Mirrors
// BatchCreateAccountsMaxItems + BatchDeleteAccountsMaxItems so every
// operator-facing batch surface shares one ceiling.
const BatchGroupMembersMaxItems = 1000

// BatchAddAccountsToGroup adds N accounts to one group in one call. The
// group is loaded once up-front (no N+1) and each row's add is best-effort
// — a per-row failure populates Error without aborting the rest.
// Idempotent on already-member rows: re-adding a member returns nil error
// instead of a "duplicate" failure since the caller's "this account should
// be in the group" intent is already satisfied.
//
// Dedups account ids within the batch (first occurrence wins) so an
// accidental double-id doesn't surface as a second add. Outer error is
// reserved for catastrophic precondition failures: zero ids, > MaxItems,
// group not found.
func (s *Service) BatchAddAccountsToGroup(ctx context.Context, groupID int, accountIDs []int) ([]contract.BatchGroupMemberResult, error) {
	if groupID <= 0 {
		return nil, ErrInvalidInput
	}
	if len(accountIDs) == 0 {
		return nil, ErrInvalidInput
	}
	if len(accountIDs) > BatchGroupMembersMaxItems {
		return nil, ErrInvalidInput
	}
	// Group must exist — every per-row add would otherwise fail with the
	// same "group not found" and the operator just gets noise.
	if _, err := s.store.FindGroupByID(ctx, groupID); err != nil {
		return nil, err
	}

	// Pre-fetch existing members so we can fast-path the idempotent
	// "already a member" case without a per-row store call.
	existing, err := s.store.ListGroupMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	alreadyMember := make(map[int]struct{}, len(existing))
	for _, m := range existing {
		alreadyMember[m.AccountID] = struct{}{}
	}

	results := make([]contract.BatchGroupMemberResult, 0, len(accountIDs))
	seen := make(map[int]struct{}, len(accountIDs))
	for i, accountID := range accountIDs {
		row := contract.BatchGroupMemberResult{Index: i, AccountID: accountID}
		if accountID <= 0 {
			row.Error = "invalid account id"
			results = append(results, row)
			continue
		}
		if _, dup := seen[accountID]; dup {
			row.Error = "duplicate account id in batch"
			results = append(results, row)
			continue
		}
		seen[accountID] = struct{}{}
		if _, already := alreadyMember[accountID]; already {
			// Idempotent: silent success — the desired membership state
			// is already in place.
			results = append(results, row)
			continue
		}
		if _, addErr := s.store.AddAccountToGroup(ctx, accountID, groupID); addErr != nil {
			// "account not found" + other store errors come back per-row
			// — operator sees which ids failed without losing the rest.
			row.Error = addErr.Error()
			results = append(results, row)
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

// BatchRemoveAccountsFromGroup is the sibling of BatchAddAccountsToGroup.
// Same idempotent semantics (not-a-member counts as success since the
// desired absence is already in place). Same per-row error handling.
func (s *Service) BatchRemoveAccountsFromGroup(ctx context.Context, groupID int, accountIDs []int) ([]contract.BatchGroupMemberResult, error) {
	if groupID <= 0 {
		return nil, ErrInvalidInput
	}
	if len(accountIDs) == 0 {
		return nil, ErrInvalidInput
	}
	if len(accountIDs) > BatchGroupMembersMaxItems {
		return nil, ErrInvalidInput
	}
	if _, err := s.store.FindGroupByID(ctx, groupID); err != nil {
		return nil, err
	}

	results := make([]contract.BatchGroupMemberResult, 0, len(accountIDs))
	seen := make(map[int]struct{}, len(accountIDs))
	for i, accountID := range accountIDs {
		row := contract.BatchGroupMemberResult{Index: i, AccountID: accountID}
		if accountID <= 0 {
			row.Error = "invalid account id"
			results = append(results, row)
			continue
		}
		if _, dup := seen[accountID]; dup {
			row.Error = "duplicate account id in batch"
			results = append(results, row)
			continue
		}
		seen[accountID] = struct{}{}
		if err := s.store.RemoveAccountFromGroup(ctx, accountID, groupID); err != nil {
			// Some stores return a typed not-found-member; others return
			// nil and just no-op. Match the convention used by the single
			// Remove — surface the error verbatim but let the caller treat
			// "not found" as idempotent at the handler/UI layer.
			if !strings.Contains(err.Error(), "not found") {
				row.Error = err.Error()
			}
		}
		results = append(results, row)
	}
	return results, nil
}

func (s *Service) ListGroupMembers(ctx context.Context, groupID int) ([]contract.AccountGroupMember, error) {
	if groupID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListGroupMembers(ctx, groupID)
}

func (s *Service) FindByID(ctx context.Context, id int) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	return s.store.FindByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int, req contract.UpdateRequest) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.Name = name
	}
	if req.RuntimeClass != nil {
		if !validRuntimeClass(*req.RuntimeClass) {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.RuntimeClass = *req.RuntimeClass
	}
	if req.Credential != nil {
		if len(*req.Credential) == 0 {
			return contract.ProviderAccount{}, ErrCredentialMissing
		}
		ciphertext, err := s.encryptCredential(*req.Credential)
		if err != nil {
			return contract.ProviderAccount{}, err
		}
		account.CredentialCiphertext = ciphertext
		account.CredentialVersion = credentialVersionV1
	}
	if req.Metadata != nil {
		merged := cloneMap(account.Metadata)
		if merged == nil {
			merged = map[string]any{}
		}
		for k, v := range *req.Metadata {
			merged[k] = v
		}
		account.Metadata = merged
	}
	if req.ProxyID != nil {
		proxyID, err := s.normalizeAvailableProxyID(ctx, *req.ProxyID)
		if err != nil {
			return contract.ProviderAccount{}, err
		}
		account.ProxyID = proxyID
	}
	if req.Status != nil {
		account.Status = *req.Status
	}
	if req.Priority != nil {
		account.Priority = *req.Priority
	}
	if req.Weight != nil {
		if *req.Weight < 0 {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.Weight = *req.Weight
	}
	if req.RiskLevel != nil {
		riskLevel, ok := normalizeRiskLevel(*req.RiskLevel)
		if !ok {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.RiskLevel = &riskLevel
	}
	if req.UpstreamClient != nil {
		account.UpstreamClient = cloneString(*req.UpstreamClient)
	}
	if req.Extra != nil {
		account.Extra = cloneMap(*req.Extra)
	}
	if req.Notes != nil {
		account.Notes = strings.TrimSpace(*req.Notes)
	}
	if req.Concurrency != nil {
		if *req.Concurrency < 0 {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.Concurrency = *req.Concurrency
	}
	if req.RateMultiplier != nil {
		if *req.RateMultiplier < 0 {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.RateMultiplier = *req.RateMultiplier
	}
	if req.LoadFactor != nil {
		account.LoadFactor = *req.LoadFactor
	}
	if req.ExpiresAt != nil {
		account.ExpiresAt = req.ExpiresAt
	}
	if req.ClearExpiresAt {
		account.ExpiresAt = nil
	}
	if req.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *req.AutoPauseOnExpired
	}
	if req.Schedulable != nil {
		account.Schedulable = *req.Schedulable
	}
	if req.RuntimeClass != nil {
		account.AccountType = contract.RuntimeClassToAccountType(account.RuntimeClass)
	}
	account.UpdatedAt = s.clock.Now()
	return s.persistAccount(ctx, account)
}

// persistAccount is the single mutation funnel for every ProviderAccount
// write. It canonicalizes metadata (alias keys → canonical names; see
// metadata_canonical.go) immediately before handing the row to the store so
// no legacy alias can leak past the service boundary regardless of which
// caller (admin Update, batch field patch, refresh worker, health probe,
// manual pause, …) initiated the write. Callers that build account state
// in-place must use this path instead of s.store.Update directly.
func (s *Service) persistAccount(ctx context.Context, account contract.ProviderAccount) (contract.ProviderAccount, error) {
	account.Metadata = CanonicalizeAccountMetadata(account.Metadata)
	return s.store.Update(ctx, account)
}

func (s *Service) BatchUpdateStatus(ctx context.Context, ids []int, status contract.Status) contract.BatchUpdateResult {
	return s.BatchUpdateFields(ctx, ids, contract.UpdateRequest{Status: &status})
}

// BatchUpdateFields applies a partial UpdateRequest across many provider
// accounts. Each non-nil field is passed through to Service.Update per id;
// per-account failures collect in Errors without aborting the batch. Callers
// requiring strict atomic semantics (all-or-nothing) must validate inputs
// up-front since this is a best-effort multi-call wrapper. Used by the
// admin /accounts/batch endpoint, which carries status + optional
// scheduler-tier fields (priority/weight/risk_level) in one request.
func (s *Service) BatchUpdateFields(ctx context.Context, ids []int, req contract.UpdateRequest) contract.BatchUpdateResult {
	result := contract.BatchUpdateResult{
		Updated: make([]contract.ProviderAccount, 0, len(ids)),
		Errors:  make([]string, 0),
	}
	if len(ids) == 0 {
		result.Errors = append(result.Errors, ErrInvalidInput.Error())
		return result
	}
	if req.Status != nil && !validAccountStatus(*req.Status) {
		result.Errors = append(result.Errors, ErrInvalidInput.Error())
		return result
	}
	for _, id := range ids {
		updated, err := s.Update(ctx, id, req)
		if err != nil {
			result.Errors = append(result.Errors, strings.TrimSpace(err.Error()))
			continue
		}
		result.Updated = append(result.Updated, updated)
	}
	return result
}

func (s *Service) BatchClearErrorState(ctx context.Context, ids []int) contract.BatchUpdateResult {
	return s.batchPerAccount(ctx, ids, s.ClearErrorState)
}

func (s *Service) BatchRecover(ctx context.Context, ids []int) contract.BatchUpdateResult {
	return s.batchPerAccount(ctx, ids, s.Recover)
}

func (s *Service) batchPerAccount(ctx context.Context, ids []int, op func(context.Context, int) (contract.ProviderAccount, error)) contract.BatchUpdateResult {
	result := contract.BatchUpdateResult{
		Updated: make([]contract.ProviderAccount, 0, len(ids)),
		Errors:  make([]string, 0),
	}
	if len(ids) == 0 {
		result.Errors = append(result.Errors, ErrInvalidInput.Error())
		return result
	}
	for _, id := range ids {
		updated, err := op(ctx, id)
		if err != nil {
			result.Errors = append(result.Errors, strings.TrimSpace(err.Error()))
			continue
		}
		result.Updated = append(result.Updated, updated)
	}
	return result
}

func (s *Service) ClearErrorState(ctx context.Context, id int) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	metadata := cloneMap(account.Metadata)
	for _, key := range []string{
		"cooldown_active",
		"cooldown_reason",
		"cooldown_until",
		"cooldown_strikes",
		"cooldown_last_at",
		"circuit_open",
		"last_error_at",
		"last_error_class",
		"last_error_message",
		"needs_reauth_reason",
		"quota_exhausted",
	} {
		delete(metadata, key)
	}
	metadata["last_error_cleared_at"] = s.clock.Now().Format(time.RFC3339)
	status := account.Status
	if status == contract.StatusDead || status == contract.StatusNeedsReauth || status == contract.StatusSuspended {
		status = contract.StatusActive
	}
	account.NeedsReauthAt = nil
	account.RefreshAttempts = 0
	account.RefreshLastError = ""
	account.Status = status
	account.Metadata = metadata
	account.UpdatedAt = s.clock.Now()
	return s.persistAccount(ctx, account)
}

func (s *Service) RPMStatus(ctx context.Context, accountID int) (contract.RPMStatus, error) {
	if accountID <= 0 {
		return contract.RPMStatus{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, accountID)
	if err != nil {
		return contract.RPMStatus{}, err
	}
	resetAt := metadataOptionalTime(account.Metadata, "rpm_reset_at", "rpm_window_reset_at")
	windowSeconds := metadataInt(account.Metadata, "rpm_window_seconds", "window_seconds")
	if windowSeconds <= 0 {
		windowSeconds = 60
	}
	return contract.RPMStatus{
		AccountID:     account.ID,
		RPMUsed:       metadataInt(account.Metadata, "rpm_used"),
		RPMLimit:      metadataOptionalInt(account.Metadata, "rpm_limit"),
		WindowSeconds: windowSeconds,
		ResetAt:       resetAt,
	}, nil
}

func (s *Service) ProxyQuality(ctx context.Context, accountID int) (contract.ProxyQuality, error) {
	if accountID <= 0 {
		return contract.ProxyQuality{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, accountID)
	if err != nil {
		return contract.ProxyQuality{}, err
	}
	snapshots, err := s.store.ListHealthSnapshotsByAccount(ctx, accountID, 50)
	if err != nil {
		return contract.ProxyQuality{}, err
	}
	quality := contract.ProxyQuality{
		AccountID: account.ID,
		ProxyID:   cloneString(account.ProxyID),
		Metadata:  proxyQualityMetadata(account.Metadata),
	}
	if len(snapshots) > 0 {
		latest := snapshots[0]
		quality.SuccessRate = clampRatio(latest.SuccessRate)
		quality.ErrorRate = clampRatio(latest.ErrorRate)
		quality.LatencyP95MS = latest.LatencyP95MS
		quality.SampleCount = len(snapshots)
		lastCheckedAt := latest.SnapshotAt
		quality.LastCheckedAt = &lastCheckedAt
		return quality, nil
	}
	quality.SuccessRate = metadataFloat32(account.Metadata, "proxy_success_rate", "success_rate")
	quality.ErrorRate = metadataFloat32(account.Metadata, "proxy_error_rate", "error_rate")
	quality.LatencyP95MS = metadataInt(account.Metadata, "proxy_latency_p95_ms", "latency_p95_ms", "p95_latency_ms")
	quality.SampleCount = metadataInt(account.Metadata, "proxy_sample_count", "sample_count")
	quality.LastCheckedAt = metadataOptionalTime(account.Metadata, "proxy_last_checked_at", "last_checked_at")
	return quality, nil
}

func (s *Service) BindProxy(ctx context.Context, id int, proxyID *string) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	normalized, err := s.normalizeAvailableProxyID(ctx, proxyID)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return s.Update(ctx, id, contract.UpdateRequest{ProxyID: &normalized})
}

func (s *Service) normalizeAvailableProxyID(ctx context.Context, value *string) (*string, error) {
	normalized, err := normalizeProxyID(value)
	if err != nil || normalized == nil {
		return normalized, err
	}
	if _, err := s.ResolveProxyURL(ctx, normalized); err != nil {
		return nil, ErrProxyUnavailable
	}
	return normalized, nil
}

func (s *Service) ResolveProxyURL(ctx context.Context, proxyID *string) (*string, error) {
	if proxyID == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*proxyID)
	if trimmed == "" {
		return nil, nil
	}
	if strings.Contains(trimmed, "://") {
		return nil, ErrInvalidInput
	}
	id, err := strconv.Atoi(trimmed)
	if err != nil || id <= 0 {
		return nil, ErrInvalidInput
	}
	return s.resolveProxyURLByID(ctx, id, map[int]struct{}{})
}

func (s *Service) resolveProxyURLByID(ctx context.Context, id int, seen map[int]struct{}) (*string, error) {
	if _, exists := seen[id]; exists {
		return nil, ErrProxyUnavailable
	}
	seen[id] = struct{}{}
	proxy, err := s.store.FindProxyByID(ctx, id)
	if err != nil {
		return nil, err
	}
	hasStructured := proxy.Host != ""
	hasLegacy := proxy.URLCiphertext != ""
	if proxy.Status != contract.ProxyStatusActive || (!hasStructured && !hasLegacy) {
		return nil, ErrProxyUnavailable
	}
	if proxyExpired(proxy, s.clock.Now()) {
		switch proxy.FallbackMode {
		case contract.ProxyFallbackModeDirect:
			return nil, nil
		case contract.ProxyFallbackModeProxy:
			if proxy.BackupProxyID == nil || *proxy.BackupProxyID <= 0 {
				return nil, ErrProxyUnavailable
			}
			return s.resolveProxyURLByID(ctx, *proxy.BackupProxyID, seen)
		default:
			return nil, ErrProxyUnavailable
		}
	}
	// Prefer structured fields (sub2api style) over encrypted URL blob.
	if hasStructured {
		rawURL := proxy.URL()
		if rawURL == "" {
			return nil, ErrProxyUnavailable
		}
		// If password is encrypted, decrypt and inject into URL.
		if proxy.PasswordCiphertext != "" {
			decrypted, err := s.decryptCredential(proxy.PasswordCiphertext)
			if err == nil {
				if pw, ok := decrypted["password"].(string); ok && pw != "" {
					rawURL = buildProxyURL(proxy.Protocol, proxy.Host, proxy.Port, proxy.Username, pw)
				}
			}
		}
		if err := validateProxyURL(proxy.Type, rawURL); err != nil {
			return nil, err
		}
		return &rawURL, nil
	}
	rawURL, err := s.decryptProxyURL(proxy)
	if err != nil {
		return nil, err
	}
	if err := validateProxyURL(proxy.Type, rawURL); err != nil {
		return nil, err
	}
	return &rawURL, nil
}

func (s *Service) Recover(ctx context.Context, id int) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	metadata := cloneMap(account.Metadata)
	for _, key := range []string{
		"cooldown_active",
		"cooldown_reason",
		"cooldown_until",
		"cooldown_strikes",
		"cooldown_last_at",
		"circuit_open",
		"last_error_class",
		"last_error_at",
		"last_error_message",
		"needs_reauth_reason",
		"quota_exhausted",
	} {
		delete(metadata, key)
	}
	metadata["last_recovered_at"] = s.clock.Now().Format(time.RFC3339)
	account.NeedsReauthAt = nil
	account.RefreshAttempts = 0
	account.RefreshLastError = ""
	status := contract.StatusActive
	account.Status = status
	account.Metadata = metadata
	account.UpdatedAt = s.clock.Now()
	return s.persistAccount(ctx, account)
}

func (s *Service) RecordHealthSnapshot(ctx context.Context, snapshot contract.AccountHealthSnapshot) (contract.AccountHealthSnapshot, error) {
	if snapshot.AccountID <= 0 || snapshot.ProviderID <= 0 {
		return contract.AccountHealthSnapshot{}, ErrInvalidInput
	}
	if strings.TrimSpace(snapshot.Status) == "" {
		snapshot.Status = "healthy"
	}
	if strings.TrimSpace(snapshot.CircuitState) == "" {
		snapshot.CircuitState = "closed"
	}
	snapshot.SuccessRate = clampRatio(snapshot.SuccessRate)
	snapshot.ErrorRate = clampRatio(snapshot.ErrorRate)
	if snapshot.SnapshotAt.IsZero() {
		snapshot.SnapshotAt = s.clock.Now()
	}
	return s.store.RecordHealthSnapshot(ctx, snapshot)
}

// ProbeAccount probes one provider account and persists the resulting health state.
func (s *Service) ProbeAccount(ctx context.Context, id int, prober contract.AccountProber, policy contract.AccountProbePolicy) (snapshot contract.AccountHealthSnapshot, updated contract.ProviderAccount, err error) {
	ctx, span := platformotel.StartSpan(ctx, "accounts.ProbeAccount",
		attribute.Int("srapi.account.id", id),
	)
	defer func() {
		platformotel.EndSpan(span, err, accountProbeTraceErrorType(err), accountProbeTraceAttrs(snapshot, updated, err)...)
	}()

	if id <= 0 || prober == nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, err
	}
	credential, err := s.decryptCredential(account.CredentialCiphertext)
	if err != nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, err
	}
	policy = normalizeProbePolicy(policy)
	result, err := prober.ProbeAccount(ctx, account, credential)
	if err != nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, err
	}
	if result.CheckedAt.IsZero() {
		result.CheckedAt = s.clock.Now()
	}
	history, err := s.store.ListHealthSnapshotsByAccount(ctx, account.ID, policy.HistoryLimit)
	if err != nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, err
	}
	snapshot, update := probeHealthState(account, result, history, policy)
	recorded, err := s.RecordHealthSnapshot(ctx, snapshot)
	if err != nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, err
	}
	update.Metadata["last_health_snapshot_id"] = recorded.ID
	updated, err = s.persistAccount(ctx, update)
	if err != nil {
		return contract.AccountHealthSnapshot{}, contract.ProviderAccount{}, err
	}
	return recorded, updated, nil
}

func accountProbeTraceErrorType(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, ErrCredentialMissing):
		return "credential_missing"
	case errors.Is(err, ErrProxyUnavailable):
		return "proxy_unavailable"
	default:
		return "account_probe_error"
	}
}

func accountProbeTraceAttrs(snapshot contract.AccountHealthSnapshot, updated contract.ProviderAccount, err error) []attribute.KeyValue {
	outcome := "error"
	if err == nil {
		outcome = snapshot.Status
		if outcome == "" {
			outcome = "healthy"
		}
	}
	attrs := []attribute.KeyValue{attribute.String("srapi.account.probe_outcome", outcome)}
	if updated.ID > 0 {
		attrs = append(attrs,
			attribute.Int("srapi.account.id", updated.ID),
			attribute.Int("srapi.provider.id", updated.ProviderID),
			attribute.String("srapi.account.runtime_class", string(updated.RuntimeClass)),
			attribute.String("srapi.account.status", string(updated.Status)),
		)
		if errorClass, ok := updated.Metadata["last_error_class"].(string); ok && strings.TrimSpace(errorClass) != "" {
			attrs = append(attrs, attribute.String("srapi.account.error_class", strings.TrimSpace(errorClass)))
		}
	}
	if snapshot.AccountID > 0 {
		attrs = append(attrs,
			attribute.String("srapi.account.health_status", snapshot.Status),
			attribute.String("srapi.account.circuit_state", snapshot.CircuitState),
			attribute.Int("srapi.account.probe_latency_ms", snapshot.LatencyP95MS),
		)
	}
	return attrs
}

func (s *Service) LatestHealthSnapshotByAccount(ctx context.Context, accountID int) (contract.AccountHealthSnapshot, error) {
	if accountID <= 0 {
		return contract.AccountHealthSnapshot{}, ErrInvalidInput
	}
	return s.store.LatestHealthSnapshotByAccount(ctx, accountID)
}

func (s *Service) ListHealthSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]contract.AccountHealthSnapshot, error) {
	if accountID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListHealthSnapshotsByAccount(ctx, accountID, limit)
}

func (s *Service) RecordQuotaSnapshot(ctx context.Context, snapshot contract.AccountQuotaSnapshot) (contract.AccountQuotaSnapshot, error) {
	if snapshot.AccountID <= 0 || snapshot.ProviderID <= 0 || strings.TrimSpace(snapshot.QuotaType) == "" {
		return contract.AccountQuotaSnapshot{}, ErrInvalidInput
	}
	if strings.TrimSpace(snapshot.Remaining) == "" {
		snapshot.Remaining = "0"
	}
	if strings.TrimSpace(snapshot.Used) == "" {
		snapshot.Used = "0"
	}
	if strings.TrimSpace(snapshot.QuotaLimit) == "" {
		snapshot.QuotaLimit = "0"
	}
	snapshot.RemainingRatio = clampRatio(snapshot.RemainingRatio)
	if snapshot.SnapshotAt.IsZero() {
		snapshot.SnapshotAt = s.clock.Now()
	}
	return s.store.RecordQuotaSnapshot(ctx, snapshot)
}

func (s *Service) ListQuotaSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]contract.AccountQuotaSnapshot, error) {
	if accountID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListQuotaSnapshotsByAccount(ctx, accountID, limit)
}

// LatestHealthSnapshotsByAccounts resolves the most recent health snapshot for
// every given account, preferring the store's batched reader and falling back
// to per-account reads for stores without one. Accounts without snapshots are
// absent from the result.
func (s *Service) LatestHealthSnapshotsByAccounts(ctx context.Context, accountIDs []int) (map[int]contract.AccountHealthSnapshot, error) {
	accountIDs = dedupePositiveIDs(accountIDs)
	if len(accountIDs) == 0 {
		return map[int]contract.AccountHealthSnapshot{}, nil
	}
	if reader, ok := s.store.(contract.BatchSnapshotReader); ok {
		return reader.LatestHealthSnapshotsByAccounts(ctx, accountIDs)
	}
	out := make(map[int]contract.AccountHealthSnapshot, len(accountIDs))
	for _, accountID := range accountIDs {
		snapshot, err := s.store.LatestHealthSnapshotByAccount(ctx, accountID)
		if err != nil {
			continue
		}
		out[accountID] = snapshot
	}
	return out, nil
}

// LatestQuotaSnapshotsByAccounts resolves, per account, the most recent quota
// snapshot of each quota type, preferring the store's batched reader and
// falling back to per-account reads for stores without one.
func (s *Service) LatestQuotaSnapshotsByAccounts(ctx context.Context, accountIDs []int) (map[int][]contract.AccountQuotaSnapshot, error) {
	accountIDs = dedupePositiveIDs(accountIDs)
	if len(accountIDs) == 0 {
		return map[int][]contract.AccountQuotaSnapshot{}, nil
	}
	if reader, ok := s.store.(contract.BatchSnapshotReader); ok {
		return reader.LatestQuotaSnapshotsByAccounts(ctx, accountIDs)
	}
	out := make(map[int][]contract.AccountQuotaSnapshot, len(accountIDs))
	for _, accountID := range accountIDs {
		snapshots, err := s.store.ListQuotaSnapshotsByAccount(ctx, accountID, 1)
		if err != nil || len(snapshots) == 0 {
			continue
		}
		out[accountID] = snapshots
	}
	return out, nil
}

func dedupePositiveIDs(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// DecryptCredential returns the decrypted credential map for the given account
// ID. Concurrent calls for the same ID are coalesced via singleflight so the
// DB lookup + AES-GCM decrypt runs at most once per in-flight window.
func (s *Service) DecryptCredential(ctx context.Context, id int) (map[string]any, error) {
	if id <= 0 {
		return nil, ErrInvalidInput
	}
	key := strconv.Itoa(id)
	result, err, _ := s.decryptGroup.Do(key, func() (any, error) {
		return s.decryptCredentialByID(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}

// decryptCredentialByID is the inner implementation of DecryptCredential:
// fetch the account row and AES-GCM decrypt its credential ciphertext.
func (s *Service) decryptCredentialByID(ctx context.Context, id int) (map[string]any, error) {
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.decryptCredential(account.CredentialCiphertext)
}

func (s *Service) decryptProxyURL(proxy contract.ProxyDefinition) (string, error) {
	payload, err := s.decryptCredential(proxy.URLCiphertext)
	if err != nil {
		return "", err
	}
	rawURL, ok := payload["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return "", ErrInvalidInput
	}
	return strings.TrimSpace(rawURL), nil
}

func validProxyType(proxyType contract.ProxyType) bool {
	switch proxyType {
	case contract.ProxyTypeHTTP, contract.ProxyTypeHTTPS, contract.ProxyTypeSOCKS5, contract.ProxyTypeSOCKS5H:
		return true
	default:
		return false
	}
}

func validProxyStatus(status contract.ProxyStatus) bool {
	switch status {
	case contract.ProxyStatusActive, contract.ProxyStatusDisabled:
		return true
	default:
		return false
	}
}

func validProxyFallbackMode(mode contract.ProxyFallbackMode) bool {
	switch mode {
	case contract.ProxyFallbackModeNone, contract.ProxyFallbackModeDirect, contract.ProxyFallbackModeProxy:
		return true
	default:
		return false
	}
}

func (s *Service) validateProxyFallback(ctx context.Context, proxyID int, mode contract.ProxyFallbackMode, backupProxyID *int) error {
	if !validProxyFallbackMode(mode) {
		return ErrInvalidInput
	}
	if mode != contract.ProxyFallbackModeProxy {
		return nil
	}
	if backupProxyID == nil || *backupProxyID <= 0 {
		return ErrInvalidInput
	}
	if proxyID > 0 && *backupProxyID == proxyID {
		return ErrInvalidInput
	}
	backup, err := s.store.FindProxyByID(ctx, *backupProxyID)
	if err != nil {
		return err
	}
	if backup.Status != contract.ProxyStatusActive || backup.URLCiphertext == "" {
		return ErrInvalidInput
	}
	return nil
}

func proxyExpired(proxy contract.ProxyDefinition, now time.Time) bool {
	if proxy.ExpiresAt == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return !now.UTC().Before(proxy.ExpiresAt.UTC())
}

func validateProxyURL(proxyType contract.ProxyType, rawURL string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return ErrInvalidInput
	}
	if contract.ProxyType(strings.ToLower(parsed.Scheme)) != proxyType {
		return ErrInvalidInput
	}
	return nil
}

func validGroupStatus(status contract.GroupStatus) bool {
	switch status {
	case contract.GroupStatusActive, contract.GroupStatusDisabled:
		return true
	default:
		return false
	}
}

func normalizeRateMultiplier(value *string) (string, bool) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "1.00000000", true
	}
	trimmed := strings.TrimSpace(*value)
	if strings.ContainsAny(trimmed, "eE") {
		return "", false
	}
	rat, ok := new(big.Rat).SetString(trimmed)
	if !ok || rat.Sign() < 0 {
		return "", false
	}
	return rat.FloatString(8), true
}

func validAccountStatus(status contract.Status) bool {
	switch status {
	case contract.StatusActive, contract.StatusDisabled, contract.StatusNeedsReauth, contract.StatusSuspended, contract.StatusDead, contract.StatusArchived:
		return true
	default:
		return false
	}
}

func validRuntimeClass(runtimeClass contract.RuntimeClass) bool {
	switch runtimeClass {
	case contract.RuntimeClassAPIKey,
		contract.RuntimeClassOauthRefresh,
		contract.RuntimeClassOauthDeviceCode,
		contract.RuntimeClassWebSessionCookie,
		contract.RuntimeClassCliClientToken,
		contract.RuntimeClassCustomReverseProxy:
		return true
	default:
		return false
	}
}

func normalizeRiskLevel(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "normal":
		return "normal", true
	case "medium":
		return "medium", true
	case "high":
		return "high", true
	default:
		return "", false
	}
}

func clampRatio(value float32) float32 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizeProbePolicy(policy contract.AccountProbePolicy) contract.AccountProbePolicy {
	if policy.HistoryLimit <= 0 {
		policy.HistoryLimit = 5
	}
	if policy.FailureThreshold <= 0 {
		policy.FailureThreshold = 3
	}
	if policy.ErrorRateThreshold <= 0 {
		policy.ErrorRateThreshold = 0.5
	}
	if policy.MinSamplesForErrorRate <= 0 {
		policy.MinSamplesForErrorRate = 3
	}
	if policy.Cooldown <= 0 {
		policy.Cooldown = 5 * time.Minute
	}
	return policy
}

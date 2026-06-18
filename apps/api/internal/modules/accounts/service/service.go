package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	"go.opentelemetry.io/otel/attribute"
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
	proxyID, err := normalizeProxyID(req.ProxyID)
	if err != nil {
		return contract.ProviderAccount{}, err
	}

	stored, err := s.store.Create(ctx, contract.CreateStoredAccount{
		ProviderID:           req.ProviderID,
		Name:                 name,
		RuntimeClass:         req.RuntimeClass,
		CredentialCiphertext: credentialCiphertext,
		CredentialVersion:    credentialVersionV1,
		Metadata:             cloneMap(req.Metadata),
		ProxyID:              proxyID,
		Status:               status,
		Priority:             priority,
		Weight:               weight,
		RiskLevel:            &riskLevel,
		UpstreamClient:       req.UpstreamClient,
	})
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return stored, nil
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
		if account.DeletedAt != nil {
			continue
		}
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
		account, err := s.store.FindByID(ctx, item.AccountID)
		if err != nil {
			// Idempotent: NotFound is not a failure since the caller's intent
			// for that id is already moot. Match both the typed sentinel and
			// the ad-hoc string the memory store still returns (mirrors the
			// fallback at the batch-delete path above).
			if errors.Is(err, ErrAccountNotFound) || strings.Contains(err.Error(), "account not found") {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		metadata := cloneMap(account.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		// Zero clears the override (mirrors sub2api: a "set 0" call means
		// "remove the cap"). The scheduler treats absent + zero identically.
		if item.MaxConcurrency == 0 {
			delete(metadata, "max_concurrency")
		} else {
			metadata["max_concurrency"] = item.MaxConcurrency
		}
		if _, err := s.Update(ctx, item.AccountID, contract.UpdateRequest{Metadata: &metadata}); err != nil {
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

func (s *Service) CreateProxy(ctx context.Context, req contract.CreateProxyRequest) (contract.ProxyDefinition, error) {
	name := strings.TrimSpace(req.Name)
	proxyType := req.Type
	rawURL := strings.TrimSpace(req.URL)
	if name == "" || rawURL == "" || !validProxyType(proxyType) {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	if err := validateProxyURL(proxyType, rawURL); err != nil {
		return contract.ProxyDefinition{}, err
	}
	ciphertext, err := s.encryptCredential(map[string]any{"url": rawURL})
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	status := contract.ProxyStatusActive
	if req.Status != nil {
		if !validProxyStatus(*req.Status) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		status = *req.Status
	}
	countryCode := normalizeCountryCode(req.CountryCode)
	countryName := normalizeCountryName(req.CountryName)
	return s.store.CreateProxy(ctx, contract.CreateStoredProxy{
		Name:          name,
		Type:          proxyType,
		URLCiphertext: ciphertext,
		URLVersion:    credentialVersionV1,
		Status:        status,
		Metadata:      cloneMap(req.Metadata),
		CountryCode:   countryCode,
		CountryName:   countryName,
	})
}

func (s *Service) UpdateProxy(ctx context.Context, id int, req contract.UpdateProxyRequest) (contract.ProxyDefinition, error) {
	if id <= 0 {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	proxy, err := s.store.FindProxyByID(ctx, id)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		proxy.Name = name
	}
	if req.Type != nil {
		if !validProxyType(*req.Type) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		proxy.Type = *req.Type
	}
	if req.URL != nil {
		rawURL := strings.TrimSpace(*req.URL)
		if rawURL == "" {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		if err := validateProxyURL(proxy.Type, rawURL); err != nil {
			return contract.ProxyDefinition{}, err
		}
		ciphertext, err := s.encryptCredential(map[string]any{"url": rawURL})
		if err != nil {
			return contract.ProxyDefinition{}, err
		}
		proxy.URLCiphertext = ciphertext
		proxy.URLVersion = credentialVersionV1
	} else if req.Type != nil && proxy.URLCiphertext != "" {
		rawURL, err := s.decryptProxyURL(proxy)
		if err != nil {
			return contract.ProxyDefinition{}, err
		}
		if err := validateProxyURL(proxy.Type, rawURL); err != nil {
			return contract.ProxyDefinition{}, err
		}
	}
	if req.Status != nil {
		if !validProxyStatus(*req.Status) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		proxy.Status = *req.Status
	}
	if req.Metadata != nil {
		proxy.Metadata = cloneMap(*req.Metadata)
	}
	if req.CountryCode != nil {
		proxy.CountryCode = normalizeCountryCode(req.CountryCode)
	}
	if req.CountryName != nil {
		proxy.CountryName = normalizeCountryName(req.CountryName)
	}
	proxy.UpdatedAt = s.clock.Now()
	return s.store.UpdateProxy(ctx, proxy)
}

// proxyCounterResetMetadataKey records the last time the rolling success/failure
// counters were zeroed. Lives on metadata so adding a rolling window does not
// require a separate ent column — see RecordProxyProbe.
const proxyCounterResetMetadataKey = "_probe_counter_reset_at"

// proxyCounterResetWindow is how long the rolling availability window is in
// wall-clock time. After this many days of probe results the counters get
// zeroed on the next probe, giving the UI a "since-last-reset" availability
// that approximates a trailing 7-day window without a separate snapshot table.
const proxyCounterResetWindow = 7 * 24 * time.Hour

// RecordProxyProbe folds one probe outcome into the proxy's rolling counters
// and updates last_probed_at + last_probe_latency_ms. Called by the
// proxy_probe worker after each pass and by the operator-initiated "probe
// now" handler so the availability percentage stays current between worker
// ticks.
//
// The counters reset every ~7 days (see proxyCounterResetWindow) so the
// availability percentage is a rolling-window value rather than a lifetime
// success rate — fresh signals weigh as much as old ones after enough time
// passes. The reset timestamp lives in metadata under
// proxyCounterResetMetadataKey to keep the ent schema delta minimal.
func (s *Service) RecordProxyProbe(ctx context.Context, proxyID int, success bool, latencyMs int) (contract.ProxyDefinition, error) {
	if proxyID <= 0 {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	// Serialize per-proxy so two concurrent probes (e.g. worker + admin "probe
	// now") can't both observe the same counter snapshot and both write back,
	// dropping one increment. The memory store has its own mutex; the SQL
	// backend is vulnerable to this lost-update race.
	lock := s.proxyLockFor(proxyID)
	lock.Lock()
	defer lock.Unlock()
	proxy, err := s.store.FindProxyByID(ctx, proxyID)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	now := s.clock.Now().UTC()
	metadata := cloneMap(proxy.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	lastReset := metadataOptionalTime(metadata, proxyCounterResetMetadataKey)
	if lastReset == nil || now.Sub(*lastReset) >= proxyCounterResetWindow {
		proxy.ProbeSuccessCount = 0
		proxy.ProbeFailureCount = 0
		metadata[proxyCounterResetMetadataKey] = now.Format(time.RFC3339)
	}
	if success {
		proxy.ProbeSuccessCount++
		if latencyMs > 0 {
			proxy.LastProbeLatencyMs = latencyMs
		}
	} else {
		proxy.ProbeFailureCount++
	}
	probedAt := now
	proxy.LastProbedAt = &probedAt
	proxy.Metadata = metadata
	proxy.UpdatedAt = now
	return s.store.UpdateProxy(ctx, proxy)
}

// proxyLockFor returns a per-proxy mutex used to serialize the
// FindProxyByID → mutate → UpdateProxy sequence in RecordProxyProbe.
// Loaded lazily; stale entries are left in the map (low cardinality in
// practice — one per proxy that has ever been probed).
func (s *Service) proxyLockFor(proxyID int) *sync.Mutex {
	if existing, ok := s.proxyLocks.Load(proxyID); ok {
		return existing.(*sync.Mutex)
	}
	created := &sync.Mutex{}
	actual, _ := s.proxyLocks.LoadOrStore(proxyID, created)
	return actual.(*sync.Mutex)
}

// normalizeCountryCode trims and uppercases the operator-supplied country
// code, capping at the schema's 2-char MaxLen so a stray longer string from a
// future client cannot get past the contract layer.
func normalizeCountryCode(value *string) string {
	if value == nil {
		return ""
	}
	trimmed := strings.ToUpper(strings.TrimSpace(*value))
	if len(trimmed) > 2 {
		trimmed = trimmed[:2]
	}
	return trimmed
}

func normalizeCountryName(value *string) string {
	if value == nil {
		return ""
	}
	trimmed := strings.TrimSpace(*value)
	if len(trimmed) > 128 {
		trimmed = trimmed[:128]
	}
	return trimmed
}

func (s *Service) FindProxyByID(ctx context.Context, id int) (contract.ProxyDefinition, error) {
	if id <= 0 {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	return s.store.FindProxyByID(ctx, id)
}

// DeleteProxy soft-deletes a proxy and unbinds any accounts that route through
// it (by id), which fall back to a direct connection.
func (s *Service) DeleteProxy(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if _, err := s.store.FindProxyByID(ctx, id); err != nil {
		return err
	}
	return s.store.SoftDeleteProxy(ctx, id)
}

// defaultProxyTestTarget is a small, plain-text, dependable response over
// HTTPS used by TestProxy when the caller doesn't pass a custom target.
// Cloudflare's /cdn-cgi/trace returns a few hundred bytes of key=value text
// and is widely used for exactly this kind of probe.
const defaultProxyTestTarget = "https://www.cloudflare.com/cdn-cgi/trace"

// proxyLastTestMetadataKey is the reserved metadata key that TestProxy and
// BatchTestProxies use to persist the most recent probe outcome on the proxy
// row. The leading underscore signals "internal — don't touch from the form
// editor"; the value is an object with {at, ok, latency_ms, error_class,
// status_code, target_url}. Stored on metadata to avoid an ent schema change
// for what is essentially a per-row diagnostic cache.
const proxyLastTestMetadataKey = "_last_test"

// batchTestProxyConcurrency caps how many in-flight probes BatchTestProxies
// runs at once. Each one is a real HTTPS handshake — too many simultaneous
// dials would hammer the box, too few would make a moderate selection feel
// slow. 8 lets a 50-row selection finish in well under a minute with the
// 8s per-probe cap.
const batchTestProxyConcurrency = 8

// batchTestProxyMaxRows caps how many proxies a single batch can probe. A
// runaway-loop guard, not a feature limit — typical operator selections are
// well under this.
const batchTestProxyMaxRows = 200

// proxyTestTimeout caps how long a single TestProxy call can wait — a wedged
// proxy or unresponsive target shouldn't tie up an admin browser tab.
const proxyTestTimeout = 8 * time.Second

// TestProxy issues a one-shot HTTPS GET to targetURL routed through the
// proxy. The result categorizes the failure mode for the UI: bad_proxy_url
// when the stored ciphertext won't decrypt, bad_target_url when targetURL
// isn't a usable URL, timeout when the wall-clock cap expires before any
// response, transport_error when the dial / TLS / proxy CONNECT step fails,
// and bad_status when the upstream returns non-2xx. OK is true only on a
// 2xx response.
//
// targetURL may be empty — defaultProxyTestTarget is used in that case.
func (s *Service) TestProxy(ctx context.Context, id int, targetURL string) (contract.ProxyTestResult, error) {
	if id <= 0 {
		return contract.ProxyTestResult{}, ErrInvalidInput
	}
	proxy, err := s.store.FindProxyByID(ctx, id)
	if err != nil {
		return contract.ProxyTestResult{}, err
	}
	target := strings.TrimSpace(targetURL)
	if target == "" {
		target = defaultProxyTestTarget
	}
	parsedTarget, err := url.Parse(target)
	if err != nil || parsedTarget.Scheme == "" || parsedTarget.Host == "" {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_target_url",
			TargetURL:  target,
		}, nil
	}

	proxyRawURL, err := s.decryptProxyURL(proxy)
	if err != nil {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_proxy_url",
			TargetURL:  target,
		}, nil
	}
	parsedProxyURL, err := url.Parse(proxyRawURL)
	if err != nil || parsedProxyURL.Scheme == "" || parsedProxyURL.Host == "" {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_proxy_url",
			TargetURL:  target,
		}, nil
	}

	transport := &http.Transport{Proxy: http.ProxyURL(parsedProxyURL)}
	client := &http.Client{Transport: transport, Timeout: proxyTestTimeout}
	defer transport.CloseIdleConnections()

	probeCtx, cancel := context.WithTimeout(ctx, proxyTestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, target, nil)
	if err != nil {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_target_url",
			TargetURL:  target,
		}, nil
	}
	start := time.Now()
	resp, err := client.Do(req)
	latencyMS := int(time.Since(start) / time.Millisecond)
	var result contract.ProxyTestResult
	if err != nil {
		// Categorize the error: context-deadline + URL deadline errors map to
		// timeout; anything else is a transport-level failure (dial / TLS /
		// CONNECT refused). The frontend gets a stable, narrow set of classes.
		errClass := "transport_error"
		if errors.Is(err, context.DeadlineExceeded) {
			errClass = "timeout"
		}
		result = contract.ProxyTestResult{
			OK:         false,
			LatencyMS:  latencyMS,
			ErrorClass: errClass,
			TargetURL:  target,
		}
	} else {
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result = contract.ProxyTestResult{
				OK:         true,
				LatencyMS:  latencyMS,
				StatusCode: resp.StatusCode,
				TargetURL:  target,
			}
		} else {
			result = contract.ProxyTestResult{
				OK:         false,
				LatencyMS:  latencyMS,
				StatusCode: resp.StatusCode,
				ErrorClass: "bad_status",
				TargetURL:  target,
			}
		}
	}
	// Persist the outcome onto the proxy's metadata so the list view can show
	// a "last test" badge across page loads. A persist failure is silently
	// dropped — the probe result the caller is about to see is what matters;
	// the cache is best-effort.
	_ = s.persistProxyTestResult(ctx, proxy, result)
	return result, nil
}

// persistProxyTestResult merges a result snapshot into proxy.Metadata under
// proxyLastTestMetadataKey and writes the proxy back via the store.
//
// Loads a fresh copy from the store before mutating so we don't race with
// concurrent edits to other metadata fields — the cost is one extra round-trip
// per Test, which is negligible compared to the probe itself.
func (s *Service) persistProxyTestResult(ctx context.Context, original contract.ProxyDefinition, result contract.ProxyTestResult) error {
	fresh, err := s.store.FindProxyByID(ctx, original.ID)
	if err != nil {
		return err
	}
	if fresh.Metadata == nil {
		fresh.Metadata = map[string]any{}
	}
	snapshot := map[string]any{
		"at":          time.Now().UTC().Format(time.RFC3339),
		"ok":          result.OK,
		"latency_ms":  result.LatencyMS,
		"status_code": result.StatusCode,
		"error_class": result.ErrorClass,
		"target_url":  result.TargetURL,
	}
	fresh.Metadata[proxyLastTestMetadataKey] = snapshot
	if _, err := s.store.UpdateProxy(ctx, fresh); err != nil {
		return err
	}
	return nil
}

// BatchTestProxies runs TestProxy for each id in parallel (capped at
// batchTestProxyConcurrency), in input order, with the default target URL
// for every row. Returns one row per requested id — a row whose proxy was
// missing surfaces with ErrorClass="not_found" rather than failing the
// whole call. Caller-visible duplicates in ids are returned verbatim in
// the result (same id appears twice → two rows) so the frontend can
// align by index without re-grouping.
func (s *Service) BatchTestProxies(ctx context.Context, ids []int) ([]contract.ProxyBatchTestRow, error) {
	if len(ids) == 0 || len(ids) > batchTestProxyMaxRows {
		return nil, ErrInvalidInput
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidInput
		}
	}
	rows := make([]contract.ProxyBatchTestRow, len(ids))
	sem := make(chan struct{}, batchTestProxyConcurrency)
	var wg sync.WaitGroup
	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			result, err := s.TestProxy(ctx, id, "")
			if err != nil {
				rows[i] = contract.ProxyBatchTestRow{
					ProxyID: id,
					Result: contract.ProxyTestResult{
						OK:         false,
						ErrorClass: "not_found",
						TargetURL:  defaultProxyTestTarget,
					},
				}
				return
			}
			rows[i] = contract.ProxyBatchTestRow{ProxyID: id, Result: result}
		}()
	}
	wg.Wait()
	return rows, nil
}

// BatchCreateProxyResult is per-row outcome from BatchCreateProxies.
// Created is the inserted row, SkippedReason is set when the row was a
// soft-skip (duplicate name), Err is set on a hard validation failure.
type BatchCreateProxyResult struct {
	Index         int
	Name          string
	Created       *contract.ProxyDefinition
	SkippedReason string
	Err           error
}

// BatchCreateProxies inserts many proxies in one call. Dedupes by name
// against existing proxies AND within the request itself — a duplicate name
// surfaces as SkippedReason="duplicate_name" rather than failing the whole
// call. Hard validation errors (bad URL, bad type) surface in Err on that
// row; other rows still succeed. Order matches the input.
func (s *Service) BatchCreateProxies(ctx context.Context, reqs []contract.CreateProxyRequest) ([]BatchCreateProxyResult, error) {
	if len(reqs) == 0 {
		return nil, ErrInvalidInput
	}
	existing, err := s.store.ListProxies(ctx)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]struct{}, len(existing))
	for _, p := range existing {
		taken[strings.ToLower(strings.TrimSpace(p.Name))] = struct{}{}
	}
	out := make([]BatchCreateProxyResult, len(reqs))
	for i, req := range reqs {
		name := strings.TrimSpace(req.Name)
		row := BatchCreateProxyResult{Index: i, Name: name}
		if name == "" {
			row.Err = ErrInvalidInput
			out[i] = row
			continue
		}
		key := strings.ToLower(name)
		if _, dup := taken[key]; dup {
			row.SkippedReason = "duplicate_name"
			out[i] = row
			continue
		}
		created, createErr := s.CreateProxy(ctx, req)
		if createErr != nil {
			row.Err = createErr
		} else {
			taken[key] = struct{}{}
			row.Created = &created
		}
		out[i] = row
	}
	return out, nil
}

// BatchDeleteProxyResult is per-id outcome from BatchDeleteProxies. Err is
// set when the id couldn't be deleted (e.g. not found); successful rows have
// Err == nil. Maintains input order.
type BatchDeleteProxyResult struct {
	ID  int
	Err error
}

// BatchDeleteProxies soft-deletes the named ids. Same semantics as the
// per-id DeleteProxy — accounts routed through a deleted proxy fall back to
// a direct connection. Missing ids surface in Err on that row without
// failing the call.
func (s *Service) BatchDeleteProxies(ctx context.Context, ids []int) ([]BatchDeleteProxyResult, error) {
	if len(ids) == 0 {
		return nil, ErrInvalidInput
	}
	out := make([]BatchDeleteProxyResult, len(ids))
	for i, id := range ids {
		row := BatchDeleteProxyResult{ID: id}
		if err := s.DeleteProxy(ctx, id); err != nil {
			row.Err = err
		}
		out[i] = row
	}
	return out, nil
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

func (s *Service) ListProxies(ctx context.Context) ([]contract.ProxyDefinition, error) {
	proxies, err := s.store.ListProxies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProxyDefinition, 0, len(proxies))
	for _, proxy := range proxies {
		out = append(out, proxy)
	}
	return out, nil
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
		account.Metadata = cloneMap(*req.Metadata)
	}
	if req.ProxyID != nil {
		proxyID, err := normalizeProxyID(*req.ProxyID)
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
	account.UpdatedAt = s.clock.Now()
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
	return s.Update(ctx, id, contract.UpdateRequest{
		Status:   &status,
		Metadata: &metadata,
	})
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
	normalized, err := normalizeProxyID(proxyID)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return s.Update(ctx, id, contract.UpdateRequest{ProxyID: &normalized})
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
	proxy, err := s.store.FindProxyByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if proxy.Status != contract.ProxyStatusActive || proxy.URLCiphertext == "" {
		return nil, ErrProxyUnavailable
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
		"quota_exhausted",
	} {
		delete(metadata, key)
	}
	metadata["last_recovered_at"] = s.clock.Now().Format(time.RFC3339)
	status := contract.StatusActive
	return s.Update(ctx, id, contract.UpdateRequest{
		Status:   &status,
		Metadata: &metadata,
	})
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
	updated, err = s.store.Update(ctx, update)
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

func (s *Service) DecryptCredential(ctx context.Context, id int) (map[string]any, error) {
	if id <= 0 {
		return nil, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.decryptCredential(account.CredentialCiphertext)
}

func (s *Service) encryptCredential(payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, raw, []byte(credentialVersionV1))
	return fmt.Sprintf("%s:%s:%s", credentialVersionV1, base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(ciphertext)), nil
}

func (s *Service) decryptCredential(ciphertext string) (map[string]any, error) {
	parts := strings.Split(ciphertext, ":")
	if len(parts) != 3 || parts[0] != credentialVersionV1 {
		return nil, ErrInvalidInput
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, []byte(credentialVersionV1))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
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
	case contract.ProxyTypeHTTP, contract.ProxyTypeHTTPS, contract.ProxyTypeSOCKS5:
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

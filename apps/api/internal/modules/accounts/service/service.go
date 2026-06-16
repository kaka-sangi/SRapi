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
	"net/url"
	"sort"
	"strconv"
	"strings"
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
	return s.store.CreateProxy(ctx, contract.CreateStoredProxy{
		Name:          name,
		Type:          proxyType,
		URLCiphertext: ciphertext,
		URLVersion:    credentialVersionV1,
		Status:        status,
		Metadata:      cloneMap(req.Metadata),
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
	proxy.UpdatedAt = s.clock.Now()
	return s.store.UpdateProxy(ctx, proxy)
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

func probeHealthState(account contract.ProviderAccount, result contract.AccountProbeResult, history []contract.AccountHealthSnapshot, policy contract.AccountProbePolicy) (contract.AccountHealthSnapshot, contract.ProviderAccount) {
	samples := probeSamples(result, history, policy.HistoryLimit)
	successes := 0
	failures := 0
	latencies := make([]int, 0, len(samples))
	for _, sample := range samples {
		if sample.success {
			successes++
		} else {
			failures++
		}
		if sample.latencyMS > 0 {
			latencies = append(latencies, sample.latencyMS)
		}
	}
	total := successes + failures
	successRate := float32(0)
	errorRate := float32(0)
	if total > 0 {
		successRate = float32(successes) / float32(total)
		errorRate = float32(failures) / float32(total)
	}
	consecutiveFailures := consecutiveProbeFailures(samples)
	unhealthy := consecutiveFailures >= policy.FailureThreshold ||
		(total >= policy.MinSamplesForErrorRate && errorRate > policy.ErrorRateThreshold)

	status := "healthy"
	circuitState := "closed"
	var cooldownUntil *time.Time
	if unhealthy {
		status = "unhealthy"
		circuitState = "open"
		until := result.CheckedAt.Add(policy.Cooldown)
		cooldownUntil = &until
	} else if !result.OK {
		status = "degraded"
		circuitState = "half_open"
	}

	snapshot := contract.AccountHealthSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		Status:         status,
		SuccessRate:    clampRatio(successRate),
		ErrorRate:      clampRatio(errorRate),
		LatencyP50MS:   percentileLatency(latencies, 0.50),
		LatencyP95MS:   percentileLatency(latencies, 0.95),
		RateLimitCount: probeErrorCount(samples, "rate_limit"),
		TimeoutCount:   probeErrorCount(samples, "timeout"),
		CooldownUntil:  cooldownUntil,
		CircuitState:   circuitState,
		SnapshotAt:     result.CheckedAt,
	}

	updated := account
	metadata := cloneMap(account.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["health_score"] = snapshot.SuccessRate
	metadata["health_state"] = status
	metadata["last_probe_at"] = result.CheckedAt.Format(time.RFC3339)
	metadata["last_probe_ok"] = result.OK
	metadata["last_probe_latency_ms"] = result.LatencyMS
	metadata["last_probe_status_code"] = result.StatusCode
	metadata["consecutive_probe_failures"] = consecutiveFailures
	for key, value := range result.Metadata {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		metadata["last_probe_"+key] = value
	}
	errorClass := strings.TrimSpace(result.ErrorClass)
	if errorClass == "" {
		errorClass = "probe_failed"
	}
	if unhealthy {
		metadata["cooldown_active"] = true
		metadata["cooldown_reason"] = errorClass
		metadata["cooldown_until"] = cooldownUntil.Format(time.RFC3339)
		metadata["circuit_open"] = true
		if !result.OK {
			metadata["last_error_class"] = errorClass
			metadata["last_error_message"] = errorClass
		}
	} else if result.OK {
		delete(metadata, "cooldown_active")
		delete(metadata, "cooldown_reason")
		delete(metadata, "cooldown_until")
		delete(metadata, "circuit_open")
		delete(metadata, "last_error_class")
		delete(metadata, "last_error_message")
	} else {
		metadata["last_error_class"] = errorClass
		metadata["last_error_message"] = errorClass
		delete(metadata, "cooldown_active")
		delete(metadata, "cooldown_reason")
		delete(metadata, "cooldown_until")
		delete(metadata, "circuit_open")
	}
	updated.Metadata = metadata
	updated.UpdatedAt = result.CheckedAt
	return snapshot, updated
}

type probeSample struct {
	success    bool
	latencyMS  int
	errorClass string
}

func probeSamples(result contract.AccountProbeResult, history []contract.AccountHealthSnapshot, limit int) []probeSample {
	samples := make([]probeSample, 0, len(history)+1)
	samples = append(samples, probeSample{
		success:    result.OK,
		latencyMS:  result.LatencyMS,
		errorClass: strings.TrimSpace(result.ErrorClass),
	})
	for _, snapshot := range history {
		samples = append(samples, probeSample{
			success:    snapshot.SuccessRate >= 0.5 && !strings.EqualFold(snapshot.CircuitState, "open"),
			latencyMS:  snapshot.LatencyP50MS,
			errorClass: snapshotStatusErrorClass(snapshot),
		})
		if len(samples) >= limit {
			break
		}
	}
	return samples
}

func consecutiveProbeFailures(samples []probeSample) int {
	count := 0
	for _, sample := range samples {
		if sample.success {
			break
		}
		count++
	}
	return count
}

func probeErrorCount(samples []probeSample, errorClass string) int {
	count := 0
	for _, sample := range samples {
		if strings.EqualFold(sample.errorClass, errorClass) {
			count++
		}
	}
	return count
}

func percentileLatency(values []int, percentile float64) int {
	if len(values) == 0 {
		return 0
	}
	sort.Ints(values)
	idx := int(float64(len(values))*percentile+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func snapshotStatusErrorClass(snapshot contract.AccountHealthSnapshot) string {
	if snapshot.TimeoutCount > 0 {
		return "timeout"
	}
	if snapshot.RateLimitCount > 0 {
		return "rate_limit"
	}
	if !strings.EqualFold(snapshot.Status, "healthy") {
		return strings.TrimSpace(snapshot.Status)
	}
	return ""
}

func metadataOptionalInt(metadata map[string]any, keys ...string) *int {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return nil
	}
	parsed := intValue(value)
	return &parsed
}

func metadataInt(metadata map[string]any, keys ...string) int {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0
	}
	return intValue(value)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
		floatValue, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return int(floatValue)
		}
	}
	return 0
}

func metadataFloat32(metadata map[string]any, keys ...string) float32 {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float32:
		return clampRatio(typed)
	case float64:
		return clampRatio(float32(typed))
	case int:
		return clampRatio(float32(typed))
	case int64:
		return clampRatio(float32(typed))
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return clampRatio(float32(parsed))
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 32)
		if err == nil {
			return clampRatio(float32(parsed))
		}
	}
	return 0
}

func metadataOptionalTime(metadata map[string]any, keys ...string) *time.Time {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		cloned := typed
		return &cloned
	case string:
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(typed))
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func metadataValue(metadata map[string]any, keys ...string) (any, bool) {
	if metadata == nil {
		return nil, false
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func proxyQualityMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for _, key := range []string{
		"proxy_provider",
		"proxy_region",
		"proxy_country",
		"proxy_city",
		"proxy_type",
		"egress_ip_hash",
	} {
		if value, ok := metadata[key]; ok {
			out[key] = value
		}
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func normalizeProxyID(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	id, err := strconv.Atoi(trimmed)
	if err != nil || id <= 0 {
		return nil, ErrInvalidInput
	}
	return &trimmed, nil
}

func normalizePositiveIDs(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

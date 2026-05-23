package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
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
	if req.ProviderID <= 0 || name == "" || req.RuntimeClass == "" {
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

	stored, err := s.store.Create(ctx, contract.CreateStoredAccount{
		ProviderID:           req.ProviderID,
		Name:                 name,
		RuntimeClass:         req.RuntimeClass,
		CredentialCiphertext: credentialCiphertext,
		CredentialVersion:    credentialVersionV1,
		Metadata:             cloneMap(req.Metadata),
		ProxyID:              req.ProxyID,
		Status:               status,
		Priority:             priority,
		Weight:               weight,
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

func (s *Service) ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error) {
	if accountID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListGroupIDsByAccount(ctx, accountID)
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
	return s.store.CreateGroup(ctx, contract.CreateStoredAccountGroup{
		Name:          name,
		Description:   strings.TrimSpace(req.Description),
		ProviderScope: cloneMap(req.ProviderScope),
		ModelScope:    cloneMap(req.ModelScope),
		StrategyHint:  strategy,
		Status:        status,
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

func (s *Service) ListGroups(ctx context.Context) ([]contract.AccountGroup, error) {
	return s.store.ListGroups(ctx)
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
		if *req.RuntimeClass == "" {
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
		account.ProxyID = cloneString(*req.ProxyID)
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
	if req.UpstreamClient != nil {
		account.UpstreamClient = cloneString(*req.UpstreamClient)
	}
	account.UpdatedAt = s.clock.Now()
	return s.store.Update(ctx, account)
}

func (s *Service) BatchUpdateStatus(ctx context.Context, ids []int, status contract.Status) contract.BatchUpdateResult {
	result := contract.BatchUpdateResult{
		Updated: make([]contract.ProviderAccount, 0, len(ids)),
		Errors:  make([]string, 0),
	}
	if len(ids) == 0 || !validAccountStatus(status) {
		result.Errors = append(result.Errors, ErrInvalidInput.Error())
		return result
	}
	for _, id := range ids {
		updated, err := s.Update(ctx, id, contract.UpdateRequest{Status: &status})
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
	normalized := cloneString(proxyID)
	if normalized != nil {
		trimmed := strings.TrimSpace(*normalized)
		if trimmed == "" {
			normalized = nil
		} else {
			normalized = &trimmed
		}
	}
	return s.Update(ctx, id, contract.UpdateRequest{ProxyID: &normalized})
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

func validGroupStatus(status contract.GroupStatus) bool {
	switch status {
	case contract.GroupStatusActive, contract.GroupStatusDisabled:
		return true
	default:
		return false
	}
}

func validAccountStatus(status contract.Status) bool {
	switch status {
	case contract.StatusActive, contract.StatusDisabled, contract.StatusNeedsReauth, contract.StatusSuspended, contract.StatusDead:
		return true
	default:
		return false
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

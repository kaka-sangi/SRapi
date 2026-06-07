package accounts

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entaccountgroup "github.com/srapi/srapi/apps/api/ent/accountgroup"
	entaccountgroupmember "github.com/srapi/srapi/apps/api/ent/accountgroupmember"
	entaccounthealthsnapshot "github.com/srapi/srapi/apps/api/ent/accounthealthsnapshot"
	entaccountquotasnapshot "github.com/srapi/srapi/apps/api/ent/accountquotasnapshot"
	entaccount "github.com/srapi/srapi/apps/api/ent/provideraccount"
	entproxy "github.com/srapi/srapi/apps/api/ent/proxy"
	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

var ErrInvalidStore = errors.New("invalid accounts ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateStoredAccount) (contract.ProviderAccount, error) {
	created, err := s.client.ProviderAccount.Create().
		SetProviderID(input.ProviderID).
		SetName(input.Name).
		SetRuntimeClass(string(input.RuntimeClass)).
		SetAccountType(string(input.RuntimeClass)).
		SetNillableUpstreamClient(input.UpstreamClient).
		SetCredentialCiphertext([]byte(input.CredentialCiphertext)).
		SetCredentialVersion(credentialVersionToInt(input.CredentialVersion)).
		SetNillableProxyID(input.ProxyID).
		SetStatus(string(input.Status)).
		SetPriority(input.Priority).
		SetWeight(float64(input.Weight)).
		SetMetadataJSON(cloneMap(input.Metadata)).
		Save(ctx)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return toAccount(created), nil
}

func (s *Store) Update(ctx context.Context, account contract.ProviderAccount) (contract.ProviderAccount, error) {
	update := s.client.ProviderAccount.UpdateOneID(account.ID).
		Where(entaccount.DeletedAtIsNil()).
		SetProviderID(account.ProviderID).
		SetName(account.Name).
		SetRuntimeClass(string(account.RuntimeClass)).
		SetAccountType(string(account.RuntimeClass)).
		SetNillableUpstreamClient(account.UpstreamClient).
		SetCredentialCiphertext([]byte(account.CredentialCiphertext)).
		SetCredentialVersion(credentialVersionToInt(account.CredentialVersion)).
		SetStatus(string(account.Status)).
		SetPriority(account.Priority).
		SetWeight(float64(account.Weight)).
		SetMetadataJSON(cloneMap(account.Metadata))
	if account.ProxyID == nil {
		update.ClearProxyID()
	} else {
		update.SetProxyID(*account.ProxyID)
	}
	if account.RiskLevel != nil {
		update.SetRiskLevel(*account.RiskLevel)
	}
	if !account.UpdatedAt.IsZero() {
		update.SetUpdatedAt(account.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return toAccount(updated), nil
}

func (s *Store) FindByID(ctx context.Context, id int) (contract.ProviderAccount, error) {
	found, err := s.client.ProviderAccount.Query().
		Where(entaccount.IDEQ(id), entaccount.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return toAccount(found), nil
}

func (s *Store) List(ctx context.Context) ([]contract.ProviderAccount, error) {
	rows, err := s.client.ProviderAccount.Query().
		Where(entaccount.DeletedAtIsNil()).
		Order(entaccount.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProviderAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAccount(row))
	}
	return out, nil
}

func (s *Store) ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error) {
	rows, err := s.client.AccountGroupMember.Query().
		Where(entaccountgroupmember.AccountIDEQ(accountID)).
		Order(entaccountgroupmember.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.AccountGroupID)
	}
	return out, nil
}

func (s *Store) CreateProxy(ctx context.Context, input contract.CreateStoredProxy) (contract.ProxyDefinition, error) {
	created, err := s.client.Proxy.Create().
		SetName(input.Name).
		SetType(string(input.Type)).
		SetURLCiphertext([]byte(input.URLCiphertext)).
		SetURLVersion(credentialVersionToInt(input.URLVersion)).
		SetStatus(string(input.Status)).
		SetMetadataJSON(cloneMap(input.Metadata)).
		Save(ctx)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	return toProxy(created), nil
}

func (s *Store) UpdateProxy(ctx context.Context, proxy contract.ProxyDefinition) (contract.ProxyDefinition, error) {
	update := s.client.Proxy.UpdateOneID(proxy.ID).
		Where(entproxy.DeletedAtIsNil()).
		SetName(proxy.Name).
		SetType(string(proxy.Type)).
		SetURLCiphertext([]byte(proxy.URLCiphertext)).
		SetURLVersion(credentialVersionToInt(proxy.URLVersion)).
		SetStatus(string(proxy.Status)).
		SetMetadataJSON(cloneMap(proxy.Metadata))
	if !proxy.UpdatedAt.IsZero() {
		update.SetUpdatedAt(proxy.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	return toProxy(updated), nil
}

func (s *Store) FindProxyByID(ctx context.Context, id int) (contract.ProxyDefinition, error) {
	found, err := s.client.Proxy.Query().
		Where(entproxy.IDEQ(id), entproxy.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	return toProxy(found), nil
}

func (s *Store) ListProxies(ctx context.Context) ([]contract.ProxyDefinition, error) {
	rows, err := s.client.Proxy.Query().
		Where(entproxy.DeletedAtIsNil()).
		Order(entproxy.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProxyDefinition, 0, len(rows))
	for _, row := range rows {
		out = append(out, toProxy(row))
	}
	return out, nil
}

func (s *Store) CreateGroup(ctx context.Context, input contract.CreateStoredAccountGroup) (contract.AccountGroup, error) {
	created, err := s.client.AccountGroup.Create().
		SetName(input.Name).
		SetDescription(input.Description).
		SetProviderScopeJSON(cloneMap(input.ProviderScope)).
		SetModelScopeJSON(cloneMap(input.ModelScope)).
		SetStrategyHint(input.StrategyHint).
		SetStatus(string(input.Status)).
		Save(ctx)
	if err != nil {
		return contract.AccountGroup{}, err
	}
	return toGroup(created), nil
}

func (s *Store) UpdateGroup(ctx context.Context, group contract.AccountGroup) (contract.AccountGroup, error) {
	update := s.client.AccountGroup.UpdateOneID(group.ID).
		SetName(group.Name).
		SetDescription(group.Description).
		SetProviderScopeJSON(cloneMap(group.ProviderScope)).
		SetModelScopeJSON(cloneMap(group.ModelScope)).
		SetStrategyHint(group.StrategyHint).
		SetStatus(string(group.Status))
	if !group.UpdatedAt.IsZero() {
		update.SetUpdatedAt(group.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return contract.AccountGroup{}, err
	}
	return toGroup(updated), nil
}

func (s *Store) FindGroupByID(ctx context.Context, id int) (contract.AccountGroup, error) {
	found, err := s.client.AccountGroup.Query().
		Where(entaccountgroup.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return contract.AccountGroup{}, err
	}
	return toGroup(found), nil
}

func (s *Store) ListGroups(ctx context.Context) ([]contract.AccountGroup, error) {
	rows, err := s.client.AccountGroup.Query().
		Order(entaccountgroup.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AccountGroup, 0, len(rows))
	for _, row := range rows {
		out = append(out, toGroup(row))
	}
	return out, nil
}

func (s *Store) DeleteGroup(ctx context.Context, id int) error {
	if _, err := s.client.AccountGroupMember.Delete().
		Where(entaccountgroupmember.AccountGroupIDEQ(id)).
		Exec(ctx); err != nil {
		return err
	}
	return s.client.AccountGroup.DeleteOneID(id).Exec(ctx)
}

func (s *Store) AddAccountToGroup(ctx context.Context, accountID int, groupID int) (contract.AccountGroupMember, error) {
	found, err := s.client.AccountGroupMember.Query().
		Where(
			entaccountgroupmember.AccountIDEQ(accountID),
			entaccountgroupmember.AccountGroupIDEQ(groupID),
		).
		Only(ctx)
	if err == nil {
		return toGroupMember(found), nil
	}
	if !ent.IsNotFound(err) {
		return contract.AccountGroupMember{}, err
	}
	created, err := s.client.AccountGroupMember.Create().
		SetAccountID(accountID).
		SetAccountGroupID(groupID).
		Save(ctx)
	if err != nil {
		return contract.AccountGroupMember{}, err
	}
	return toGroupMember(created), nil
}

func (s *Store) RemoveAccountFromGroup(ctx context.Context, accountID int, groupID int) error {
	_, err := s.client.AccountGroupMember.Delete().
		Where(
			entaccountgroupmember.AccountIDEQ(accountID),
			entaccountgroupmember.AccountGroupIDEQ(groupID),
		).
		Exec(ctx)
	return err
}

func (s *Store) ListGroupMembers(ctx context.Context, groupID int) ([]contract.AccountGroupMember, error) {
	rows, err := s.client.AccountGroupMember.Query().
		Where(entaccountgroupmember.AccountGroupIDEQ(groupID)).
		Order(entaccountgroupmember.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AccountGroupMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, toGroupMember(row))
	}
	return out, nil
}

func (s *Store) RecordHealthSnapshot(ctx context.Context, snapshot contract.AccountHealthSnapshot) (contract.AccountHealthSnapshot, error) {
	created, err := s.client.AccountHealthSnapshot.Create().
		SetAccountID(snapshot.AccountID).
		SetProviderID(snapshot.ProviderID).
		SetStatus(snapshot.Status).
		SetSuccessRate(float64(snapshot.SuccessRate)).
		SetErrorRate(float64(snapshot.ErrorRate)).
		SetLatencyP50Ms(snapshot.LatencyP50MS).
		SetLatencyP95Ms(snapshot.LatencyP95MS).
		SetRateLimitCount(snapshot.RateLimitCount).
		SetTimeoutCount(snapshot.TimeoutCount).
		SetNillableCooldownUntil(snapshot.CooldownUntil).
		SetCircuitState(snapshot.CircuitState).
		SetSnapshotAt(snapshot.SnapshotAt).
		Save(ctx)
	if err != nil {
		return contract.AccountHealthSnapshot{}, err
	}
	return toHealthSnapshot(created), nil
}

func (s *Store) LatestHealthSnapshotByAccount(ctx context.Context, accountID int) (contract.AccountHealthSnapshot, error) {
	found, err := s.client.AccountHealthSnapshot.Query().
		Where(entaccounthealthsnapshot.AccountIDEQ(accountID)).
		Order(ent.Desc(entaccounthealthsnapshot.FieldSnapshotAt), ent.Desc(entaccounthealthsnapshot.FieldID)).
		First(ctx)
	if err != nil {
		return contract.AccountHealthSnapshot{}, err
	}
	return toHealthSnapshot(found), nil
}

func (s *Store) ListHealthSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]contract.AccountHealthSnapshot, error) {
	query := s.client.AccountHealthSnapshot.Query().
		Where(entaccounthealthsnapshot.AccountIDEQ(accountID)).
		Order(ent.Desc(entaccounthealthsnapshot.FieldSnapshotAt), ent.Desc(entaccounthealthsnapshot.FieldID))
	if limit > 0 {
		query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AccountHealthSnapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, toHealthSnapshot(row))
	}
	return out, nil
}

func (s *Store) RecordQuotaSnapshot(ctx context.Context, snapshot contract.AccountQuotaSnapshot) (contract.AccountQuotaSnapshot, error) {
	created, err := s.client.AccountQuotaSnapshot.Create().
		SetAccountID(snapshot.AccountID).
		SetProviderID(snapshot.ProviderID).
		SetQuotaType(snapshot.QuotaType).
		SetRemaining(snapshot.Remaining).
		SetUsed(snapshot.Used).
		SetQuotaLimit(snapshot.QuotaLimit).
		SetRemainingRatio(float64(snapshot.RemainingRatio)).
		SetNillableResetAt(snapshot.ResetAt).
		SetSnapshotAt(snapshot.SnapshotAt).
		Save(ctx)
	if err != nil {
		return contract.AccountQuotaSnapshot{}, err
	}
	return toQuotaSnapshot(created), nil
}

func (s *Store) ListQuotaSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]contract.AccountQuotaSnapshot, error) {
	query := s.client.AccountQuotaSnapshot.Query().
		Where(entaccountquotasnapshot.AccountIDEQ(accountID)).
		Order(ent.Desc(entaccountquotasnapshot.FieldSnapshotAt), ent.Desc(entaccountquotasnapshot.FieldID))
	if limit > 0 {
		query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AccountQuotaSnapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, toQuotaSnapshot(row))
	}
	return out, nil
}

func toAccount(row *ent.ProviderAccount) contract.ProviderAccount {
	return contract.ProviderAccount{
		ID:                   row.ID,
		ProviderID:           row.ProviderID,
		Name:                 row.Name,
		RuntimeClass:         contract.RuntimeClass(row.RuntimeClass),
		UpstreamClient:       cloneString(row.UpstreamClient),
		CredentialCiphertext: string(row.CredentialCiphertext),
		CredentialVersion:    credentialVersionToString(row.CredentialVersion),
		ProxyID:              cloneString(row.ProxyID),
		Status:               contract.Status(row.Status),
		Priority:             row.Priority,
		Weight:               float32(row.Weight),
		RiskLevel:            nonEmptyStringPtr(row.RiskLevel),
		Metadata:             cloneMap(row.MetadataJSON),
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		DeletedAt:            cloneTime(row.DeletedAt),
	}
}

func toProxy(row *ent.Proxy) contract.ProxyDefinition {
	return contract.ProxyDefinition{
		ID:            row.ID,
		Name:          row.Name,
		Type:          contract.ProxyType(row.Type),
		URLCiphertext: string(row.URLCiphertext),
		URLVersion:    credentialVersionToString(row.URLVersion),
		Status:        contract.ProxyStatus(row.Status),
		Metadata:      cloneMap(row.MetadataJSON),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		DeletedAt:     cloneTime(row.DeletedAt),
	}
}

func toGroup(row *ent.AccountGroup) contract.AccountGroup {
	return contract.AccountGroup{
		ID:            row.ID,
		Name:          row.Name,
		Description:   row.Description,
		ProviderScope: cloneMap(row.ProviderScopeJSON),
		ModelScope:    cloneMap(row.ModelScopeJSON),
		StrategyHint:  row.StrategyHint,
		Status:        contract.GroupStatus(row.Status),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func toGroupMember(row *ent.AccountGroupMember) contract.AccountGroupMember {
	return contract.AccountGroupMember{
		ID:             row.ID,
		AccountID:      row.AccountID,
		AccountGroupID: row.AccountGroupID,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func toHealthSnapshot(row *ent.AccountHealthSnapshot) contract.AccountHealthSnapshot {
	return contract.AccountHealthSnapshot{
		ID:             row.ID,
		AccountID:      row.AccountID,
		ProviderID:     row.ProviderID,
		Status:         row.Status,
		SuccessRate:    float32(row.SuccessRate),
		ErrorRate:      float32(row.ErrorRate),
		LatencyP50MS:   row.LatencyP50Ms,
		LatencyP95MS:   row.LatencyP95Ms,
		RateLimitCount: row.RateLimitCount,
		TimeoutCount:   row.TimeoutCount,
		CooldownUntil:  cloneTime(row.CooldownUntil),
		CircuitState:   row.CircuitState,
		SnapshotAt:     row.SnapshotAt,
	}
}

func toQuotaSnapshot(row *ent.AccountQuotaSnapshot) contract.AccountQuotaSnapshot {
	return contract.AccountQuotaSnapshot{
		ID:             row.ID,
		AccountID:      row.AccountID,
		ProviderID:     row.ProviderID,
		QuotaType:      row.QuotaType,
		Remaining:      row.Remaining,
		Used:           row.Used,
		QuotaLimit:     row.QuotaLimit,
		RemainingRatio: float32(row.RemainingRatio),
		ResetAt:        cloneTime(row.ResetAt),
		SnapshotAt:     row.SnapshotAt,
	}
}

func credentialVersionToInt(value string) int {
	trimmed := strings.TrimSpace(strings.TrimPrefix(value, "v"))
	if trimmed == "" {
		return 1
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed <= 0 {
		return 1
	}
	return parsed
}

func credentialVersionToString(value int) string {
	if value <= 0 {
		value = 1
	}
	return "v" + strconv.Itoa(value)
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, val := range value {
		cloned[key] = val
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

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func nonEmptyStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	cloned := value
	return &cloned
}

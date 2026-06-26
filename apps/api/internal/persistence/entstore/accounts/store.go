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
	"github.com/srapi/srapi/apps/api/ent/predicate"
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
	builder := s.client.ProviderAccount.Create().
		SetProviderID(input.ProviderID).
		SetName(input.Name).
		SetPlatform(input.Platform).
		SetAccountType(string(input.AccountType)).
		SetRuntimeClass(string(input.RuntimeClass)).
		SetNillableUpstreamClient(input.UpstreamClient).
		SetCredentialCiphertext([]byte(input.CredentialCiphertext)).
		SetCredentialVersion(credentialVersionToInt(input.CredentialVersion)).
		SetNillableProxyID(input.ProxyID).
		SetStatus(string(input.Status)).
		SetPriority(input.Priority).
		SetWeight(float64(input.Weight)).
		SetNillableRiskLevel(input.RiskLevel).
		SetMetadataJSON(cloneMap(input.Metadata)).
		SetNotes(input.Notes).
		SetConcurrency(input.Concurrency).
		SetRateMultiplier(input.RateMultiplier).
		SetSchedulable(input.Schedulable).
		SetAutoPauseOnExpired(input.AutoPauseOnExpired).
		SetExtraJSON(cloneMap(input.Extra))
	if input.LoadFactor != nil {
		builder.SetLoadFactor(*input.LoadFactor)
	}
	if input.ExpiresAt != nil {
		builder.SetExpiresAt(*input.ExpiresAt)
	}
	created, err := builder.Save(ctx)
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
		SetPlatform(account.Platform).
		SetAccountType(string(account.AccountType)).
		SetRuntimeClass(string(account.RuntimeClass)).
		SetNillableUpstreamClient(account.UpstreamClient).
		SetCredentialCiphertext([]byte(account.CredentialCiphertext)).
		SetCredentialVersion(credentialVersionToInt(account.CredentialVersion)).
		SetStatus(string(account.Status)).
		SetPriority(account.Priority).
		SetWeight(float64(account.Weight)).
		SetMetadataJSON(cloneMap(account.Metadata)).
		SetNotes(account.Notes).
		SetConcurrency(account.Concurrency).
		SetRateMultiplier(account.RateMultiplier).
		SetSchedulable(account.Schedulable).
		SetErrorMessage(account.ErrorMessage).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired).
		SetTempUnschedulableReason(account.TempUnschedulableReason).
		SetSessionWindowStatus(account.SessionWindowStatus).
		SetExtraJSON(cloneMap(account.Extra)).
		SetRefreshAttempts(account.RefreshAttempts).
		SetRefreshLastError(account.RefreshLastError)
	if account.ProxyID == nil {
		update.ClearProxyID()
	} else {
		update.SetProxyID(*account.ProxyID)
	}
	if account.RiskLevel != nil {
		update.SetRiskLevel(*account.RiskLevel)
	}
	if account.LoadFactor != nil {
		update.SetLoadFactor(*account.LoadFactor)
	} else {
		update.ClearLoadFactor()
	}
	setOrClearNillableTime(update, account.TokenExpiresAt, "token_expires_at")
	setOrClearNillableTime(update, account.LastRefreshedAt, "last_refreshed_at")
	setOrClearNillableTime(update, account.NeedsReauthAt, "needs_reauth_at")
	setOrClearNillableTime(update, account.LastUsedAt, "last_used_at")
	setOrClearNillableTime(update, account.ExpiresAt, "expires_at")
	setOrClearNillableTime(update, account.RateLimitedAt, "rate_limited_at")
	setOrClearNillableTime(update, account.RateLimitResetAt, "rate_limit_reset_at")
	setOrClearNillableTime(update, account.OverloadUntil, "overload_until")
	setOrClearNillableTime(update, account.TempUnschedulableUntil, "temp_unschedulable_until")
	setOrClearNillableTime(update, account.SessionWindowStart, "session_window_start")
	setOrClearNillableTime(update, account.SessionWindowEnd, "session_window_end")
	if !account.UpdatedAt.IsZero() {
		update.SetUpdatedAt(account.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return toAccount(updated), nil
}

type timeFieldSetter interface {
	SetTokenExpiresAt(time.Time) *ent.ProviderAccountUpdateOne
	SetLastRefreshedAt(time.Time) *ent.ProviderAccountUpdateOne
	SetNeedsReauthAt(time.Time) *ent.ProviderAccountUpdateOne
	SetLastUsedAt(time.Time) *ent.ProviderAccountUpdateOne
	SetExpiresAt(time.Time) *ent.ProviderAccountUpdateOne
	SetRateLimitedAt(time.Time) *ent.ProviderAccountUpdateOne
	SetRateLimitResetAt(time.Time) *ent.ProviderAccountUpdateOne
	SetOverloadUntil(time.Time) *ent.ProviderAccountUpdateOne
	SetTempUnschedulableUntil(time.Time) *ent.ProviderAccountUpdateOne
	SetSessionWindowStart(time.Time) *ent.ProviderAccountUpdateOne
	SetSessionWindowEnd(time.Time) *ent.ProviderAccountUpdateOne
}

func setOrClearNillableTime(update *ent.ProviderAccountUpdateOne, value *time.Time, field string) {
	if value != nil {
		switch field {
		case "token_expires_at":
			update.SetTokenExpiresAt(*value)
		case "last_refreshed_at":
			update.SetLastRefreshedAt(*value)
		case "needs_reauth_at":
			update.SetNeedsReauthAt(*value)
		case "last_used_at":
			update.SetLastUsedAt(*value)
		case "expires_at":
			update.SetExpiresAt(*value)
		case "rate_limited_at":
			update.SetRateLimitedAt(*value)
		case "rate_limit_reset_at":
			update.SetRateLimitResetAt(*value)
		case "overload_until":
			update.SetOverloadUntil(*value)
		case "temp_unschedulable_until":
			update.SetTempUnschedulableUntil(*value)
		case "session_window_start":
			update.SetSessionWindowStart(*value)
		case "session_window_end":
			update.SetSessionWindowEnd(*value)
		}
	} else {
		switch field {
		case "token_expires_at":
			update.ClearTokenExpiresAt()
		case "last_refreshed_at":
			update.ClearLastRefreshedAt()
		case "needs_reauth_at":
			update.ClearNeedsReauthAt()
		case "last_used_at":
			update.ClearLastUsedAt()
		case "expires_at":
			update.ClearExpiresAt()
		case "rate_limited_at":
			update.ClearRateLimitedAt()
		case "rate_limit_reset_at":
			update.ClearRateLimitResetAt()
		case "overload_until":
			update.ClearOverloadUntil()
		case "temp_unschedulable_until":
			update.ClearTempUnschedulableUntil()
		case "session_window_start":
			update.ClearSessionWindowStart()
		case "session_window_end":
			update.ClearSessionWindowEnd()
		}
	}
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

// ListPage implements contract.PageReader: filter, count, and slice in SQL
// with ORDER BY id DESC so the newest accounts come back first. Pushes the
// status / provider / runtime_class / search / group filters down to SQL —
// the prior admin handler loaded the full provider_accounts table just to
// filter it in Go memory.
func (s *Store) ListPage(ctx context.Context, filter contract.ListFilter, limit, offset int) (contract.ListPageResult, error) {
	predicates, err := s.accountPagePredicates(ctx, filter)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	base := s.client.ProviderAccount.Query().Where(predicates...)
	total, err := base.Clone().Count(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	query := base.Order(ent.Desc(entaccount.FieldID))
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	out := make([]contract.ProviderAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAccount(row))
	}
	return contract.ListPageResult{Items: out, Total: total}, nil
}

func (s *Store) accountPagePredicates(ctx context.Context, filter contract.ListFilter) ([]predicate.ProviderAccount, error) {
	preds := []predicate.ProviderAccount{entaccount.DeletedAtIsNil()}

	// Status: when unset, hide archived rows unless caller opts in. When set,
	// match exactly. Mirrors filterAccounts behavior the old handler used.
	if filter.Status == "" {
		if !filter.IncludeArchived {
			preds = append(preds, entaccount.StatusNEQ(string(contract.StatusArchived)))
		}
	} else {
		preds = append(preds, entaccount.StatusEQ(string(filter.Status)))
	}
	if filter.ProviderID != nil {
		preds = append(preds, entaccount.ProviderIDEQ(*filter.ProviderID))
	}
	if filter.Platform != "" {
		preds = append(preds, entaccount.PlatformEQ(filter.Platform))
	}
	if filter.RuntimeClass != "" {
		preds = append(preds, entaccount.RuntimeClassEQ(string(filter.RuntimeClass)))
	}
	if filter.AccountType != "" {
		preds = append(preds, entaccount.AccountTypeEQ(string(filter.AccountType)))
	}
	if filter.SchedulableOnly {
		preds = append(preds, entaccount.SchedulableEQ(true))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		searchPreds := []predicate.ProviderAccount{
			entaccount.NameContainsFold(search),
			entaccount.UpstreamClientContainsFold(search),
		}
		if id, ok := atoiIfDigits(search); ok {
			searchPreds = append(searchPreds, entaccount.IDEQ(id))
		}
		preds = append(preds, entaccount.Or(searchPreds...))
	}
	// Group membership lives on a separate edge table; resolve member ids in
	// one query so the page can stay a single IDIn predicate. The Limit(1)
	// cheap-path catches the "no members" case before composing a wide IN().
	if filter.GroupID != nil && *filter.GroupID > 0 {
		ids, err := s.client.AccountGroupMember.Query().
			Where(entaccountgroupmember.AccountGroupIDEQ(*filter.GroupID)).
			Select(entaccountgroupmember.FieldAccountID).
			Ints(ctx)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			// Force a no-row predicate so the count and listing both return 0.
			preds = append(preds, entaccount.IDIn())
		} else {
			preds = append(preds, entaccount.IDIn(ids...))
		}
	}
	return preds, nil
}

// atoiIfDigits returns (value, true) only when `s` is a non-empty positive
// integer literal. Caller uses this to extend a name/upstream search with an
// exact id match without leaking "name contains 4" into "id equals 4" false
// positives.
func atoiIfDigits(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (s *Store) ListActiveByProviderIDs(ctx context.Context, providerIDs []int) ([]contract.ProviderAccount, error) {
	if len(providerIDs) == 0 {
		return nil, nil
	}
	rows, err := s.client.ProviderAccount.Query().
		Where(
			entaccount.DeletedAtIsNil(),
			entaccount.ProviderIDIn(providerIDs...),
			entaccount.StatusEQ(string(contract.StatusActive)),
		).
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

func (s *Store) ListOAuthDueForRefresh(ctx context.Context, deadline time.Time) ([]contract.ProviderAccount, error) {
	rows, err := s.client.ProviderAccount.Query().
		Where(
			entaccount.DeletedAtIsNil(),
			entaccount.StatusEQ(string(contract.StatusActive)),
			entaccount.RuntimeClassIn(
				string(contract.RuntimeClassOauthRefresh),
				string(contract.RuntimeClassOauthDeviceCode),
			),
			entaccount.NeedsReauthAtIsNil(),
			entaccount.TokenExpiresAtNotNil(),
			entaccount.TokenExpiresAtLTE(deadline),
		).
		Order(entaccount.ByTokenExpiresAt()).
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

func (s *Store) ListOAuthKeepaliveCandidates(ctx context.Context, staleBefore time.Time, refreshDeadline time.Time, batchSize int) ([]contract.ProviderAccount, error) {
	query := s.client.ProviderAccount.Query().
		Where(
			entaccount.DeletedAtIsNil(),
			entaccount.StatusEQ(string(contract.StatusActive)),
			entaccount.RuntimeClassIn(
				string(contract.RuntimeClassOauthRefresh),
				string(contract.RuntimeClassOauthDeviceCode),
			),
			entaccount.NeedsReauthAtIsNil(),
			entaccount.Or(
				entaccount.TokenExpiresAtIsNil(),
				entaccount.TokenExpiresAtGT(refreshDeadline),
			),
			entaccount.Or(
				entaccount.And(
					entaccount.LastRefreshedAtNotNil(),
					entaccount.LastRefreshedAtLT(staleBefore),
				),
				entaccount.And(
					entaccount.LastRefreshedAtIsNil(),
					entaccount.CreatedAtLT(staleBefore),
				),
			),
		).
		Order(entaccount.ByLastRefreshedAt())
	if batchSize > 0 {
		query = query.Limit(batchSize)
	}
	rows, err := query.All(ctx)
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

func (s *Store) ListGroupIDsByAccounts(ctx context.Context, accountIDs []int) (map[int][]int, error) {
	ids := normalizePositiveIDs(accountIDs)
	out := make(map[int][]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	for _, accountID := range ids {
		out[accountID] = nil
	}
	rows, err := s.client.AccountGroupMember.Query().
		Where(entaccountgroupmember.AccountIDIn(ids...)).
		Order(entaccountgroupmember.ByAccountID(), entaccountgroupmember.ByAccountGroupID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.AccountID] = append(out[row.AccountID], row.AccountGroupID)
	}
	return out, nil
}

func (s *Store) CreateProxy(ctx context.Context, input contract.CreateStoredProxy) (contract.ProxyDefinition, error) {
	fallbackMode := input.FallbackMode
	if fallbackMode == "" {
		fallbackMode = contract.ProxyFallbackModeNone
	}
	created, err := s.client.Proxy.Create().
		SetName(input.Name).
		SetType(string(input.Type)).
		SetProtocol(input.Protocol).
		SetHost(input.Host).
		SetPort(input.Port).
		SetUsername(input.Username).
		SetPasswordCiphertext([]byte(input.PasswordCiphertext)).
		SetURLCiphertext([]byte(input.URLCiphertext)).
		SetURLVersion(credentialVersionToInt(input.URLVersion)).
		SetStatus(string(input.Status)).
		SetMetadataJSON(cloneMap(input.Metadata)).
		SetCountryCode(input.CountryCode).
		SetCountryName(input.CountryName).
		SetNillableExpiresAt(input.ExpiresAt).
		SetFallbackMode(string(fallbackMode)).
		SetNillableBackupProxyID(input.BackupProxyID).
		SetExpiryWarnDays(input.ExpiryWarnDays).
		Save(ctx)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	return toProxy(created), nil
}

func (s *Store) UpdateProxy(ctx context.Context, proxy contract.ProxyDefinition) (contract.ProxyDefinition, error) {
	if proxy.FallbackMode == "" {
		proxy.FallbackMode = contract.ProxyFallbackModeNone
	}
	update := s.client.Proxy.UpdateOneID(proxy.ID).
		SetName(proxy.Name).
		SetType(string(proxy.Type)).
		SetProtocol(proxy.Protocol).
		SetHost(proxy.Host).
		SetPort(proxy.Port).
		SetUsername(proxy.Username).
		SetPasswordCiphertext([]byte(proxy.PasswordCiphertext)).
		SetURLCiphertext([]byte(proxy.URLCiphertext)).
		SetURLVersion(credentialVersionToInt(proxy.URLVersion)).
		SetStatus(string(proxy.Status)).
		SetMetadataJSON(cloneMap(proxy.Metadata)).
		SetCountryCode(proxy.CountryCode).
		SetCountryName(proxy.CountryName).
		SetFallbackMode(string(proxy.FallbackMode)).
		SetExpiryWarnDays(proxy.ExpiryWarnDays).
		SetProbeSuccessCount(proxy.ProbeSuccessCount).
		SetProbeFailureCount(proxy.ProbeFailureCount).
		SetLastProbeLatencyMs(proxy.LastProbeLatencyMs)
	if proxy.ExpiresAt != nil {
		update.SetExpiresAt(*proxy.ExpiresAt)
	} else {
		update.ClearExpiresAt()
	}
	if proxy.BackupProxyID != nil {
		update.SetBackupProxyID(*proxy.BackupProxyID)
	} else {
		update.ClearBackupProxyID()
	}
	if proxy.LastProbedAt != nil {
		update.SetLastProbedAt(*proxy.LastProbedAt)
	} else {
		update.ClearLastProbedAt()
	}
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
		Where(entproxy.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	return toProxy(found), nil
}

func (s *Store) ListProxies(ctx context.Context) ([]contract.ProxyDefinition, error) {
	rows, err := s.client.Proxy.Query().
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

func (s *Store) DeleteProxy(ctx context.Context, id int) error {
	// Clear bindings: accounts referencing this proxy fall back to a direct
	// connection.
	if _, err := s.client.ProviderAccount.Update().
		Where(entaccount.ProxyIDEQ(strconv.Itoa(id))).
		ClearProxyID().
		Save(ctx); err != nil {
		return err
	}
	err := s.client.Proxy.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.New("proxy not found")
		}
		return err
	}
	return nil
}

func (s *Store) CreateGroup(ctx context.Context, input contract.CreateStoredAccountGroup) (contract.AccountGroup, error) {
	created, err := s.client.AccountGroup.Create().
		SetName(input.Name).
		SetDescription(input.Description).
		SetProviderScopeJSON(cloneMap(input.ProviderScope)).
		SetModelScopeJSON(cloneMap(input.ModelScope)).
		SetStrategyHint(input.StrategyHint).
		SetRateMultiplier(input.RateMultiplier).
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
		SetRateMultiplier(group.RateMultiplier).
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

func (s *Store) FindGroupsByID(ctx context.Context, ids []int) ([]contract.AccountGroup, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.client.AccountGroup.Query().
		Where(entaccountgroup.IDIn(ids...)).
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

// quotaSnapshotScanCap bounds how many rows a per-account quota listing will
// pull before the per-type limit is applied in Go. Snapshots are ordered
// newest-first, so the cap only drops history far beyond what any caller pages
// through; without it a busy account makes this query fetch its entire
// snapshot history.
const quotaSnapshotScanCap = 1000

func (s *Store) ListQuotaSnapshotsByAccount(ctx context.Context, accountID int, limit int) ([]contract.AccountQuotaSnapshot, error) {
	query := s.client.AccountQuotaSnapshot.Query().
		Where(entaccountquotasnapshot.AccountIDEQ(accountID)).
		Order(ent.Desc(entaccountquotasnapshot.FieldSnapshotAt), ent.Desc(entaccountquotasnapshot.FieldID)).
		Limit(quotaSnapshotScanCap)
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AccountQuotaSnapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, toQuotaSnapshot(row))
	}
	return limitQuotaSnapshotsByType(out, limit), nil
}

func limitQuotaSnapshotsByType(snapshots []contract.AccountQuotaSnapshot, limit int) []contract.AccountQuotaSnapshot {
	if limit <= 0 {
		return snapshots
	}
	countByType := map[string]int{}
	out := make([]contract.AccountQuotaSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		quotaType := strings.TrimSpace(snapshot.QuotaType)
		if countByType[quotaType] >= limit {
			continue
		}
		countByType[quotaType]++
		out = append(out, snapshot)
	}
	return out
}

func (s *Store) Delete(ctx context.Context, id int) error {
	if _, err := s.client.AccountHealthSnapshot.Delete().
		Where(entaccounthealthsnapshot.AccountIDEQ(id)).
		Exec(ctx); err != nil {
		return err
	}
	if _, err := s.client.AccountQuotaSnapshot.Delete().
		Where(entaccountquotasnapshot.AccountIDEQ(id)).
		Exec(ctx); err != nil {
		return err
	}
	if _, err := s.client.AccountGroupMember.Delete().
		Where(entaccountgroupmember.AccountIDEQ(id)).
		Exec(ctx); err != nil {
		return err
	}
	return s.client.ProviderAccount.DeleteOneID(id).Exec(ctx)
}

func toAccount(row *ent.ProviderAccount) contract.ProviderAccount {
	return contract.ProviderAccount{
		ID:                      row.ID,
		ProviderID:              row.ProviderID,
		Name:                    row.Name,
		Platform:                row.Platform,
		AccountType:             contract.AccountType(row.AccountType),
		RuntimeClass:            contract.RuntimeClass(row.RuntimeClass),
		UpstreamClient:          cloneString(row.UpstreamClient),
		CredentialCiphertext:    string(row.CredentialCiphertext),
		CredentialVersion:       credentialVersionToString(row.CredentialVersion),
		ProxyID:                 cloneString(row.ProxyID),
		Status:                  contract.Status(row.Status),
		Priority:                row.Priority,
		Weight:                  float32(row.Weight),
		RiskLevel:               nonEmptyStringPtr(row.RiskLevel),
		Metadata:                cloneMap(row.MetadataJSON),
		Notes:                   row.Notes,
		Concurrency:             row.Concurrency,
		RateMultiplier:          row.RateMultiplier,
		LoadFactor:              cloneInt(row.LoadFactor),
		Schedulable:             row.Schedulable,
		ErrorMessage:            row.ErrorMessage,
		LastUsedAt:              cloneTime(row.LastUsedAt),
		ExpiresAt:               cloneTime(row.ExpiresAt),
		AutoPauseOnExpired:      row.AutoPauseOnExpired,
		RateLimitedAt:           cloneTime(row.RateLimitedAt),
		RateLimitResetAt:        cloneTime(row.RateLimitResetAt),
		OverloadUntil:           cloneTime(row.OverloadUntil),
		TempUnschedulableUntil:  cloneTime(row.TempUnschedulableUntil),
		TempUnschedulableReason: row.TempUnschedulableReason,
		SessionWindowStart:      cloneTime(row.SessionWindowStart),
		SessionWindowEnd:        cloneTime(row.SessionWindowEnd),
		SessionWindowStatus:     row.SessionWindowStatus,
		Extra:                   cloneMap(row.ExtraJSON),
		CreatedAt:               row.CreatedAt,
		UpdatedAt:               row.UpdatedAt,
		DeletedAt:               cloneTime(row.DeletedAt),
		TokenExpiresAt:          cloneTime(row.TokenExpiresAt),
		LastRefreshedAt:         cloneTime(row.LastRefreshedAt),
		NeedsReauthAt:           cloneTime(row.NeedsReauthAt),
		RefreshAttempts:         row.RefreshAttempts,
		RefreshLastError:        row.RefreshLastError,
	}
}

func toProxy(row *ent.Proxy) contract.ProxyDefinition {
	return contract.ProxyDefinition{
		ID:                 row.ID,
		Name:               row.Name,
		Type:               contract.ProxyType(row.Type),
		Protocol:           row.Protocol,
		Host:               row.Host,
		Port:               row.Port,
		Username:           row.Username,
		PasswordCiphertext: string(row.PasswordCiphertext),
		URLCiphertext:      string(row.URLCiphertext),
		URLVersion:         credentialVersionToString(row.URLVersion),
		Status:             contract.ProxyStatus(row.Status),
		Metadata:           cloneMap(row.MetadataJSON),
		CountryCode:        row.CountryCode,
		CountryName:        row.CountryName,
		ExpiresAt:          cloneTime(row.ExpiresAt),
		FallbackMode:       contract.ProxyFallbackMode(row.FallbackMode),
		BackupProxyID:      cloneInt(row.BackupProxyID),
		ExpiryWarnDays:     row.ExpiryWarnDays,
		LastProbedAt:       cloneTime(row.LastProbedAt),
		ProbeSuccessCount:  row.ProbeSuccessCount,
		ProbeFailureCount:  row.ProbeFailureCount,
		LastProbeLatencyMs: row.LastProbeLatencyMs,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func toGroup(row *ent.AccountGroup) contract.AccountGroup {
	return contract.AccountGroup{
		ID:             row.ID,
		Name:           row.Name,
		Description:    row.Description,
		ProviderScope:  cloneMap(row.ProviderScopeJSON),
		ModelScope:     cloneMap(row.ModelScopeJSON),
		StrategyHint:   row.StrategyHint,
		RateMultiplier: row.RateMultiplier,
		Status:         contract.GroupStatus(row.Status),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
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

func normalizePositiveIDs(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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

func cloneInt(value *int) *int {
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

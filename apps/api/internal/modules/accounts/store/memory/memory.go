package memory

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

type Store struct {
	mu                  sync.Mutex
	nextID              int
	nextProxyID         int
	nextGroupID         int
	nextGroupMemberID   int
	nextHealthID        int
	nextQuotaID         int
	byID                map[int]contract.ProviderAccount
	byName              map[string]int
	proxiesByID         map[int]contract.ProxyDefinition
	proxiesByName       map[string]int
	groupsByID          map[int]contract.AccountGroup
	groupsByName        map[string]int
	groupMembersByID    map[int]contract.AccountGroupMember
	healthSnapshotsByID map[int]contract.AccountHealthSnapshot
	quotaSnapshotsByID  map[int]contract.AccountQuotaSnapshot
}

func New() *Store {
	return &Store{
		nextID:              1,
		nextProxyID:         1,
		nextGroupID:         1,
		nextGroupMemberID:   1,
		nextHealthID:        1,
		nextQuotaID:         1,
		byID:                map[int]contract.ProviderAccount{},
		byName:              map[string]int{},
		proxiesByID:         map[int]contract.ProxyDefinition{},
		proxiesByName:       map[string]int{},
		groupsByID:          map[int]contract.AccountGroup{},
		groupsByName:        map[string]int{},
		groupMembersByID:    map[int]contract.AccountGroupMember{},
		healthSnapshotsByID: map[int]contract.AccountHealthSnapshot{},
		quotaSnapshotsByID:  map[int]contract.AccountQuotaSnapshot{},
	}
}

func (s *Store) Create(_ context.Context, input contract.CreateStoredAccount) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	account := contract.ProviderAccount{
		ID:                   s.nextID,
		ProviderID:           input.ProviderID,
		Name:                 input.Name,
		RuntimeClass:         input.RuntimeClass,
		CredentialCiphertext: input.CredentialCiphertext,
		CredentialVersion:    input.CredentialVersion,
		ProxyID:              input.ProxyID,
		Status:               input.Status,
		Priority:             input.Priority,
		Weight:               input.Weight,
		UpstreamClient:       input.UpstreamClient,
		Metadata:             input.Metadata,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	s.byID[account.ID] = account
	s.byName[strings.ToLower(account.Name)] = account.ID
	s.nextID++
	return account, nil
}

func (s *Store) Update(_ context.Context, account contract.ProviderAccount) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[account.ID]; !ok {
		return contract.ProviderAccount{}, errors.New("account not found")
	}
	stored := account
	stored.Metadata = cloneMap(account.Metadata)
	s.byID[stored.ID] = stored
	s.byName[strings.ToLower(stored.Name)] = stored.ID
	return cloneAccount(stored), nil
}

func (s *Store) FindByID(_ context.Context, id int) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.byID[id]
	if !ok {
		return contract.ProviderAccount{}, errors.New("account not found")
	}
	return cloneAccount(account), nil
}

func (s *Store) List(_ context.Context) ([]contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.ProviderAccount, 0, len(s.byID))
	for _, account := range s.byID {
		out = append(out, cloneAccount(account))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListGroupIDsByAccount(_ context.Context, accountID int) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[accountID]; !ok {
		return nil, errors.New("account not found")
	}
	out := make([]int, 0)
	for _, member := range s.groupMembersByID {
		if member.AccountID == accountID {
			out = append(out, member.AccountGroupID)
		}
	}
	sort.Ints(out)
	return out, nil
}

func (s *Store) CreateProxy(_ context.Context, input contract.CreateStoredProxy) (contract.ProxyDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.proxiesByName[strings.ToLower(input.Name)]; exists {
		return contract.ProxyDefinition{}, errors.New("proxy already exists")
	}
	now := time.Now().UTC()
	proxy := contract.ProxyDefinition{
		ID:            s.nextProxyID,
		Name:          input.Name,
		Type:          input.Type,
		URLCiphertext: input.URLCiphertext,
		URLVersion:    input.URLVersion,
		Status:        input.Status,
		Metadata:      cloneMap(input.Metadata),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.proxiesByID[proxy.ID] = proxy
	s.proxiesByName[strings.ToLower(proxy.Name)] = proxy.ID
	s.nextProxyID++
	return cloneProxy(proxy), nil
}

func (s *Store) UpdateProxy(_ context.Context, proxy contract.ProxyDefinition) (contract.ProxyDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.proxiesByID[proxy.ID]; !ok {
		return contract.ProxyDefinition{}, errors.New("proxy not found")
	}
	for id, existing := range s.proxiesByID {
		if id != proxy.ID && strings.EqualFold(existing.Name, proxy.Name) {
			return contract.ProxyDefinition{}, errors.New("proxy already exists")
		}
	}
	stored := cloneProxy(proxy)
	s.proxiesByID[stored.ID] = stored
	s.proxiesByName[strings.ToLower(stored.Name)] = stored.ID
	return cloneProxy(stored), nil
}

func (s *Store) FindProxyByID(_ context.Context, id int) (contract.ProxyDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	proxy, ok := s.proxiesByID[id]
	if !ok || proxy.DeletedAt != nil {
		return contract.ProxyDefinition{}, errors.New("proxy not found")
	}
	return cloneProxy(proxy), nil
}

func (s *Store) SoftDeleteProxy(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	proxy, ok := s.proxiesByID[id]
	if !ok || proxy.DeletedAt != nil {
		return errors.New("proxy not found")
	}
	now := time.Now().UTC()
	proxy.DeletedAt = &now
	proxy.Status = contract.ProxyStatusDisabled
	proxy.UpdatedAt = now
	s.proxiesByID[id] = proxy
	delete(s.proxiesByName, strings.ToLower(proxy.Name))
	// Clear bindings: accounts whose proxy_id points at this proxy by id fall
	// back to a direct connection (raw-URL proxy_id values are left intact).
	target := strconv.Itoa(id)
	for accountID, account := range s.byID {
		if account.ProxyID != nil && *account.ProxyID == target {
			account.ProxyID = nil
			account.UpdatedAt = now
			s.byID[accountID] = account
		}
	}
	return nil
}

func (s *Store) ListProxies(_ context.Context) ([]contract.ProxyDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.ProxyDefinition, 0, len(s.proxiesByID))
	for _, proxy := range s.proxiesByID {
		if proxy.DeletedAt != nil {
			continue
		}
		out = append(out, cloneProxy(proxy))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) CreateGroup(_ context.Context, input contract.CreateStoredAccountGroup) (contract.AccountGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	group := contract.AccountGroup{
		ID:            s.nextGroupID,
		Name:          input.Name,
		Description:   input.Description,
		ProviderScope: cloneMap(input.ProviderScope),
		ModelScope:    cloneMap(input.ModelScope),
		StrategyHint:  input.StrategyHint,
		Status:        input.Status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.groupsByID[group.ID] = group
	s.groupsByName[strings.ToLower(group.Name)] = group.ID
	s.nextGroupID++
	return cloneGroup(group), nil
}

func (s *Store) UpdateGroup(_ context.Context, group contract.AccountGroup) (contract.AccountGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groupsByID[group.ID]; !ok {
		return contract.AccountGroup{}, errors.New("account group not found")
	}
	stored := cloneGroup(group)
	s.groupsByID[stored.ID] = stored
	s.groupsByName[strings.ToLower(stored.Name)] = stored.ID
	return cloneGroup(stored), nil
}

func (s *Store) FindGroupByID(_ context.Context, id int) (contract.AccountGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, ok := s.groupsByID[id]
	if !ok {
		return contract.AccountGroup{}, errors.New("account group not found")
	}
	return cloneGroup(group), nil
}

func (s *Store) ListGroups(_ context.Context) ([]contract.AccountGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AccountGroup, 0, len(s.groupsByID))
	for _, group := range s.groupsByID {
		out = append(out, cloneGroup(group))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) DeleteGroup(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, ok := s.groupsByID[id]
	if !ok {
		return errors.New("account group not found")
	}
	delete(s.groupsByID, id)
	delete(s.groupsByName, strings.ToLower(group.Name))
	for memberID, member := range s.groupMembersByID {
		if member.AccountGroupID == id {
			delete(s.groupMembersByID, memberID)
		}
	}
	return nil
}

func (s *Store) AddAccountToGroup(_ context.Context, accountID int, groupID int) (contract.AccountGroupMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[accountID]; !ok {
		return contract.AccountGroupMember{}, errors.New("account not found")
	}
	if _, ok := s.groupsByID[groupID]; !ok {
		return contract.AccountGroupMember{}, errors.New("account group not found")
	}
	for _, member := range s.groupMembersByID {
		if member.AccountID == accountID && member.AccountGroupID == groupID {
			return cloneGroupMember(member), nil
		}
	}
	now := time.Now().UTC()
	member := contract.AccountGroupMember{
		ID:             s.nextGroupMemberID,
		AccountID:      accountID,
		AccountGroupID: groupID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.groupMembersByID[member.ID] = member
	s.nextGroupMemberID++
	return cloneGroupMember(member), nil
}

func (s *Store) RemoveAccountFromGroup(_ context.Context, accountID int, groupID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, member := range s.groupMembersByID {
		if member.AccountID == accountID && member.AccountGroupID == groupID {
			delete(s.groupMembersByID, id)
			return nil
		}
	}
	return nil
}

func (s *Store) ListGroupMembers(_ context.Context, groupID int) ([]contract.AccountGroupMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groupsByID[groupID]; !ok {
		return nil, errors.New("account group not found")
	}
	out := make([]contract.AccountGroupMember, 0)
	for _, member := range s.groupMembersByID {
		if member.AccountGroupID == groupID {
			out = append(out, cloneGroupMember(member))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) RecordHealthSnapshot(_ context.Context, snapshot contract.AccountHealthSnapshot) (contract.AccountHealthSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[snapshot.AccountID]; !ok {
		return contract.AccountHealthSnapshot{}, errors.New("account not found")
	}
	stored := snapshot
	stored.ID = s.nextHealthID
	if stored.SnapshotAt.IsZero() {
		stored.SnapshotAt = time.Now().UTC()
	}
	s.healthSnapshotsByID[stored.ID] = stored
	s.nextHealthID++
	return cloneHealthSnapshot(stored), nil
}

func (s *Store) LatestHealthSnapshotByAccount(_ context.Context, accountID int) (contract.AccountHealthSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest contract.AccountHealthSnapshot
	found := false
	for _, snapshot := range s.healthSnapshotsByID {
		if snapshot.AccountID != accountID {
			continue
		}
		if !found || snapshot.SnapshotAt.After(latest.SnapshotAt) || (snapshot.SnapshotAt.Equal(latest.SnapshotAt) && snapshot.ID > latest.ID) {
			latest = snapshot
			found = true
		}
	}
	if !found {
		return contract.AccountHealthSnapshot{}, errors.New("account health snapshot not found")
	}
	return cloneHealthSnapshot(latest), nil
}

func (s *Store) ListHealthSnapshotsByAccount(_ context.Context, accountID int, limit int) ([]contract.AccountHealthSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AccountHealthSnapshot, 0)
	for _, snapshot := range s.healthSnapshotsByID {
		if snapshot.AccountID == accountID {
			out = append(out, cloneHealthSnapshot(snapshot))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SnapshotAt.Equal(out[j].SnapshotAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].SnapshotAt.After(out[j].SnapshotAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) RecordQuotaSnapshot(_ context.Context, snapshot contract.AccountQuotaSnapshot) (contract.AccountQuotaSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[snapshot.AccountID]; !ok {
		return contract.AccountQuotaSnapshot{}, errors.New("account not found")
	}
	stored := snapshot
	stored.ID = s.nextQuotaID
	if stored.SnapshotAt.IsZero() {
		stored.SnapshotAt = time.Now().UTC()
	}
	s.quotaSnapshotsByID[stored.ID] = stored
	s.nextQuotaID++
	return cloneQuotaSnapshot(stored), nil
}

func (s *Store) ListQuotaSnapshotsByAccount(_ context.Context, accountID int, limit int) ([]contract.AccountQuotaSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AccountQuotaSnapshot, 0)
	for _, snapshot := range s.quotaSnapshotsByID {
		if snapshot.AccountID == accountID {
			out = append(out, cloneQuotaSnapshot(snapshot))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SnapshotAt.Equal(out[j].SnapshotAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].SnapshotAt.After(out[j].SnapshotAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func cloneAccount(value contract.ProviderAccount) contract.ProviderAccount {
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func cloneProxy(value contract.ProxyDefinition) contract.ProxyDefinition {
	value.Metadata = cloneMap(value.Metadata)
	if value.DeletedAt != nil {
		cloned := *value.DeletedAt
		value.DeletedAt = &cloned
	}
	return value
}

func cloneGroup(value contract.AccountGroup) contract.AccountGroup {
	value.ProviderScope = cloneMap(value.ProviderScope)
	value.ModelScope = cloneMap(value.ModelScope)
	return value
}

func cloneGroupMember(value contract.AccountGroupMember) contract.AccountGroupMember {
	return value
}

func cloneHealthSnapshot(value contract.AccountHealthSnapshot) contract.AccountHealthSnapshot {
	if value.CooldownUntil != nil {
		cloned := *value.CooldownUntil
		value.CooldownUntil = &cloned
	}
	return value
}

func cloneQuotaSnapshot(value contract.AccountQuotaSnapshot) contract.AccountQuotaSnapshot {
	if value.ResetAt != nil {
		cloned := *value.ResetAt
		value.ResetAt = &cloned
	}
	return value
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

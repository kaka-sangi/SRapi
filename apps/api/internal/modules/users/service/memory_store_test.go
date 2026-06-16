package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

type memoryStore struct {
	mu             sync.Mutex
	nextID         int
	nextRoleID     int
	nextIdentityID int
	byID           map[int]contract.StoredUser
	byEmail        map[string]int
	roles          map[contract.Role]contract.RoleDefinition
	identities     map[int]contract.UserAuthIdentity
	identityByKey  map[string]int
}

func newMemoryStore() *memoryStore {
	store := &memoryStore{
		nextID:         1,
		nextRoleID:     1,
		nextIdentityID: 1,
		byID:           map[int]contract.StoredUser{},
		byEmail:        map[string]int{},
		roles:          map[contract.Role]contract.RoleDefinition{},
		identities:     map[int]contract.UserAuthIdentity{},
		identityByKey:  map[string]int{},
	}
	store.seedBuiltInRoles()
	return store
}

func (s *memoryStore) Create(_ context.Context, input contract.CreateStoredUser) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if _, ok := s.byEmail[email]; ok {
		return contract.StoredUser{}, ErrUserAlreadyExists
	}
	now := time.Now().UTC()
	user := contract.StoredUser{
		User: contract.User{
			ID:              s.nextID,
			Email:           email,
			Name:            input.Name,
			Status:          input.Status,
			WorkspaceID:     cloneInt(input.WorkspaceID),
			Roles:           append([]contract.Role(nil), input.Roles...),
			Balance:         input.Balance,
			Currency:        input.Currency,
			RPMLimit:        cloneInt(input.RPMLimit),
			CreatedAt:       now,
			EmailVerifiedAt: cloneTime(input.EmailVerifiedAt),
		},
		PasswordHash:    input.PasswordHash,
		EmailVerifiedAt: cloneTime(input.EmailVerifiedAt),
	}
	user = s.withRolePermissions(user)
	s.byID[user.ID] = user
	s.byEmail[email] = user.ID
	s.nextID++
	return cloneUser(user), nil
}

func (s *memoryStore) FindByID(_ context.Context, id int) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	return cloneUser(s.withRolePermissions(user)), nil
}

func (s *memoryStore) FindByEmail(_ context.Context, email string) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	return cloneUser(s.withRolePermissions(s.byID[id])), nil
}

func (s *memoryStore) List(_ context.Context, filter contract.ListUsersFilter) ([]contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	users := make([]contract.StoredUser, 0, len(s.byID))
	for _, user := range s.byID {
		if filter.Status != nil && user.Status != *filter.Status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(user.Email), query) && !strings.Contains(strings.ToLower(user.Name), query) {
			continue
		}
		if filter.Role != nil {
			hasRole := false
			for _, role := range user.Roles {
				if role == *filter.Role {
					hasRole = true
					break
				}
			}
			if !hasRole {
				continue
			}
		}
		users = append(users, cloneUser(s.withRolePermissions(user)))
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	return users, nil
}

func (s *memoryStore) ListByIDs(_ context.Context, ids []int) ([]contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := make([]contract.StoredUser, 0, len(ids))
	for _, id := range ids {
		user, ok := s.byID[id]
		if !ok {
			return nil, ErrUserNotFound
		}
		users = append(users, cloneUser(s.withRolePermissions(user)))
	}
	return users, nil
}

func (s *memoryStore) Update(_ context.Context, id int, input contract.UpdateStoredUser) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	if input.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*input.Email))
		if existingID, exists := s.byEmail[email]; exists && existingID != id {
			return contract.StoredUser{}, ErrUserAlreadyExists
		}
		delete(s.byEmail, strings.ToLower(user.Email))
		user.Email = email
		s.byEmail[email] = id
	}
	if input.Name != nil {
		user.Name = strings.TrimSpace(*input.Name)
	}
	if input.PasswordHash != nil {
		user.PasswordHash = *input.PasswordHash
	}
	if input.Status != nil {
		user.Status = *input.Status
	}
	if input.WorkspaceID != nil {
		user.WorkspaceID = cloneInt(*input.WorkspaceID)
	}
	if input.Roles != nil {
		user.Roles = append([]contract.Role(nil), (*input.Roles)...)
	}
	if input.Balance != nil {
		user.Balance = *input.Balance
	}
	if input.Currency != nil {
		user.Currency = *input.Currency
	}
	if input.RPMLimit != nil {
		user.RPMLimit = cloneInt(*input.RPMLimit)
	}
	if input.EmailVerifiedAt != nil {
		if *input.EmailVerifiedAt == nil {
			user.EmailVerifiedAt = nil
			user.User.EmailVerifiedAt = nil
		} else {
			emailVerifiedAt := **input.EmailVerifiedAt
			user.EmailVerifiedAt = &emailVerifiedAt
			user.User.EmailVerifiedAt = &emailVerifiedAt
		}
	}
	user = s.withRolePermissions(user)
	s.byID[id] = user
	return cloneUser(user), nil
}

func (s *memoryStore) Delete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return ErrUserNotFound
	}
	delete(s.byID, id)
	delete(s.byEmail, strings.ToLower(user.Email))
	for identityID, identity := range s.identities {
		if identity.UserID != id {
			continue
		}
		for key, id := range s.identityByKey {
			if id == identityID {
				delete(s.identityByKey, key)
				break
			}
		}
		delete(s.identities, identityID)
	}
	return nil
}

func (s *memoryStore) UpdateLastLogin(_ context.Context, id int, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return ErrUserNotFound
	}
	user.LastLoginAt = &at
	s.byID[id] = user
	return nil
}

func (s *memoryStore) setStatus(id int, status contract.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.byID[id]
	user.Status = status
	s.byID[id] = user
}

func (s *memoryStore) CreateRole(_ context.Context, input contract.CreateStoredRole) (contract.RoleDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := contract.Role(strings.TrimSpace(string(input.Name)))
	if _, ok := s.roles[name]; ok {
		return contract.RoleDefinition{}, ErrUserAlreadyExists
	}
	now := time.Now().UTC()
	role := contract.RoleDefinition{
		ID:          s.nextRoleID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Permissions: append([]string(nil), input.Permissions...),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.roles[name] = role
	s.nextRoleID++
	return cloneRole(role), nil
}

func (s *memoryStore) ListRoles(_ context.Context) ([]contract.RoleDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.RoleDefinition, 0, len(s.roles))
	for _, role := range s.roles {
		out = append(out, cloneRole(role))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *memoryStore) UpdateRole(_ context.Context, id int, input contract.UpdateStoredRole) (contract.RoleDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, role := range s.roles {
		if role.ID != id {
			continue
		}
		if input.Description != nil {
			role.Description = strings.TrimSpace(*input.Description)
		}
		if input.Permissions != nil {
			role.Permissions = append([]string(nil), (*input.Permissions)...)
		}
		role.UpdatedAt = time.Now().UTC()
		s.roles[name] = role
		return cloneRole(role), nil
	}
	return contract.RoleDefinition{}, contract.ErrNotFound
}

func (s *memoryStore) DeleteRole(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, role := range s.roles {
		if role.ID == id {
			delete(s.roles, name)
			return nil
		}
	}
	return contract.ErrNotFound
}

func (s *memoryStore) ListAuthIdentities(_ context.Context, userID int) ([]contract.UserAuthIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[userID]; !ok {
		return nil, contract.ErrNotFound
	}
	out := make([]contract.UserAuthIdentity, 0)
	for _, identity := range s.identities {
		if identity.UserID != userID {
			continue
		}
		out = append(out, cloneIdentity(identity))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			return out[i].ID < out[j].ID
		}
		return out[i].Provider < out[j].Provider
	})
	return out, nil
}

func (s *memoryStore) FindAuthIdentityByProviderSubject(_ context.Context, provider contract.AuthIdentityProvider, providerKey string, providerSubjectHash string) (contract.UserAuthIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := identityKey(provider, providerKey, providerSubjectHash)
	if key == "" {
		return contract.UserAuthIdentity{}, contract.ErrNotFound
	}
	id, ok := s.identityByKey[key]
	if !ok {
		return contract.UserAuthIdentity{}, contract.ErrNotFound
	}
	identity, ok := s.identities[id]
	if !ok {
		return contract.UserAuthIdentity{}, contract.ErrNotFound
	}
	return cloneIdentity(identity), nil
}

func (s *memoryStore) UpsertAuthIdentity(_ context.Context, input contract.CreateUserAuthIdentity) (contract.UserAuthIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[input.UserID]; !ok {
		return contract.UserAuthIdentity{}, contract.ErrNotFound
	}
	key := identityKey(input.Provider, input.ProviderKey, input.ProviderSubjectHash)
	if key == "" {
		return contract.UserAuthIdentity{}, contract.ErrNotFound
	}
	now := time.Now().UTC()
	identity := contract.UserAuthIdentity{
		UserID:        input.UserID,
		Provider:      input.Provider,
		ProviderKey:   strings.TrimSpace(input.ProviderKey),
		SubjectHint:   strings.TrimSpace(input.SubjectHint),
		DisplayName:   strings.TrimSpace(input.DisplayName),
		Email:         strings.ToLower(strings.TrimSpace(input.Email)),
		EmailVerified: input.EmailVerified,
		AvatarURL:     strings.TrimSpace(input.AvatarURL),
		External:      true,
		VerifiedAt:    cloneTime(input.VerifiedAt),
		LastUsedAt:    cloneTime(input.LastUsedAt),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if existingID, ok := s.identityByKey[key]; ok {
		identity.ID = existingID
		existing := s.identities[existingID]
		if existing.UserID != input.UserID {
			return contract.UserAuthIdentity{}, contract.ErrAlreadyExists
		}
		identity.CreatedAt = existing.CreatedAt
	} else {
		identity.ID = s.nextIdentityID
		s.nextIdentityID++
		s.identityByKey[key] = identity.ID
	}
	s.identities[identity.ID] = identity
	return cloneIdentity(identity), nil
}

func (s *memoryStore) DeleteAuthIdentity(_ context.Context, userID int, identityID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity, ok := s.identities[identityID]
	if !ok || identity.UserID != userID {
		return contract.ErrNotFound
	}
	for key, id := range s.identityByKey {
		if id == identityID {
			delete(s.identityByKey, key)
			break
		}
	}
	delete(s.identities, identityID)
	return nil
}

func (s *memoryStore) seedBuiltInRoles() {
	for _, role := range []contract.Role{contract.RoleOwner, contract.RoleAdmin, contract.RoleOperator, contract.RoleUser} {
		definition := contract.BuiltInRoleDefinition(role)
		now := time.Now().UTC()
		s.roles[role] = contract.RoleDefinition{
			ID:          s.nextRoleID,
			Name:        role,
			Description: definition.Description,
			Permissions: append([]string(nil), definition.Permissions...),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		s.nextRoleID++
	}
}

func (s *memoryStore) withRolePermissions(user contract.StoredUser) contract.StoredUser {
	seen := map[string]bool{}
	permissions := make([]string, 0)
	for _, roleName := range user.Roles {
		role, ok := s.roles[roleName]
		if !ok {
			continue
		}
		for _, permission := range role.Permissions {
			if seen[permission] {
				continue
			}
			seen[permission] = true
			permissions = append(permissions, permission)
		}
	}
	user.Permissions = permissions
	return user
}

func cloneUser(user contract.StoredUser) contract.StoredUser {
	user.Roles = append([]contract.Role(nil), user.Roles...)
	user.Permissions = append([]string(nil), user.Permissions...)
	user.WorkspaceID = cloneInt(user.WorkspaceID)
	user.RPMLimit = cloneInt(user.RPMLimit)
	user.LastLoginAt = cloneTime(user.LastLoginAt)
	user.EmailVerifiedAt = cloneTime(user.EmailVerifiedAt)
	user.User.EmailVerifiedAt = cloneTime(user.User.EmailVerifiedAt)
	return user
}

func cloneRole(role contract.RoleDefinition) contract.RoleDefinition {
	role.Permissions = append([]string(nil), role.Permissions...)
	return role
}

func cloneIdentity(identity contract.UserAuthIdentity) contract.UserAuthIdentity {
	identity.VerifiedAt = cloneTime(identity.VerifiedAt)
	identity.LastUsedAt = cloneTime(identity.LastUsedAt)
	return identity
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func identityKey(provider contract.AuthIdentityProvider, providerKey string, subjectHash string) string {
	provider = contract.AuthIdentityProvider(strings.ToLower(strings.TrimSpace(string(provider))))
	providerKey = strings.TrimSpace(providerKey)
	subjectHash = strings.TrimSpace(subjectHash)
	if provider == "" || providerKey == "" || subjectHash == "" {
		return ""
	}
	return string(provider) + "\x00" + providerKey + "\x00" + subjectHash
}

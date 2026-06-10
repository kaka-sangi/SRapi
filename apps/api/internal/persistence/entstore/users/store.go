package users

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/ent/predicate"
	entrole "github.com/srapi/srapi/apps/api/ent/role"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	entuserauthidentity "github.com/srapi/srapi/apps/api/ent/userauthidentity"
	entuserrole "github.com/srapi/srapi/apps/api/ent/userrole"
	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

var ErrInvalidStore = errors.New("invalid users ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	store := &Store{client: client}
	if err := store.EnsureBuiltInRoles(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) EnsureBuiltInRoles(ctx context.Context) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, roleName := range []contract.Role{contract.RoleOwner, contract.RoleAdmin, contract.RoleOperator, contract.RoleUser} {
		if _, err := ensureRole(ctx, tx, roleName); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateStoredUser) (contract.StoredUser, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.StoredUser{}, err
	}

	created, err := tx.User.Create().
		SetEmail(email).
		SetName(strings.TrimSpace(input.Name)).
		SetPasswordHash(input.PasswordHash).
		SetStatus(string(input.Status)).
		SetNillableWorkspaceID(input.WorkspaceID).
		SetNillableEmailVerifiedAt(input.EmailVerifiedAt).
		SetBalance(input.Balance).
		SetCurrency(input.Currency).
		SetNillableRpmLimit(input.RPMLimit).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			return contract.StoredUser{}, contract.ErrAlreadyExists
		}
		return contract.StoredUser{}, err
	}

	if created.WorkspaceID == nil {
		workspace, err := tx.Workspace.Create().
			SetName(personalWorkspaceName(input.Name)).
			SetSlug(fmt.Sprintf("personal-%d", created.ID)).
			SetOwnerUserID(created.ID).
			SetType("personal").
			SetStatus("active").
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return contract.StoredUser{}, err
		}
		created, err = tx.User.UpdateOneID(created.ID).
			Where(entuser.DeletedAtIsNil()).
			SetWorkspaceID(workspace.ID).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return contract.StoredUser{}, err
		}
	}

	for _, roleName := range normalizeRoles(input.Roles) {
		role, err := ensureRole(ctx, tx, roleName)
		if err != nil {
			_ = tx.Rollback()
			return contract.StoredUser{}, err
		}
		if _, err := tx.UserRole.Create().SetUserID(created.ID).SetRoleID(role.ID).Save(ctx); err != nil {
			_ = tx.Rollback()
			return contract.StoredUser{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return contract.StoredUser{}, err
	}
	return s.FindByID(ctx, created.ID)
}

func (s *Store) FindByID(ctx context.Context, id int) (contract.StoredUser, error) {
	found, err := s.client.User.Query().
		Where(entuser.IDEQ(id), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.StoredUser{}, contract.ErrNotFound
		}
		return contract.StoredUser{}, err
	}
	return s.toStoredUser(ctx, found)
}

func (s *Store) FindByEmail(ctx context.Context, email string) (contract.StoredUser, error) {
	found, err := s.client.User.Query().
		Where(entuser.EmailEQ(strings.ToLower(strings.TrimSpace(email))), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.StoredUser{}, contract.ErrNotFound
		}
		return contract.StoredUser{}, err
	}
	return s.toStoredUser(ctx, found)
}

func (s *Store) List(ctx context.Context, filter contract.ListUsersFilter) ([]contract.StoredUser, error) {
	query := s.client.User.Query().Where(entuser.DeletedAtIsNil())
	if filter.Status != nil {
		query.Where(entuser.StatusEQ(string(*filter.Status)))
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		predicates := []predicate.User{
			entuser.EmailContainsFold(q),
			entuser.NameContainsFold(q),
		}
		if id, err := strconv.Atoi(q); err == nil && id > 0 {
			predicates = append(predicates, entuser.IDEQ(id))
		}
		query.Where(entuser.Or(predicates...))
	}
	rows, err := query.Order(entuser.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.StoredUser, 0, len(rows))
	for _, row := range rows {
		user, err := s.toStoredUser(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, user)
	}
	return out, nil
}

func (s *Store) ListByIDs(ctx context.Context, ids []int) ([]contract.StoredUser, error) {
	out := make([]contract.StoredUser, 0, len(ids))
	for _, id := range ids {
		user, err := s.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, user)
	}
	return out, nil
}

func (s *Store) Update(ctx context.Context, id int, input contract.UpdateStoredUser) (contract.StoredUser, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.StoredUser{}, err
	}
	update := tx.User.UpdateOneID(id).Where(entuser.DeletedAtIsNil())
	if input.Email != nil {
		update.SetEmail(strings.ToLower(strings.TrimSpace(*input.Email)))
	}
	if input.Name != nil {
		update.SetName(strings.TrimSpace(*input.Name))
	}
	if input.PasswordHash != nil {
		update.SetPasswordHash(*input.PasswordHash)
	}
	if input.Status != nil {
		update.SetStatus(string(*input.Status))
	}
	if input.Balance != nil {
		update.SetBalance(*input.Balance)
	}
	if input.Currency != nil {
		update.SetCurrency(*input.Currency)
	}
	if input.RPMLimit != nil {
		if *input.RPMLimit == nil {
			update.ClearRpmLimit()
		} else {
			update.SetRpmLimit(**input.RPMLimit)
		}
	}
	if input.EmailVerifiedAt != nil {
		if *input.EmailVerifiedAt == nil {
			update.ClearEmailVerifiedAt()
		} else {
			update.SetEmailVerifiedAt(**input.EmailVerifiedAt)
		}
	}
	if input.WorkspaceID != nil {
		if *input.WorkspaceID == nil {
			update.ClearWorkspaceID()
		} else {
			update.SetWorkspaceID(**input.WorkspaceID)
		}
	}
	updated, err := update.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.StoredUser{}, contract.ErrNotFound
		}
		if ent.IsConstraintError(err) {
			return contract.StoredUser{}, contract.ErrAlreadyExists
		}
		return contract.StoredUser{}, err
	}
	if input.Roles != nil {
		if _, err := tx.UserRole.Delete().Where(entuserrole.UserIDEQ(id)).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return contract.StoredUser{}, err
		}
		for _, roleName := range normalizeRoles(*input.Roles) {
			role, err := ensureRole(ctx, tx, roleName)
			if err != nil {
				_ = tx.Rollback()
				return contract.StoredUser{}, err
			}
			if _, err := tx.UserRole.Create().SetUserID(id).SetRoleID(role.ID).Save(ctx); err != nil {
				_ = tx.Rollback()
				return contract.StoredUser{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return contract.StoredUser{}, err
	}
	return s.toStoredUser(ctx, updated)
}

func (s *Store) Delete(ctx context.Context, id int) error {
	now := time.Now().UTC()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	updated, err := tx.User.UpdateOneID(id).
		Where(entuser.DeletedAtIsNil()).
		SetDeletedAt(now).
		SetEmail(fmt.Sprintf("deleted+%d+%d@deleted.local", id, now.UnixNano())).
		SetStatus(string(contract.StatusDisabled)).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	if _, err := tx.UserAuthIdentity.Delete().Where(entuserauthidentity.UserIDEQ(id)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if updated.WorkspaceID != nil {
		_, err := tx.Workspace.UpdateOneID(*updated.WorkspaceID).
			SetDeletedAt(now).
			SetSlug(fmt.Sprintf("deleted-%d-%d", *updated.WorkspaceID, now.UnixNano())).
			SetStatus("disabled").
			Save(ctx)
		if err != nil && !ent.IsNotFound(err) {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateLastLogin(ctx context.Context, id int, at time.Time) error {
	_, err := s.client.User.UpdateOneID(id).
		Where(entuser.DeletedAtIsNil()).
		SetLastLoginAt(at).
		Save(ctx)
	if ent.IsNotFound(err) {
		return contract.ErrNotFound
	}
	return err
}

func (s *Store) CreateRole(ctx context.Context, input contract.CreateStoredRole) (contract.RoleDefinition, error) {
	created, err := s.client.Role.Create().
		SetName(strings.TrimSpace(string(input.Name))).
		SetDescription(strings.TrimSpace(input.Description)).
		SetPermissionsJSON(append([]string(nil), input.Permissions...)).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return contract.RoleDefinition{}, contract.ErrAlreadyExists
		}
		return contract.RoleDefinition{}, err
	}
	return toRoleDefinition(created), nil
}

func (s *Store) ListRoles(ctx context.Context) ([]contract.RoleDefinition, error) {
	rows, err := s.client.Role.Query().Order(entrole.ByName()).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.RoleDefinition, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRoleDefinition(row))
	}
	return out, nil
}

func (s *Store) UpdateRole(ctx context.Context, id int, input contract.UpdateStoredRole) (contract.RoleDefinition, error) {
	update := s.client.Role.UpdateOneID(id)
	if input.Description != nil {
		update = update.SetDescription(strings.TrimSpace(*input.Description))
	}
	if input.Permissions != nil {
		update = update.SetPermissionsJSON(append([]string(nil), (*input.Permissions)...))
	}
	saved, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.RoleDefinition{}, contract.ErrNotFound
		}
		return contract.RoleDefinition{}, err
	}
	return toRoleDefinition(saved), nil
}

func (s *Store) DeleteRole(ctx context.Context, id int) error {
	if err := s.client.Role.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) ListAuthIdentities(ctx context.Context, userID int) ([]contract.UserAuthIdentity, error) {
	exists, err := s.client.User.Query().
		Where(entuser.IDEQ(userID), entuser.DeletedAtIsNil()).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, contract.ErrNotFound
	}
	rows, err := s.client.UserAuthIdentity.Query().
		Where(entuserauthidentity.UserIDEQ(userID), entuserauthidentity.DeletedAtIsNil()).
		Order(entuserauthidentity.ByProvider(), entuserauthidentity.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.UserAuthIdentity, 0, len(rows))
	for _, row := range rows {
		out = append(out, toUserAuthIdentity(row))
	}
	return out, nil
}

func (s *Store) FindAuthIdentityByProviderSubject(ctx context.Context, provider contract.AuthIdentityProvider, providerKey string, providerSubjectHash string) (contract.UserAuthIdentity, error) {
	providerValue := strings.ToLower(strings.TrimSpace(string(provider)))
	providerKey = strings.TrimSpace(providerKey)
	providerSubjectHash = strings.TrimSpace(providerSubjectHash)
	if providerValue == "" || providerKey == "" || providerSubjectHash == "" {
		return contract.UserAuthIdentity{}, contract.ErrNotFound
	}
	row, err := s.client.UserAuthIdentity.Query().
		Where(
			entuserauthidentity.ProviderEQ(providerValue),
			entuserauthidentity.ProviderKeyEQ(providerKey),
			entuserauthidentity.ProviderSubjectHashEQ(providerSubjectHash),
			entuserauthidentity.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.UserAuthIdentity{}, contract.ErrNotFound
		}
		return contract.UserAuthIdentity{}, err
	}
	return toUserAuthIdentity(row), nil
}

func (s *Store) UpsertAuthIdentity(ctx context.Context, input contract.CreateUserAuthIdentity) (contract.UserAuthIdentity, error) {
	provider := strings.ToLower(strings.TrimSpace(string(input.Provider)))
	providerKey := strings.TrimSpace(input.ProviderKey)
	subjectHash := strings.TrimSpace(input.ProviderSubjectHash)
	if input.UserID <= 0 || provider == "" || providerKey == "" || subjectHash == "" {
		return contract.UserAuthIdentity{}, contract.ErrNotFound
	}
	existing, err := s.client.UserAuthIdentity.Query().
		Where(
			entuserauthidentity.ProviderEQ(provider),
			entuserauthidentity.ProviderKeyEQ(providerKey),
			entuserauthidentity.ProviderSubjectHashEQ(subjectHash),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return contract.UserAuthIdentity{}, err
	}
	if existing != nil && existing.DeletedAt == nil {
		if existing.UserID != input.UserID {
			return contract.UserAuthIdentity{}, contract.ErrAlreadyExists
		}
	}
	if existing != nil {
		update := s.client.UserAuthIdentity.UpdateOneID(existing.ID).
			SetUserID(input.UserID).
			SetProvider(provider).
			SetProviderKey(providerKey).
			SetProviderSubjectHash(subjectHash).
			SetSubjectHint(strings.TrimSpace(input.SubjectHint)).
			SetDisplayName(strings.TrimSpace(input.DisplayName)).
			SetEmail(strings.ToLower(strings.TrimSpace(input.Email))).
			SetEmailVerified(input.EmailVerified).
			SetAvatarURL(strings.TrimSpace(input.AvatarURL)).
			ClearDeletedAt().
			SetNillableVerifiedAt(input.VerifiedAt).
			SetNillableLastUsedAt(input.LastUsedAt)
		row, err := update.Save(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return contract.UserAuthIdentity{}, contract.ErrNotFound
			}
			if ent.IsConstraintError(err) {
				return contract.UserAuthIdentity{}, contract.ErrAlreadyExists
			}
			return contract.UserAuthIdentity{}, err
		}
		return toUserAuthIdentity(row), nil
	}
	row, err := s.client.UserAuthIdentity.Create().
		SetUserID(input.UserID).
		SetProvider(provider).
		SetProviderKey(providerKey).
		SetProviderSubjectHash(subjectHash).
		SetSubjectHint(strings.TrimSpace(input.SubjectHint)).
		SetDisplayName(strings.TrimSpace(input.DisplayName)).
		SetEmail(strings.ToLower(strings.TrimSpace(input.Email))).
		SetEmailVerified(input.EmailVerified).
		SetAvatarURL(strings.TrimSpace(input.AvatarURL)).
		SetNillableVerifiedAt(input.VerifiedAt).
		SetNillableLastUsedAt(input.LastUsedAt).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return contract.UserAuthIdentity{}, contract.ErrAlreadyExists
		}
		return contract.UserAuthIdentity{}, err
	}
	return toUserAuthIdentity(row), nil
}

func (s *Store) DeleteAuthIdentity(ctx context.Context, userID int, identityID int) error {
	affected, err := s.client.UserAuthIdentity.Delete().
		Where(entuserauthidentity.IDEQ(identityID), entuserauthidentity.UserIDEQ(userID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrNotFound
	}
	return nil
}

func (s *Store) toStoredUser(ctx context.Context, user *ent.User) (contract.StoredUser, error) {
	roles, permissions, err := s.rolesForUser(ctx, user.ID)
	if err != nil {
		return contract.StoredUser{}, err
	}
	return contract.StoredUser{
		User: contract.User{
			ID:              user.ID,
			Email:           user.Email,
			Name:            user.Name,
			Status:          contract.Status(user.Status),
			WorkspaceID:     cloneInt(user.WorkspaceID),
			Roles:           roles,
			Permissions:     permissions,
			Balance:         user.Balance,
			Currency:        user.Currency,
			RPMLimit:        cloneInt(user.RpmLimit),
			CreatedAt:       user.CreatedAt,
			LastLoginAt:     user.LastLoginAt,
			EmailVerifiedAt: user.EmailVerifiedAt,
		},
		PasswordHash:    user.PasswordHash,
		EmailVerifiedAt: user.EmailVerifiedAt,
	}, nil
}

func (s *Store) rolesForUser(ctx context.Context, userID int) ([]contract.Role, []string, error) {
	joins, err := s.client.UserRole.Query().
		Where(entuserrole.UserIDEQ(userID)).
		Order(entuserrole.ByID()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	roleIDs := make([]int, 0, len(joins))
	for _, join := range joins {
		roleIDs = append(roleIDs, join.RoleID)
	}
	if len(roleIDs) == 0 {
		return nil, nil, nil
	}
	roles, err := s.client.Role.Query().Where(entrole.IDIn(roleIDs...)).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]contract.Role, 0, len(roles))
	permissions := make([]string, 0)
	seenPermission := map[string]bool{}
	roleByID := make(map[int]*ent.Role, len(roles))
	for _, role := range roles {
		roleByID[role.ID] = role
	}
	for _, id := range roleIDs {
		if role, ok := roleByID[id]; ok {
			out = append(out, contract.Role(role.Name))
			for _, permission := range role.PermissionsJSON {
				if seenPermission[permission] {
					continue
				}
				seenPermission[permission] = true
				permissions = append(permissions, permission)
			}
		}
	}
	return out, permissions, nil
}

func ensureRole(ctx context.Context, tx *ent.Tx, roleName contract.Role) (*ent.Role, error) {
	name := strings.TrimSpace(string(roleName))
	found, err := tx.Role.Query().Where(entrole.NameEQ(name)).Only(ctx)
	if err == nil {
		if !contract.IsBuiltInRole(roleName) {
			return found, nil
		}
		definition := contract.BuiltInRoleDefinition(roleName)
		if found.Description == definition.Description && sameStrings(found.PermissionsJSON, definition.Permissions) {
			return found, nil
		}
		return tx.Role.UpdateOneID(found.ID).
			SetDescription(definition.Description).
			SetPermissionsJSON(append([]string(nil), definition.Permissions...)).
			Save(ctx)
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	if !contract.IsBuiltInRole(contract.Role(name)) {
		return nil, fmt.Errorf("role %q not found", name)
	}
	definition := contract.BuiltInRoleDefinition(contract.Role(name))
	created, err := tx.Role.Create().
		SetName(name).
		SetDescription(definition.Description).
		SetPermissionsJSON(append([]string(nil), definition.Permissions...)).
		Save(ctx)
	if err == nil {
		return created, nil
	}
	if ent.IsConstraintError(err) {
		return tx.Role.Query().Where(entrole.NameEQ(name)).Only(ctx)
	}
	return nil, err
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func toRoleDefinition(row *ent.Role) contract.RoleDefinition {
	return contract.RoleDefinition{
		ID:          row.ID,
		Name:        contract.Role(row.Name),
		Description: row.Description,
		Permissions: append([]string(nil), row.PermissionsJSON...),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toUserAuthIdentity(row *ent.UserAuthIdentity) contract.UserAuthIdentity {
	return contract.UserAuthIdentity{
		ID:            row.ID,
		UserID:        row.UserID,
		Provider:      contract.AuthIdentityProvider(row.Provider),
		ProviderKey:   row.ProviderKey,
		SubjectHint:   row.SubjectHint,
		DisplayName:   row.DisplayName,
		Email:         row.Email,
		EmailVerified: row.EmailVerified,
		AvatarURL:     row.AvatarURL,
		External:      true,
		VerifiedAt:    cloneTime(row.VerifiedAt),
		LastUsedAt:    cloneTime(row.LastUsedAt),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func normalizeRoles(roles []contract.Role) []contract.Role {
	out := make([]contract.Role, 0, len(roles))
	seen := map[contract.Role]bool{}
	for _, role := range roles {
		role = contract.Role(strings.TrimSpace(string(role)))
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func personalWorkspaceName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Personal Workspace"
	}
	return name + " Workspace"
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

package users

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/ent/predicate"
	entrole "github.com/srapi/srapi/apps/api/ent/role"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
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
	return &Store{client: client}, nil
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
		SetNillableEmailVerifiedAt(input.EmailVerifiedAt).
		SetBalance(input.Balance).
		SetCurrency(input.Currency).
		SetNillableRpmLimit(input.RPMLimit).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.StoredUser{}, err
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
		return contract.StoredUser{}, err
	}
	return s.toStoredUser(ctx, found)
}

func (s *Store) FindByEmail(ctx context.Context, email string) (contract.StoredUser, error) {
	found, err := s.client.User.Query().
		Where(entuser.EmailEQ(strings.ToLower(strings.TrimSpace(email))), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
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
	updated, err := update.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
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

func (s *Store) UpdateLastLogin(ctx context.Context, id int, at time.Time) error {
	_, err := s.client.User.UpdateOneID(id).
		Where(entuser.DeletedAtIsNil()).
		SetLastLoginAt(at).
		Save(ctx)
	return err
}

func (s *Store) toStoredUser(ctx context.Context, user *ent.User) (contract.StoredUser, error) {
	roles, err := s.rolesForUser(ctx, user.ID)
	if err != nil {
		return contract.StoredUser{}, err
	}
	return contract.StoredUser{
		User: contract.User{
			ID:          user.ID,
			Email:       user.Email,
			Name:        user.Name,
			Status:      contract.Status(user.Status),
			Roles:       roles,
			Balance:     user.Balance,
			Currency:    user.Currency,
			RPMLimit:    cloneInt(user.RpmLimit),
			CreatedAt:   user.CreatedAt,
			LastLoginAt: user.LastLoginAt,
		},
		PasswordHash:    user.PasswordHash,
		EmailVerifiedAt: user.EmailVerifiedAt,
	}, nil
}

func (s *Store) rolesForUser(ctx context.Context, userID int) ([]contract.Role, error) {
	joins, err := s.client.UserRole.Query().
		Where(entuserrole.UserIDEQ(userID)).
		Order(entuserrole.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	roleIDs := make([]int, 0, len(joins))
	for _, join := range joins {
		roleIDs = append(roleIDs, join.RoleID)
	}
	if len(roleIDs) == 0 {
		return nil, nil
	}
	roles, err := s.client.Role.Query().Where(entrole.IDIn(roleIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Role, 0, len(roles))
	roleByID := make(map[int]contract.Role, len(roles))
	for _, role := range roles {
		roleByID[role.ID] = contract.Role(role.Name)
	}
	for _, id := range roleIDs {
		if role, ok := roleByID[id]; ok {
			out = append(out, role)
		}
	}
	return out, nil
}

func ensureRole(ctx context.Context, tx *ent.Tx, roleName contract.Role) (*ent.Role, error) {
	name := strings.TrimSpace(string(roleName))
	found, err := tx.Role.Query().Where(entrole.NameEQ(name)).Only(ctx)
	if err == nil {
		return found, nil
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	created, err := tx.Role.Create().SetName(name).Save(ctx)
	if err == nil {
		return created, nil
	}
	if ent.IsConstraintError(err) {
		return tx.Role.Query().Where(entrole.NameEQ(name)).Only(ctx)
	}
	return nil, err
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

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

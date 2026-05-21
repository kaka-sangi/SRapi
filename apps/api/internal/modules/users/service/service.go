package service

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const defaultBcryptCost = 12

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type Service struct {
	store contract.Store
	clock Clock
}

type CreateRequest struct {
	Email    string
	Name     string
	Password string
	Roles    []contract.Role
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (contract.StoredUser, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	name := strings.TrimSpace(req.Name)
	if email == "" || !strings.Contains(email, "@") || name == "" || len(req.Password) < 8 {
		return contract.StoredUser{}, ErrInvalidInput
	}
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return contract.StoredUser{}, err
	}
	status := contract.StatusActive
	roles := normalizeRoles(req.Roles)
	if len(roles) == 0 {
		roles = []contract.Role{contract.RoleUser}
	}
	stored, err := s.store.Create(ctx, contract.CreateStoredUser{
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
		Status:       status,
		Roles:        roles,
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		return contract.StoredUser{}, err
	}
	return stored, nil
}

func (s *Service) AuthenticatePassword(ctx context.Context, email, password string) (contract.StoredUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return contract.StoredUser{}, ErrInvalidInput
	}
	user, err := s.store.FindByEmail(ctx, email)
	if err != nil {
		return contract.StoredUser{}, ErrInvalidCredentials
	}
	if user.Status == contract.StatusDisabled {
		return contract.StoredUser{}, ErrUserDisabled
	}
	if err := ComparePassword(user.PasswordHash, password); err != nil {
		return contract.StoredUser{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) FindByID(ctx context.Context, id int) (contract.StoredUser, error) {
	if id <= 0 {
		return contract.StoredUser{}, ErrInvalidInput
	}
	return s.store.FindByID(ctx, id)
}

func (s *Service) TouchLastLogin(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.UpdateLastLogin(ctx, id, s.clock.Now())
}

func PublicUser(user contract.StoredUser) contract.User {
	return user.User
}

func HashPassword(password string) (string, error) {
	return hashPassword(password)
}

func ComparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), defaultBcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func normalizeRoles(roles []contract.Role) []contract.Role {
	if len(roles) == 0 {
		return []contract.Role{contract.RoleUser}
	}
	cloned := make([]contract.Role, 0, len(roles))
	for _, role := range roles {
		role = contract.Role(strings.TrimSpace(string(role)))
		switch role {
		case contract.RoleOwner, contract.RoleAdmin, contract.RoleOperator, contract.RoleUser:
			cloned = append(cloned, role)
		}
	}
	return cloned
}

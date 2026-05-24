package service

import (
	"context"
	"math/big"
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
	Status   *contract.Status
	Balance  string
	Currency string
	RPMLimit *int
}

type UpdateRequest struct {
	Email    *string
	Name     *string
	Password *string
	Roles    *[]contract.Role
	Status   *contract.Status
	RPMLimit **int
}

type ListRequest struct {
	Status *contract.Status
	Query  string
}

type BalanceOperation = contract.BalanceOperation

const (
	BalanceOperationSet       = contract.BalanceOperationSet
	BalanceOperationIncrement = contract.BalanceOperationIncrement
	BalanceOperationDecrement = contract.BalanceOperationDecrement
)

type BalanceUpdateRequest = contract.BalanceUpdateRequest

type BatchUpdateRequest struct {
	UserIDs  []int
	Status   *contract.Status
	Roles    *[]contract.Role
	RPMLimit **int
}

type BatchUpdateResult struct {
	Updated []contract.StoredUser
	Errors  []string
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
	if req.Status != nil {
		if !validStatus(*req.Status) {
			return contract.StoredUser{}, ErrInvalidInput
		}
		status = *req.Status
	}
	roles := normalizeRoles(req.Roles)
	if len(roles) == 0 {
		roles = []contract.Role{contract.RoleUser}
	}
	balance := "0.00000000"
	if strings.TrimSpace(req.Balance) != "" {
		normalized, ok := normalizeMoney(req.Balance)
		if !ok {
			return contract.StoredUser{}, ErrInvalidInput
		}
		balance = normalized
	}
	stored, err := s.store.Create(ctx, contract.CreateStoredUser{
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
		Status:       status,
		Roles:        roles,
		Balance:      balance,
		Currency:     normalizeCurrency(req.Currency),
		RPMLimit:     cloneInt(req.RPMLimit),
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

func (s *Service) List(ctx context.Context, req ListRequest) ([]contract.StoredUser, error) {
	if req.Status != nil && !validStatus(*req.Status) {
		return nil, ErrInvalidInput
	}
	return s.store.List(ctx, contract.ListUsersFilter{
		Status: req.Status,
		Query:  strings.TrimSpace(req.Query),
	})
}

func (s *Service) Update(ctx context.Context, id int, req UpdateRequest) (contract.StoredUser, error) {
	if id <= 0 {
		return contract.StoredUser{}, ErrInvalidInput
	}
	input := contract.UpdateStoredUser{}
	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if email == "" || !strings.Contains(email, "@") {
			return contract.StoredUser{}, ErrInvalidInput
		}
		input.Email = &email
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.StoredUser{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if req.Password != nil {
		passwordHash, err := HashPassword(*req.Password)
		if err != nil {
			return contract.StoredUser{}, err
		}
		input.PasswordHash = &passwordHash
	}
	if req.Status != nil {
		if !validStatus(*req.Status) {
			return contract.StoredUser{}, ErrInvalidInput
		}
		input.Status = req.Status
	}
	if req.Roles != nil {
		roles := normalizeRoles(*req.Roles)
		if len(roles) == 0 {
			return contract.StoredUser{}, ErrInvalidInput
		}
		input.Roles = &roles
	}
	if req.RPMLimit != nil {
		input.RPMLimit = cloneIntPtr(req.RPMLimit)
	}
	return s.store.Update(ctx, id, input)
}

func (s *Service) SetStatus(ctx context.Context, id int, status contract.Status) (contract.StoredUser, error) {
	return s.Update(ctx, id, UpdateRequest{Status: &status})
}

func (s *Service) UpdateRPMLimit(ctx context.Context, id int, rpmLimit *int) (contract.StoredUser, error) {
	return s.Update(ctx, id, UpdateRequest{RPMLimit: &rpmLimit})
}

func (s *Service) UpdateBalance(ctx context.Context, id int, req BalanceUpdateRequest) (contract.StoredUser, error) {
	if id <= 0 {
		return contract.StoredUser{}, ErrInvalidInput
	}
	amount, ok := decimalRat(req.Amount)
	if !ok || amount.Sign() < 0 {
		return contract.StoredUser{}, ErrInvalidInput
	}
	user, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.StoredUser{}, err
	}
	balance, ok := decimalRat(user.Balance)
	if !ok {
		return contract.StoredUser{}, ErrInvalidInput
	}
	switch req.Operation {
	case BalanceOperationSet:
		balance = amount
	case BalanceOperationIncrement:
		balance = new(big.Rat).Add(balance, amount)
	case BalanceOperationDecrement:
		balance = new(big.Rat).Sub(balance, amount)
	default:
		return contract.StoredUser{}, ErrInvalidInput
	}
	if balance.Sign() < 0 {
		return contract.StoredUser{}, ErrInvalidInput
	}
	normalizedBalance := formatRatFixed(balance, 8)
	currency := normalizeCurrency(user.Currency)
	if strings.TrimSpace(req.Currency) != "" {
		currency = normalizeCurrency(req.Currency)
	}
	return s.store.Update(ctx, id, contract.UpdateStoredUser{
		Balance:  &normalizedBalance,
		Currency: &currency,
	})
}

func (s *Service) BatchUpdate(ctx context.Context, req BatchUpdateRequest) BatchUpdateResult {
	result := BatchUpdateResult{
		Updated: make([]contract.StoredUser, 0, len(req.UserIDs)),
		Errors:  make([]string, 0),
	}
	if len(req.UserIDs) == 0 {
		result.Errors = append(result.Errors, ErrInvalidInput.Error())
		return result
	}
	if req.Status != nil && !validStatus(*req.Status) {
		result.Errors = append(result.Errors, ErrInvalidInput.Error())
		return result
	}
	for _, id := range req.UserIDs {
		updated, err := s.Update(ctx, id, UpdateRequest{
			Status:   req.Status,
			Roles:    req.Roles,
			RPMLimit: req.RPMLimit,
		})
		if err != nil {
			result.Errors = append(result.Errors, strings.TrimSpace(err.Error()))
			continue
		}
		result.Updated = append(result.Updated, updated)
	}
	return result
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

func validStatus(status contract.Status) bool {
	switch status {
	case contract.StatusActive, contract.StatusDisabled, contract.StatusPending:
		return true
	default:
		return false
	}
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "USD"
	}
	return value
}

func normalizeMoney(value string) (string, bool) {
	rat, ok := decimalRat(value)
	if !ok || rat.Sign() < 0 {
		return "", false
	}
	return formatRatFixed(rat, 8), true
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(value)
	return rat, ok
}

func formatRatFixed(value *big.Rat, places int) string {
	if value == nil {
		value = new(big.Rat)
	}
	return value.FloatString(places)
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPtr(value **int) **int {
	if value == nil {
		return nil
	}
	cloned := cloneInt(*value)
	return &cloned
}

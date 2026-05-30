package service

import (
	"context"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const defaultBcryptCost = 12

var (
	roleNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	permissionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*:[a-z][a-z0-9_]*$`)
)

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
	Email           string
	Name            string
	Password        string
	Roles           []contract.Role
	Status          *contract.Status
	Balance         string
	Currency        string
	RPMLimit        *int
	EmailVerifiedAt *time.Time
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

// CreateRoleRequest creates a role catalog entry with resource:action permissions.
type CreateRoleRequest struct {
	Name        string
	Description string
	Permissions []string
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

type ChangePasswordRequest struct {
	CurrentPassword string
	NewPassword     string
}

type UpdateProfileRequest struct {
	Name string
}

// BindAuthIdentityRequest binds a verified external sign-in identity to a user.
type BindAuthIdentityRequest struct {
	UserID              int
	Provider            contract.AuthIdentityProvider
	ProviderKey         string
	ProviderSubjectHash string
	SubjectHint         string
	DisplayName         string
	Email               string
	EmailVerified       bool
	AvatarURL           string
	VerifiedAt          time.Time
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
	if err := s.validateRolesExist(ctx, roles); err != nil {
		return contract.StoredUser{}, err
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
		Email:           email,
		Name:            name,
		PasswordHash:    passwordHash,
		Status:          status,
		Roles:           roles,
		Balance:         balance,
		Currency:        normalizeCurrency(req.Currency),
		RPMLimit:        cloneInt(req.RPMLimit),
		EmailVerifiedAt: cloneAuthIdentityTime(req.EmailVerifiedAt),
	})
	if err != nil {
		if errors.Is(err, contract.ErrAlreadyExists) {
			return contract.StoredUser{}, ErrUserAlreadyExists
		}
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
	user, err := s.store.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	return user, nil
}

func (s *Service) FindByEmail(ctx context.Context, email string) (contract.StoredUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return contract.StoredUser{}, ErrInvalidInput
	}
	user, err := s.store.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	return user, nil
}

// VerifyEmail marks a user's primary email address as verified.
func (s *Service) VerifyEmail(ctx context.Context, id int, verifiedAt time.Time) (contract.StoredUser, error) {
	if id <= 0 || verifiedAt.IsZero() {
		return contract.StoredUser{}, ErrInvalidInput
	}
	at := verifiedAt.UTC()
	atPtr := &at
	updated, err := s.store.Update(ctx, id, contract.UpdateStoredUser{EmailVerifiedAt: &atPtr})
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	return updated, nil
}

// Delete removes a user that should not remain visible or sign-in capable.
func (s *Service) Delete(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if err := s.store.Delete(ctx, id); err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
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

// CreateRole validates and persists a role definition.
func (s *Service) CreateRole(ctx context.Context, req CreateRoleRequest) (contract.RoleDefinition, error) {
	name := contract.Role(strings.ToLower(strings.TrimSpace(req.Name)))
	if !validRoleName(name) {
		return contract.RoleDefinition{}, ErrInvalidInput
	}
	if err := s.validateNewRoleName(ctx, name); err != nil {
		return contract.RoleDefinition{}, err
	}
	permissions, err := normalizePermissions(req.Permissions)
	if err != nil {
		return contract.RoleDefinition{}, err
	}
	role, err := s.store.CreateRole(ctx, contract.CreateStoredRole{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Permissions: permissions,
	})
	if err != nil {
		if errors.Is(err, contract.ErrAlreadyExists) {
			return contract.RoleDefinition{}, ErrUserAlreadyExists
		}
		return contract.RoleDefinition{}, err
	}
	return role, nil
}

// ListRoles returns persisted role definitions.
func (s *Service) ListRoles(ctx context.Context) ([]contract.RoleDefinition, error) {
	return s.store.ListRoles(ctx)
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
		current, err := s.store.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, contract.ErrNotFound) {
				return contract.StoredUser{}, ErrUserNotFound
			}
			return contract.StoredUser{}, err
		}
		if current.Email != email {
			var unverifiedAt *time.Time
			input.EmailVerifiedAt = &unverifiedAt
		}
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
		if err := s.validateRolesExist(ctx, roles); err != nil {
			return contract.StoredUser{}, err
		}
		input.Roles = &roles
	}
	if req.RPMLimit != nil {
		input.RPMLimit = cloneIntPtr(req.RPMLimit)
	}
	updated, err := s.store.Update(ctx, id, input)
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		if errors.Is(err, contract.ErrAlreadyExists) {
			return contract.StoredUser{}, ErrUserAlreadyExists
		}
		return contract.StoredUser{}, err
	}
	return updated, nil
}

func (s *Service) SetStatus(ctx context.Context, id int, status contract.Status) (contract.StoredUser, error) {
	return s.Update(ctx, id, UpdateRequest{Status: &status})
}

func (s *Service) UpdateRPMLimit(ctx context.Context, id int, rpmLimit *int) (contract.StoredUser, error) {
	return s.Update(ctx, id, UpdateRequest{RPMLimit: &rpmLimit})
}

// UpdateProfile updates the small set of fields current users can edit themselves.
func (s *Service) UpdateProfile(ctx context.Context, id int, req UpdateProfileRequest) (contract.StoredUser, error) {
	name := strings.TrimSpace(req.Name)
	if id <= 0 || name == "" || len(name) > 120 {
		return contract.StoredUser{}, ErrInvalidInput
	}
	updated, err := s.store.Update(ctx, id, contract.UpdateStoredUser{Name: &name})
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	return updated, nil
}

// ResetPassword replaces a user's password after an out-of-band recovery flow has verified the caller.
func (s *Service) ResetPassword(ctx context.Context, id int, newPassword string) (contract.StoredUser, error) {
	if id <= 0 || strings.TrimSpace(newPassword) == "" {
		return contract.StoredUser{}, ErrInvalidInput
	}
	user, err := s.store.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	if user.Status == contract.StatusDisabled {
		return contract.StoredUser{}, ErrUserDisabled
	}
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return contract.StoredUser{}, err
	}
	updated, err := s.store.Update(ctx, id, contract.UpdateStoredUser{PasswordHash: &passwordHash})
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	return updated, nil
}

// ChangePassword verifies the current password before replacing the stored password hash.
func (s *Service) ChangePassword(ctx context.Context, id int, req ChangePasswordRequest) (contract.StoredUser, error) {
	if id <= 0 || strings.TrimSpace(req.CurrentPassword) == "" || strings.TrimSpace(req.NewPassword) == "" {
		return contract.StoredUser{}, ErrInvalidInput
	}
	user, err := s.store.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	if user.Status == contract.StatusDisabled {
		return contract.StoredUser{}, ErrUserDisabled
	}
	if err := ComparePassword(user.PasswordHash, req.CurrentPassword); err != nil {
		return contract.StoredUser{}, ErrInvalidCredentials
	}
	passwordHash, err := HashPassword(req.NewPassword)
	if err != nil {
		return contract.StoredUser{}, err
	}
	updated, err := s.store.Update(ctx, id, contract.UpdateStoredUser{PasswordHash: &passwordHash})
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	return updated, nil
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
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
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
	updated, err := s.store.Update(ctx, id, contract.UpdateStoredUser{
		Balance:  &normalizedBalance,
		Currency: &currency,
	})
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return contract.StoredUser{}, ErrUserNotFound
		}
		return contract.StoredUser{}, err
	}
	return updated, nil
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
	if req.Roles != nil {
		roles := normalizeRoles(*req.Roles)
		if len(roles) == 0 {
			result.Errors = append(result.Errors, ErrInvalidInput.Error())
			return result
		}
		if err := s.validateRolesExist(ctx, roles); err != nil {
			result.Errors = append(result.Errors, strings.TrimSpace(err.Error()))
			return result
		}
		req.Roles = &roles
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
	if err := s.store.UpdateLastLogin(ctx, id, s.clock.Now()); err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
}

// ListAuthIdentities returns the current sign-in identities for a user.
func (s *Service) ListAuthIdentities(ctx context.Context, userID int) ([]contract.UserAuthIdentity, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	user, err := s.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	external, err := s.store.ListAuthIdentities(ctx, userID)
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	identities := make([]contract.UserAuthIdentity, 0, len(external)+1)
	identities = append(identities, emailAuthIdentity(user.User))
	externalCount := len(external)
	for _, identity := range external {
		identity.Provider = normalizeAuthIdentityProvider(identity.Provider)
		identity.ProviderKey = strings.TrimSpace(identity.ProviderKey)
		identity.SubjectHint = strings.TrimSpace(identity.SubjectHint)
		identity.DisplayName = strings.TrimSpace(identity.DisplayName)
		identity.Email = strings.ToLower(strings.TrimSpace(identity.Email))
		identity.AvatarURL = strings.TrimSpace(identity.AvatarURL)
		identity.External = true
		identity.UserID = userID
		identity.CanUnbind = canUnbindExternalIdentity(user.User, externalCount)
		if !identity.CanUnbind {
			identity.UnbindBlockedBy = "last_sign_in_method"
		}
		identities = append(identities, identity)
	}
	return identities, nil
}

// BindAuthIdentity attaches a verified external sign-in identity to the user.
func (s *Service) BindAuthIdentity(ctx context.Context, req BindAuthIdentityRequest) ([]contract.UserAuthIdentity, error) {
	if req.UserID <= 0 {
		return nil, ErrInvalidInput
	}
	user, err := s.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if user.Status == contract.StatusDisabled {
		return nil, ErrUserDisabled
	}
	provider := normalizeAuthIdentityProvider(req.Provider)
	providerKey := strings.TrimSpace(req.ProviderKey)
	subjectHash := strings.TrimSpace(req.ProviderSubjectHash)
	if provider == "" || provider == contract.AuthIdentityProviderEmail || providerKey == "" || subjectHash == "" {
		return nil, ErrInvalidInput
	}
	existing, err := s.store.FindAuthIdentityByProviderSubject(ctx, provider, providerKey, subjectHash)
	if err == nil && existing.UserID != req.UserID {
		return nil, ErrIdentityAlreadyBound
	}
	if err != nil && !errors.Is(err, contract.ErrNotFound) {
		return nil, err
	}
	now := s.clock.Now().UTC()
	verifiedAt := req.VerifiedAt.UTC()
	if verifiedAt.IsZero() {
		verifiedAt = now
	}
	lastUsedAt := now
	_, err = s.store.UpsertAuthIdentity(ctx, contract.CreateUserAuthIdentity{
		UserID:              req.UserID,
		Provider:            provider,
		ProviderKey:         providerKey,
		ProviderSubjectHash: subjectHash,
		SubjectHint:         strings.TrimSpace(req.SubjectHint),
		DisplayName:         strings.TrimSpace(req.DisplayName),
		Email:               strings.ToLower(strings.TrimSpace(req.Email)),
		EmailVerified:       req.EmailVerified,
		AvatarURL:           strings.TrimSpace(req.AvatarURL),
		VerifiedAt:          &verifiedAt,
		LastUsedAt:          &lastUsedAt,
	})
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		if errors.Is(err, contract.ErrAlreadyExists) {
			return nil, ErrIdentityAlreadyBound
		}
		return nil, err
	}
	return s.ListAuthIdentities(ctx, req.UserID)
}

// UnbindAuthIdentity removes one external sign-in identity from the user.
func (s *Service) UnbindAuthIdentity(ctx context.Context, userID int, identityID int) ([]contract.UserAuthIdentity, error) {
	if userID <= 0 || identityID <= 0 {
		return nil, ErrInvalidInput
	}
	identities, err := s.ListAuthIdentities(ctx, userID)
	if err != nil {
		return nil, err
	}
	var target contract.UserAuthIdentity
	for _, identity := range identities {
		if identity.ID == identityID {
			target = identity
			break
		}
	}
	if target.ID == 0 {
		return nil, ErrIdentityNotFound
	}
	if !target.External {
		return nil, ErrInvalidInput
	}
	if !target.CanUnbind {
		return nil, ErrIdentityUnbindBlocked
	}
	if err := s.store.DeleteAuthIdentity(ctx, userID, identityID); err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return nil, ErrIdentityNotFound
		}
		return nil, err
	}
	return s.ListAuthIdentities(ctx, userID)
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
	seen := map[contract.Role]bool{}
	for _, role := range roles {
		role = contract.Role(strings.ToLower(strings.TrimSpace(string(role))))
		if !validRoleName(role) || seen[role] {
			continue
		}
		seen[role] = true
		cloned = append(cloned, role)
	}
	return cloned
}

func validRoleName(role contract.Role) bool {
	return roleNamePattern.MatchString(string(role))
}

func normalizePermissions(permissions []string) ([]string, error) {
	out := make([]string, 0, len(permissions))
	seen := map[string]bool{}
	for _, permission := range permissions {
		permission = strings.ToLower(strings.TrimSpace(permission))
		if permission == "" {
			continue
		}
		if !permissionPattern.MatchString(permission) {
			return nil, ErrInvalidInput
		}
		if seen[permission] {
			continue
		}
		seen[permission] = true
		out = append(out, permission)
	}
	return out, nil
}

func (s *Service) validateNewRoleName(ctx context.Context, name contract.Role) error {
	if contract.IsBuiltInRole(name) {
		return ErrUserAlreadyExists
	}
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if role.Name == name {
			return ErrUserAlreadyExists
		}
	}
	return nil
}

func (s *Service) validateRolesExist(ctx context.Context, roles []contract.Role) error {
	existing, err := s.store.ListRoles(ctx)
	if err != nil {
		return err
	}
	known := make(map[contract.Role]bool, len(existing))
	for _, role := range existing {
		known[role.Name] = true
	}
	for _, role := range roles {
		if contract.IsBuiltInRole(role) || known[role] {
			continue
		}
		return ErrInvalidInput
	}
	return nil
}

func validStatus(status contract.Status) bool {
	switch status {
	case contract.StatusActive, contract.StatusDisabled, contract.StatusPending:
		return true
	default:
		return false
	}
}

func normalizeAuthIdentityProvider(provider contract.AuthIdentityProvider) contract.AuthIdentityProvider {
	switch contract.AuthIdentityProvider(strings.ToLower(strings.TrimSpace(string(provider)))) {
	case contract.AuthIdentityProviderEmail:
		return contract.AuthIdentityProviderEmail
	case contract.AuthIdentityProviderOIDC:
		return contract.AuthIdentityProviderOIDC
	case contract.AuthIdentityProviderGitHub:
		return contract.AuthIdentityProviderGitHub
	case contract.AuthIdentityProviderGoogle:
		return contract.AuthIdentityProviderGoogle
	case contract.AuthIdentityProviderLinuxDo:
		return contract.AuthIdentityProviderLinuxDo
	case contract.AuthIdentityProviderWeChat:
		return contract.AuthIdentityProviderWeChat
	case contract.AuthIdentityProviderDingTalk:
		return contract.AuthIdentityProviderDingTalk
	default:
		return ""
	}
}

func emailAuthIdentity(user contract.User) contract.UserAuthIdentity {
	email := strings.ToLower(strings.TrimSpace(user.Email))
	displayName := strings.TrimSpace(user.Name)
	if displayName == "" {
		displayName = email
	}
	return contract.UserAuthIdentity{
		UserID:        user.ID,
		Provider:      contract.AuthIdentityProviderEmail,
		ProviderKey:   "local",
		SubjectHint:   email,
		DisplayName:   displayName,
		Email:         email,
		EmailVerified: user.EmailVerifiedAt != nil,
		External:      false,
		VerifiedAt:    cloneAuthIdentityTime(user.EmailVerifiedAt),
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.CreatedAt,
		CanUnbind:     false,
	}
}

func canUnbindExternalIdentity(user contract.User, externalCount int) bool {
	if externalCount > 1 {
		return true
	}
	return strings.TrimSpace(user.Email) != ""
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

func cloneAuthIdentityTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

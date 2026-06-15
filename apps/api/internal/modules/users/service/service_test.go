package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	"golang.org/x/crypto/bcrypt"
)

func newTestService(t *testing.T, store contract.Store) *Service {
	t.Helper()
	svc, err := NewWithPasswordCost(store, nil, bcrypt.MinCost)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestCreateHashesPasswordAndDefaultsRole(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)

	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if created.PasswordHash == "" {
		t.Fatal("expected password hash")
	}
	if len(created.Roles) != 1 || created.Roles[0] != contract.RoleUser {
		t.Fatalf("expected default user role, got %#v", created.Roles)
	}
	if created.Balance != "0.00000000" || created.Currency != "USD" {
		t.Fatalf("expected default balance, got %s %s", created.Balance, created.Currency)
	}
}

func TestCreateUsesConfiguredPasswordHashCost(t *testing.T) {
	store := newMemoryStore()
	svc, err := NewWithPasswordCost(store, nil, bcrypt.MinCost)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "cost@srapi.local",
		Name:     "Cost",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(created.PasswordHash))
	if err != nil {
		t.Fatalf("read bcrypt cost: %v", err)
	}
	if cost != bcrypt.MinCost {
		t.Fatalf("expected configured bcrypt cost %d, got %d", bcrypt.MinCost, cost)
	}
}

func TestAuthenticatePasswordAcceptsValidPassword(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
		Roles:    []contract.Role{contract.RoleAdmin},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	authed, err := svc.AuthenticatePassword(context.Background(), created.Email, "password123")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authed.ID != created.ID {
		t.Fatalf("expected user id %d, got %d", created.ID, authed.ID)
	}
}

func TestVerifyEmailSetsVerifiedAt(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "verify@srapi.local",
		Name:     "Verify",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	verifiedAt := time.Date(2026, 5, 29, 18, 30, 0, 0, time.UTC)

	updated, err := svc.VerifyEmail(context.Background(), created.ID, verifiedAt)
	if err != nil {
		t.Fatalf("verify email: %v", err)
	}
	if updated.EmailVerifiedAt == nil || !updated.EmailVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("expected stored email verified at %s, got %v", verifiedAt, updated.EmailVerifiedAt)
	}
	if updated.User.EmailVerifiedAt == nil || !updated.User.EmailVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("expected public user email verified at %s, got %v", verifiedAt, updated.User.EmailVerifiedAt)
	}
}

func TestDeleteRemovesUserAndAllowsEmailReuse(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "delete@srapi.local",
		Name:     "Delete",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := svc.FindByID(context.Background(), created.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected deleted user not found, got %v", err)
	}
	recreated, err := svc.Create(context.Background(), CreateRequest{
		Email:    "delete@srapi.local",
		Name:     "Recreated",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("reuse email after delete: %v", err)
	}
	if recreated.ID == created.ID {
		t.Fatalf("expected a new user id after delete")
	}
}

func TestListAuthIdentitiesIncludesEmailAndExternalIdentities(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "identity@srapi.local",
		Name:     "Identity User",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	verifiedAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	if _, err := svc.VerifyEmail(context.Background(), created.ID, verifiedAt); err != nil {
		t.Fatalf("verify email: %v", err)
	}
	externalVerifiedAt := time.Date(2026, 5, 30, 11, 0, 0, 0, time.UTC)
	if _, err := store.UpsertAuthIdentity(context.Background(), contract.CreateUserAuthIdentity{
		UserID:              created.ID,
		Provider:            contract.AuthIdentityProviderOIDC,
		ProviderKey:         "https://issuer.example",
		ProviderSubjectHash: "sha256:subject",
		SubjectHint:         "subj...1234",
		DisplayName:         "External Identity",
		Email:               "external@srapi.local",
		EmailVerified:       true,
		VerifiedAt:          &externalVerifiedAt,
	}); err != nil {
		t.Fatalf("upsert identity: %v", err)
	}

	identities, err := svc.ListAuthIdentities(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	if len(identities) != 2 {
		t.Fatalf("expected email plus external identity, got %+v", identities)
	}
	if identities[0].Provider != contract.AuthIdentityProviderEmail || identities[0].External || identities[0].CanUnbind {
		t.Fatalf("unexpected email identity: %+v", identities[0])
	}
	if !identities[0].EmailVerified || identities[0].VerifiedAt == nil || !identities[0].VerifiedAt.Equal(verifiedAt) {
		t.Fatalf("expected verified email identity, got %+v", identities[0])
	}
	if identities[1].Provider != contract.AuthIdentityProviderOIDC || !identities[1].External || !identities[1].CanUnbind {
		t.Fatalf("unexpected external identity: %+v", identities[1])
	}
	if identities[1].SubjectHint != "subj...1234" || identities[1].VerifiedAt == nil || !identities[1].VerifiedAt.Equal(externalVerifiedAt) {
		t.Fatalf("expected external metadata, got %+v", identities[1])
	}
}

func TestBindAuthIdentityIsIdempotentAndRejectsDifferentOwner(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	first, err := svc.Create(context.Background(), CreateRequest{
		Email:    "first-bind@srapi.local",
		Name:     "First Bind",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}
	second, err := svc.Create(context.Background(), CreateRequest{
		Email:    "second-bind@srapi.local",
		Name:     "Second Bind",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}

	identities, err := svc.BindAuthIdentity(context.Background(), BindAuthIdentityRequest{
		UserID:              first.ID,
		Provider:            contract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:oauth-subject",
		SubjectHint:         "oidc:subject",
		DisplayName:         "OAuth User",
		Email:               "OAuth@Example.COM",
		EmailVerified:       true,
		AvatarURL:           "https://cdn.example/avatar.png",
	})
	if err != nil {
		t.Fatalf("bind identity: %v", err)
	}
	if len(identities) != 2 || identities[1].UserID != first.ID || identities[1].Email != "oauth@example.com" {
		t.Fatalf("unexpected bound identities: %+v", identities)
	}

	updated, err := svc.BindAuthIdentity(context.Background(), BindAuthIdentityRequest{
		UserID:              first.ID,
		Provider:            contract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:oauth-subject",
		SubjectHint:         "oidc:subject",
		DisplayName:         "Updated OAuth User",
		Email:               "updated@example.com",
		EmailVerified:       true,
	})
	if err != nil {
		t.Fatalf("rebind same identity to same user: %v", err)
	}
	if len(updated) != 2 || updated[1].DisplayName != "Updated OAuth User" || updated[1].ID != identities[1].ID {
		t.Fatalf("expected idempotent metadata update, got %+v", updated)
	}

	if _, err := svc.BindAuthIdentity(context.Background(), BindAuthIdentityRequest{
		UserID:              second.ID,
		Provider:            contract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:oauth-subject",
		SubjectHint:         "oidc:subject",
	}); !errors.Is(err, ErrIdentityAlreadyBound) {
		t.Fatalf("expected identity already bound error, got %v", err)
	}
	found, err := store.FindAuthIdentityByProviderSubject(context.Background(), contract.AuthIdentityProviderOIDC, "issuer-main", "sha256:oauth-subject")
	if err != nil {
		t.Fatalf("find identity: %v", err)
	}
	if found.UserID != first.ID {
		t.Fatalf("expected original owner %d, got %+v", first.ID, found)
	}
}

func TestUnbindAuthIdentityRemovesExternalIdentity(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "unbind@srapi.local",
		Name:     "Unbind User",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	identity, err := store.UpsertAuthIdentity(context.Background(), contract.CreateUserAuthIdentity{
		UserID:              created.ID,
		Provider:            contract.AuthIdentityProviderLinuxDo,
		ProviderKey:         "linuxdo",
		ProviderSubjectHash: "sha256:linuxdo-subject",
		SubjectHint:         "linuxdo-user",
	})
	if err != nil {
		t.Fatalf("upsert identity: %v", err)
	}

	identities, err := svc.UnbindAuthIdentity(context.Background(), created.ID, identity.ID)
	if err != nil {
		t.Fatalf("unbind identity: %v", err)
	}
	if len(identities) != 1 || identities[0].Provider != contract.AuthIdentityProviderEmail {
		t.Fatalf("expected only local email identity after unbind, got %+v", identities)
	}
}

func TestUnbindAuthIdentityRejectsDerivedEmailIdentity(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "email-only@srapi.local",
		Name:     "Email Only",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := svc.UnbindAuthIdentity(context.Background(), created.ID, 1); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("expected derived email identity to be unaddressable, got %v", err)
	}
}

func TestUnbindAuthIdentityRejectsLastSignInMethod(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	user, err := store.Create(context.Background(), contract.CreateStoredUser{
		Email:        "",
		Name:         "External Only",
		PasswordHash: "external-only",
		Status:       contract.StatusActive,
		Roles:        []contract.Role{contract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create stored user: %v", err)
	}
	identity, err := store.UpsertAuthIdentity(context.Background(), contract.CreateUserAuthIdentity{
		UserID:              user.ID,
		Provider:            contract.AuthIdentityProviderOIDC,
		ProviderKey:         "https://issuer.example",
		ProviderSubjectHash: "sha256:only-subject",
		SubjectHint:         "only-user",
	})
	if err != nil {
		t.Fatalf("upsert identity: %v", err)
	}

	if _, err := svc.UnbindAuthIdentity(context.Background(), user.ID, identity.ID); !errors.Is(err, ErrIdentityUnbindBlocked) {
		t.Fatalf("expected last sign-in method unbind to be blocked, got %v", err)
	}
}

func TestUpdateEmailClearsVerifiedAt(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "old-email@srapi.local",
		Name:     "Email Change",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	verifiedAt := time.Date(2026, 5, 29, 19, 0, 0, 0, time.UTC)
	if _, err := svc.VerifyEmail(context.Background(), created.ID, verifiedAt); err != nil {
		t.Fatalf("verify email: %v", err)
	}
	newEmail := "new-email@srapi.local"

	updated, err := svc.Update(context.Background(), created.ID, UpdateRequest{Email: &newEmail})
	if err != nil {
		t.Fatalf("update email: %v", err)
	}
	if updated.Email != newEmail {
		t.Fatalf("expected email update, got %q", updated.Email)
	}
	if updated.EmailVerifiedAt != nil || updated.User.EmailVerifiedAt != nil {
		t.Fatalf("expected email change to clear verified timestamp, got %+v", updated)
	}
}

func TestCustomRoleCarriesPermissions(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	role, err := svc.CreateRole(context.Background(), CreateRoleRequest{
		Name:        "payment_reader",
		Description: "Payment reader",
		Permissions: []string{contract.PermissionPaymentRead, contract.PermissionPaymentRead},
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if role.Name != "payment_reader" || len(role.Permissions) != 1 || role.Permissions[0] != contract.PermissionPaymentRead {
		t.Fatalf("unexpected role definition: %+v", role)
	}
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "reader@srapi.local",
		Name:     "Reader",
		Password: "password123",
		Roles:    []contract.Role{"payment_reader"},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if len(created.Permissions) != 1 || created.Permissions[0] != contract.PermissionPaymentRead {
		t.Fatalf("expected role permission on user, got %+v", created.Permissions)
	}
}

func TestBuiltInRolesUsePermissionCatalog(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	roles, err := svc.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	byName := map[contract.Role]contract.RoleDefinition{}
	for _, role := range roles {
		byName[role.Name] = role
	}

	allPermissions := permissionSet(contract.AllPermissions())
	for _, roleName := range []contract.Role{contract.RoleOwner, contract.RoleAdmin} {
		role, ok := byName[roleName]
		if !ok {
			t.Fatalf("expected built-in role %s", roleName)
		}
		if !samePermissionSet(permissionSet(role.Permissions), allPermissions) {
			t.Fatalf("expected %s to have all permissions, got %+v", roleName, role.Permissions)
		}
	}

	operator, ok := byName[contract.RoleOperator]
	if !ok {
		t.Fatalf("expected built-in operator role")
	}
	operatorPermissions := permissionSet(operator.Permissions)
	for _, permission := range contract.ReadOnlyPermissions() {
		if !operatorPermissions[permission] {
			t.Fatalf("expected operator read permission %s", permission)
		}
	}
	for _, permission := range []string{contract.PermissionOpsWrite, contract.PermissionAccountWrite, contract.PermissionRiskControlWrite} {
		if !operatorPermissions[permission] {
			t.Fatalf("expected operator write permission %s", permission)
		}
	}
	for _, permission := range []string{contract.PermissionUserWrite, contract.PermissionRoleWrite, contract.PermissionProviderWrite} {
		if operatorPermissions[permission] {
			t.Fatalf("operator should not have broad write permission %s", permission)
		}
	}

	user, ok := byName[contract.RoleUser]
	if !ok {
		t.Fatalf("expected built-in user role")
	}
	if len(user.Permissions) != 0 {
		t.Fatalf("expected user role without admin permissions, got %+v", user.Permissions)
	}
}

func TestUpdateAndDeleteRole(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	ctx := context.Background()
	created, err := svc.CreateRole(ctx, CreateRoleRequest{
		Name:        "payment_reader",
		Description: "Payment reader",
		Permissions: []string{contract.PermissionPaymentRead},
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	newDesc := "Payments + refunds reader"
	updated, err := svc.UpdateRole(ctx, created.ID, UpdateRoleRequest{
		Description: &newDesc,
		Permissions: &[]string{contract.PermissionPaymentRead, contract.PermissionAuditLogRead},
	})
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if updated.Description != newDesc || len(updated.Permissions) != 2 {
		t.Fatalf("unexpected updated role: %+v", updated)
	}

	// Built-in roles cannot be modified or deleted.
	var adminID int
	roles, err := svc.ListRoles(ctx)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	for _, role := range roles {
		if role.Name == contract.RoleAdmin {
			adminID = role.ID
		}
	}
	if adminID == 0 {
		t.Fatalf("expected seeded admin role")
	}
	if _, err := svc.UpdateRole(ctx, adminID, UpdateRoleRequest{Permissions: &[]string{"thing:read"}}); !errors.Is(err, ErrRoleImmutable) {
		t.Fatalf("expected ErrRoleImmutable updating built-in, got %v", err)
	}
	if err := svc.DeleteRole(ctx, adminID); !errors.Is(err, ErrRoleImmutable) {
		t.Fatalf("expected ErrRoleImmutable deleting built-in, got %v", err)
	}

	// Unknown id → not found.
	if _, err := svc.UpdateRole(ctx, 999999, UpdateRoleRequest{Permissions: &[]string{"thing:read"}}); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}

	if _, err := svc.CreateRole(ctx, CreateRoleRequest{Name: "unknown_permission", Permissions: []string{"thing:read"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unknown permission, got %v", err)
	}

	// Delete the custom role, then deleting again is not-found.
	if err := svc.DeleteRole(ctx, created.ID); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	if err := svc.DeleteRole(ctx, created.ID); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound after delete, got %v", err)
	}
}

func permissionSet(permissions []string) map[string]bool {
	out := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		out[permission] = true
	}
	return out
}

func samePermissionSet(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for permission := range left {
		if !right[permission] {
			return false
		}
	}
	return true
}

func TestAuthenticatePasswordRejectsWrongPassword(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = svc.AuthenticatePassword(context.Background(), created.Email, "wrongpassword")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticatePasswordRejectsDisabledUser(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	store.setStatus(created.ID, contract.StatusDisabled)

	_, err = svc.AuthenticatePassword(context.Background(), created.Email, "password123")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
}

func TestChangePasswordRequiresCurrentPasswordAndUpdatesHash(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "change-password@srapi.local",
		Name:     "Change Password",
		Password: "oldpassword123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := svc.ChangePassword(context.Background(), created.ID, ChangePasswordRequest{
		CurrentPassword: "wrongpassword",
		NewPassword:     "newpassword123",
	}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	updated, err := svc.ChangePassword(context.Background(), created.ID, ChangePasswordRequest{
		CurrentPassword: "oldpassword123",
		NewPassword:     "newpassword123",
	})
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	if updated.PasswordHash == created.PasswordHash {
		t.Fatal("expected password hash to change")
	}
	if _, err := svc.AuthenticatePassword(context.Background(), created.Email, "oldpassword123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected old password to fail, got %v", err)
	}
	if _, err := svc.AuthenticatePassword(context.Background(), created.Email, "newpassword123"); err != nil {
		t.Fatalf("expected new password to authenticate: %v", err)
	}
}

func TestChangePasswordRejectsShortNewPassword(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "short-password@srapi.local",
		Name:     "Short Password",
		Password: "oldpassword123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// A new password shorter than the minimum length is rejected on change.
	if _, err := svc.ChangePassword(context.Background(), created.ID, ChangePasswordRequest{
		CurrentPassword: "oldpassword123",
		NewPassword:     "short7!",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for short new password, got %v", err)
	}

	// The original password remains usable after a rejected change.
	if _, err := svc.AuthenticatePassword(context.Background(), created.Email, "oldpassword123"); err != nil {
		t.Fatalf("expected original password to still authenticate: %v", err)
	}

	// A new password at the minimum length passes the length gate.
	if _, err := svc.ChangePassword(context.Background(), created.ID, ChangePasswordRequest{
		CurrentPassword: "oldpassword123",
		NewPassword:     "password8",
	}); err != nil {
		t.Fatalf("expected eight-character new password to be accepted: %v", err)
	}
}

func TestUpdateProfileOnlyChangesDisplayName(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "profile@srapi.local",
		Name:     "Original Name",
		Password: "password123",
		Roles:    []contract.Role{contract.RoleUser},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	updated, err := svc.UpdateProfile(context.Background(), created.ID, UpdateProfileRequest{Name: "  Updated Name  "})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Name != "Updated Name" {
		t.Fatalf("expected trimmed updated name, got %q", updated.Name)
	}
	if updated.Email != created.Email || updated.Status != created.Status || len(updated.Roles) != 1 || updated.Roles[0] != contract.RoleUser {
		t.Fatalf("profile update changed protected fields: %+v", updated)
	}
	if _, err := svc.UpdateProfile(context.Background(), created.ID, UpdateProfileRequest{Name: ""}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid empty name, got %v", err)
	}
}

func TestAvatarServiceNormalizesUploadAndDecoratesUser(t *testing.T) {
	store := newAvatarMemoryStore()
	svc, err := NewAvatarService(store, nil)
	if err != nil {
		t.Fatalf("new avatar service: %v", err)
	}
	userID := 7
	actorID := 3

	uploaded, err := svc.Upsert(context.Background(), userID, bytes.NewReader(testAvatarJPEG(t)), &actorID)
	if err != nil {
		t.Fatalf("upsert avatar: %v", err)
	}
	if uploaded.ContentType != "image/png" || uploaded.ByteSize <= 0 || uploaded.SHA256 == "" || uploaded.Width != 2 || uploaded.Height != 2 {
		t.Fatalf("unexpected uploaded avatar: %+v", uploaded)
	}
	if _, _, err := image.Decode(bytes.NewReader(uploaded.Content)); err != nil {
		t.Fatalf("expected normalized image to decode: %v", err)
	}

	loaded, err := svc.Get(context.Background(), userID)
	if err != nil {
		t.Fatalf("get avatar: %v", err)
	}
	if loaded.SHA256 != uploaded.SHA256 || !bytes.Equal(loaded.Content, uploaded.Content) {
		t.Fatalf("loaded avatar mismatch: got %+v want %+v", loaded, uploaded)
	}

	decorated := svc.DecorateUser(context.Background(), contract.User{ID: userID, Email: "avatar@srapi.local"}, "/api/v1/users/7/avatar")
	if decorated.AvatarURL != "/api/v1/users/7/avatar" || decorated.AvatarMIME != "image/png" || decorated.AvatarSHA256 != uploaded.SHA256 || decorated.AvatarByteSize != uploaded.ByteSize || decorated.AvatarUpdatedAt == nil {
		t.Fatalf("expected decorated user avatar metadata, got %+v", decorated)
	}

	if err := svc.Delete(context.Background(), userID, &actorID); err != nil {
		t.Fatalf("delete avatar: %v", err)
	}
	if _, err := svc.Get(context.Background(), userID); !errors.Is(err, ErrAvatarNotFound) {
		t.Fatalf("expected avatar not found after delete, got %v", err)
	}
}

func TestAvatarServiceRejectsInvalidAndTooLargeUploads(t *testing.T) {
	store := newAvatarMemoryStore()
	svc, err := NewAvatarService(store, nil)
	if err != nil {
		t.Fatalf("new avatar service: %v", err)
	}
	if _, err := svc.Upsert(context.Background(), 1, strings.NewReader("not an image"), nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for non-image, got %v", err)
	}
	if _, err := svc.Upsert(context.Background(), 1, io.LimitReader(zeroReader{}, MaxAvatarUploadBytes+1), nil); !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("expected too large upload, got %v", err)
	}
}

func TestUpdateBalanceUsesDecimalMath(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "billing@srapi.local",
		Name:     "Billing",
		Password: "password123",
		Balance:  "1.00000000",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	updated, err := svc.UpdateBalance(context.Background(), created.ID, BalanceUpdateRequest{
		Operation: BalanceOperationIncrement,
		Amount:    "0.33333333",
	})
	if err != nil {
		t.Fatalf("update balance: %v", err)
	}
	if updated.Balance != "1.33333333" {
		t.Fatalf("expected exact decimal balance, got %s", updated.Balance)
	}

	updated, err = svc.UpdateBalance(context.Background(), created.ID, BalanceUpdateRequest{
		Operation: BalanceOperationDecrement,
		Amount:    "0.33333333",
	})
	if err != nil {
		t.Fatalf("update balance: %v", err)
	}
	if updated.Balance != "1.00000000" {
		t.Fatalf("expected exact decimal balance, got %s", updated.Balance)
	}
}

func testAvatarJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 24, G: 80, B: 210, A: 255})
	img.Set(1, 0, color.RGBA{R: 210, G: 80, B: 24, A: 255})
	img.Set(0, 1, color.RGBA{R: 24, G: 160, B: 80, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

type avatarMemoryStore struct {
	values map[string]map[string]any
}

func newAvatarMemoryStore() *avatarMemoryStore {
	return &avatarMemoryStore{values: map[string]map[string]any{}}
}

func (s *avatarMemoryStore) Get(_ context.Context, key string) (map[string]any, bool, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, false, nil
	}
	return cloneAnyMap(value), true, nil
}

func (s *avatarMemoryStore) Set(_ context.Context, key string, value map[string]any, _ *int) error {
	s.values[key] = cloneAnyMap(value)
	return nil
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func TestListUpdateAndBatchUsers(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	first, err := svc.Create(context.Background(), CreateRequest{
		Email:    "first@srapi.local",
		Name:     "First",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := svc.Create(context.Background(), CreateRequest{
		Email:    "second@srapi.local",
		Name:     "Second",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	rpmLimit := 120
	rpmLimitPtr := &rpmLimit
	status := contract.StatusDisabled
	roles := []contract.Role{contract.RoleOperator}
	result := svc.BatchUpdate(context.Background(), BatchUpdateRequest{
		UserIDs:  []int{first.ID, second.ID},
		Status:   &status,
		Roles:    &roles,
		RPMLimit: &rpmLimitPtr,
	})
	if len(result.Errors) != 0 || len(result.Updated) != 2 {
		t.Fatalf("unexpected batch result: %+v", result)
	}
	listed, err := svc.List(context.Background(), ListRequest{Status: &status, Query: "first"})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != first.ID || listed[0].RPMLimit == nil || *listed[0].RPMLimit != rpmLimit {
		t.Fatalf("unexpected listed users: %+v", listed)
	}
}

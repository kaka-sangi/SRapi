package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

func TestLoginCreatesSessionAndTouchesUser(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:        1,
				Email:     "admin@srapi.local",
				Name:      "Admin",
				Status:    userscontract.StatusActive,
				Roles:     []userscontract.Role{userscontract.RoleAdmin},
				CreatedAt: now.Add(-time.Hour),
			},
		},
		password: "password123",
	}
	sessions := newMemoryStore()
	svc, err := New(users, sessions, time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.Login(context.Background(), "admin@srapi.local", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Session.ID == "" || result.Session.CSRFToken == "" {
		t.Fatal("expected generated session id and csrf token")
	}
	if !result.Session.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected expiry: %s", result.Session.ExpiresAt)
	}
	if users.lastLoginUserID != 1 {
		t.Fatalf("expected last login touch for user 1, got %d", users.lastLoginUserID)
	}
}

func TestLoginRequiresSecondFactorWhenEnabled(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:        1,
				Email:     "admin@srapi.local",
				Name:      "Admin",
				Status:    userscontract.StatusActive,
				CreatedAt: now.Add(-time.Hour),
			},
		},
		password: "password123",
	}
	sessions := newMemoryStore()
	secondFactor := &fakeSecondFactor{enabled: true}
	svc, err := NewWithSecondFactor(users, sessions, time.Hour, fixedClock{now: now}, secondFactor, "second_factor_challenge_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.Login(context.Background(), "admin@srapi.local", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !result.RequiresSecondFactor || result.SecondFactorChallengeID == "" || result.SecondFactorChallengeUntil == nil {
		t.Fatalf("expected second factor challenge, got %+v", result)
	}
	if result.Session.ID != "" {
		t.Fatalf("expected no session before second factor, got %+v", result.Session)
	}
	if users.lastLoginUserID != 0 {
		t.Fatalf("expected last login untouched before second factor, got %d", users.lastLoginUserID)
	}
}

func TestCompleteSecondFactorLoginCreatesSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:        1,
				Email:     "admin@srapi.local",
				Name:      "Admin",
				Status:    userscontract.StatusActive,
				CreatedAt: now.Add(-time.Hour),
			},
		},
		password: "password123",
	}
	sessions := newMemoryStore()
	secondFactor := &fakeSecondFactor{enabled: true, code: "123456"}
	svc, err := NewWithSecondFactor(users, sessions, time.Hour, fixedClock{now: now}, secondFactor, "second_factor_challenge_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	pending, err := svc.Login(context.Background(), "admin@srapi.local", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	result, err := svc.CompleteSecondFactorLogin(context.Background(), pending.SecondFactorChallengeID, "123456")
	if err != nil {
		t.Fatalf("complete second factor: %v", err)
	}
	if result.Session.ID == "" || result.Session.CSRFToken == "" {
		t.Fatalf("expected session after second factor, got %+v", result.Session)
	}
	if users.lastLoginUserID != 1 {
		t.Fatalf("expected last login touch for user 1, got %d", users.lastLoginUserID)
	}
}

func TestAuthenticateSessionReturnsCurrentUserAndTouchesSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:        7,
				Email:     "user@srapi.local",
				Name:      "User",
				Status:    userscontract.StatusActive,
				Roles:     []userscontract.Role{userscontract.RoleUser},
				CreatedAt: now.Add(-time.Hour),
			},
		},
		password: "password123",
	}
	sessions := newMemoryStore()
	_, err := sessions.Create(context.Background(), authcontract.CreateSession{
		ID:        "sess_existing",
		UserID:    7,
		CSRFToken: "csrf_existing",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc, err := New(users, sessions, time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.AuthenticateSession(context.Background(), "sess_existing")
	if err != nil {
		t.Fatalf("authenticate session: %v", err)
	}
	if result.User.ID != 7 {
		t.Fatalf("expected user 7, got %d", result.User.ID)
	}
	if result.Session.LastSeenAt == nil || !result.Session.LastSeenAt.Equal(now) {
		t.Fatalf("expected last seen %s, got %v", now, result.Session.LastSeenAt)
	}
}

func TestAuthenticateSessionRejectsExpiredSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	sessions := newMemoryStore()
	_, err := sessions.Create(context.Background(), authcontract.CreateSession{
		ID:        "sess_expired",
		UserID:    7,
		CSRFToken: "csrf_existing",
		ExpiresAt: now.Add(-time.Second),
		CreatedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc, err := New(&fakeUserService{}, sessions, time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.AuthenticateSession(context.Background(), "sess_expired")
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
	if _, findErr := sessions.FindByID(context.Background(), "sess_expired"); !errors.Is(findErr, ErrSessionNotFound) {
		t.Fatalf("expected expired session to be deleted, got %v", findErr)
	}
}

func TestLogoutUserRevokesAllUserSessions(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	sessions := newMemoryStore()
	for _, input := range []authcontract.CreateSession{
		{ID: "sess_user_first", UserID: 7, CSRFToken: "csrf_first", ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "sess_user_second", UserID: 7, CSRFToken: "csrf_second", ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "sess_other_user", UserID: 8, CSRFToken: "csrf_other", ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	} {
		if _, err := sessions.Create(context.Background(), input); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	svc, err := New(&fakeUserService{}, sessions, time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.LogoutUser(context.Background(), 7); err != nil {
		t.Fatalf("logout user: %v", err)
	}
	if _, err := sessions.FindByID(context.Background(), "sess_user_first"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected first session revoked, got %v", err)
	}
	if _, err := sessions.FindByID(context.Background(), "sess_user_second"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected second session revoked, got %v", err)
	}
	if _, err := sessions.FindByID(context.Background(), "sess_other_user"); err != nil {
		t.Fatalf("expected other user session to remain active: %v", err)
	}
}

func TestRequestPasswordResetStoresHashAndOutboxWithoutEnumeration(t *testing.T) {
	now := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:     9,
				Email:  "reset@srapi.local",
				Name:   "Reset",
				Status: userscontract.StatusActive,
			},
		},
	}
	sessions := newMemoryStore()
	eventsStore := eventsmemory.New()
	events, err := eventsservice.New(eventsStore, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	svc, err := NewWithSecondFactor(users, sessions, time.Hour, fixedClock{now: now}, nil, "password_reset_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetEventEnqueuer(events)

	result, err := svc.RequestPasswordReset(context.Background(), "reset@srapi.local")
	if err != nil {
		t.Fatalf("request password reset: %v", err)
	}
	if !result.Accepted || result.UserID == nil || *result.UserID != 9 {
		t.Fatalf("unexpected reset request result: %+v", result)
	}
	outbox, err := events.ListOutbox(context.Background())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "AuthPasswordResetRequested" {
		t.Fatalf("expected one password reset outbox event, got %+v", outbox)
	}
	if outbox[0].Payload["reset_token_ciphertext"] == "" || outbox[0].Payload["recipient_email_hash"] == "reset@srapi.local" {
		t.Fatalf("outbox did not protect reset delivery data: %+v", outbox[0].Payload)
	}

	missingResult, err := svc.RequestPasswordReset(context.Background(), "missing@srapi.local")
	if err != nil {
		t.Fatalf("request missing password reset: %v", err)
	}
	if !missingResult.Accepted || missingResult.UserID != nil {
		t.Fatalf("missing user reset response leaked state: %+v", missingResult)
	}
	outbox, err = events.ListOutbox(context.Background())
	if err != nil {
		t.Fatalf("list outbox again: %v", err)
	}
	if len(outbox) != 1 {
		t.Fatalf("missing user should not enqueue delivery, got %d events", len(outbox))
	}
}

func TestConfirmPasswordResetConsumesTokenAndRevokesSessions(t *testing.T) {
	now := time.Date(2026, 5, 29, 15, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:     11,
				Email:  "confirm@srapi.local",
				Name:   "Confirm",
				Status: userscontract.StatusActive,
			},
		},
	}
	sessions := newMemoryStore()
	eventsStore := eventsmemory.New()
	events, err := eventsservice.New(eventsStore, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	svc, err := NewWithSecondFactor(users, sessions, time.Hour, fixedClock{now: now}, nil, "password_reset_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetEventEnqueuer(events)
	if _, err := sessions.Create(context.Background(), authcontract.CreateSession{
		ID:        "sess_reset_user",
		UserID:    11,
		CSRFToken: "csrf_reset_user",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	rawToken := "pwreset_test_confirm"
	if _, err := sessions.CreatePasswordResetToken(context.Background(), authcontract.CreatePasswordResetToken{
		UserID:    11,
		TokenHash: svc.passwordResetTokenHash(rawToken),
		ExpiresAt: now.Add(time.Minute),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	if err := svc.ConfirmPasswordReset(context.Background(), rawToken, "newpassword123"); err != nil {
		t.Fatalf("confirm password reset: %v", err)
	}
	if users.resetPasswordUserID != 11 || users.resetPassword != "newpassword123" {
		t.Fatalf("expected password reset call, got id=%d password=%q", users.resetPasswordUserID, users.resetPassword)
	}
	if _, err := sessions.FindByID(context.Background(), "sess_reset_user"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected reset to revoke sessions, got %v", err)
	}
	if err := svc.ConfirmPasswordReset(context.Background(), rawToken, "anotherpassword123"); !errors.Is(err, ErrPasswordResetInvalid) {
		t.Fatalf("expected single-use token rejection, got %v", err)
	}
}

func TestRequestEmailVerificationStoresHashAndOutboxWithoutEnumeration(t *testing.T) {
	now := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:     12,
				Email:  "verify@srapi.local",
				Name:   "Verify",
				Status: userscontract.StatusActive,
			},
		},
	}
	sessions := newMemoryStore()
	eventsStore := eventsmemory.New()
	events, err := eventsservice.New(eventsStore, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	svc, err := NewWithSecondFactor(users, sessions, time.Hour, fixedClock{now: now}, nil, "email_verify_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetEventEnqueuer(events)

	result, err := svc.RequestEmailVerification(context.Background(), "verify@srapi.local")
	if err != nil {
		t.Fatalf("request email verification: %v", err)
	}
	if !result.Accepted || result.UserID == nil || *result.UserID != 12 {
		t.Fatalf("unexpected email verification request result: %+v", result)
	}
	if len(sessions.emailVerifyTokens) != 1 {
		t.Fatalf("expected one stored verification receipt, got %d", len(sessions.emailVerifyTokens))
	}
	for hash := range sessions.emailVerifyTokens {
		if hash == "" || hash == "emailverify_plaintext" || strings.Contains(hash, "emailverify_") {
			t.Fatalf("verification receipt stored an unsafe token hash: %q", hash)
		}
	}
	outbox, err := events.ListOutbox(context.Background())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "AuthEmailVerificationRequested" {
		t.Fatalf("expected one email verification outbox event, got %+v", outbox)
	}
	if outbox[0].Payload["verification_token_ciphertext"] == "" || outbox[0].Payload["recipient_email_hash"] == "verify@srapi.local" {
		t.Fatalf("outbox did not protect email verification delivery data: %+v", outbox[0].Payload)
	}

	missingResult, err := svc.RequestEmailVerification(context.Background(), "missing@srapi.local")
	if err != nil {
		t.Fatalf("request missing email verification: %v", err)
	}
	if !missingResult.Accepted || missingResult.UserID != nil {
		t.Fatalf("missing user verification response leaked state: %+v", missingResult)
	}
	outbox, err = events.ListOutbox(context.Background())
	if err != nil {
		t.Fatalf("list outbox again: %v", err)
	}
	if len(outbox) != 1 {
		t.Fatalf("missing user should not enqueue delivery, got %d events", len(outbox))
	}
}

func TestConfirmEmailVerificationConsumesTokenAndMarksEmailVerified(t *testing.T) {
	now := time.Date(2026, 5, 29, 17, 0, 0, 0, time.UTC)
	users := &fakeUserService{
		user: userscontract.StoredUser{
			User: userscontract.User{
				ID:     13,
				Email:  "confirm-verify@srapi.local",
				Name:   "Confirm Verify",
				Status: userscontract.StatusActive,
			},
		},
	}
	sessions := newMemoryStore()
	svc, err := NewWithSecondFactor(users, sessions, time.Hour, fixedClock{now: now}, nil, "email_verify_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	rawToken := "emailverify_test_confirm"
	if _, err := sessions.CreateEmailVerificationToken(context.Background(), authcontract.CreateEmailVerificationToken{
		UserID:    13,
		TokenHash: svc.emailVerificationTokenHash(rawToken),
		ExpiresAt: now.Add(time.Minute),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create verification token: %v", err)
	}

	if err := svc.ConfirmEmailVerification(context.Background(), rawToken); err != nil {
		t.Fatalf("confirm email verification: %v", err)
	}
	if users.verifyEmailUserID != 13 || users.verifyEmailAt == nil || !users.verifyEmailAt.Equal(now) {
		t.Fatalf("expected email verification call, got id=%d at=%v", users.verifyEmailUserID, users.verifyEmailAt)
	}
	if err := svc.ConfirmEmailVerification(context.Background(), rawToken); !errors.Is(err, ErrEmailVerificationInvalid) {
		t.Fatalf("expected single-use token rejection, got %v", err)
	}
}

func TestPendingOAuthSessionStoresHashOnlyAndConsumesOnce(t *testing.T) {
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	sessions := newMemoryStore()
	svc, err := NewWithSecondFactor(&fakeUserService{}, sessions, time.Hour, fixedClock{now: now}, nil, "pending_oauth_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	targetUserID := 42

	created, err := svc.CreatePendingOAuthSession(context.Background(), CreatePendingOAuthSessionRequest{
		Intent:              authcontract.PendingOAuthIntentBindCurrentUser,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "https://issuer.example",
		ProviderSubjectHash: "sha256:subject",
		TargetUserID:        &targetUserID,
		RedirectTo:          "//evil.example/path",
		Profile: PendingOAuthProfile{
			SubjectHint:   "subj...1234",
			ResolvedEmail: "User@Srapi.Local",
			DisplayName:   "OIDC User",
			EmailVerified: true,
			AvatarURL:     "https://issuer.example/avatar.png",
		},
	})
	if err != nil {
		t.Fatalf("create pending oauth session: %v", err)
	}
	if created.SessionToken == "" {
		t.Fatal("expected pending oauth session token")
	}
	if len(sessions.pendingOAuth) != 1 {
		t.Fatalf("expected one pending oauth session, got %d", len(sessions.pendingOAuth))
	}
	for tokenHash, stored := range sessions.pendingOAuth {
		if tokenHash == created.SessionToken || strings.Contains(tokenHash, "oauth_pending") {
			t.Fatalf("pending oauth stored raw token material: key=%q token=%q", tokenHash, created.SessionToken)
		}
		if stored.SessionTokenHash != tokenHash || stored.RedirectTo != "/" || stored.ResolvedEmail != "user@srapi.local" {
			t.Fatalf("unexpected stored pending oauth session: %+v", stored)
		}
		if stored.TargetUserID == nil || *stored.TargetUserID != targetUserID {
			t.Fatalf("expected target user id %d, got %+v", targetUserID, stored.TargetUserID)
		}
	}

	found, err := svc.FindPendingOAuthSession(context.Background(), created.SessionToken)
	if err != nil {
		t.Fatalf("find pending oauth session: %v", err)
	}
	if found.ConsumedAt != nil || found.ID != created.Session.ID || found.ResolvedEmail != "user@srapi.local" {
		t.Fatalf("unexpected pending oauth preview: %+v", found)
	}

	consumed, err := svc.ConsumePendingOAuthSession(context.Background(), created.SessionToken)
	if err != nil {
		t.Fatalf("consume pending oauth session: %v", err)
	}
	if consumed.ConsumedAt == nil || !consumed.ConsumedAt.Equal(now) {
		t.Fatalf("expected consumed timestamp %s, got %+v", now, consumed)
	}
	if _, err := svc.ConsumePendingOAuthSession(context.Background(), created.SessionToken); !errors.Is(err, ErrPendingOAuthInvalid) {
		t.Fatalf("expected single-use pending oauth session, got %v", err)
	}
	if _, err := svc.FindPendingOAuthSession(context.Background(), created.SessionToken); !errors.Is(err, ErrPendingOAuthInvalid) {
		t.Fatalf("expected consumed pending oauth session to be hidden from preview, got %v", err)
	}
}

func TestHashOAuthProviderSubjectIsStableAndScoped(t *testing.T) {
	now := time.Date(2026, 5, 30, 9, 15, 0, 0, time.UTC)
	svc, err := NewWithSecondFactor(&fakeUserService{}, newMemoryStore(), time.Hour, fixedClock{now: now}, nil, "pending_oauth_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	first, err := svc.HashOAuthProviderSubject(userscontract.AuthIdentityProviderOIDC, "issuer-main", "subject-123")
	if err != nil {
		t.Fatalf("hash oauth provider subject: %v", err)
	}
	second, err := svc.HashOAuthProviderSubject(userscontract.AuthIdentityProviderOIDC, "issuer-main", "subject-123")
	if err != nil {
		t.Fatalf("hash oauth provider subject again: %v", err)
	}
	otherProviderKey, err := svc.HashOAuthProviderSubject(userscontract.AuthIdentityProviderOIDC, "issuer-other", "subject-123")
	if err != nil {
		t.Fatalf("hash scoped oauth provider subject: %v", err)
	}
	if first == "" || first != second || first == otherProviderKey || strings.Contains(first, "subject-123") {
		t.Fatalf("unexpected oauth subject hashes: first=%q second=%q other=%q", first, second, otherProviderKey)
	}
}

func TestPreparePendingOAuthBindLoginRequiresSecondFactorBoundToPendingSession(t *testing.T) {
	now := time.Date(2026, 5, 30, 9, 20, 0, 0, time.UTC)
	user := userscontract.StoredUser{
		User: userscontract.User{
			ID:        7,
			Email:     "owner@srapi.local",
			Name:      "Owner",
			Status:    userscontract.StatusActive,
			CreatedAt: now.Add(-time.Hour),
		},
	}
	users := &fakeUserService{user: user, password: "password123"}
	sessions := newMemoryStore()
	secondFactor := &fakeSecondFactor{enabled: true, code: "123456"}
	svc, err := NewWithSecondFactor(users, sessions, time.Hour, fixedClock{now: now}, secondFactor, "pending_oauth_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	pending, err := svc.CreatePendingOAuthSession(context.Background(), CreatePendingOAuthSessionRequest{
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:subject",
		Profile: PendingOAuthProfile{
			SubjectHint:   "oidc:subject",
			ResolvedEmail: "owner@srapi.local",
			EmailVerified: true,
		},
	})
	if err != nil {
		t.Fatalf("create pending oauth session: %v", err)
	}

	prepared, err := svc.PreparePendingOAuthBindLogin(context.Background(), pending.SessionToken, "owner@srapi.local", "password123", true)
	if err != nil {
		t.Fatalf("prepare pending oauth bind login: %v", err)
	}
	if !prepared.RequiresSecondFactor || prepared.SecondFactorChallengeID == "" || prepared.SecondFactorChallengeUntil == nil {
		t.Fatalf("expected pending oauth 2fa challenge, got %+v", prepared)
	}
	if users.lastLoginUserID != 0 {
		t.Fatalf("expected no session/login touch before second factor, got %d", users.lastLoginUserID)
	}
	if _, err := svc.CompletePendingOAuthBindLoginSecondFactor(context.Background(), "oauth_pending_wrong", prepared.SecondFactorChallengeID, "123456"); !errors.Is(err, ErrSecondFactorInvalid) {
		t.Fatalf("expected challenge to be bound to pending token, got %v", err)
	}

	completed, err := svc.CompletePendingOAuthBindLoginSecondFactor(context.Background(), pending.SessionToken, prepared.SecondFactorChallengeID, "123456")
	if err != nil {
		t.Fatalf("complete pending oauth bind login second factor: %v", err)
	}
	if completed.User.ID != user.ID || completed.Session.ID != pending.Session.ID {
		t.Fatalf("unexpected completion result: %+v", completed)
	}
	if !completed.AdoptDisplayName {
		t.Fatalf("expected adopt-display-name preference to survive the signed 2fa challenge")
	}
}

func TestPendingOAuthActionTokenIsBoundToPendingSession(t *testing.T) {
	now := time.Date(2026, 5, 30, 9, 25, 0, 0, time.UTC)
	sessions := newMemoryStore()
	svc, err := NewWithSecondFactor(&fakeUserService{}, sessions, time.Hour, fixedClock{now: now}, nil, "pending_oauth_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	pending, err := svc.CreatePendingOAuthSession(context.Background(), CreatePendingOAuthSessionRequest{
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:subject",
		Profile: PendingOAuthProfile{
			SubjectHint:   "oidc:subject",
			ResolvedEmail: "owner@srapi.local",
			EmailVerified: true,
		},
	})
	if err != nil {
		t.Fatalf("create pending oauth session: %v", err)
	}

	action, err := svc.IssuePendingOAuthActionToken(context.Background(), pending.SessionToken, "create_account")
	if err != nil {
		t.Fatalf("issue action token: %v", err)
	}
	if action.Token == "" || !action.ExpiresAt.Equal(now.Add(challengeTTL)) {
		t.Fatalf("unexpected action token: %+v", action)
	}
	if strings.Contains(action.Token, pending.SessionToken) || strings.Contains(action.Token, "sha256:subject") {
		t.Fatalf("action token leaked pending material: %q", action.Token)
	}
	otherPending, err := svc.CreatePendingOAuthSession(context.Background(), CreatePendingOAuthSessionRequest{
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:other-subject",
		Profile: PendingOAuthProfile{
			SubjectHint:   "oidc:other",
			ResolvedEmail: "other@srapi.local",
			EmailVerified: true,
		},
	})
	if err != nil {
		t.Fatalf("create other pending oauth session: %v", err)
	}
	if _, err := svc.VerifyPendingOAuthActionToken(context.Background(), otherPending.SessionToken, "create_account", action.Token); !errors.Is(err, ErrCSRFTokenInvalid) {
		t.Fatalf("expected action token to be bound to pending token, got %v", err)
	}
	verified, err := svc.VerifyPendingOAuthActionToken(context.Background(), pending.SessionToken, "create_account", action.Token)
	if err != nil {
		t.Fatalf("verify action token: %v", err)
	}
	if verified.ID != pending.Session.ID {
		t.Fatalf("unexpected verified session: %+v", verified)
	}
}

func TestPendingOAuthEmailCompletionUsesEncryptedOutboxAndCookieBoundConfirm(t *testing.T) {
	now := time.Date(2026, 5, 30, 14, 0, 0, 0, time.UTC)
	eventsStore := eventsmemory.New()
	eventsSvc, err := eventsservice.New(eventsStore, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	sessions := newMemoryStore()
	svc, err := NewWithSecondFactor(&fakeUserService{}, sessions, time.Hour, fixedClock{now: now}, nil, "pending_oauth_email_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetEventEnqueuer(eventsSvc)
	pending, err := svc.CreatePendingOAuthSession(context.Background(), CreatePendingOAuthSessionRequest{
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: strings.Repeat("b", 64),
		Profile: PendingOAuthProfile{
			SubjectHint: "oidc:bbbbbbbb",
		},
	})
	if err != nil {
		t.Fatalf("create pending oauth: %v", err)
	}

	result, err := svc.RequestPendingOAuthEmailCompletion(context.Background(), pending.SessionToken, "Complete@SRapi.Local")
	if err != nil {
		t.Fatalf("request pending oauth email completion: %v", err)
	}
	if !result.Accepted || !result.ExpiresAt.Equal(now.Add(pendingOAuthEmailCompletionTTL)) {
		t.Fatalf("unexpected email completion result: %+v", result)
	}
	outbox, err := eventsSvc.ListOutbox(context.Background())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "PendingOAuthEmailCompletionRequested" {
		t.Fatalf("expected one pending oauth email event, got %+v", outbox)
	}
	payload := outbox[0].Payload
	tokenCiphertext := fmt.Sprint(payload["verification_token_ciphertext"])
	if tokenCiphertext == "" || strings.Contains(fmt.Sprint(payload), "Complete@SRapi.Local") || strings.Contains(fmt.Sprint(payload), pending.SessionToken) || strings.Contains(fmt.Sprint(payload), strings.Repeat("b", 64)) {
		t.Fatalf("pending oauth email completion payload leaked sensitive material: %+v", payload)
	}

	rawTokenPayload, err := svc.decryptPendingOAuthEmailCompletionToken(tokenCiphertext)
	if err != nil {
		t.Fatalf("decrypt pending oauth email token: %v", err)
	}
	otherPending, err := svc.CreatePendingOAuthSession(context.Background(), CreatePendingOAuthSessionRequest{
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: strings.Repeat("c", 64),
		Profile: PendingOAuthProfile{
			SubjectHint: "oidc:cccccccc",
		},
	})
	if err != nil {
		t.Fatalf("create other pending oauth: %v", err)
	}
	if _, err := svc.ConfirmPendingOAuthEmailCompletion(context.Background(), otherPending.SessionToken, tokenCiphertext); !errors.Is(err, ErrPendingOAuthEmailInvalid) {
		t.Fatalf("expected other pending cookie to reject token, got %v", err)
	}
	updated, err := svc.ConfirmPendingOAuthEmailCompletion(context.Background(), pending.SessionToken, tokenCiphertext)
	if err != nil {
		t.Fatalf("confirm pending oauth email completion: %v", err)
	}
	if updated.ResolvedEmail != "complete@srapi.local" || !updated.EmailVerified {
		t.Fatalf("expected verified pending email, got %+v raw=%q", updated, rawTokenPayload)
	}
	if _, err := svc.ConfirmPendingOAuthEmailCompletion(context.Background(), pending.SessionToken, tokenCiphertext); !errors.Is(err, ErrPendingOAuthEmailInvalid) && !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected second confirm to fail, got %v", err)
	}
}

func TestPendingOAuthSessionRejectsExpiredSession(t *testing.T) {
	now := time.Date(2026, 5, 30, 9, 30, 0, 0, time.UTC)
	sessions := newMemoryStore()
	svc, err := NewWithSecondFactor(&fakeUserService{}, sessions, time.Hour, fixedClock{now: now}, nil, "pending_oauth_test_secret_32_bytes")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	rawToken := "oauth_pending_expired"
	if _, err := sessions.CreatePendingOAuthSession(context.Background(), authcontract.CreatePendingOAuthSession{
		SessionTokenHash:    svc.pendingOAuthSessionTokenHash(rawToken),
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderGitHub,
		ProviderKey:         "github",
		ProviderSubjectHash: "sha256:expired",
		ExpiresAt:           now.Add(-time.Minute),
		CreatedAt:           now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed pending oauth session: %v", err)
	}

	if _, err := svc.ConsumePendingOAuthSession(context.Background(), rawToken); !errors.Is(err, ErrPendingOAuthInvalid) {
		t.Fatalf("expected expired pending oauth rejection, got %v", err)
	}
	if _, err := svc.FindPendingOAuthSession(context.Background(), rawToken); !errors.Is(err, ErrPendingOAuthInvalid) {
		t.Fatalf("expected expired pending oauth preview rejection, got %v", err)
	}
}

func TestPendingOAuthSessionRequiresServerSecret(t *testing.T) {
	now := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	svc, err := New(&fakeUserService{}, newMemoryStore(), time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreatePendingOAuthSession(context.Background(), CreatePendingOAuthSessionRequest{
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderGitHub,
		ProviderKey:         "github",
		ProviderSubjectHash: "sha256:subject",
	})
	if !errors.Is(err, ErrPendingOAuthUnavailable) {
		t.Fatalf("expected pending oauth unavailable without server secret, got %v", err)
	}
	if _, err := svc.ConsumePendingOAuthSession(context.Background(), "oauth_pending_missing_secret"); !errors.Is(err, ErrPendingOAuthUnavailable) {
		t.Fatalf("expected pending oauth consume unavailable without server secret, got %v", err)
	}
	if _, err := svc.FindPendingOAuthSession(context.Background(), "oauth_pending_missing_secret"); !errors.Is(err, ErrPendingOAuthUnavailable) {
		t.Fatalf("expected pending oauth find unavailable without server secret, got %v", err)
	}
}

func TestStartOAuthAuthorizationBuildsPKCEURLAndEncryptedCookie(t *testing.T) {
	now := time.Date(2026, 5, 30, 11, 0, 0, 0, time.UTC)
	svc, err := NewWithSecondFactor(&fakeUserService{}, newMemoryStore(), time.Hour, fixedClock{now: now}, nil, "oauth_start_test_secret_32_bytes_min")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.StartOAuthAuthorization(StartOAuthAuthorizationRequest{
		Intent:     authcontract.PendingOAuthIntentLogin,
		Provider:   userscontract.AuthIdentityProviderOIDC,
		RedirectTo: "/dashboard",
		Config: OAuthAuthorizationProviderConfig{
			Provider:     userscontract.AuthIdentityProviderOIDC,
			ProviderKey:  "issuer-main",
			ClientID:     "client-123",
			AuthorizeURL: "https://idp.example/authorize",
			RedirectURI:  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
			Scopes:       []string{"openid", "email", "profile"},
		},
	})
	if err != nil {
		t.Fatalf("start oauth authorization: %v", err)
	}
	if result.AuthorizationURL == "" || result.FlowCookieValue == "" || !result.ExpiresAt.Equal(now.Add(oauthAuthorizationFlowTTL)) {
		t.Fatalf("unexpected oauth result: %+v", result)
	}
	parsed, err := url.Parse(result.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "idp.example" || parsed.Path != "/authorize" {
		t.Fatalf("unexpected authorization endpoint: %s", result.AuthorizationURL)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"response_type":         "code",
		"client_id":             "client-123",
		"redirect_uri":          "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
		"scope":                 "openid email profile",
		"code_challenge_method": "S256",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q in %s", key, got, want, result.AuthorizationURL)
		}
	}
	if query.Get("state") == "" || query.Get("nonce") == "" || query.Get("code_challenge") == "" {
		t.Fatalf("expected state, nonce, and code challenge in %s", result.AuthorizationURL)
	}
	if strings.Contains(result.FlowCookieValue, query.Get("state")) || strings.Contains(result.FlowCookieValue, query.Get("nonce")) || strings.Contains(result.FlowCookieValue, "client-123") {
		t.Fatalf("encrypted flow cookie leaked oauth material: %q", result.FlowCookieValue)
	}
	flow, err := svc.DecodeOAuthAuthorizationFlow(result.FlowCookieValue)
	if err != nil {
		t.Fatalf("decode flow cookie: %v", err)
	}
	if flow.Provider != userscontract.AuthIdentityProviderOIDC || flow.ProviderKey != "issuer-main" || flow.RedirectTo != "/dashboard" || flow.State != query.Get("state") || flow.Nonce != query.Get("nonce") {
		t.Fatalf("unexpected decoded flow: %+v query=%s", flow, result.AuthorizationURL)
	}
	if flow.CodeVerifier == "" || !flow.CreatedAt.Equal(now) || !flow.ExpiresAt.Equal(now.Add(oauthAuthorizationFlowTTL)) {
		t.Fatalf("unexpected decoded flow timing/verifier: %+v", flow)
	}
}

func TestStartOAuthAuthorizationRejectsUnsafeProviderURLAndNormalizesRedirect(t *testing.T) {
	now := time.Date(2026, 5, 30, 11, 30, 0, 0, time.UTC)
	svc, err := NewWithSecondFactor(&fakeUserService{}, newMemoryStore(), time.Hour, fixedClock{now: now}, nil, "oauth_start_test_secret_32_bytes_min")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.StartOAuthAuthorization(StartOAuthAuthorizationRequest{
		Provider: userscontract.AuthIdentityProviderGitHub,
		Config: OAuthAuthorizationProviderConfig{
			Provider:     userscontract.AuthIdentityProviderGitHub,
			ProviderKey:  "github",
			ClientID:     "github-client",
			AuthorizeURL: "http://github.example/login/oauth/authorize",
			RedirectURI:  "http://localhost:8080/api/v1/auth/oauth/github/callback",
		},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid provider url, got %v", err)
	}

	result, err := svc.StartOAuthAuthorization(StartOAuthAuthorizationRequest{
		Provider:   userscontract.AuthIdentityProviderGitHub,
		RedirectTo: "//evil.example/callback",
		Config: OAuthAuthorizationProviderConfig{
			Provider:     userscontract.AuthIdentityProviderGitHub,
			ProviderKey:  "github",
			ClientID:     "github-client",
			AuthorizeURL: "https://github.example/login/oauth/authorize",
			RedirectURI:  "https://api.example/api/v1/auth/oauth/github/callback",
		},
	})
	if err != nil {
		t.Fatalf("start github oauth authorization: %v", err)
	}
	flow, err := svc.DecodeOAuthAuthorizationFlow(result.FlowCookieValue)
	if err != nil {
		t.Fatalf("decode flow cookie: %v", err)
	}
	if flow.RedirectTo != "/" {
		t.Fatalf("expected unsafe redirect to normalize to /, got %q", flow.RedirectTo)
	}
}

func TestStartOAuthAuthorizationRequiresServerSecret(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	svc, err := New(&fakeUserService{}, newMemoryStore(), time.Hour, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.StartOAuthAuthorization(StartOAuthAuthorizationRequest{
		Provider: userscontract.AuthIdentityProviderGitHub,
		Config: OAuthAuthorizationProviderConfig{
			Provider:     userscontract.AuthIdentityProviderGitHub,
			ProviderKey:  "github",
			ClientID:     "github-client",
			AuthorizeURL: "https://github.example/login/oauth/authorize",
			RedirectURI:  "http://localhost:8080/api/v1/auth/oauth/github/callback",
		},
	})
	if !errors.Is(err, ErrOAuthUnavailable) {
		t.Fatalf("expected oauth unavailable without server secret, got %v", err)
	}
}

func TestValidateCSRFRejectsMismatch(t *testing.T) {
	err := ValidateCSRF(authcontract.Session{CSRFToken: "csrf_expected"}, "csrf_wrong")
	if !errors.Is(err, ErrCSRFTokenInvalid) {
		t.Fatalf("expected ErrCSRFTokenInvalid, got %v", err)
	}
}

type fakeUserService struct {
	user                userscontract.StoredUser
	password            string
	lastLoginUserID     int
	resetPasswordUserID int
	resetPassword       string
	verifyEmailUserID   int
	verifyEmailAt       *time.Time
}

type fakeSecondFactor struct {
	enabled bool
	code    string
}

func (f *fakeSecondFactor) IsEnabled(_ context.Context, _ int) (bool, error) {
	return f.enabled, nil
}

func (f *fakeSecondFactor) VerifyLogin(_ context.Context, _ int, code string) error {
	if code == f.code {
		return nil
	}
	return ErrSecondFactorInvalid
}

func (s *fakeUserService) AuthenticatePassword(_ context.Context, email, password string) (userscontract.StoredUser, error) {
	if s.user.Email == email && s.password == password {
		return s.user, nil
	}
	return userscontract.StoredUser{}, errors.New("invalid credentials")
}

func (s *fakeUserService) FindByID(_ context.Context, id int) (userscontract.StoredUser, error) {
	if s.user.ID == id {
		return s.user, nil
	}
	return userscontract.StoredUser{}, errors.New("user not found")
}

func (s *fakeUserService) FindByEmail(_ context.Context, email string) (userscontract.StoredUser, error) {
	if s.user.Email == email {
		return s.user, nil
	}
	return userscontract.StoredUser{}, errors.New("user not found")
}

func (s *fakeUserService) ResetPassword(_ context.Context, id int, newPassword string) (userscontract.StoredUser, error) {
	if s.user.ID != id {
		return userscontract.StoredUser{}, errors.New("user not found")
	}
	s.resetPasswordUserID = id
	s.resetPassword = newPassword
	return s.user, nil
}

func (s *fakeUserService) VerifyEmail(_ context.Context, id int, verifiedAt time.Time) (userscontract.StoredUser, error) {
	if s.user.ID != id {
		return userscontract.StoredUser{}, errors.New("user not found")
	}
	s.verifyEmailUserID = id
	s.verifyEmailAt = &verifiedAt
	s.user.EmailVerifiedAt = &verifiedAt
	s.user.User.EmailVerifiedAt = &verifiedAt
	return s.user, nil
}

func (s *fakeUserService) TouchLastLogin(_ context.Context, id int) error {
	s.lastLoginUserID = id
	return nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

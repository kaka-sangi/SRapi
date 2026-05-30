package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entauthsession "github.com/srapi/srapi/apps/api/ent/authsession"
	entemailverificationtoken "github.com/srapi/srapi/apps/api/ent/emailverificationtoken"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	entpasswordresettoken "github.com/srapi/srapi/apps/api/ent/passwordresettoken"
	entpendingoauthsession "github.com/srapi/srapi/apps/api/ent/pendingoauthsession"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStorePersistsSessionAndCSRFHashes(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/auth-store.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	createdAt := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(time.Hour)
	created, err := store.Create(ctx, authcontract.CreateSession{
		ID:        "sess_persisted_secret",
		UserID:    42,
		CSRFToken: "csrf_persisted_secret",
		ExpiresAt: expiresAt,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID != "sess_persisted_secret" || created.CSRFToken != "csrf_persisted_secret" {
		t.Fatalf("login create must return plaintext tokens once, got %+v", created)
	}

	row, err := client.AuthSession.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load raw auth session row: %v", err)
	}
	if row.SessionIDHash == "sess_persisted_secret" || row.CsrfTokenHash == "csrf_persisted_secret" {
		t.Fatalf("auth session row stored plaintext tokens: %+v", row)
	}
	if len(row.SessionIDHash) != 64 || len(row.CsrfTokenHash) != 64 {
		t.Fatalf("expected sha256 hex hashes, got session=%q csrf=%q", row.SessionIDHash, row.CsrfTokenHash)
	}

	found, err := store.FindByID(ctx, "sess_persisted_secret")
	if err != nil {
		t.Fatalf("find session: %v", err)
	}
	if found.ID != "sess_persisted_secret" || !strings.HasPrefix(found.CSRFToken, "sha256:") {
		t.Fatalf("expected raw session id and hashed csrf marker, got %+v", found)
	}
	if err := authservice.ValidateCSRF(found, "csrf_persisted_secret"); err != nil {
		t.Fatalf("validate hashed csrf: %v", err)
	}
	if err := authservice.ValidateCSRF(found, "csrf_wrong"); !errors.Is(err, authservice.ErrCSRFTokenInvalid) {
		t.Fatalf("expected hashed csrf mismatch to fail, got %v", err)
	}

	touchedAt := createdAt.Add(5 * time.Minute)
	if err := store.Touch(ctx, found.ID, touchedAt); err != nil {
		t.Fatalf("touch session: %v", err)
	}
	touched, err := store.FindByID(ctx, found.ID)
	if err != nil {
		t.Fatalf("find touched session: %v", err)
	}
	if touched.LastSeenAt == nil || !touched.LastSeenAt.Equal(touchedAt) {
		t.Fatalf("expected last seen %s, got %v", touchedAt, touched.LastSeenAt)
	}

	if err := store.Delete(ctx, found.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := store.FindByID(ctx, found.ID); err == nil {
		t.Fatal("expected deleted session to be unavailable")
	}
	revoked, err := client.AuthSession.Query().
		Where(entauthsession.SessionIDHash(row.SessionIDHash)).
		Only(ctx)
	if err != nil {
		t.Fatalf("load revoked row: %v", err)
	}
	if revoked.Status != "revoked" || revoked.DeletedAt == nil {
		t.Fatalf("expected soft-deleted revoked row, got %+v", revoked)
	}
}

func TestCleanupExpiredSessionsExpiresOnlyActiveRows(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/auth-cleanup.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	expired, err := store.Create(ctx, authcontract.CreateSession{
		ID:        "sess_expired",
		UserID:    7,
		CSRFToken: "csrf_expired",
		ExpiresAt: now.Add(-time.Minute),
		CreatedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	active, err := store.Create(ctx, authcontract.CreateSession{
		ID:        "sess_active",
		UserID:    7,
		CSRFToken: "csrf_active",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("create active session: %v", err)
	}
	revoked, err := store.Create(ctx, authcontract.CreateSession{
		ID:        "sess_revoked",
		UserID:    7,
		CSRFToken: "csrf_revoked",
		ExpiresAt: now.Add(-time.Hour),
		CreatedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create revoked session: %v", err)
	}
	if err := store.Delete(ctx, revoked.ID); err != nil {
		t.Fatalf("revoke session: %v", err)
	}

	result, err := store.CleanupExpiredSessions(ctx, now)
	if err != nil {
		t.Fatalf("cleanup expired sessions: %v", err)
	}
	if result.Selected != 1 || result.Expired != 1 {
		t.Fatalf("expected one active expired session cleaned, got %+v", result)
	}
	if _, err := store.FindByID(ctx, expired.ID); err == nil {
		t.Fatal("expected expired session to be unavailable after cleanup")
	}
	if _, err := store.FindByID(ctx, active.ID); err != nil {
		t.Fatalf("expected unexpired active session to remain available: %v", err)
	}

	expiredRow, err := client.AuthSession.Query().
		Where(entauthsession.SessionIDHash(tokenHash(expired.ID))).
		Only(ctx)
	if err != nil {
		t.Fatalf("load expired row: %v", err)
	}
	if expiredRow.Status != "expired" || expiredRow.DeletedAt == nil {
		t.Fatalf("expected expired row to be soft-deleted as expired, got %+v", expiredRow)
	}
	revokedRow, err := client.AuthSession.Query().
		Where(entauthsession.SessionIDHash(tokenHash(revoked.ID))).
		Only(ctx)
	if err != nil {
		t.Fatalf("load revoked row: %v", err)
	}
	if revokedRow.Status != "revoked" {
		t.Fatalf("expected revoked row to stay revoked, got %+v", revokedRow)
	}
}

func TestDeleteByUserIDRevokesOnlyActiveUserSessions(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/auth-delete-user.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	first, err := store.Create(ctx, authcontract.CreateSession{
		ID:        "sess_user_first",
		UserID:    7,
		CSRFToken: "csrf_user_first",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	second, err := store.Create(ctx, authcontract.CreateSession{
		ID:        "sess_user_second",
		UserID:    7,
		CSRFToken: "csrf_user_second",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	other, err := store.Create(ctx, authcontract.CreateSession{
		ID:        "sess_other_user",
		UserID:    8,
		CSRFToken: "csrf_other_user",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}

	if err := store.DeleteByUserID(ctx, 7); err != nil {
		t.Fatalf("delete by user: %v", err)
	}
	if _, err := store.FindByID(ctx, first.ID); err == nil {
		t.Fatal("expected first user session to be revoked")
	}
	if _, err := store.FindByID(ctx, second.ID); err == nil {
		t.Fatal("expected second user session to be revoked")
	}
	if _, err := store.FindByID(ctx, other.ID); err != nil {
		t.Fatalf("expected other user session to remain active: %v", err)
	}
}

func TestPasswordResetTokensAreHashOnlyAndSingleUse(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/auth-reset.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	token, err := store.CreatePasswordResetToken(ctx, authcontract.CreatePasswordResetToken{
		UserID:    42,
		TokenHash: strings.Repeat("a", 64),
		ExpiresAt: now.Add(time.Minute),
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create reset token: %v", err)
	}
	if token.TokenHash != strings.Repeat("a", 64) || token.UserID != 42 {
		t.Fatalf("unexpected created token: %+v", token)
	}
	row, err := client.PasswordResetToken.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load reset row: %v", err)
	}
	if row.TokenHash != strings.Repeat("a", 64) {
		t.Fatalf("expected hash-only reset row, got %+v", row)
	}

	consumed, err := store.ConsumePasswordResetToken(ctx, strings.Repeat("a", 64), now.Add(time.Second))
	if err != nil {
		t.Fatalf("consume reset token: %v", err)
	}
	if consumed.UsedAt == nil || !consumed.UsedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("expected used_at set, got %+v", consumed)
	}
	if _, err := store.ConsumePasswordResetToken(ctx, strings.Repeat("a", 64), now.Add(2*time.Second)); err == nil {
		t.Fatal("expected consumed token to be single-use")
	}

	if _, err := store.CreatePasswordResetToken(ctx, authcontract.CreatePasswordResetToken{
		UserID:    42,
		TokenHash: strings.Repeat("b", 64),
		ExpiresAt: now.Add(-time.Second),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create expired reset token: %v", err)
	}
	if _, err := store.ConsumePasswordResetToken(ctx, strings.Repeat("b", 64), now); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
	expiredRow, err := client.PasswordResetToken.Query().
		Where(entpasswordresettoken.TokenHash(strings.Repeat("b", 64))).
		Only(ctx)
	if err != nil {
		t.Fatalf("load expired token row: %v", err)
	}
	if expiredRow.UsedAt != nil {
		t.Fatalf("expired token should not be marked used, got %+v", expiredRow)
	}
}

func TestEmailVerificationTokensAreHashOnlyAndSingleUse(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/auth-email-verify.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 18, 0, 0, 0, time.UTC)
	token, err := store.CreateEmailVerificationToken(ctx, authcontract.CreateEmailVerificationToken{
		UserID:    42,
		TokenHash: strings.Repeat("c", 64),
		ExpiresAt: now.Add(time.Minute),
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create email verification token: %v", err)
	}
	if token.TokenHash != strings.Repeat("c", 64) || token.UserID != 42 {
		t.Fatalf("unexpected created token: %+v", token)
	}
	row, err := client.EmailVerificationToken.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load email verification row: %v", err)
	}
	if row.TokenHash != strings.Repeat("c", 64) {
		t.Fatalf("expected hash-only email verification row, got %+v", row)
	}

	consumed, err := store.ConsumeEmailVerificationToken(ctx, strings.Repeat("c", 64), now.Add(time.Second))
	if err != nil {
		t.Fatalf("consume email verification token: %v", err)
	}
	if consumed.UsedAt == nil || !consumed.UsedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("expected used_at set, got %+v", consumed)
	}
	if _, err := store.ConsumeEmailVerificationToken(ctx, strings.Repeat("c", 64), now.Add(2*time.Second)); err == nil {
		t.Fatal("expected consumed email verification token to be single-use")
	}

	if _, err := store.CreateEmailVerificationToken(ctx, authcontract.CreateEmailVerificationToken{
		UserID:    42,
		TokenHash: strings.Repeat("d", 64),
		ExpiresAt: now.Add(-time.Second),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create expired email verification token: %v", err)
	}
	if _, err := store.ConsumeEmailVerificationToken(ctx, strings.Repeat("d", 64), now); err == nil {
		t.Fatal("expected expired email verification token to be rejected")
	}
	expiredRow, err := client.EmailVerificationToken.Query().
		Where(entemailverificationtoken.TokenHash(strings.Repeat("d", 64))).
		Only(ctx)
	if err != nil {
		t.Fatalf("load expired email verification token row: %v", err)
	}
	if expiredRow.UsedAt != nil {
		t.Fatalf("expired email verification token should not be marked used, got %+v", expiredRow)
	}
}

func TestPendingOAuthSessionsAreHashOnlyAndSingleUse(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/pending-oauth.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 5, 30, 11, 0, 0, 0, time.UTC)
	targetUserID := 7
	session, err := store.CreatePendingOAuthSession(ctx, authcontract.CreatePendingOAuthSession{
		SessionTokenHash:    strings.Repeat("e", 64),
		Intent:              authcontract.PendingOAuthIntentBindCurrentUser,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "https://issuer.example",
		ProviderSubjectHash: strings.Repeat("f", 64),
		SubjectHint:         "subj...1234",
		TargetUserID:        &targetUserID,
		RedirectTo:          "/profile",
		ResolvedEmail:       "User@Srapi.Local",
		DisplayName:         "OIDC User",
		EmailVerified:       true,
		AvatarURL:           "https://issuer.example/avatar.png",
		ExpiresAt:           now.Add(time.Minute),
		CreatedAt:           now,
	})
	if err != nil {
		t.Fatalf("create pending oauth session: %v", err)
	}
	if session.SessionTokenHash != strings.Repeat("e", 64) || session.Provider != userscontract.AuthIdentityProviderOIDC {
		t.Fatalf("unexpected pending oauth session: %+v", session)
	}
	row, err := client.PendingOAuthSession.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load pending oauth row: %v", err)
	}
	if row.SessionTokenHash != strings.Repeat("e", 64) || row.ProviderSubjectHash != strings.Repeat("f", 64) {
		t.Fatalf("expected hash-only pending oauth row, got %+v", row)
	}
	if row.ResolvedEmail != "user@srapi.local" || row.TargetUserID == nil || *row.TargetUserID != targetUserID {
		t.Fatalf("unexpected normalized pending oauth row: %+v", row)
	}

	found, err := store.FindPendingOAuthSession(ctx, strings.Repeat("e", 64), now.Add(500*time.Millisecond))
	if err != nil {
		t.Fatalf("find pending oauth session: %v", err)
	}
	if found.ConsumedAt != nil || found.ID != session.ID || found.ResolvedEmail != "user@srapi.local" {
		t.Fatalf("unexpected pending oauth preview: %+v", found)
	}
	if _, err := store.CompletePendingOAuthEmail(ctx, strings.Repeat("e", 64), "other@srapi.local", now.Add(600*time.Millisecond)); err == nil {
		t.Fatal("expected pending oauth email completion to reject sessions that already have email")
	}

	consumed, err := store.ConsumePendingOAuthSession(ctx, strings.Repeat("e", 64), now.Add(time.Second))
	if err != nil {
		t.Fatalf("consume pending oauth session: %v", err)
	}
	if consumed.ConsumedAt == nil || !consumed.ConsumedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("expected consumed_at set, got %+v", consumed)
	}
	if _, err := store.ConsumePendingOAuthSession(ctx, strings.Repeat("e", 64), now.Add(2*time.Second)); err == nil {
		t.Fatal("expected consumed pending oauth session to be single-use")
	}
	if _, err := store.FindPendingOAuthSession(ctx, strings.Repeat("e", 64), now.Add(2*time.Second)); err == nil {
		t.Fatal("expected consumed pending oauth session to be hidden from preview")
	}

	if _, err := store.CreatePendingOAuthSession(ctx, authcontract.CreatePendingOAuthSession{
		SessionTokenHash:    strings.Repeat("a", 64),
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderGitHub,
		ProviderKey:         "github",
		ProviderSubjectHash: strings.Repeat("b", 64),
		ExpiresAt:           now.Add(-time.Second),
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("create expired pending oauth session: %v", err)
	}
	if _, err := store.ConsumePendingOAuthSession(ctx, strings.Repeat("a", 64), now); err == nil {
		t.Fatal("expected expired pending oauth session to be rejected")
	}
	if _, err := store.FindPendingOAuthSession(ctx, strings.Repeat("a", 64), now); err == nil {
		t.Fatal("expected expired pending oauth session preview to be rejected")
	}
	expiredRow, err := client.PendingOAuthSession.Query().
		Where(entpendingoauthsession.SessionTokenHash(strings.Repeat("a", 64))).
		Only(ctx)
	if err != nil {
		t.Fatalf("load expired pending oauth row: %v", err)
	}
	if expiredRow.ConsumedAt != nil {
		t.Fatalf("expired pending oauth session should not be marked consumed, got %+v", expiredRow)
	}
}

func TestPendingOAuthEmailCompletionUpdatesOnlyActiveEmaillessSession(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/pending-oauth-email.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC)
	session, err := store.CreatePendingOAuthSession(ctx, authcontract.CreatePendingOAuthSession{
		SessionTokenHash:    strings.Repeat("1", 64),
		Intent:              authcontract.PendingOAuthIntentLogin,
		Provider:            userscontract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: strings.Repeat("2", 64),
		SubjectHint:         "oidc:22222222",
		RedirectTo:          "/dashboard",
		DisplayName:         "OIDC User",
		ExpiresAt:           now.Add(time.Minute),
		CreatedAt:           now,
	})
	if err != nil {
		t.Fatalf("create pending oauth session: %v", err)
	}
	if session.ResolvedEmail != "" || session.EmailVerified {
		t.Fatalf("expected emailless pending oauth session, got %+v", session)
	}

	updated, err := store.CompletePendingOAuthEmail(ctx, strings.Repeat("1", 64), "Complete@SRapi.Local", now.Add(time.Second))
	if err != nil {
		t.Fatalf("complete pending oauth email: %v", err)
	}
	if updated.ResolvedEmail != "complete@srapi.local" || !updated.EmailVerified || !updated.UpdatedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("unexpected completed pending oauth session: %+v", updated)
	}
	if _, err := store.CompletePendingOAuthEmail(ctx, strings.Repeat("1", 64), "again@srapi.local", now.Add(2*time.Second)); err == nil {
		t.Fatal("expected repeated email completion to fail")
	}
}

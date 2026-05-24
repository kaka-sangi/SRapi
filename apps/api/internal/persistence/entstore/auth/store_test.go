package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entauthsession "github.com/srapi/srapi/apps/api/ent/authsession"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"

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

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
)

const testMasterKey = "0123456789abcdef0123456789abcdef"

type stubRefresher struct {
	credential map[string]any
	err        error
	calls      int
}

func (s *stubRefresher) RefreshAccount(_ context.Context, _ RefreshRequest) (RefreshResult, error) {
	s.calls++
	if s.err != nil {
		return RefreshResult{}, s.err
	}
	return RefreshResult{Credential: cloneRefreshMap(s.credential)}, nil
}

func cloneRefreshMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for k, v := range value {
		out[k] = v
	}
	return out
}

func newRefreshTestService(t *testing.T, now time.Time) (*Service, *accountmemory.Store) {
	t.Helper()
	store := accountmemory.New()
	svc, err := New(store, testMasterKey, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, store
}

func newOAuthAccount(t *testing.T, ctx context.Context, svc *Service, expiresAt time.Time) contract.ProviderAccount {
	t.Helper()
	credential := map[string]any{
		"access_token":  "initial-access",
		"refresh_token": "initial-refresh",
		"expires_at":    expiresAt.UTC().Format(time.RFC3339),
	}
	created, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   1,
		Name:         "oauth-account",
		RuntimeClass: contract.RuntimeClassOauthRefresh,
		Credential:   credential,
	})
	if err != nil {
		t.Fatalf("create oauth account: %v", err)
	}
	return created
}

func TestRefreshAccessTokenSuccessResetsState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	svc, _ := newRefreshTestService(t, now)
	expires := now.Add(2 * time.Minute) // inside the refresh window
	account := newOAuthAccount(t, ctx, svc, expires)

	// Seed a previous failure so we can assert the reset.
	account.RefreshAttempts = 3
	account.RefreshLastError = "previous transient failure"
	account.NeedsReauthAt = nil
	if _, err := svc.store.Update(ctx, account); err != nil {
		t.Fatalf("seed prior failure: %v", err)
	}

	newExpiry := now.Add(45 * time.Minute)
	refresher := &stubRefresher{credential: map[string]any{
		"access_token":  "fresh-access",
		"refresh_token": "fresh-refresh",
		"expires_at":    newExpiry.Format(time.RFC3339),
	}}

	updated, err := svc.RefreshAccessToken(ctx, account.ID, refresher)
	if err != nil {
		t.Fatalf("RefreshAccessToken returned err: %v", err)
	}
	if refresher.calls != 1 {
		t.Fatalf("expected refresher invoked once, got %d", refresher.calls)
	}
	if updated.RefreshAttempts != 0 {
		t.Fatalf("expected refresh_attempts reset to 0, got %d", updated.RefreshAttempts)
	}
	if updated.RefreshLastError != "" {
		t.Fatalf("expected refresh_last_error cleared, got %q", updated.RefreshLastError)
	}
	if updated.NeedsReauthAt != nil {
		t.Fatalf("expected needs_reauth_at cleared, got %v", updated.NeedsReauthAt)
	}
	if updated.LastRefreshedAt == nil || !updated.LastRefreshedAt.Equal(now) {
		t.Fatalf("expected last_refreshed_at = %v, got %v", now, updated.LastRefreshedAt)
	}
	if updated.TokenExpiresAt == nil || !updated.TokenExpiresAt.Equal(newExpiry) {
		t.Fatalf("expected token_expires_at = %v, got %v", newExpiry, updated.TokenExpiresAt)
	}

	// The persisted credential ciphertext must round-trip to the new token.
	decrypted, err := svc.DecryptCredential(ctx, updated.ID)
	if err != nil {
		t.Fatalf("decrypt credential: %v", err)
	}
	if access, _ := decrypted["access_token"].(string); access != "fresh-access" {
		t.Fatalf("expected new access_token persisted, got %q", access)
	}
}

func TestRefreshAccessTokenTransientFailureBelowThresholdDoesNotFlipReauth(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	svc, _ := newRefreshTestService(t, now)
	account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))

	refresher := &stubRefresher{err: errors.New("upstream timeout")}
	updated, err := svc.RefreshAccessToken(ctx, account.ID, refresher)
	if err == nil {
		t.Fatalf("expected refresh error to bubble up")
	}
	// updated reflects the in-memory mutation even on the error path.
	if updated.RefreshAttempts != 1 {
		t.Fatalf("expected refresh_attempts incremented to 1, got %d", updated.RefreshAttempts)
	}
	if updated.NeedsReauthAt != nil {
		t.Fatalf("expected needs_reauth_at unset for first transient failure, got %v", updated.NeedsReauthAt)
	}
	if !strings.Contains(updated.RefreshLastError, "upstream timeout") {
		t.Fatalf("expected refresh_last_error to capture message, got %q", updated.RefreshLastError)
	}

	// Round-trip the persisted state to confirm we wrote it.
	persisted, err := svc.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if persisted.RefreshAttempts != 1 || persisted.NeedsReauthAt != nil {
		t.Fatalf("persisted state mismatch: attempts=%d needs_reauth=%v", persisted.RefreshAttempts, persisted.NeedsReauthAt)
	}
}

func TestRefreshAccessTokenTransientFailureCrossingThresholdFlipsReauth(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	svc, _ := newRefreshTestService(t, now)
	account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))

	// Seed at threshold-1 so the next failure crosses.
	account.RefreshAttempts = refreshFailureThreshold - 1
	if _, err := svc.store.Update(ctx, account); err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	refresher := &stubRefresher{err: errors.New("503 Service Unavailable")}
	updated, err := svc.RefreshAccessToken(ctx, account.ID, refresher)
	if err == nil {
		t.Fatalf("expected refresh error")
	}
	if updated.RefreshAttempts != refreshFailureThreshold {
		t.Fatalf("expected refresh_attempts=%d, got %d", refreshFailureThreshold, updated.RefreshAttempts)
	}
	if updated.NeedsReauthAt == nil {
		t.Fatalf("expected needs_reauth_at set after crossing threshold, got nil")
	}
	if !updated.NeedsReauthAt.Equal(now) {
		t.Fatalf("expected needs_reauth_at=%v, got %v", now, updated.NeedsReauthAt)
	}
}

func TestRefreshAccessTokenPermanentErrorFlipsReauthImmediately(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	svc, _ := newRefreshTestService(t, now)
	account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))

	refresher := &stubRefresher{err: errors.New("invalid_grant: refresh token revoked")}
	updated, err := svc.RefreshAccessToken(ctx, account.ID, refresher)
	if err == nil {
		t.Fatalf("expected refresh error")
	}
	if updated.RefreshAttempts != 1 {
		t.Fatalf("expected attempts=1 after first permanent failure, got %d", updated.RefreshAttempts)
	}
	if updated.NeedsReauthAt == nil {
		t.Fatalf("expected needs_reauth_at set immediately on permanent error")
	}
}

func TestRefreshAccessTokenSuccessAfterFailuresClearsState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	svc, _ := newRefreshTestService(t, now)
	account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))

	// Force the account into a half-broken state (transient failures + needs_reauth set).
	account.RefreshAttempts = refreshFailureThreshold
	account.RefreshLastError = "503 upstream"
	flag := now.Add(-1 * time.Hour)
	account.NeedsReauthAt = &flag
	if _, err := svc.store.Update(ctx, account); err != nil {
		t.Fatalf("seed broken state: %v", err)
	}

	newExpiry := now.Add(30 * time.Minute)
	refresher := &stubRefresher{credential: map[string]any{
		"access_token":  "recovered-access",
		"refresh_token": "recovered-refresh",
		"expires_at":    newExpiry.Format(time.RFC3339),
	}}

	updated, err := svc.RefreshAccessToken(ctx, account.ID, refresher)
	if err != nil {
		t.Fatalf("RefreshAccessToken returned err: %v", err)
	}
	if updated.RefreshAttempts != 0 {
		t.Fatalf("expected refresh_attempts cleared, got %d", updated.RefreshAttempts)
	}
	if updated.RefreshLastError != "" {
		t.Fatalf("expected refresh_last_error cleared, got %q", updated.RefreshLastError)
	}
	if updated.NeedsReauthAt != nil {
		t.Fatalf("expected needs_reauth_at cleared after a success, got %v", updated.NeedsReauthAt)
	}
	if updated.TokenExpiresAt == nil || !updated.TokenExpiresAt.Equal(newExpiry) {
		t.Fatalf("expected token_expires_at=%v, got %v", newExpiry, updated.TokenExpiresAt)
	}
}

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	// Assert the exact timestamp matches the service clock so an audit replay
	// can correlate needs_reauth_at with the refresh attempt that flipped it,
	// not just "some time near now".
	if !updated.NeedsReauthAt.Equal(now) {
		t.Fatalf("expected needs_reauth_at=%v, got %v", now, updated.NeedsReauthAt)
	}
}

// TestIsPermanentRefreshErrorMatchesAllPatterns exercises every alternative
// in permanentRefreshErrorRegex (the original test only covered
// invalid_grant). Case-variation is folded into the table so a future
// case-sensitive refactor would surface here.
func TestIsPermanentRefreshErrorMatchesAllPatterns(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    bool
	}{
		{"invalid_grant_lower", "invalid_grant", true},
		{"invalid_grant_mixed", "OAuth Invalid_Grant: refresh expired", true},
		{"invalid_client", "invalid_client", true},
		{"unauthorized_client", "ERROR unauthorized_client", true},
		{"invalid_token", "session expired: invalid_token", true},
		{"access_denied", "access_denied by upstream", true},
		{"consent_required", "consent_required: re-grant", true},
		{"login_required", "login_required after timeout", true},
		{"transient_network", "network timeout reading body", false},
		{"transient_5xx", "503 service unavailable", false},
		{"transient_generic", "context deadline exceeded", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := isPermanentRefreshError(errors.New(tc.message))
			if got != tc.want {
				t.Fatalf("isPermanentRefreshError(%q) = %v, want %v", tc.message, got, tc.want)
			}
		})
	}
	if isPermanentRefreshError(nil) {
		t.Fatalf("isPermanentRefreshError(nil) must return false")
	}
}

// TestTruncateRefreshErrorCapsAtMaxLength guards the contract the admin row
// depends on: a verbose upstream body cannot bloat the column. 600 chars in,
// exactly maxRefreshErrorLength chars out.
func TestTruncateRefreshErrorCapsAtMaxLength(t *testing.T) {
	long := strings.Repeat("x", 600)
	got := truncateRefreshError(long)
	if len(got) != maxRefreshErrorLength {
		t.Fatalf("expected truncated length %d, got %d", maxRefreshErrorLength, len(got))
	}
	if got != strings.Repeat("x", maxRefreshErrorLength) {
		t.Fatalf("expected truncated string of %d x's, got %q", maxRefreshErrorLength, got)
	}
	short := "  oops  "
	if truncateRefreshError(short) != "oops" {
		t.Fatalf("expected trimmed short message, got %q", truncateRefreshError(short))
	}
}

// TestRefreshAccessTokenSuccessClearsMetadataHints seeds the three transient
// hint keys ShouldRefreshOAuthCredential reads and asserts they're cleared
// after a successful refresh — otherwise the gateway flips the account
// straight back into "needs refresh" on the next call.
func TestRefreshAccessTokenSuccessClearsMetadataHints(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	svc, _ := newRefreshTestService(t, now)
	account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))

	// Seed the hint keys directly on the account row so the worker would
	// otherwise see "needs to re-refresh" on the next pass.
	account.Metadata = map[string]any{
		"force_refresh":        true,
		"access_token_expired": true,
		"needs_reauth_reason":  "operator-marked",
		"keep_this":            "ok",
	}
	if _, err := svc.store.Update(ctx, account); err != nil {
		t.Fatalf("seed metadata hints: %v", err)
	}

	newExpiry := now.Add(45 * time.Minute)
	refresher := &stubRefresher{credential: map[string]any{
		"access_token":  "fresh",
		"refresh_token": "fresh",
		"expires_at":    newExpiry.Format(time.RFC3339),
	}}
	updated, err := svc.RefreshAccessToken(ctx, account.ID, refresher)
	if err != nil {
		t.Fatalf("RefreshAccessToken returned err: %v", err)
	}
	for _, key := range []string{"force_refresh", "access_token_expired", "needs_reauth_reason"} {
		if _, present := updated.Metadata[key]; present {
			t.Fatalf("expected metadata key %q cleared, still present in %+v", key, updated.Metadata)
		}
	}
	if _, ok := updated.Metadata["keep_this"]; !ok {
		t.Fatalf("expected unrelated metadata key %q preserved, got %+v", "keep_this", updated.Metadata)
	}
}

// TestRefreshAccessTokenSerializesConcurrentCallsPerAccount fires N goroutines
// at the same account concurrently and asserts the final RefreshAttempts
// count equals N — i.e. no Find/Update race lost an increment. This regresses
// the SQL-backend lost-update race the per-account mutex closes.
func TestRefreshAccessTokenSerializesConcurrentCallsPerAccount(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	svc, _ := newRefreshTestService(t, now)
	account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))

	refresher := &stubRefresher{err: errors.New("transient upstream timeout")}
	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			// Errors are expected (transient). We only care about the bookkeeping.
			_, _ = svc.RefreshAccessToken(ctx, account.ID, refresher)
		}()
	}
	wg.Wait()
	persisted, err := svc.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if persisted.RefreshAttempts != goroutines {
		t.Fatalf("expected RefreshAttempts=%d after %d concurrent transient failures, got %d", goroutines, goroutines, persisted.RefreshAttempts)
	}
	if refresher.calls != goroutines {
		t.Fatalf("expected refresher invoked %d times, got %d", goroutines, refresher.calls)
	}
	// Threshold-crossed flip happens once attempts >= refreshFailureThreshold,
	// and with 8 attempts we should have a stable needs_reauth_at equal to now.
	if persisted.NeedsReauthAt == nil || !persisted.NeedsReauthAt.Equal(now) {
		t.Fatalf("expected needs_reauth_at=%v, got %v", now, persisted.NeedsReauthAt)
	}
}

// TestRefreshAccessTokenWithOutcomeReturnsStructuredClass asserts the
// outcome carries the right Class on each branch — the audit snapshot
// downstream uses this to answer "WHY did needs_reauth fire?".
func TestRefreshAccessTokenWithOutcomeReturnsStructuredClass(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctx := context.Background()
		now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
		svc, _ := newRefreshTestService(t, now)
		account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))
		refresher := &stubRefresher{credential: map[string]any{
			"access_token":  "ok",
			"refresh_token": "ok",
			"expires_at":    now.Add(30 * time.Minute).Format(time.RFC3339),
		}}
		outcome, err := svc.RefreshAccessTokenWithOutcome(ctx, account.ID, refresher)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if outcome.Class != RefreshOutcomeSuccess {
			t.Fatalf("expected class=%s, got %s", RefreshOutcomeSuccess, outcome.Class)
		}
	})
	t.Run("permanent_error", func(t *testing.T) {
		ctx := context.Background()
		now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
		svc, _ := newRefreshTestService(t, now)
		account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))
		refresher := &stubRefresher{err: errors.New("invalid_grant: revoked")}
		outcome, err := svc.RefreshAccessTokenWithOutcome(ctx, account.ID, refresher)
		if err == nil {
			t.Fatalf("expected err")
		}
		if outcome.Class != RefreshOutcomePermanentError {
			t.Fatalf("expected class=%s, got %s", RefreshOutcomePermanentError, outcome.Class)
		}
		if !outcome.NeedsReauthFlipped {
			t.Fatalf("expected NeedsReauthFlipped=true on first permanent error")
		}
	})
	t.Run("threshold_exceeded", func(t *testing.T) {
		ctx := context.Background()
		now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
		svc, _ := newRefreshTestService(t, now)
		account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))
		account.RefreshAttempts = refreshFailureThreshold - 1
		if _, err := svc.store.Update(ctx, account); err != nil {
			t.Fatalf("seed attempts: %v", err)
		}
		refresher := &stubRefresher{err: errors.New("503 upstream")}
		outcome, err := svc.RefreshAccessTokenWithOutcome(ctx, account.ID, refresher)
		if err == nil {
			t.Fatalf("expected err")
		}
		if outcome.Class != RefreshOutcomeThresholdExceeded {
			t.Fatalf("expected class=%s, got %s", RefreshOutcomeThresholdExceeded, outcome.Class)
		}
		if !outcome.NeedsReauthFlipped {
			t.Fatalf("expected NeedsReauthFlipped=true after crossing threshold")
		}
	})
	t.Run("transient_error", func(t *testing.T) {
		ctx := context.Background()
		now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
		svc, _ := newRefreshTestService(t, now)
		account := newOAuthAccount(t, ctx, svc, now.Add(2*time.Minute))
		refresher := &stubRefresher{err: errors.New("network timeout")}
		outcome, err := svc.RefreshAccessTokenWithOutcome(ctx, account.ID, refresher)
		if err == nil {
			t.Fatalf("expected err")
		}
		if outcome.Class != RefreshOutcomeTransientError {
			t.Fatalf("expected class=%s, got %s", RefreshOutcomeTransientError, outcome.Class)
		}
		if outcome.NeedsReauthFlipped {
			t.Fatalf("expected NeedsReauthFlipped=false below threshold")
		}
	})
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

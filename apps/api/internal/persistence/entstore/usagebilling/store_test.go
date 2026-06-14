package usagebilling

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apikeystore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/apikeys"
	subscriptionstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/subscriptions"
	usagestore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/usage"

	_ "github.com/mattn/go-sqlite3"
)

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "store.db") + "?_fk=1"
}

type fixture struct {
	store      *Store
	keyStore   *apikeystore.Store
	usageStore *usagestore.Store
	userID     int
	keyID      int
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	t.Cleanup(func() { _ = client.Close() })

	subStore, err := subscriptionstore.New(client)
	if err != nil {
		t.Fatalf("new subscription store: %v", err)
	}
	keyStore, err := apikeystore.New(client)
	if err != nil {
		t.Fatalf("new api key store: %v", err)
	}
	usageStore, err := usagestore.New(client)
	if err != nil {
		t.Fatalf("new usage store: %v", err)
	}
	store, err := New(client, subStore, keyStore)
	if err != nil {
		t.Fatalf("new usagebilling store: %v", err)
	}

	ws, err := client.Workspace.Create().SetName("w").SetSlug("w").SetStatus("active").SetType("personal").Save(ctx)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	user, err := client.User.Create().SetEmail("u@srapi.local").SetName("u").SetPasswordHash("h").SetStatus("active").SetWorkspaceID(ws.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := keyStore.Create(ctx, apikeycontract.CreateStoredKey{
		UserID: user.ID,
		Name:   "k",
		Prefix: "sk_test",
		Hash:   "hmac-sha256:h",
		Status: apikeycontract.StatusActive,
		Scopes: []string{"gateway:invoke"},
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return fixture{store: store, keyStore: keyStore, usageStore: usageStore, userID: user.ID, keyID: key.ID}
}

func (f fixture) recordUsage(t *testing.T, requestID, cost string) int {
	t.Helper()
	log, err := f.usageStore.Create(context.Background(), usagecontract.UsageLog{
		RequestID:    requestID,
		AttemptNo:    1,
		UserID:       f.userID,
		APIKeyID:     f.keyID,
		Model:        "m",
		Success:      true,
		BillableCost: cost,
		// Pin created_at to a fixed past instant so the time-windowed sweep query
		// is deterministic regardless of the host timezone (sqlite, unlike
		// Postgres timestamptz, does not normalize zones).
		CreatedAt: time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("record usage: %v", err)
	}
	return log.ID
}

func (f fixture) costUsed(t *testing.T) string {
	t.Helper()
	key, err := f.keyStore.FindByID(context.Background(), f.keyID)
	if err != nil {
		t.Fatalf("find key: %v", err)
	}
	return key.CostUsed
}

// TestApplyAggregationIsIdempotent verifies the claim marker makes a second
// apply a no-op: the API-key cost usage is incremented exactly once even when
// ApplyAggregation runs twice for the same usage_log row (the no-double-count
// guarantee that lets the live path and the reconciler both call it safely).
func TestApplyAggregationIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	id := f.recordUsage(t, "req-1", "0.50000000")

	applied, err := f.store.ApplyAggregation(ctx, id)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !applied {
		t.Fatal("expected first ApplyAggregation to claim and apply")
	}
	if got := f.costUsed(t); got != "0.50000000" {
		t.Fatalf("cost_used after first apply = %s, want 0.50000000", got)
	}

	applied2, err := f.store.ApplyAggregation(ctx, id)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if applied2 {
		t.Fatal("expected second ApplyAggregation to be a no-op (already aggregated)")
	}
	if got := f.costUsed(t); got != "0.50000000" {
		t.Fatalf("cost_used after re-apply = %s, want 0.50000000 (no double count)", got)
	}
}

// TestSweepPendingAppliesDroppedRowsOnce verifies the reconciler sweep aggregates
// every unaggregated row exactly once and is a no-op on a second pass.
func TestSweepPendingAppliesDroppedRowsOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	for i := 0; i < 3; i++ {
		f.recordUsage(t, "req-sweep-"+strconv.Itoa(i), "0.10000000")
	}

	after := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	applied, err := f.store.SweepPending(ctx, after, before, 10)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if applied != 3 {
		t.Fatalf("first sweep applied %d, want 3", applied)
	}
	if got := f.costUsed(t); got != "0.30000000" {
		t.Fatalf("cost_used after sweep = %s, want 0.30000000", got)
	}

	applied2, err := f.store.SweepPending(ctx, after, before, 10)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if applied2 != 0 {
		t.Fatalf("second sweep applied %d, want 0 (all already aggregated)", applied2)
	}
	if got := f.costUsed(t); got != "0.30000000" {
		t.Fatalf("cost_used after second sweep = %s, want 0.30000000 (no double count)", got)
	}
}

// TestApplyAggregationSkipsFailedRows verifies failed requests are neither
// claimed by the sweep nor billed (the reconciler only targets success rows).
func TestApplyAggregationSkipsFailedRows(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.usageStore.Create(ctx, usagecontract.UsageLog{
		RequestID: "req-failed", AttemptNo: 1, UserID: f.userID, APIKeyID: f.keyID,
		Model: "m", Success: false, BillableCost: "0.00000000",
	}); err != nil {
		t.Fatalf("record failed usage: %v", err)
	}
	applied, err := f.store.SweepPending(ctx, time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if applied != 0 {
		t.Fatalf("sweep applied %d failed rows, want 0", applied)
	}
	if got := f.costUsed(t); got != "0.00000000" {
		t.Fatalf("cost_used = %s, want 0.00000000 (failed row not billed)", got)
	}
}

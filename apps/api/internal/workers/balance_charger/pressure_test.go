package balancecharger

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/srapi/srapi/apps/api/ent"
	entbillingledger "github.com/srapi/srapi/apps/api/ent/billingledger"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	entbilling "github.com/srapi/srapi/apps/api/internal/persistence/entstore/billing"
)

const (
	balanceChargerPressureDSNEnv = "SRAPI_BALANCE_CHARGER_PRESSURE_DSN"
	pressureUsageLogCount        = 10000
	pressureBatchSize            = 500
)

func TestBalanceChargerPostgresPressureDrainsDefaultBacklog(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(balanceChargerPressureDSNEnv))
	if dsn == "" {
		t.Skipf("set %s to run the PostgreSQL balance_charger pressure test", balanceChargerPressureDSNEnv)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	client := openPressureEntClient(t, ctx, dsn)
	user := seedPressureUsageLogs(t, ctx, client, pressureUsageLogCount)

	store, err := entbilling.New(client)
	if err != nil {
		t.Fatalf("create billing ent store: %v", err)
	}
	worker, err := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		BatchLimit:       pressureBatchSize,
		MaxBatchesPerRun: pressureUsageLogCount / pressureBatchSize,
		Clock:            fixedClock{now: time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("create balance charger worker: %v", err)
	}

	startedAt := time.Now()
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run balance charger pressure test: %v", err)
	}
	elapsed := time.Since(startedAt)

	if result.Selected != pressureUsageLogCount || result.Charged != pressureUsageLogCount {
		t.Fatalf("expected %d charged usage logs, got selected=%d charged=%d", pressureUsageLogCount, result.Selected, result.Charged)
	}
	if len(result.Batches) != pressureUsageLogCount/pressureBatchSize {
		t.Fatalf("expected %d ledger batches, got %d", pressureUsageLogCount/pressureBatchSize, len(result.Batches))
	}

	pending, err := client.UsageLog.Query().
		Where(entusagelog.SuccessEQ(true), entusagelog.ChargedAtIsNil()).
		Count(ctx)
	if err != nil {
		t.Fatalf("count pending usage logs: %v", err)
	}
	if pending != 0 {
		t.Fatalf("expected no pending usage logs, got %d", pending)
	}

	charged, err := client.UsageLog.Query().
		Where(entusagelog.ChargedAtNotNil()).
		Count(ctx)
	if err != nil {
		t.Fatalf("count charged usage logs: %v", err)
	}
	if charged != pressureUsageLogCount {
		t.Fatalf("expected %d charged usage logs, got %d", pressureUsageLogCount, charged)
	}

	ledgers, err := client.BillingLedger.Query().
		Where(entbillingledger.UserIDEQ(user.ID)).
		Count(ctx)
	if err != nil {
		t.Fatalf("count billing ledgers: %v", err)
	}
	if ledgers != pressureUsageLogCount/pressureBatchSize {
		t.Fatalf("expected %d billing ledgers, got %d", pressureUsageLogCount/pressureBatchSize, ledgers)
	}

	updatedUser, err := client.User.Query().
		Where(entuser.IDEQ(user.ID)).
		Only(ctx)
	if err != nil {
		t.Fatalf("load charged user: %v", err)
	}
	if updatedUser.Balance != "99.00000000" || updatedUser.Currency != "USD" {
		t.Fatalf("unexpected charged balance: %s %s", updatedUser.Balance, updatedUser.Currency)
	}

	t.Logf("charged %d usage logs through PostgreSQL in %s", result.Charged, elapsed)
}

func openPressureEntClient(t *testing.T, ctx context.Context, dsn string) *ent.Client {
	t.Helper()

	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open pressure admin database: %v", err)
	}
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatalf("ping pressure admin database: %v", err)
	}

	schemaName := "srapi_balance_pressure_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+quoteIdentifier(schemaName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create pressure schema: %v", err)
	}

	testDB, err := sql.Open("pgx", dsn)
	if err != nil {
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoteIdentifier(schemaName)+" CASCADE")
		_ = adminDB.Close()
		t.Fatalf("open pressure test database: %v", err)
	}
	testDB.SetMaxOpenConns(1)
	testDB.SetMaxIdleConns(1)
	if _, err := testDB.ExecContext(ctx, "SET search_path TO "+quoteIdentifier(schemaName)); err != nil {
		_ = testDB.Close()
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoteIdentifier(schemaName)+" CASCADE")
		_ = adminDB.Close()
		t.Fatalf("set pressure schema search path: %v", err)
	}

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, testDB)))
	if err := client.Schema.Create(ctx); err != nil {
		_ = client.Close()
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoteIdentifier(schemaName)+" CASCADE")
		_ = adminDB.Close()
		t.Fatalf("create pressure schema tables: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoteIdentifier(schemaName)+" CASCADE")
		_ = adminDB.Close()
	})
	return client
}

func seedPressureUsageLogs(t *testing.T, ctx context.Context, client *ent.Client, count int) *ent.User {
	t.Helper()

	now := time.Date(2026, 5, 26, 11, 0, 0, 0, time.UTC)
	user, err := client.User.Create().
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetEmail("balance-pressure@srapi.local").
		SetName("Balance Pressure").
		SetPasswordHash("pressure-test-password-hash").
		SetStatus("active").
		SetBalance("100.00000000").
		SetCurrency("USD").
		Save(ctx)
	if err != nil {
		t.Fatalf("create pressure user: %v", err)
	}

	for start := 0; start < count; start += pressureBatchSize {
		end := start + pressureBatchSize
		if end > count {
			end = count
		}
		builders := make([]*ent.UsageLogCreate, 0, end-start)
		for index := start; index < end; index++ {
			createdAt := now.Add(time.Duration(index) * time.Millisecond)
			builders = append(builders, client.UsageLog.Create().
				SetCreatedAt(createdAt).
				SetUpdatedAt(createdAt).
				SetRequestID(fmt.Sprintf("req_balance_pressure_%05d", index+1)).
				SetAttemptNo(1).
				SetUserID(user.ID).
				SetAPIKeyID(1).
				SetSuccess(true).
				SetCost("0.00010000").
				SetCurrency("USD"))
		}
		if _, err := client.UsageLog.CreateBulk(builders...).Save(ctx); err != nil {
			t.Fatalf("seed pressure usage logs %d-%d: %v", start+1, end, err)
		}
	}
	return user
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

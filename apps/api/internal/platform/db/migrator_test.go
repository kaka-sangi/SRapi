package db

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/srapi/srapi/apps/api/migrations"
)

// migratorPostgresDSNEnv lets a real PostgreSQL instance opt into the
// full ApplyMigrations integration test. The advisory lock and
// schema_migrations bookkeeping are PostgreSQL-specific and cannot run against
// the in-memory SQLite used elsewhere in the unit suite.
const migratorPostgresDSNEnv = "SRAPI_MIGRATOR_TEST_DSN"

func TestLoadMigrationsOrdersByNumberAndValidatesNames(t *testing.T) {
	fsys := fstest.MapFS{
		"000003_third.sql":  {Data: []byte("SELECT 3;")},
		"000001_first.sql":  {Data: []byte("SELECT 1;")},
		"000002_second.sql": {Data: []byte("SELECT 2;")},
		"README.md":         {Data: []byte("not a migration")},
	}

	got, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	want := []string{"000001_first", "000002_second", "000003_third"}
	if len(got) != len(want) {
		t.Fatalf("expected %d migrations, got %d", len(want), len(got))
	}
	for i, m := range got {
		if m.version != want[i] {
			t.Fatalf("migration %d: want version %q, got %q", i, want[i], m.version)
		}
	}
}

func TestLoadMigrationsRejectsBadFilename(t *testing.T) {
	fsys := fstest.MapFS{
		"1_bad.sql": {Data: []byte("SELECT 1;")},
	}
	if _, err := loadMigrations(fsys); err == nil {
		t.Fatal("expected error for malformed migration filename, got nil")
	}
}

func TestLoadMigrationsRejectsDuplicateNumber(t *testing.T) {
	fsys := fstest.MapFS{
		"000001_first.sql": {Data: []byte("SELECT 1;")},
		"000001_other.sql": {Data: []byte("SELECT 2;")},
	}
	if _, err := loadMigrations(fsys); err == nil {
		t.Fatal("expected error for duplicate migration number, got nil")
	}
}

// TestEmbeddedMigrationsLoad guards that the embedded migration filesystem is
// wired correctly and that every shipped file passes the same naming rules the
// applier enforces. This catches embed-path regressions without needing a DB.
func TestEmbeddedMigrationsLoad(t *testing.T) {
	got, err := loadMigrations(migrations.UpFS())
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected embedded migrations, got none")
	}
	if got[0].number != 1 {
		t.Fatalf("expected first embedded migration to be 000001, got %06d", got[0].number)
	}
	for i := 1; i < len(got); i++ {
		if got[i].number <= got[i-1].number {
			t.Fatalf("embedded migrations not strictly ascending at %d: %06d then %06d", i, got[i-1].number, got[i].number)
		}
	}
	if !strings.Contains(got[0].sql, "CREATE TABLE") {
		t.Fatalf("expected initial migration to contain CREATE TABLE statements")
	}
}

func TestParseMigrationNumber(t *testing.T) {
	cases := []struct {
		name   string
		number int
		ok     bool
	}{
		{"000001_initial_schema.sql", 1, true},
		{"000027_model_group_tpm.sql", 27, true},
		{"00001_short.sql", 0, false},
		{"000000_zero.sql", 0, false},
		{"000001.sql", 0, false},
		{"000001_BadCase.sql", 0, false},
		{"abcdef_subject.sql", 0, false},
	}
	for _, tc := range cases {
		number, ok := parseMigrationNumber(tc.name)
		if ok != tc.ok || (ok && number != tc.number) {
			t.Fatalf("parseMigrationNumber(%q) = (%d, %v), want (%d, %v)", tc.name, number, ok, tc.number, tc.ok)
		}
	}
}

// TestApplyMigrationsIdempotentOnPostgres exercises the full applier (advisory
// lock, schema_migrations bookkeeping, per-migration transactions, idempotent
// re-run) against a real PostgreSQL instance. It is skipped unless a DSN is
// provided because the unit suite otherwise runs on SQLite, which does not
// support PostgreSQL advisory locks.
func TestApplyMigrationsIdempotentOnPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(migratorPostgresDSNEnv))
	if dsn == "" {
		t.Skipf("set %s to run the PostgreSQL migrator integration test", migratorPostgresDSNEnv)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	// Build a db.Client backed by the DSN so ApplyMigrations gets a real
	// *sql.DB to take the advisory lock on.
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open pgx database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	client := &Client{db: sqlDB}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := client.ApplyMigrations(ctx, migrations.UpFS(), logger); err != nil {
		t.Fatalf("first ApplyMigrations: %v", err)
	}

	// Second run must be a no-op (idempotent) and must not error.
	if err := client.ApplyMigrations(ctx, migrations.UpFS(), logger); err != nil {
		t.Fatalf("second (idempotent) ApplyMigrations: %v", err)
	}

	// Every shipped migration must be recorded.
	loaded, err := loadMigrations(migrations.UpFS())
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	var count int
	if err := client.db.QueryRowContext(ctx, "SELECT count(*) FROM "+schemaMigrationsTable).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != len(loaded) {
		t.Fatalf("expected %d recorded migrations, got %d", len(loaded), count)
	}
}

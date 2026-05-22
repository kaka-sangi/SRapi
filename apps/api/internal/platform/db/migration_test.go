package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entschema "entgo.io/ent/dialect/sql/schema"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/ent/migrate"

	_ "github.com/mattn/go-sqlite3"
)

func TestEntSchemaAppliesToEmptyDatabase(t *testing.T) {
	const dsn = "file:srapi_ent?mode=memory&cache=shared&_fk=1"

	client := enttest.Open(t, dialect.SQLite, dsn)
	defer client.Close()

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	sort.Strings(got)

	want := []string{
		"account_group_members",
		"account_groups",
		"account_health_snapshots",
		"account_quota_snapshots",
		"affiliate_ledgers",
		"affiliate_rules",
		"api_key_groups",
		"api_keys",
		"audit_logs",
		"billing_ledgers",
		"capability_definitions",
		"domain_events_inboxes",
		"domain_events_outboxes",
		"idempotency_records",
		"invite_codes",
		"invite_relationships",
		"model_alias",
		"model_provider_mappings",
		"model_registries",
		"obs_alert_events",
		"obs_slo_definitions",
		"payment_audit_logs",
		"payment_orders",
		"payment_provider_instances",
		"pricing_rules",
		"provider_accounts",
		"providers",
		"roles",
		"scheduler_decisions",
		"scheduler_feedbacks",
		"scheduler_strategies",
		"settings",
		"subscription_plans",
		"usage_logs",
		"user_roles",
		"user_subscriptions",
		"users",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected tables:\nwant: %v\ngot:  %v", want, got)
	}
}

func TestPostgresInitialMigrationMatchesEntSchema(t *testing.T) {
	got, err := os.ReadFile(filepath.Clean("../../../migrations/postgres/up/000001_initial_schema.sql"))
	if err != nil {
		t.Fatalf("read postgres initial migration: %v", err)
	}
	want, err := postgresInitialDDL(t.Context())
	if err != nil {
		t.Fatalf("generate postgres ddl from Ent schema: %v", err)
	}
	if normalizeSQL(string(got)) != want {
		t.Fatal("postgres initial migration drifted from Ent schema; regenerate apps/api/migrations/postgres/up/000001_initial_schema.sql")
	}
}

func TestPostgresInitialDownMigrationCoversAllTables(t *testing.T) {
	got, err := os.ReadFile(filepath.Clean("../../../migrations/postgres/down/000001_initial_schema.sql"))
	if err != nil {
		t.Fatalf("read postgres initial down migration: %v", err)
	}
	var want strings.Builder
	want.WriteString("-- Drop initial SRapi MVP schema.\n")
	for i := len(migrate.Tables) - 1; i >= 0; i-- {
		fmt.Fprintf(&want, "DROP TABLE IF EXISTS %q;\n", migrate.Tables[i].Name)
	}
	if normalizeSQL(string(got)) != normalizeSQL(want.String()) {
		t.Fatal("postgres initial down migration does not match Ent table list")
	}
}

func postgresInitialDDL(ctx context.Context) (string, error) {
	ddl, err := entschema.DDL(ctx, entschema.DDLArgs{
		Dialect: dialect.Postgres,
		Version: "16",
		Tables:  migrate.Tables,
	})
	if err != nil {
		return "", err
	}
	return normalizeSQL(ddl), nil
}

func normalizeSQL(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n") + "\n"
}

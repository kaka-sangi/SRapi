package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

func TestDSNBuildsPostgresURL(t *testing.T) {
	cfg := config.DependencyConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "srapi",
		Password: "secret",
		Database: "srapi",
		SSLMode:  "disable",
	}

	dsn := DSN(cfg)
	if !strings.HasPrefix(dsn, "postgres://srapi:secret@localhost:5432/srapi?") {
		t.Fatalf("unexpected dsn: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("expected sslmode in dsn: %s", dsn)
	}
}

func TestDropObsoleteSchemaArtifactsAllowsUsageLogAttempts(t *testing.T) {
	sqlDB, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/schema-repair.db?_fk=1")
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	for _, statement := range []string{
		`CREATE TABLE usage_logs (id integer primary key, request_id text not null, attempt_no integer not null default 1)`,
		`CREATE UNIQUE INDEX "usagelog_request_id" ON "usage_logs" ("request_id")`,
		`CREATE UNIQUE INDEX "usagelog_request_id_attempt_no" ON "usage_logs" ("request_id", "attempt_no")`,
		`INSERT INTO usage_logs (request_id, attempt_no) VALUES ('req_fallback', 1)`,
	} {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("exec setup statement %q: %v", statement, err)
		}
	}

	client := &Client{db: sqlDB}
	if err := client.dropObsoleteSchemaArtifacts(context.Background()); err != nil {
		t.Fatalf("drop obsolete schema artifacts: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO usage_logs (request_id, attempt_no) VALUES ('req_fallback', 2)`); err != nil {
		t.Fatalf("insert second fallback attempt: %v", err)
	}
}

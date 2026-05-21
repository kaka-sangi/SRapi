package db

import (
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
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

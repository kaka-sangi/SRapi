package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

// migrationAdvisoryLockkey is a fixed PostgreSQL advisory-lock key used to
// serialize migration application across concurrently starting app instances.
// The value is the ASCII bytes "SRAPDIGR" interpreted as a big-endian int64
// (0x5352415044494752); it is arbitrary but stable so every instance contends
// on the same lock. Keep it constant: changing it would defeat the cross-process
// mutual exclusion on a rolling deploy that mixes old and new binaries.
const migrationAdvisoryLockKey int64 = 0x5352415044494752

// schemaMigrationsTable tracks which versioned migrations have been applied.
const schemaMigrationsTable = "schema_migrations"

// migrationFile is one versioned migration discovered on the embedded filesystem.
type migrationFile struct {
	version string // the full filename without extension, e.g. "000001_initial_schema"
	number  int    // the numeric prefix, used for ordering and validation
	sql     string // the file contents
}

// ApplyMigrations applies every not-yet-applied versioned migration found in
// fsys (whose root must contain the NNNNNN_subject.sql files) in ascending
// order. It is safe to call from multiple instances concurrently: the whole
// apply is guarded by a PostgreSQL advisory lock. Each migration runs in its
// own transaction together with the row that records it, so a failure never
// marks a migration applied. The call is idempotent — a second invocation
// against an up-to-date database applies nothing. Any error is returned so the
// caller can refuse to boot against a half-migrated schema.
func (c *Client) ApplyMigrations(ctx context.Context, fsys fs.FS, logger *slog.Logger) error {
	if c == nil || c.db == nil {
		return errors.New("apply migrations: database client is not initialized")
	}
	if logger == nil {
		logger = slog.Default()
	}

	migrations, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if len(migrations) == 0 {
		return errors.New("apply migrations: no versioned migrations found")
	}

	// Hold a single connection for the lifetime of the apply: advisory locks are
	// session-scoped, so the lock and unlock must run on the same connection.
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		// Best-effort unlock; closing the connection also releases the lock.
		if _, unlockErr := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationAdvisoryLockKey); unlockErr != nil {
			logger.Warn("release migration advisory lock", "error", unlockErr)
		}
	}()

	if err := ensureSchemaMigrationsTable(ctx, conn); err != nil {
		return fmt.Errorf("ensure %s table: %w", schemaMigrationsTable, err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}

	var (
		appliedCount int
		skippedCount int
	)
	for _, migration := range migrations {
		if _, ok := applied[migration.version]; ok {
			skippedCount++
			continue
		}
		if err := applyOne(ctx, conn, migration); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.version, err)
		}
		logger.Info("applied database migration", "version", migration.version)
		appliedCount++
	}

	logger.Info("database migrations up to date",
		"applied", appliedCount,
		"skipped", skippedCount,
		"total", len(migrations),
	)
	return nil
}

// applyOne runs a single migration and records it in the same transaction so a
// partial failure never marks the version applied.
func applyOne(ctx context.Context, conn *sql.Conn, migration migrationFile) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO `+schemaMigrationsTable+` (version, applied_at) VALUES ($1, now())`,
		migration.version,
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func ensureSchemaMigrationsTable(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+schemaMigrationsTable+` (
		version text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(),
		PRIMARY KEY (version)
	)`)
	return err
}

func appliedVersions(ctx context.Context, conn *sql.Conn) (map[string]struct{}, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM `+schemaMigrationsTable)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	applied := map[string]struct{}{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

// loadMigrations reads and validates every *.sql file at the root of fsys and
// returns them sorted ascending by their numeric prefix.
func loadMigrations(fsys fs.FS) ([]migrationFile, error) {
	if fsys == nil {
		return nil, errors.New("nil migration filesystem")
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	var out []migrationFile
	seen := map[int]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		number, ok := parseMigrationNumber(name)
		if !ok {
			return nil, fmt.Errorf("migration %q must be named NNNNNN_subject.sql", name)
		}
		if existing, dup := seen[number]; dup {
			return nil, fmt.Errorf("migration number %06d is used by both %s and %s", number, existing, name)
		}
		seen[number] = name

		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		out = append(out, migrationFile{
			version: strings.TrimSuffix(name, ".sql"),
			number:  number,
			sql:     string(raw),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].number < out[j].number })
	return out, nil
}

// parseMigrationNumber validates the NNNNNN_subject.sql convention used across
// the migrations directory (mirrors the rules enforced by migration-check) and
// returns the numeric prefix.
func parseMigrationNumber(name string) (int, bool) {
	prefix, subject, ok := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
	if !ok || len(prefix) != 6 || subject == "" {
		return 0, false
	}
	number, err := strconv.Atoi(prefix)
	if err != nil || number <= 0 {
		return 0, false
	}
	for _, r := range subject {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return 0, false
	}
	return number, true
}

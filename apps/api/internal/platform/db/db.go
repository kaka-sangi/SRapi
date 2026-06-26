package db

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/internal/config"
)

type Client struct {
	db  *sql.DB
	ent *ent.Client
}

func Open(cfg config.DependencyConfig) (*Client, error) {
	dsn := DSN(cfg)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	// Bound the pool so a busy/replicated deployment can't exhaust Postgres
	// max_connections (database/sql defaults to unlimited open connections).
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetimeSeconds > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSeconds) * time.Second)
	}
	if cfg.ConnMaxIdleTimeSeconds > 0 {
		db.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTimeSeconds) * time.Second)
	}
	return &Client{
		db:  db,
		ent: ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db))),
	}, nil
}

func DSN(cfg config.DependencyConfig) string {
	u := url.URL{
		Scheme: "postgres",
		Host:   cfg.Address(),
		Path:   strings.TrimPrefix(cfg.Database, "/"),
	}
	if strings.TrimSpace(cfg.User) != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}
	query := u.Query()
	if strings.TrimSpace(cfg.SSLMode) != "" {
		query.Set("sslmode", cfg.SSLMode)
	} else {
		query.Set("sslmode", "disable")
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.db == nil {
		return nil
	}
	var ok int
	var readOnly string
	if err := c.db.QueryRowContext(ctx, "SELECT 1, current_setting('transaction_read_only')").Scan(&ok, &readOnly); err != nil {
		return err
	}
	if strings.EqualFold(readOnly, "on") {
		return errors.New("database is read-only")
	}
	return nil
}

func (c *Client) Ent() *ent.Client {
	if c == nil {
		return nil
	}
	return c.ent
}

// SQLDB exposes the shared database handle for platform-level infrastructure
// that needs database/session primitives without going through Ent.
func (c *Client) SQLDB() *sql.DB {
	if c == nil {
		return nil
	}
	return c.db
}

func (c *Client) CreateSchema(ctx context.Context) error {
	if c == nil || c.ent == nil {
		return nil
	}
	if err := c.ent.Schema.Create(ctx); err != nil {
		return err
	}
	return c.dropObsoleteSchemaArtifacts(ctx)
}

func (c *Client) dropObsoleteSchemaArtifacts(ctx context.Context) error {
	if c == nil || c.db == nil {
		return nil
	}
	_, err := c.db.ExecContext(ctx, `DROP INDEX IF EXISTS "usagelog_request_id"`)
	return err
}

// Stats returns the underlying connection pool statistics for monitoring.
// Callers can check InUse vs MaxOpenConnections to detect saturation.
func (c *Client) Stats() sql.DBStats {
	if c == nil || c.db == nil {
		return sql.DBStats{}
	}
	return c.db.Stats()
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.ent != nil {
		return c.ent.Close()
	}
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

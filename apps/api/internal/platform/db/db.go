package db

import (
	"context"
	"database/sql"
	"net/url"
	"strings"

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
	return c.db.PingContext(ctx)
}

func (c *Client) Ent() *ent.Client {
	if c == nil {
		return nil
	}
	return c.ent
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

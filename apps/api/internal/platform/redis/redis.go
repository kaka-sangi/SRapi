package redis

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/srapi/srapi/apps/api/internal/config"
)

type Client struct {
	client *redis.Client
	addr   string
	db     int
}

func Open(cfg config.DependencyConfig) (*Client, error) {
	db := 0
	if value := strings.TrimSpace(cfg.Database); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			db = parsed
		}
	}
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Address(),
		Password:     cfg.Password,
		DB:           db,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  time.Duration(cfg.DialTimeoutSeconds) * time.Second,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		PoolTimeout:  time.Duration(cfg.PoolTimeoutSeconds) * time.Second,
	})
	return &Client{client: client, addr: cfg.Address(), db: db}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Ping(ctx).Err()
}

func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *Client) Address() string {
	if c == nil {
		return ""
	}
	return c.addr
}

func (c *Client) Database() int {
	if c == nil {
		return 0
	}
	return c.db
}

func (c *Client) Raw() *redis.Client {
	if c == nil {
		return nil
	}
	return c.client
}

func (c *Client) Options() *redis.Options {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Options()
}

package leadergate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
)

const lockNamespace uint64 = 0x53524150494c4452

// Gate coordinates worker execution across replicated API processes.
type Gate struct {
	db *sql.DB
}

// New returns a PostgreSQL advisory-lock-backed gate.
func New(db *sql.DB) (*Gate, error) {
	if db == nil {
		return nil, errors.New("leader gate requires database handle")
	}
	return &Gate{db: db}, nil
}

// Do runs fn only when this process acquires the worker's advisory lock.
// It returns ran=false when another process currently owns the worker.
func (g *Gate) Do(ctx context.Context, name string, fn func(context.Context) error) (bool, error) {
	if g == nil || g.db == nil {
		return false, errors.New("leader gate is not initialized")
	}
	if fn == nil {
		return false, errors.New("leader gate function is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errors.New("leader gate worker name is empty")
	}
	conn, err := g.db.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire leader gate connection: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	lockKey := LockKey(name)
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired); err != nil {
		return false, fmt.Errorf("try acquire leader gate %q: %w", name, err)
	}
	if !acquired {
		return false, nil
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	if err := fn(ctx); err != nil {
		return true, err
	}
	return true, nil
}

// LockKey maps a worker name to a stable signed PostgreSQL advisory lock key.
func LockKey(name string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(name)))
	return int64(hasher.Sum64() ^ lockNamespace)
}

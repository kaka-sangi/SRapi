package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/srapi/srapi/apps/api/internal/platform/leadergate"
)

type workerLeaderGuard struct {
	gate *leadergate.Gate
}

func newWorkerLeaderGuard(dbClient dependencySQLDB) (*workerLeaderGuard, error) {
	if dbClient == nil || dbClient.SQLDB() == nil {
		return nil, nil
	}
	gate, err := leadergate.New(dbClient.SQLDB())
	if err != nil {
		return nil, fmt.Errorf("create worker leader gate: %w", err)
	}
	return &workerLeaderGuard{gate: gate}, nil
}

func (g *workerLeaderGuard) Run(ctx context.Context, workerName string, fn func(context.Context) error) (bool, error) {
	if g == nil || g.gate == nil {
		if err := fn(ctx); err != nil {
			return true, err
		}
		return true, nil
	}
	return g.gate.Do(ctx, workerName, fn)
}

func optionalWorkerGuard(guards ...*workerLeaderGuard) *workerLeaderGuard {
	if len(guards) == 0 {
		return nil
	}
	return guards[0]
}

type dependencySQLDB interface {
	SQLDB() *sql.DB
}

package runonceguard

import "context"

// Guard decides whether a worker may run one pass in this process.
type Guard interface {
	Run(ctx context.Context, workerName string, fn func(context.Context) error) (bool, error)
}

// Run executes fn through guard. With no guard, it runs locally.
func Run(ctx context.Context, guard Guard, workerName string, fn func(context.Context) error) (bool, error) {
	if guard == nil {
		if err := fn(ctx); err != nil {
			return true, err
		}
		return true, nil
	}
	return guard.Run(ctx, workerName, fn)
}

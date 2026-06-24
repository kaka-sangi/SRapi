package service

import (
	"context"

	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
)

// SeedDefaultRules delegates to contract.SeedDefaultRules.
func SeedDefaultRules(ctx context.Context, store contract.Store) (int, error) {
	return contract.SeedDefaultRules(ctx, store)
}

package service

import (
	"context"

	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
)

// DefaultRules returns the recommended preset rules for a fresh deployment.
// Each rule is created only if the store is empty (first boot); subsequent
// starts leave operator customizations untouched.
var DefaultRules = []contract.CreateRule{
	{
		Name:        "Expose rate-limit details",
		Enabled:     true,
		Priority:    10,
		Action:      contract.ActionExpose,
		StatusCodes: []int{429},
		Classes:     []string{"rate_limit"},
	},
	{
		Name:        "Expose quota exhausted",
		Enabled:     true,
		Priority:    20,
		Action:      contract.ActionExpose,
		StatusCodes: []int{429},
		Classes:     []string{"quota_exhausted"},
	},
	{
		Name:        "Expose authentication errors",
		Enabled:     true,
		Priority:    30,
		Action:      contract.ActionExpose,
		StatusCodes: []int{401, 403},
		Classes:     []string{"auth_failed", "auth_error", "permission_denied"},
	},
	{
		Name:        "Expose model not found",
		Enabled:     true,
		Priority:    40,
		Action:      contract.ActionExpose,
		StatusCodes: []int{404},
		Classes:     []string{"model_not_found", "not_found"},
	},
	{
		Name:        "Expose invalid request (4xx)",
		Enabled:     true,
		Priority:    50,
		Action:      contract.ActionExpose,
		StatusCodes: []int{400, 422},
		Classes:     []string{"invalid_request", "invalid_request_error"},
	},
	{
		Name:           "Mask server errors",
		Enabled:        true,
		Priority:       100,
		Action:         contract.ActionMask,
		StatusCodes:    []int{500, 502, 503, 504},
		ResponseStatus: intPtr(502),
		CustomMessage:  "The upstream provider encountered an internal error. Please retry.",
	},
	{
		Name:        "Expose content policy violations",
		Enabled:     true,
		Priority:    45,
		Action:      contract.ActionExpose,
		StatusCodes: []int{400},
		Keywords:    []string{"content_policy", "safety", "moderation", "flagged"},
	},
	{
		Name:        "Expose overloaded (retry later)",
		Enabled:     true,
		Priority:    55,
		Action:      contract.ActionExpose,
		StatusCodes: []int{529},
		Classes:     []string{"overloaded"},
	},
}

// SeedDefaultRules inserts the preset rules into an empty store. Returns the
// number of rules created (0 if the store already has rules).
func SeedDefaultRules(ctx context.Context, store contract.Store) (int, error) {
	existing, err := store.ListRules(ctx)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		return 0, nil
	}
	for _, rule := range DefaultRules {
		if _, err := store.CreateRule(ctx, rule); err != nil {
			return 0, err
		}
	}
	return len(DefaultRules), nil
}

func intPtr(v int) *int { return &v }

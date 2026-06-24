package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a rule does not exist.
var ErrNotFound = errors.New("error passthrough rule not found")

// Action decides what happens to a matched upstream error message.
type Action string

const (
	// ActionExpose forwards the raw upstream provider error message to the caller.
	ActionExpose Action = "expose"
	// ActionMask replaces the upstream message with a generic gateway message.
	ActionMask Action = "mask"
)

// Rule is one global error-passthrough rule. Empty match lists mean "match any".
type Rule struct {
	ID             int
	Name           string
	Enabled        bool
	Priority       int
	Action         Action
	StatusCodes    []int
	Classes        []string
	Keywords       []string
	ResponseStatus *int
	CustomMessage  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateRule struct {
	Name           string
	Enabled        bool
	Priority       int
	Action         Action
	StatusCodes    []int
	Classes        []string
	Keywords       []string
	ResponseStatus *int
	CustomMessage  string
}

type UpdateRule struct {
	Name           *string
	Enabled        *bool
	Priority       *int
	Action         *Action
	StatusCodes    *[]int
	Classes        *[]string
	Keywords       *[]string
	ResponseStatus **int
	CustomMessage  *string
}

// Resolution is the gateway-facing decision produced by the first matched rule.
type Resolution struct {
	Action         Action
	ResponseStatus *int
	CustomMessage  string
}

// Store persists global error-passthrough rules.
type Store interface {
	CreateRule(ctx context.Context, input CreateRule) (Rule, error)
	UpdateRule(ctx context.Context, id int, input UpdateRule) (Rule, error)
	DeleteRule(ctx context.Context, id int) error
	ListRules(ctx context.Context) ([]Rule, error)
}

// DefaultRules are the recommended preset rules for a fresh deployment.
var DefaultRules = []CreateRule{
	{Name: "Expose rate-limit details", Enabled: true, Priority: 10, Action: ActionExpose, StatusCodes: []int{429}, Classes: []string{"rate_limit"}},
	{Name: "Expose quota exhausted", Enabled: true, Priority: 20, Action: ActionExpose, StatusCodes: []int{429}, Classes: []string{"quota_exhausted"}},
	{Name: "Expose authentication errors", Enabled: true, Priority: 30, Action: ActionExpose, StatusCodes: []int{401, 403}, Classes: []string{"auth_failed", "auth_error", "permission_denied"}},
	{Name: "Expose model not found", Enabled: true, Priority: 40, Action: ActionExpose, StatusCodes: []int{404}, Classes: []string{"model_not_found", "not_found"}},
	{Name: "Expose invalid request (4xx)", Enabled: true, Priority: 50, Action: ActionExpose, StatusCodes: []int{400, 422}, Classes: []string{"invalid_request", "invalid_request_error"}},
	{Name: "Expose content policy violations", Enabled: true, Priority: 45, Action: ActionExpose, StatusCodes: []int{400}, Keywords: []string{"content_policy", "safety", "moderation", "flagged"}},
	{Name: "Expose overloaded (retry later)", Enabled: true, Priority: 55, Action: ActionExpose, StatusCodes: []int{529}, Classes: []string{"overloaded"}},
	{Name: "Mask server errors", Enabled: true, Priority: 100, Action: ActionMask, StatusCodes: []int{500, 502, 503, 504}, ResponseStatus: intPtr(502), CustomMessage: "The upstream provider encountered an internal error. Please retry."},
}

// SeedDefaultRules inserts the preset rules into an empty store. Returns the
// number of rules created (0 if the store already has rules).
func SeedDefaultRules(ctx context.Context, store Store) (int, error) {
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

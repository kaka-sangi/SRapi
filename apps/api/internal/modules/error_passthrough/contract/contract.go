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

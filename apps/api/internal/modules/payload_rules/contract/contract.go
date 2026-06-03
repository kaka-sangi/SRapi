package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a payload rule does not exist.
var ErrNotFound = errors.New("payload rule not found")

// Action decides how a matched rule mutates the upstream request body.
type Action string

const (
	// ActionDefault sets each param path only when it is absent.
	ActionDefault Action = "default"
	// ActionOverride always sets each param path, overwriting any value.
	ActionOverride Action = "override"
	// ActionFilter removes each param path from the body.
	ActionFilter Action = "filter"
)

// Rule is one operator-configured payload-transform rule. It matches by model
// glob (e.g. "gpt-*", "*") and upstream protocol ("" = any), then applies its
// params. For default/override, Params maps a dotted JSON path -> value; for
// filter, the Params keys are the dotted paths to remove (values ignored).
type Rule struct {
	ID            int
	Name          string
	Enabled       bool
	Priority      int
	Action        Action
	MatchModel    string
	MatchProtocol string
	Params        map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateRule struct {
	Name          string
	Enabled       bool
	Priority      int
	Action        Action
	MatchModel    string
	MatchProtocol string
	Params        map[string]any
}

type UpdateRule struct {
	Name          *string
	Enabled       *bool
	Priority      *int
	Action        *Action
	MatchModel    *string
	MatchProtocol *string
	Params        *map[string]any
}

// ResolvedTransform is one flattened (action, path, value) op a matching rule
// contributes; the runtime maps these onto the provider-adapter transform type.
type ResolvedTransform struct {
	Action string
	Path   string
	Value  any
}

// Store persists operator payload-transform rules.
type Store interface {
	CreateRule(ctx context.Context, input CreateRule) (Rule, error)
	UpdateRule(ctx context.Context, id int, input UpdateRule) (Rule, error)
	DeleteRule(ctx context.Context, id int) error
	ListRules(ctx context.Context) ([]Rule, error)
}

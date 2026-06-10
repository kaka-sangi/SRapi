package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a scheduled test plan does not exist.
var ErrNotFound = errors.New("scheduled test plan not found")

// ScopeType selects which provider accounts a plan probes.
type ScopeType string

const (
	// ScopeAll probes every active account.
	ScopeAll ScopeType = "all"
	// ScopeAccount probes a single account by id.
	ScopeAccount ScopeType = "account"
	// ScopeGroup probes every active account in an account group.
	ScopeGroup ScopeType = "group"
)

// Run status values.
const (
	RunStatusOK      = "ok"
	RunStatusWarning = "warning"
	RunStatusPartial = "partial"
	RunStatusFailed  = "failed"
)

// Run trigger values.
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

// Plan is one scheduled connectivity-test plan.
type Plan struct {
	ID              int
	Name            string
	Enabled         bool
	ScopeType       ScopeType
	ScopeID         *int
	IntervalSeconds int
	CronExpression  string
	ProbeModel      string
	MaxResults      int
	AutoRecover     bool
	LastRunAt       *time.Time
	LastStatus      string
	LastSummary     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreatePlan is the create payload.
type CreatePlan struct {
	Name            string
	Enabled         bool
	ScopeType       ScopeType
	ScopeID         *int
	IntervalSeconds int
	CronExpression  string
	ProbeModel      string
	MaxResults      int
	AutoRecover     bool
}

// UpdatePlan is the partial-update payload.
type UpdatePlan struct {
	Name            *string
	Enabled         *bool
	ScopeType       *ScopeType
	ScopeID         *int
	ClearScopeID    bool
	IntervalSeconds *int
	CronExpression  *string
	ProbeModel      *string
	MaxResults      *int
	AutoRecover     *bool
}

// RunOutcome is recorded by the scheduler/run-now path after a plan executes.
type RunOutcome struct {
	Trigger    string
	Status     string
	Selected   int
	Probed     int
	Skipped    int
	Failed     int
	Unhealthy  int
	Recovered  int
	Summary    string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Run is one recorded execution of a plan.
type Run struct {
	ID         int
	PlanID     int
	Trigger    string
	Status     string
	Selected   int
	Probed     int
	Skipped    int
	Failed     int
	Unhealthy  int
	Recovered  int
	Summary    string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Store persists scheduled test plans and their run history.
type Store interface {
	CreatePlan(ctx context.Context, input CreatePlan) (Plan, error)
	UpdatePlan(ctx context.Context, id int, input UpdatePlan) (Plan, error)
	DeletePlan(ctx context.Context, id int) error
	FindPlanByID(ctx context.Context, id int) (Plan, error)
	ListPlans(ctx context.Context) ([]Plan, error)
	// MarkPlanRun updates last_run_at/last_status/last_summary after a run.
	MarkPlanRun(ctx context.Context, id int, ranAt time.Time, status string, summary string) (Plan, error)
	RecordRun(ctx context.Context, planID int, outcome RunOutcome) (Run, error)
	ListRunsByPlan(ctx context.Context, planID int, limit int) ([]Run, error)
}

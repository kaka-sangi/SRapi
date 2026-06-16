package contract

import (
	"context"
	"errors"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

const (
	// TriggerManual identifies an operator-requested monitor run.
	TriggerManual = "manual"
	// TriggerScheduled identifies a worker-scheduled monitor run.
	TriggerScheduled = "scheduled"
)

// ErrNotFound is returned when a monitor definition, template, or run does not exist.
var ErrNotFound = errors.New("channel monitor not found")

// Scope identifies what a monitor definition probes.
type Scope string

const (
	// ScopeAccount probes a single provider account by id.
	ScopeAccount Scope = "account"
	// ScopeGroup probes every account in an account group by group id.
	ScopeGroup Scope = "group"
	// ScopeProvider probes every account belonging to a provider by provider id.
	ScopeProvider Scope = "provider"
	// ScopeModel probes accounts whose provider serves a model matching scope_ref (glob).
	ScopeModel Scope = "model"
)

// CustomRequest carries the operator-managed probe override. Empty fields fall
// back to the config-map-driven probe defaults.
type CustomRequest struct {
	Method              string
	URL                 string
	Headers             map[string]string
	Body                string
	ExpectedStatusCodes []int
	ResponseJSONPath    string
	ResponseContains    string
}

// Definition is one operator-managed synthetic probe.
type Definition struct {
	ID        int
	Name      string
	Enabled   bool
	Scope     Scope
	ScopeRef  string
	Interval  int // seconds
	Model     string
	Request   CustomRequest
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DefinitionWithSummary pairs a definition with a thin summary of its most recent
// run, so the admin list view can show last_run_at + ok/latency at a glance
// without an extra request per row. LastRun is nil when no runs exist yet.
type DefinitionWithSummary struct {
	Definition
	LastRun *RunSummary
}

// RunSummary is a row-level snapshot of the most recent run, deliberately
// smaller than RunResult (no per-check results).
type RunSummary struct {
	At        time.Time
	OK        bool
	LatencyMS int
}

type CreateDefinition struct {
	Name     string
	Enabled  bool
	Scope    Scope
	ScopeRef string
	Interval int
	Model    string
	Request  CustomRequest
}

type UpdateDefinition struct {
	Name     *string
	Enabled  *bool
	Scope    *Scope
	ScopeRef *string
	Interval *int
	Model    *string
	Request  *CustomRequest
}

// Template is a reusable probe request body that can be applied to many monitors.
type Template struct {
	ID          int
	Name        string
	Description string
	Request     CustomRequest
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateTemplate struct {
	Name        string
	Description string
	Request     CustomRequest
}

type UpdateTemplate struct {
	Name        *string
	Description *string
	Request     *CustomRequest
}

// CheckResult is one per-account / per-model probe outcome captured by a run.
type CheckResult struct {
	AccountID   int            `json:"account_id"`
	AccountName string         `json:"account_name"`
	ProviderID  int            `json:"provider_id"`
	Model       string         `json:"model"`
	OK          bool           `json:"ok"`
	StatusCode  int            `json:"status_code"`
	LatencyMS   int            `json:"latency_ms"`
	ErrorClass  string         `json:"error_class,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// RunResult is one persisted execution of a monitor definition.
type RunResult struct {
	ID           int
	MonitorID    int
	RunID        string
	OK           bool
	CheckedCount int
	OKCount      int
	LatencyMS    int
	Trigger      string
	Results      []CheckResult
	CreatedAt    time.Time
}

// RecordRun is a run outcome to persist.
type RecordRun struct {
	MonitorID    int
	RunID        string
	OK           bool
	CheckedCount int
	OKCount      int
	LatencyMS    int
	Trigger      string
	Results      []CheckResult
}

// Store persists monitor definitions, request templates, and run history.
type Store interface {
	CreateDefinition(ctx context.Context, input CreateDefinition) (Definition, error)
	UpdateDefinition(ctx context.Context, id int, input UpdateDefinition) (Definition, error)
	DeleteDefinition(ctx context.Context, id int) error
	GetDefinition(ctx context.Context, id int) (Definition, error)
	ListDefinitions(ctx context.Context) ([]Definition, error)

	CreateTemplate(ctx context.Context, input CreateTemplate) (Template, error)
	UpdateTemplate(ctx context.Context, id int, input UpdateTemplate) (Template, error)
	DeleteTemplate(ctx context.Context, id int) error
	GetTemplate(ctx context.Context, id int) (Template, error)
	ListTemplates(ctx context.Context) ([]Template, error)

	RecordRun(ctx context.Context, input RecordRun) (RunResult, error)
	ListRuns(ctx context.Context, monitorID int, limit int) ([]RunResult, error)
}

// AccountReader supplies account state and credentials for monitor execution.
type AccountReader interface {
	List(ctx context.Context) ([]accountcontract.ProviderAccount, error)
	ListGroupMembers(ctx context.Context, groupID int) ([]accountcontract.AccountGroupMember, error)
	DecryptCredential(ctx context.Context, accountID int) (map[string]any, error)
}

// ProviderReader supplies provider metadata for monitor execution.
type ProviderReader interface {
	FindByID(ctx context.Context, id int) (providercontract.Provider, error)
}

// ModelReader supplies model and provider mapping state for model-scoped monitors.
type ModelReader interface {
	List(ctx context.Context) ([]modelcontract.Model, error)
	ListMappingsByModel(ctx context.Context, modelID int) ([]modelcontract.ModelProviderMapping, error)
}

// ProbeAdapter executes the per-account synthetic probe.
type ProbeAdapter interface {
	ProbeAccount(ctx context.Context, req provideradaptercontract.ProbeRequest) (provideradaptercontract.ProbeResponse, error)
}

// RunnerDependencies are the cross-module readers needed to execute a monitor.
type RunnerDependencies struct {
	Accounts  AccountReader
	Providers ProviderReader
	Models    ModelReader
	Adapter   ProbeAdapter
}

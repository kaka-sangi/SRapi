package contract

import (
	"context"
	"errors"
	"time"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

var ErrNotFound = errors.New("operations resource not found")

type RetentionPolicy struct {
	UsageLogs              time.Duration
	SchedulerDecisions     time.Duration
	SchedulerFeedbacks     time.Duration
	AuditLogs              time.Duration
	AccountHealthSnapshots time.Duration
}

type CleanupResult struct {
	UsageLogs              int
	SchedulerDecisions     int
	SchedulerFeedbacks     int
	AuditLogs              int
	AccountHealthSnapshots int
}

type RetentionStore interface {
	Cleanup(ctx context.Context, before RetentionCutoffs) (CleanupResult, error)
}

type RetentionCutoffs struct {
	UsageLogs              *time.Time
	SchedulerDecisions     *time.Time
	SchedulerFeedbacks     *time.Time
	AuditLogs              *time.Time
	AccountHealthSnapshots *time.Time
}

type SLOStatus string

const (
	SLOStatusActive   SLOStatus = "active"
	SLOStatusDisabled SLOStatus = "disabled"
)

type SLIType string

const (
	SLITypeAvailability SLIType = "availability"
	SLITypeLatency      SLIType = "latency"
	SLITypeFreshness    SLIType = "freshness"
	SLITypeQuality      SLIType = "quality"
)

type AlertSeverity string

const (
	AlertSeverityCritical AlertSeverity = "critical"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityTicket   AlertSeverity = "ticket"
)

type AlertStatus string

const (
	AlertStatusFiring       AlertStatus = "firing"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
	AlertStatusSuppressed   AlertStatus = "suppressed"
)

type SLOFilter struct {
	SourceEndpoint    string
	Model             string
	ProviderID        *int
	ErrorOwnerExclude []string
}

type BurnRateThreshold struct {
	Severity        AlertSeverity
	ShortWindow     time.Duration
	LongWindow      time.Duration
	BurnRate        float64
	MinRequestCount int
}

type AlertPolicy struct {
	Name       string
	Thresholds []BurnRateThreshold
}

type SLODefinition struct {
	ID          int
	Name        string
	SLIType     SLIType
	Objective   float64
	WindowDays  int
	Status      SLOStatus
	Filter      SLOFilter
	AlertPolicy AlertPolicy
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SLOEvaluation struct {
	WindowStart         time.Time
	WindowEnd           time.Time
	TotalRequests       int
	GoodRequests        int
	BadRequests         int
	ErrorRate           float64
	BurnRate            float64
	Objective           float64
	ErrorBudgetConsumed float64
}

type SLOWithEvaluation struct {
	Definition SLODefinition
	Evaluation SLOEvaluation
}

type CreateSLORequest struct {
	Name        string
	SLIType     SLIType
	Objective   float64
	WindowDays  int
	Status      *SLOStatus
	Filter      SLOFilter
	AlertPolicy AlertPolicy
}

type UpdateSLORequest struct {
	Name        *string
	Objective   *float64
	WindowDays  *int
	Status      *SLOStatus
	Filter      *SLOFilter
	AlertPolicy *AlertPolicy
}

type AlertEvent struct {
	ID             int
	SLOID          *int
	RuleID         string
	Severity       AlertSeverity
	Status         AlertStatus
	Fingerprint    string
	Summary        string
	Details        map[string]any
	StartedAt      time.Time
	ResolvedAt     *time.Time
	AcknowledgedAt *time.Time
	AcknowledgedBy *int
	SuppressedBy   *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AckAlertRequest struct {
	ActorUserID int
	Now         time.Time
}

type ObservabilityStore interface {
	CreateSLO(ctx context.Context, input SLODefinition) (SLODefinition, error)
	UpdateSLO(ctx context.Context, input SLODefinition) (SLODefinition, error)
	FindSLOByID(ctx context.Context, id int) (SLODefinition, error)
	ListSLOs(ctx context.Context) ([]SLODefinition, error)
	CreateAlert(ctx context.Context, input AlertEvent) (AlertEvent, error)
	UpdateAlert(ctx context.Context, input AlertEvent) (AlertEvent, error)
	FindAlertByID(ctx context.Context, id int) (AlertEvent, error)
	ListAlerts(ctx context.Context) ([]AlertEvent, error)
	ListUsageLogs(ctx context.Context) ([]usagecontract.UsageLog, error)
}

type Store interface {
	RetentionStore
	ObservabilityStore
}

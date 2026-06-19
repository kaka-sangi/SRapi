package contract

import (
	"context"
	"errors"
	"time"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

var ErrNotFound = errors.New("operations resource not found")
var ErrInvalidInput = errors.New("invalid operations input")

// OpsSystemLogLevel is the normalized severity for sanitized operations logs.
type OpsSystemLogLevel string

const (
	OpsSystemLogLevelDebug OpsSystemLogLevel = "debug"
	OpsSystemLogLevelInfo  OpsSystemLogLevel = "info"
	OpsSystemLogLevelWarn  OpsSystemLogLevel = "warn"
	OpsSystemLogLevelError OpsSystemLogLevel = "error"
)

// Valid reports whether the level is one of the contract-defined severities.
func (l OpsSystemLogLevel) Valid() bool {
	return l == OpsSystemLogLevelDebug || l == OpsSystemLogLevelInfo || l == OpsSystemLogLevelWarn || l == OpsSystemLogLevelError
}

// OpsSystemLog is a sanitized operational event persisted for admin evidence.
type OpsSystemLog struct {
	ID        int
	Level     OpsSystemLogLevel
	Message   string
	Source    string
	RequestID string
	TraceID   string
	Metadata  map[string]any
	CreatedAt time.Time
}

// RecordSystemLogRequest carries a sanitized event into the operations service.
type RecordSystemLogRequest struct {
	Level     OpsSystemLogLevel
	Message   string
	Source    string
	RequestID string
	TraceID   string
	Metadata  map[string]any
	CreatedAt time.Time
}

// SystemLogList is a paginated list result plus the total matching row count.
type SystemLogList struct {
	Items []OpsSystemLog
	Total int
}

// SystemLogListOptions filters and paginates operations system-log reads.
type SystemLogListOptions struct {
	Page      int
	PageSize  int
	Level     OpsSystemLogLevel
	Source    string
	Query     string
	RequestID string
	TraceID   string
	Start     *time.Time
	End       *time.Time
}

// SystemLogCleanupFilter bounds system-log cleanup operations.
type SystemLogCleanupFilter struct {
	Level     OpsSystemLogLevel
	Source    string
	Query     string
	RequestID string
	TraceID   string
	Start     *time.Time
	End       *time.Time
	DryRun    bool
	MaxDelete int
}

// SystemLogCleanupResult summarizes a bounded cleanup or dry-run.
type SystemLogCleanupResult struct {
	Matched   int
	Deleted   int
	DryRun    bool
	MaxDelete int
	Limited   bool
}

// SystemLogStats is store-level evidence used to build log health responses.
type SystemLogStats struct {
	TotalCount  int
	LevelCounts map[OpsSystemLogLevel]int
	LastLog     *OpsSystemLog
	LastError   *OpsSystemLog
}

// SystemLogHealth reports whether the operations log store is usable and fresh.
type SystemLogHealth struct {
	StorageMode      string
	Writable         bool
	Degraded         bool
	Stale            bool
	TotalCount       int
	LevelCounts      map[OpsSystemLogLevel]int
	LastLogAt        *time.Time
	LastErrorAt      *time.Time
	LastErrorSource  string
	LastErrorMessage string
	CheckedAt        time.Time
}

// SystemLogStore persists sanitized operations system logs.
type SystemLogStore interface {
	CreateSystemLog(ctx context.Context, input OpsSystemLog) (OpsSystemLog, error)
	ListSystemLogs(ctx context.Context, opts SystemLogListOptions) (SystemLogList, error)
	SystemLogStats(ctx context.Context) (SystemLogStats, error)
	CleanupSystemLogs(ctx context.Context, filter SystemLogCleanupFilter) (SystemLogCleanupResult, error)
}

type RetentionPolicy struct {
	UsageLogs              time.Duration
	SchedulerDecisions     time.Duration
	SchedulerFeedbacks     time.Duration
	AuditLogs              time.Duration
	AccountHealthSnapshots time.Duration
	BatchLimit             int
}

type CleanupResult struct {
	UsageLogs              int
	SchedulerDecisions     int
	SchedulerFeedbacks     int
	AuditLogs              int
	AccountHealthSnapshots int
	Limited                bool
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
	BatchLimit             int
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

type NotificationChannelType string

const (
	NotificationChannelTypeEmail NotificationChannelType = "email"
)

type NotificationChannelStatus string

const (
	NotificationChannelStatusActive   NotificationChannelStatus = "active"
	NotificationChannelStatusDisabled NotificationChannelStatus = "disabled"
)

type NotificationDeliveryStatus string

const (
	NotificationDeliveryStatusPending   NotificationDeliveryStatus = "pending"
	NotificationDeliveryStatusDelivered NotificationDeliveryStatus = "delivered"
	NotificationDeliveryStatusFailed    NotificationDeliveryStatus = "failed"
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

// AlertEvaluationResult summarizes one SLO burn-rate alert evaluation pass.
type AlertEvaluationResult struct {
	Evaluated int
	Breached  int
	Created   int
	Updated   int
	Resolved  int
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

// NotificationChannel configures SRapi-native Ops alert delivery. Secrets are
// deployment-owned and must not be exposed through this contract.
type NotificationChannel struct {
	ID              int
	Name            string
	Type            NotificationChannelType
	Status          NotificationChannelStatus
	MinSeverity     AlertSeverity
	EmailRecipients []string
	SendResolved    bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateNotificationChannelRequest struct {
	Name            string
	Type            NotificationChannelType
	Status          *NotificationChannelStatus
	MinSeverity     AlertSeverity
	EmailRecipients []string
	SendResolved    *bool
}

type UpdateNotificationChannelRequest struct {
	Name            *string
	Status          *NotificationChannelStatus
	MinSeverity     *AlertSeverity
	EmailRecipients *[]string
	SendResolved    *bool
}

type NotificationDelivery struct {
	ID             int
	ChannelID      int
	AlertEventID   int
	AlertStatus    AlertStatus
	Severity       AlertSeverity
	Status         NotificationDeliveryStatus
	Target         string
	AttemptCount   int
	LastError      string
	NextAttemptAt  time.Time
	DeliveredAt    *time.Time
	LastAttemptAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	AlertSummary   string
	ChannelName    string
	ChannelType    NotificationChannelType
	AlertStartedAt time.Time
	AlertUpdatedAt time.Time
}

type DeliveryListOptions struct {
	ChannelID int
	Status    NotificationDeliveryStatus
	Limit     int
}

type DueDelivery struct {
	Delivery NotificationDelivery
	Channel  NotificationChannel
	Alert    AlertEvent
}

type AckAlertRequest struct {
	ActorUserID int
	Now         time.Time
}

// AlertMetricType enumerates the generic metrics an AlertRule can evaluate.
type AlertMetricType string

const (
	AlertMetricErrorRate    AlertMetricType = "error_rate"
	AlertMetricSuccessRate  AlertMetricType = "success_rate"
	AlertMetricLatencyP95   AlertMetricType = "latency_p95"
	AlertMetricRequestCount AlertMetricType = "request_count"
)

// AlertOperator compares an observed metric value against a rule threshold.
type AlertOperator string

const (
	AlertOperatorGT  AlertOperator = "gt"
	AlertOperatorGTE AlertOperator = "gte"
	AlertOperatorLT  AlertOperator = "lt"
	AlertOperatorLTE AlertOperator = "lte"
)

// AlertRuleScope narrows the usage logs an AlertRule evaluates over.
type AlertRuleScope struct {
	SourceEndpoint string
	Model          string
	ErrorClass     string
	ProviderID     *int
}

// AlertRule is a configurable, generic metric alert rule.
type AlertRule struct {
	ID              int
	Name            string
	MetricType      AlertMetricType
	Operator        AlertOperator
	Threshold       float64
	Severity        AlertSeverity
	Enabled         bool
	WindowSeconds   int
	CooldownSeconds int
	MinRequestCount int
	Scope           AlertRuleScope
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateAlertRuleRequest struct {
	Name            string
	MetricType      AlertMetricType
	Operator        AlertOperator
	Threshold       float64
	Severity        AlertSeverity
	Enabled         *bool
	WindowSeconds   int
	CooldownSeconds int
	MinRequestCount int
	Scope           AlertRuleScope
}

type UpdateAlertRuleRequest struct {
	Name            *string
	MetricType      *AlertMetricType
	Operator        *AlertOperator
	Threshold       *float64
	Severity        *AlertSeverity
	Enabled         *bool
	WindowSeconds   *int
	CooldownSeconds *int
	MinRequestCount *int
	Scope           *AlertRuleScope
}

// AlertSilenceMatcher selects which alert events a silence suppresses.
type AlertSilenceMatcher struct {
	RuleID         string
	Severity       AlertSeverity
	SourceEndpoint string
	Model          string
	ErrorClass     string
	ProviderID     *int
}

// AlertSilence suppresses matching alert events within a bounded window.
type AlertSilence struct {
	ID        int
	Comment   string
	Matcher   AlertSilenceMatcher
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy *int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateAlertSilenceRequest struct {
	Comment   string
	Matcher   AlertSilenceMatcher
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy *int
}

// AlertRuleEvaluationResult summarizes one generic alert-rule evaluation pass.
type AlertRuleEvaluationResult struct {
	Evaluated  int
	Breached   int
	Created    int
	Updated    int
	Resolved   int
	Suppressed int
}

type ObservabilityStore interface {
	CreateSLO(ctx context.Context, input SLODefinition) (SLODefinition, error)
	UpdateSLO(ctx context.Context, input SLODefinition) (SLODefinition, error)
	FindSLOByID(ctx context.Context, id int) (SLODefinition, error)
	ListSLOs(ctx context.Context) ([]SLODefinition, error)
	DeleteSLO(ctx context.Context, id int) error
	CreateAlert(ctx context.Context, input AlertEvent) (AlertEvent, error)
	UpdateAlert(ctx context.Context, input AlertEvent) (AlertEvent, error)
	FindAlertByID(ctx context.Context, id int) (AlertEvent, error)
	ListAlerts(ctx context.Context) ([]AlertEvent, error)
	ListUsageLogs(ctx context.Context) ([]usagecontract.UsageLog, error)
	ListUsageLogsSince(ctx context.Context, since time.Time) ([]usagecontract.UsageLog, error)
	CreateAlertRule(ctx context.Context, input AlertRule) (AlertRule, error)
	UpdateAlertRule(ctx context.Context, input AlertRule) (AlertRule, error)
	FindAlertRuleByID(ctx context.Context, id int) (AlertRule, error)
	ListAlertRules(ctx context.Context) ([]AlertRule, error)
	DeleteAlertRule(ctx context.Context, id int) error
	CreateAlertSilence(ctx context.Context, input AlertSilence) (AlertSilence, error)
	ListAlertSilences(ctx context.Context) ([]AlertSilence, error)
	DeleteAlertSilence(ctx context.Context, id int) error
	CreateNotificationChannel(ctx context.Context, input NotificationChannel) (NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, input NotificationChannel) (NotificationChannel, error)
	FindNotificationChannelByID(ctx context.Context, id int) (NotificationChannel, error)
	ListNotificationChannels(ctx context.Context) ([]NotificationChannel, error)
	DeleteNotificationChannel(ctx context.Context, id int) error
	CreateNotificationDelivery(ctx context.Context, input NotificationDelivery) (NotificationDelivery, error)
	UpdateNotificationDelivery(ctx context.Context, input NotificationDelivery) (NotificationDelivery, error)
	ListNotificationDeliveries(ctx context.Context, opts DeliveryListOptions) ([]NotificationDelivery, error)
	ListDueNotificationDeliveries(ctx context.Context, now time.Time, limit int) ([]DueDelivery, error)
	FindNotificationDelivery(ctx context.Context, channelID int, alertEventID int, alertStatus AlertStatus, target string) (NotificationDelivery, error)
}

type Store interface {
	RetentionStore
	ObservabilityStore
	SystemLogStore
}

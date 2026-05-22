package contract

import (
	"context"
	"time"
)

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

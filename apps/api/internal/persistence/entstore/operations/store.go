package operations

import (
	"context"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entaccounthealthsnapshot "github.com/srapi/srapi/apps/api/ent/accounthealthsnapshot"
	entauditlog "github.com/srapi/srapi/apps/api/ent/auditlog"
	entschedulerdecision "github.com/srapi/srapi/apps/api/ent/schedulerdecision"
	entschedulerfeedback "github.com/srapi/srapi/apps/api/ent/schedulerfeedback"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

var ErrInvalidStore = errors.New("invalid operations ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Cleanup(ctx context.Context, cutoffs contract.RetentionCutoffs) (contract.CleanupResult, error) {
	var result contract.CleanupResult
	if cutoffs.UsageLogs != nil {
		deleted, err := s.client.UsageLog.Delete().
			Where(entusagelog.CreatedAtLT(*cutoffs.UsageLogs)).
			Exec(ctx)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.UsageLogs = deleted
	}
	if cutoffs.SchedulerFeedbacks != nil {
		deleted, err := s.client.SchedulerFeedback.Delete().
			Where(entschedulerfeedback.CreatedAtLT(*cutoffs.SchedulerFeedbacks)).
			Exec(ctx)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.SchedulerFeedbacks = deleted
	}
	if cutoffs.SchedulerDecisions != nil {
		deleted, err := s.client.SchedulerDecision.Delete().
			Where(entschedulerdecision.CreatedAtLT(*cutoffs.SchedulerDecisions)).
			Exec(ctx)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.SchedulerDecisions = deleted
	}
	if cutoffs.AuditLogs != nil {
		deleted, err := s.client.AuditLog.Delete().
			Where(entauditlog.CreatedAtLT(*cutoffs.AuditLogs)).
			Exec(ctx)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.AuditLogs = deleted
	}
	if cutoffs.AccountHealthSnapshots != nil {
		deleted, err := s.client.AccountHealthSnapshot.Delete().
			Where(entaccounthealthsnapshot.SnapshotAtLT(*cutoffs.AccountHealthSnapshots)).
			Exec(ctx)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.AccountHealthSnapshots = deleted
	}
	return result, nil
}

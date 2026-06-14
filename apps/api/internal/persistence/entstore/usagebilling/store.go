// Package usagebilling coordinates the cross-table billing aggregation derived
// from a usage_log row: the subscription materialized-usage increment and the
// API-key cost-usage increment, applied exactly once and gated by the
// usage_log.aggregated_at marker. The live gateway path applies it eagerly; a
// reconciler sweeps any rows whose eager apply was dropped (e.g. on a hard
// crash). Both go through ApplyAggregation, which is idempotent and atomic.
package usagebilling

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	apikeystore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/apikeys"
	subscriptionstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/subscriptions"
)

// ErrInvalidClient is returned when the store is constructed without its deps.
var ErrInvalidClient = errors.New("usagebilling: invalid dependencies")

type Store struct {
	client        *ent.Client
	subscriptions *subscriptionstore.Store
	apiKeys       *apikeystore.Store
	now           func() time.Time
}

func New(client *ent.Client, subscriptions *subscriptionstore.Store, apiKeys *apikeystore.Store) (*Store, error) {
	if client == nil || subscriptions == nil || apiKeys == nil {
		return nil, ErrInvalidClient
	}
	return &Store{
		client:        client,
		subscriptions: subscriptions,
		apiKeys:       apiKeys,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

// ApplyAggregation applies the billing aggregation for a single usage_log row
// exactly once. In one transaction it conditionally claims the row (sets
// aggregated_at only if still NULL), then — for a successful, priced row —
// increments the user's subscription materialized usage and the API key's cost
// usage. The conditional claim makes concurrent callers (the eager live path and
// the reconciler) safe: only the claimant applies; the loser is a no-op.
//
// Increments are attributed to the row's created_at so a recent row lands in the
// same window the live path would have used, and an old (long-dropped) row is
// added to whatever window is current without resetting it: rolling windows
// never reset backward by construction, and the calendar windows are guarded by
// windowRolledForward, so a past created_at can only add to — never zero — the
// current period. Returns true when this call performed (claimed) the aggregation.
func (s *Store) ApplyAggregation(ctx context.Context, usageLogID int) (bool, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	row, err := tx.UsageLog.Get(ctx, usageLogID)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if row.AggregatedAt != nil {
		_ = tx.Rollback()
		return false, nil
	}
	now := s.now()
	claimed, err := tx.UsageLog.Update().
		Where(entusagelog.IDEQ(usageLogID), entusagelog.AggregatedAtIsNil()).
		SetAggregatedAt(now).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if claimed == 0 {
		// Lost the claim race to a concurrent caller.
		_ = tx.Rollback()
		return false, nil
	}
	if row.Success {
		cost := strings.TrimSpace(row.BillableCost)
		if cost != "" {
			occurredAt := row.CreatedAt.UTC()
			if row.UserID > 0 {
				if err := s.subscriptions.ApplyUsageDeltaTx(ctx, tx, subscriptioncontract.UsageDelta{
					UserID:       row.UserID,
					BillableCost: cost,
					OccurredAt:   occurredAt,
				}); err != nil {
					_ = tx.Rollback()
					return false, err
				}
			}
			if row.APIKeyID > 0 {
				if err := s.apiKeys.ApplyCostUsageTx(ctx, tx, apikeycontract.CostUsageUpdate{
					KeyID:        row.APIKeyID,
					BillableCost: cost,
					OccurredAt:   occurredAt,
				}); err != nil && !errors.Is(err, apikeycontract.ErrKeyNotFound) {
					// A deleted key must not block the rest of the aggregation.
					_ = tx.Rollback()
					return false, err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// SweepPending aggregates up to limit successful usage_log rows created in the
// half-open window [after, before) that are still unaggregated (their eager apply
// was dropped). `before` should trail now by more than the live async-write
// timeout so the sweep doesn't race rows the live path is about to claim (though
// ApplyAggregation is safe even if it does). `after` is the floor: it MUST stay
// within the migration's backfill window so the sweep never reaches pre-feature
// rows that were aggregated by the old path but never marked — reprocessing those
// would double-count. Returns how many rows it aggregated.
func (s *Store) SweepPending(ctx context.Context, after, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	ids, err := s.client.UsageLog.Query().
		Where(
			entusagelog.AggregatedAtIsNil(),
			entusagelog.SuccessEQ(true),
			entusagelog.CreatedAtGTE(after.UTC()),
			entusagelog.CreatedAtLT(before.UTC()),
		).
		Order(entusagelog.ByCreatedAt()).
		Limit(limit).
		IDs(ctx)
	if err != nil {
		return 0, err
	}
	applied := 0
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return applied, ctx.Err()
		default:
		}
		ok, err := s.ApplyAggregation(ctx, id)
		if err != nil {
			return applied, err
		}
		if ok {
			applied++
		}
	}
	return applied, nil
}

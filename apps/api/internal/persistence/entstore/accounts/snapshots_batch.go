package accounts

import (
	"context"
	"sort"

	"github.com/srapi/srapi/apps/api/ent"
	entaccounthealthsnapshot "github.com/srapi/srapi/apps/api/ent/accounthealthsnapshot"
	entaccountquotasnapshot "github.com/srapi/srapi/apps/api/ent/accountquotasnapshot"
	contract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

// The batched latest-snapshot readers below back the gateway scheduling hot
// path: candidate assembly needs the freshest health and quota state for every
// candidate account, and doing that per account costs two queries per
// candidate per attempt. They scan only the newest bounded slice across the
// requested account set and keep the first row for each group using the same
// ordering as the per-account readers (snapshot_at desc, id desc).

const batchSnapshotScanCap = 5000

// LatestHealthSnapshotsByAccounts returns each account's most recent health
// snapshot keyed by account ID. Accounts without snapshots are absent from the
// result. Implements contract.BatchSnapshotReader.
func (s *Store) LatestHealthSnapshotsByAccounts(ctx context.Context, accountIDs []int) (map[int]contract.AccountHealthSnapshot, error) {
	out := make(map[int]contract.AccountHealthSnapshot, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	rows, err := s.client.AccountHealthSnapshot.Query().
		Where(entaccounthealthsnapshot.AccountIDIn(accountIDs...)).
		Order(ent.Desc(entaccounthealthsnapshot.FieldSnapshotAt), ent.Desc(entaccounthealthsnapshot.FieldID)).
		Limit(batchSnapshotScanCap).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, exists := out[row.AccountID]; exists {
			continue
		}
		out[row.AccountID] = toHealthSnapshot(row)
	}
	return out, nil
}

// LatestQuotaSnapshotsByAccounts returns, per account, the most recent quota
// snapshot of each quota type (the batched equivalent of
// ListQuotaSnapshotsByAccount with limit 1), newest first. Implements
// contract.BatchSnapshotReader.
func (s *Store) LatestQuotaSnapshotsByAccounts(ctx context.Context, accountIDs []int) (map[int][]contract.AccountQuotaSnapshot, error) {
	out := make(map[int][]contract.AccountQuotaSnapshot, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	rows, err := s.client.AccountQuotaSnapshot.Query().
		Where(entaccountquotasnapshot.AccountIDIn(accountIDs...)).
		Order(ent.Desc(entaccountquotasnapshot.FieldSnapshotAt), ent.Desc(entaccountquotasnapshot.FieldID)).
		Limit(batchSnapshotScanCap).
		All(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[quotaSnapshotGroup]struct{}, len(rows))
	for _, row := range rows {
		key := quotaSnapshotGroup{accountID: row.AccountID, quotaType: row.QuotaType}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out[row.AccountID] = append(out[row.AccountID], toQuotaSnapshot(row))
	}
	sortQuotaSnapshotsNewestFirst(out)
	return out, nil
}

type quotaSnapshotGroup struct {
	accountID int
	quotaType string
}

// sortQuotaSnapshotsNewestFirst orders each account's per-type snapshots the
// same way ListQuotaSnapshotsByAccount returns them (snapshot_at desc, id
// desc), so consumers that surface the windows keep a stable display order.
func sortQuotaSnapshotsNewestFirst(byAccount map[int][]contract.AccountQuotaSnapshot) {
	for _, snapshots := range byAccount {
		sort.Slice(snapshots, func(i, j int) bool {
			if snapshots[i].SnapshotAt.Equal(snapshots[j].SnapshotAt) {
				return snapshots[i].ID > snapshots[j].ID
			}
			return snapshots[i].SnapshotAt.After(snapshots[j].SnapshotAt)
		})
	}
}

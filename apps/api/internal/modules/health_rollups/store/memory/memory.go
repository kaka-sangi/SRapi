package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
)

// Store is an in-memory implementation of the availability rollup store.
type Store struct {
	mu      sync.Mutex
	rollups map[string]contract.Rollup // key: accountID|date
	seq     int
}

func New() *Store {
	return &Store{rollups: make(map[string]contract.Rollup)}
}

func key(accountID int, date string) string {
	return date + "|" + itoa(accountID)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func (s *Store) UpsertRollup(ctx context.Context, rollup contract.Rollup) (contract.Rollup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(rollup.AccountID, rollup.Date)
	if existing, ok := s.rollups[k]; ok {
		rollup.ID = existing.ID
	} else {
		s.seq++
		rollup.ID = s.seq
	}
	s.rollups[k] = rollup
	return rollup, nil
}

func (s *Store) ListRollupsByAccount(ctx context.Context, accountID int, sinceDate string) ([]contract.Rollup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Rollup, 0)
	for _, rollup := range s.rollups {
		if rollup.AccountID != accountID {
			continue
		}
		if sinceDate != "" && rollup.Date < sinceDate {
			continue
		}
		out = append(out, rollup)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

func (s *Store) ListRollupsSince(ctx context.Context, sinceDate string) ([]contract.Rollup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Rollup, 0)
	for _, rollup := range s.rollups {
		if sinceDate != "" && rollup.Date < sinceDate {
			continue
		}
		out = append(out, rollup)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out, nil
}

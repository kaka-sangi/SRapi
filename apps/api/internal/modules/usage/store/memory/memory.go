package memory

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.UsageLog
}

func New() *Store {
	return &Store{
		nextID: 1,
		byID:   map[int]contract.UsageLog{},
	}
}

func (s *Store) Create(_ context.Context, input contract.UsageLog) (contract.UsageLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log := cloneLog(input)
	log.ID = s.nextID
	if log.AttemptNo <= 0 {
		log.AttemptNo = 1
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(log.ActualCost) == "" {
		log.ActualCost = log.Cost
	}
	if strings.TrimSpace(log.RateMultiplier) == "" {
		log.RateMultiplier = "1.00000000"
	}
	if strings.TrimSpace(log.BillableCost) == "" {
		log.BillableCost = log.ActualCost
	}
	if strings.TrimSpace(log.InputCost) == "" {
		log.InputCost = "0.00000000"
	}
	if strings.TrimSpace(log.OutputCost) == "" {
		log.OutputCost = "0.00000000"
	}
	if strings.TrimSpace(log.CacheReadCost) == "" {
		log.CacheReadCost = "0.00000000"
	}
	if strings.TrimSpace(log.CacheWriteCost) == "" {
		log.CacheWriteCost = "0.00000000"
	}
	if strings.TrimSpace(log.RequestedModel) == "" {
		log.RequestedModel = log.Model
	}
	if strings.TrimSpace(log.BillingMode) == "" {
		log.BillingMode = "token"
	}
	s.byID[log.ID] = log
	s.nextID++
	return cloneLog(log), nil
}

func (s *Store) List(_ context.Context) ([]contract.UsageLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UsageLog, 0, len(s.byID))
	for _, log := range s.byID {
		out = append(out, cloneLog(log))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListWindow implements contract.WindowReader: window predicates applied while
// scanning, positive limit keeps the newest rows, ascending id output.
func (s *Store) ListWindow(_ context.Context, filter contract.QueryFilter, limit int) ([]contract.UsageLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UsageLog, 0, len(s.byID))
	for _, log := range s.byID {
		if filter.Start != nil && log.CreatedAt.Before(*filter.Start) {
			continue
		}
		if filter.End != nil && !log.CreatedAt.Before(*filter.End) {
			continue
		}
		out = append(out, cloneLog(log))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *Store) ListByUser(_ context.Context, userID int) ([]contract.UsageLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UsageLog, 0)
	for _, log := range s.byID {
		if log.UserID == userID {
			out = append(out, cloneLog(log))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListByAccountWindow(_ context.Context, filter contract.AccountWindowFilter) ([]contract.UsageLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := filter.Start.UTC()
	end := filter.End.UTC()
	out := make([]contract.UsageLog, 0)
	for _, log := range s.byID {
		if log.AccountID == nil || *log.AccountID != filter.AccountID {
			continue
		}
		if log.CreatedAt.Before(start) || !log.CreatedAt.Before(end) {
			continue
		}
		out = append(out, cloneLog(log))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[len(out)-filter.Limit:]
	}
	return out, nil
}

func (s *Store) SummarizeUserWindow(_ context.Context, filter contract.UserWindowFilter) (contract.UserWindowSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	totalCost := new(big.Rat)
	summary := contract.UserWindowSummary{
		UserID:       filter.UserID,
		ProviderID:   cloneInt(filter.ProviderID),
		Start:        filter.Start.UTC(),
		End:          filter.End.UTC(),
		SuccessOnly:  filter.SuccessOnly,
		BillableCost: "0.00000000",
	}
	for _, log := range s.byID {
		if log.UserID != filter.UserID || log.CreatedAt.Before(summary.Start) || !log.CreatedAt.Before(summary.End) {
			continue
		}
		if filter.ProviderID != nil && (log.ProviderID == nil || *log.ProviderID != *filter.ProviderID) {
			continue
		}
		if filter.SuccessOnly && !log.Success {
			continue
		}
		summary.TotalTokens += log.TotalTokens
		if cost, ok := money.DecimalRat(log.BillableCost); ok {
			totalCost.Add(totalCost, cost)
		}
	}
	summary.BillableCost = money.FormatRatFixed(totalCost, 8)
	return summary, nil
}

// CleanupLogs deletes the records matching filter, oldest first, capped at
// filter.MaxDelete. Matched counts every record the filter selects (so the
// caller can report when the cap left some in place); Deleted is 0 on a dry run.
func (s *Store) CleanupLogs(_ context.Context, filter contract.CleanupFilter) (contract.CleanupResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	matchedIDs := make([]int, 0)
	for id, log := range s.byID {
		if cleanupMatches(log, filter) {
			matchedIDs = append(matchedIDs, id)
		}
	}
	// Delete oldest first (smaller ID == earlier insert) so a capped batch
	// trims the oldest records, matching the retention worker's intent.
	sort.Ints(matchedIDs)
	result := contract.CleanupResult{
		Matched:   len(matchedIDs),
		DryRun:    filter.DryRun,
		MaxDelete: filter.MaxDelete,
	}
	if filter.DryRun {
		result.Limited = result.Matched > filter.MaxDelete
		return result, nil
	}
	for _, id := range matchedIDs {
		if result.Deleted >= filter.MaxDelete {
			break
		}
		delete(s.byID, id)
		result.Deleted++
	}
	result.Limited = result.Matched > result.Deleted
	return result, nil
}

func cleanupMatches(log contract.UsageLog, filter contract.CleanupFilter) bool {
	if filter.Model != "" && !strings.EqualFold(strings.TrimSpace(log.Model), strings.TrimSpace(filter.Model)) {
		return false
	}
	if filter.Start != nil && log.CreatedAt.Before(filter.Start.UTC()) {
		return false
	}
	if filter.End != nil && !log.CreatedAt.Before(filter.End.UTC()) {
		return false
	}
	return true
}

func cloneLog(value contract.UsageLog) contract.UsageLog {
	value.CompatibilityWarnings = cloneStrings(value.CompatibilityWarnings)
	if value.ChargedAt != nil {
		cloned := *value.ChargedAt
		value.ChargedAt = &cloned
	}
	return value
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

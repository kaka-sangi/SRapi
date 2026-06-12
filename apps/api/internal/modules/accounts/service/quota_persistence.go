package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// ApplyQuotaReport persists normalized quota signals and quota-credit metadata
// from a provider adapter report.
func (s *Service) ApplyQuotaReport(ctx context.Context, account contract.ProviderAccount, report provideradaptercontract.QuotaReport) (int, error) {
	if account.ID <= 0 || account.ProviderID <= 0 {
		return 0, ErrInvalidInput
	}
	signals := 0
	var firstErr error
	creditReport := contract.QuotaCreditReport{
		Plan:             report.Plan,
		CreditsRemaining: report.CreditsRemaining,
		CreditsUsed:      report.CreditsUsed,
		CreditsLimit:     report.CreditsLimit,
		Currency:         report.Currency,
		FetchedAt:        report.FetchedAt,
	}
	if snapshot, ok := contract.QuotaCreditSnapshotFromReport(account, creditReport); ok {
		if _, err := s.RecordQuotaSnapshot(ctx, snapshot); err != nil {
			firstErr = errors.Join(firstErr, err)
		} else {
			signals++
		}
	}
	if report.Supported {
		metadata := contract.QuotaMetadataFromReport(account.Metadata, creditReport)
		metadata = quotaSignalMetadata(metadata, report.QuotaSignals, report.FetchedAt)
		if _, err := s.Update(ctx, account.ID, contract.UpdateRequest{Metadata: &metadata}); err != nil {
			firstErr = errors.Join(firstErr, err)
		}
	}
	for _, signal := range report.QuotaSignals {
		if _, err := s.RecordQuotaSnapshot(ctx, QuotaSnapshotFromSignal(account, signal, s.clock.Now())); err != nil {
			firstErr = errors.Join(firstErr, err)
			break
		}
		signals++
	}
	return signals, firstErr
}

func quotaSignalMetadata(current map[string]any, signals []provideradaptercontract.QuotaSignal, fetchedAt time.Time) map[string]any {
	metadata := cloneMap(current)
	if metadata == nil {
		metadata = map[string]any{}
	}
	mergeQuotaSignalMetadata(metadata, signals)
	if len(signals) == 0 {
		return metadata
	}
	best, ok := lowestRemainingQuotaSignal(signals)
	if !ok {
		return metadata
	}
	metadata["quota_remaining_ratio"] = best.RemainingRatio
	metadata["quota_type"] = strings.TrimSpace(best.QuotaType)
	if best.Remaining != "" {
		metadata["quota_remaining"] = strings.TrimSpace(best.Remaining)
	}
	if best.Used != "" {
		metadata["quota_used"] = strings.TrimSpace(best.Used)
	}
	if best.QuotaLimit != "" {
		metadata["quota_limit"] = strings.TrimSpace(best.QuotaLimit)
	}
	if best.ResetAt != nil && !best.ResetAt.IsZero() {
		metadata["quota_reset_at"] = best.ResetAt.UTC().Format(time.RFC3339)
	}
	if best.RemainingRatio <= 0 {
		metadata["quota_exhausted"] = true
		exhaustedAt := best.SnapshotAt
		if exhaustedAt.IsZero() {
			exhaustedAt = fetchedAt
		}
		if exhaustedAt.IsZero() {
			exhaustedAt = time.Now().UTC()
		}
		metadata["quota_exhausted_at"] = exhaustedAt.UTC().Format(time.RFC3339)
	} else {
		delete(metadata, "quota_exhausted")
		delete(metadata, "quota_exhausted_at")
	}
	return metadata
}

func mergeQuotaSignalMetadata(metadata map[string]any, signals []provideradaptercontract.QuotaSignal) {
	for _, signal := range signals {
		for key, value := range signal.Metadata {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			switch typed := value.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					metadata[key] = trimmed
				}
			case bool, int, int64, float64:
				metadata[key] = typed
			case float32:
				metadata[key] = float64(typed)
			}
		}
	}
}

func lowestRemainingQuotaSignal(signals []provideradaptercontract.QuotaSignal) (provideradaptercontract.QuotaSignal, bool) {
	var best provideradaptercontract.QuotaSignal
	ok := false
	for _, signal := range signals {
		if strings.TrimSpace(signal.QuotaType) == "" {
			continue
		}
		if signal.RemainingRatio < 0 || signal.RemainingRatio > 1 {
			continue
		}
		if !ok || signal.RemainingRatio < best.RemainingRatio {
			best = signal
			ok = true
		}
	}
	return best, ok
}

// ApplyQuotaProviderError persists quota error metadata for operator-action
// quota failures. Non-quota provider errors are ignored.
func (s *Service) ApplyQuotaProviderError(ctx context.Context, account contract.ProviderAccount, err error) error {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	if providerErr.StatusCode != http.StatusForbidden {
		return nil
	}
	metadata := provideradaptercontract.QuotaErrorMetadata(account.Metadata, providerErr, s.clock.Now())
	status := account.Status
	if provideradaptercontract.QuotaErrorClassRequiresOperatorAction(providerErr.Class) {
		status = contract.StatusSuspended
	}
	_, updateErr := s.Update(ctx, account.ID, contract.UpdateRequest{Metadata: &metadata, Status: &status})
	return updateErr
}

// QuotaSnapshotFromSignal maps a provider quota signal into account snapshot
// storage, filling a missing signal timestamp with fallbackNow.
func QuotaSnapshotFromSignal(account contract.ProviderAccount, signal provideradaptercontract.QuotaSignal, fallbackNow time.Time) contract.AccountQuotaSnapshot {
	snapshotAt := signal.SnapshotAt
	if snapshotAt.IsZero() {
		snapshotAt = fallbackNow.UTC()
	}
	return contract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      signal.QuotaType,
		Remaining:      signal.Remaining,
		Used:           signal.Used,
		QuotaLimit:     signal.QuotaLimit,
		RemainingRatio: signal.RemainingRatio,
		ResetAt:        signal.ResetAt,
		SnapshotAt:     snapshotAt,
	}
}

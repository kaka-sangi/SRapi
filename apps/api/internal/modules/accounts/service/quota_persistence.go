package service

import (
	"context"
	"errors"
	"net/http"
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

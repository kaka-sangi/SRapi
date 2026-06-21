package service

import (
	"context"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

// ManualPauseRequest carries the operator-supplied pause window.
//
//   - Until: required, must be strictly after the service clock's now. The
//     scheduler skips the account while this timestamp is in the future and
//     auto-resumes it on or after Until without any other intervention.
//   - Reason: optional free-form text surfaced to other operators in the
//     account drawer (e.g. "stuck retry quota", "investigating slow upstream").
type ManualPauseRequest struct {
	Until  time.Time
	Reason string
}

// ApplyManualPause records an operator-initiated scheduler skip on the account
// metadata. Distinct from the health-probe cooldown so a probe success cannot
// silently lift the operator's pause; both windows flow into
// schedulercontract.RuntimeState.CooldownActive at admission time.
//
// The account row's status is intentionally left untouched — pause is an
// orthogonal scheduling skip, not a logical disable. ManualPauseRequest.Until
// is normalised to UTC RFC3339; Reason is trimmed and truncated to 200 chars
// so the metadata value stays operator-readable.
func (s *Service) ApplyManualPause(ctx context.Context, id int, req ManualPauseRequest) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	until := req.Until.UTC()
	now := s.clock.Now().UTC()
	if !until.After(now) {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	metadata := cloneMap(account.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["manual_pause_until"] = until.Format(time.RFC3339)
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 200 {
		reason = reason[:200]
	}
	if reason == "" {
		delete(metadata, "manual_pause_reason")
	} else {
		metadata["manual_pause_reason"] = reason
	}
	metadata["manual_pause_applied_at"] = now.Format(time.RFC3339)
	return s.Update(ctx, id, contract.UpdateRequest{Metadata: &metadata})
}

// ClearManualPause removes the operator-initiated pause keys from the account
// metadata. Idempotent: clearing an unpaused account is a successful no-op
// (returns the current account state) so the admin UI can call this without
// pre-checking.
func (s *Service) ClearManualPause(ctx context.Context, id int) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	metadata := cloneMap(account.Metadata)
	changed := false
	for _, key := range []string{"manual_pause_until", "manual_pause_reason", "manual_pause_applied_at"} {
		if _, ok := metadata[key]; ok {
			delete(metadata, key)
			changed = true
		}
	}
	if !changed {
		return account, nil
	}
	return s.Update(ctx, id, contract.UpdateRequest{Metadata: &metadata})
}

package httpserver

import (
	"net/http"
	"strconv"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type quotaSignalPayload struct {
	QuotaType      string     `json:"quota_type"`
	Remaining      string     `json:"remaining"`
	Used           string     `json:"used"`
	QuotaLimit     string     `json:"quota_limit"`
	RemainingRatio float32    `json:"remaining_ratio"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
}

type quotaReportPayload struct {
	Provider         string               `json:"provider"`
	Supported        bool                 `json:"supported"`
	Source           string               `json:"source"`
	Plan             string               `json:"plan"`
	CreditsRemaining string               `json:"credits_remaining"`
	CreditsUsed      string               `json:"credits_used"`
	CreditsLimit     string               `json:"credits_limit"`
	Currency         string               `json:"currency"`
	StatusCode       int                  `json:"status_code"`
	FetchedAt        time.Time            `json:"fetched_at"`
	QuotaSignals     []quotaSignalPayload `json:"quota_signals"`
}

// handleAdminAccountQuotaFetch performs an active per-account quota/subscription
// fetch through the provider adapter, persists any quota signals, and returns
// the normalized report.
func (s *Server) handleAdminAccountQuotaFetch(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), account.ProviderID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	credential, err := s.runtime.accounts.DecryptCredential(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load account credential", requestID)
		return
	}

	report, err := s.runtime.adapters.FetchAccountQuota(r.Context(), provideradaptercontract.ProbeRequest{
		Provider:   provider,
		Account:    account,
		Credential: credential,
	})
	if err != nil {
		writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "quota fetch failed", requestID)
		return
	}

	s.persistQuotaSignals(r, account, report.QuotaSignals)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account.quota_fetch", "account", strconv.Itoa(accountID), nil, map[string]any{
		"supported": report.Supported,
		"source":    report.Source,
		"plan":      report.Plan,
	}))

	signals := make([]quotaSignalPayload, 0, len(report.QuotaSignals))
	for _, signal := range report.QuotaSignals {
		signals = append(signals, quotaSignalPayload{
			QuotaType:      signal.QuotaType,
			Remaining:      signal.Remaining,
			Used:           signal.Used,
			QuotaLimit:     signal.QuotaLimit,
			RemainingRatio: signal.RemainingRatio,
			ResetAt:        signal.ResetAt,
		})
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": quotaReportPayload{
			Provider:         report.Provider,
			Supported:        report.Supported,
			Source:           report.Source,
			Plan:             report.Plan,
			CreditsRemaining: report.CreditsRemaining,
			CreditsUsed:      report.CreditsUsed,
			CreditsLimit:     report.CreditsLimit,
			Currency:         report.Currency,
			StatusCode:       report.StatusCode,
			FetchedAt:        report.FetchedAt.UTC(),
			QuotaSignals:     signals,
		},
		"request_id": requestID,
	})
}

func (s *Server) persistQuotaSignals(r *http.Request, account accountcontract.ProviderAccount, signals []provideradaptercontract.QuotaSignal) {
	for _, signal := range signals {
		snapshotAt := signal.SnapshotAt
		if snapshotAt.IsZero() {
			snapshotAt = time.Now().UTC()
		}
		if _, err := s.runtime.accounts.RecordQuotaSnapshot(r.Context(), accountcontract.AccountQuotaSnapshot{
			AccountID:      account.ID,
			ProviderID:     account.ProviderID,
			QuotaType:      signal.QuotaType,
			Remaining:      signal.Remaining,
			Used:           signal.Used,
			QuotaLimit:     signal.QuotaLimit,
			RemainingRatio: signal.RemainingRatio,
			ResetAt:        signal.ResetAt,
			SnapshotAt:     snapshotAt,
		}); err != nil {
			// Persisting snapshots is best-effort; the report is still returned.
			_ = err
			continue
		}
	}
}

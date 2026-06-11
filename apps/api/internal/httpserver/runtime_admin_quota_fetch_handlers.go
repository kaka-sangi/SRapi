package httpserver

import (
	"net/http"
	"strconv"
	"strings"
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

func (s *Server) handleAdminAccountResetQuota(w http.ResponseWriter, r *http.Request) {
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
	metadata := resetQuotaMetadata(account.Metadata, time.Now().UTC())
	status := account.Status
	if status == accountcontract.StatusSuspended || status == accountcontract.StatusDead || status == accountcontract.StatusNeedsReauth {
		status = accountcontract.StatusActive
	}
	updated, err := s.runtime.accounts.Update(r.Context(), account.ID, accountcontract.UpdateRequest{
		Metadata: &metadata,
		Status:   &status,
	})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to reset quota state", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account.quota_reset", "account", strconv.Itoa(accountID), nil, map[string]any{
		"previous_status": account.Status,
		"status":          updated.Status,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      toAPIAccount(updated),
		RequestId: requestID,
	})
}

func resetQuotaMetadata(current map[string]any, now time.Time) map[string]any {
	metadata := cloneAnyMap(current)
	for _, key := range []string{
		"quota_exhausted",
		"quota_exhausted_at",
		"quota_remaining_ratio",
		"quota_reset_at",
		"last_quota_error_class",
		"last_quota_error_status_code",
		"last_quota_error_message",
		"last_quota_error_at",
		"last_quota_error_provider_metadata",
		"validation_url",
		"provider_error_kind",
		"cooldown_active",
		"cooldown_reason",
		"cooldown_until",
		"cooldown_strikes",
		"cooldown_last_at",
	} {
		delete(metadata, key)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mapString(metadata, "last_error_class"))), "quota") {
		delete(metadata, "last_error_class")
		delete(metadata, "last_error_message")
		delete(metadata, "last_error_at")
	}
	metadata["last_quota_reset_at"] = now.UTC().Format(time.RFC3339)
	return metadata
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
	if refreshed, ok, err := s.runtime.refreshReverseProxyCredential(r.Context(), account, credential); err != nil {
		s.persistQuotaProviderError(r, account, err)
		writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "quota fetch credential refresh failed", requestID)
		return
	} else if ok {
		credential = refreshed
	}

	report, err := s.runtime.adapters.FetchAccountQuota(r.Context(), provideradaptercontract.ProbeRequest{
		Provider:   provider,
		Account:    account,
		Credential: credential,
	})
	if err != nil {
		if refreshed, retried := s.runtime.retryAfterAuthRefresh(r.Context(), account, credential, err); retried {
			credential = refreshed
			report, err = s.runtime.adapters.FetchAccountQuota(r.Context(), provideradaptercontract.ProbeRequest{
				Provider:   provider,
				Account:    account,
				Credential: credential,
			})
		}
	}
	if err != nil {
		s.persistQuotaProviderError(r, account, err)
		writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "quota fetch failed", requestID)
		return
	}

	s.persistQuotaReport(r, account, report)
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

func (s *Server) persistQuotaReport(r *http.Request, account accountcontract.ProviderAccount, report provideradaptercontract.QuotaReport) {
	if _, err := s.runtime.accounts.ApplyQuotaReport(r.Context(), account, report); err != nil {
		_ = err
	}
}

func (s *Server) persistQuotaProviderError(r *http.Request, account accountcontract.ProviderAccount, err error) {
	if updateErr := s.runtime.accounts.ApplyQuotaProviderError(r.Context(), account, err); updateErr != nil {
		// Returning the provider error matters more than metadata persistence.
		_ = updateErr
	}
}

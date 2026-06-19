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
	report, err := s.fetchAccountQuotaReportOnce(r, accountID)
	if err != nil {
		status, code, message := quotaFetchHTTPError(err)
		writeStandardError(w, status, code, message, requestID)
		return
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account.quota_fetch", "account", strconv.Itoa(accountID), nil, map[string]any{
		"supported": report.Supported,
		"source":    report.Source,
		"plan":      report.Plan,
	}))

	writeJSONAny(w, http.StatusOK, apiopenapi.AccountQuotaReportResponse{
		Data:      toAPIAccountQuotaReport(report),
		RequestId: requestID,
	})
}

func (s *Server) fetchAccountQuotaReportOnce(r *http.Request, accountID int) (provideradaptercontract.QuotaReport, error) {
	if accountID <= 0 {
		return provideradaptercontract.QuotaReport{}, quotaFetchInvalidAccountID
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		return provideradaptercontract.QuotaReport{}, quotaFetchAccountNotFound
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), account.ProviderID)
	if err != nil {
		return provideradaptercontract.QuotaReport{}, quotaFetchProviderNotFound
	}
	credential, err := s.runtime.accounts.DecryptCredential(r.Context(), accountID)
	if err != nil {
		return provideradaptercontract.QuotaReport{}, quotaFetchCredentialLoadFailed
	}
	if refreshed, ok, refreshErr := s.runtime.refreshReverseProxyCredential(r.Context(), account, credential); refreshErr != nil {
		s.persistQuotaProviderError(r, account, refreshErr)
		return provideradaptercontract.QuotaReport{}, quotaFetchCredentialRefreshFailed
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
		return provideradaptercontract.QuotaReport{}, quotaFetchFailed
	}
	s.persistQuotaReport(r, account, report)
	return report, nil
}

func quotaFetchHTTPError(err error) (int, apiopenapi.ErrorCode, string) {
	switch err {
	case quotaFetchInvalidAccountID:
		return http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id"
	case quotaFetchAccountNotFound:
		return http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found"
	case quotaFetchProviderNotFound:
		return http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found"
	case quotaFetchCredentialLoadFailed:
		return http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load account credential"
	case quotaFetchCredentialRefreshFailed:
		return http.StatusBadGateway, apiopenapi.INTERNALERROR, "quota fetch credential refresh failed"
	default:
		return http.StatusBadGateway, apiopenapi.INTERNALERROR, "quota fetch failed"
	}
}

func toAPIAccountQuotaReport(report provideradaptercontract.QuotaReport) apiopenapi.AccountQuotaReport {
	signals := make([]apiopenapi.AccountQuotaSignal, 0, len(report.QuotaSignals))
	for _, signal := range report.QuotaSignals {
		signals = append(signals, toAPIAccountQuotaSignal(signal))
	}
	return apiopenapi.AccountQuotaReport{
		Provider:         report.Provider,
		Supported:        report.Supported,
		Source:           report.Source,
		Plan:             report.Plan,
		CreditsRemaining: report.CreditsRemaining,
		CreditsUsed:      report.CreditsUsed,
		CreditsLimit:     report.CreditsLimit,
		Currency:         report.Currency,
		StatusCode:       int64(report.StatusCode),
		FetchedAt:        report.FetchedAt.UTC(),
		QuotaSignals:     signals,
	}
}

func toAPIAccountQuotaSignal(signal provideradaptercontract.QuotaSignal) apiopenapi.AccountQuotaSignal {
	return apiopenapi.AccountQuotaSignal{
		QuotaType:      signal.QuotaType,
		Remaining:      signal.Remaining,
		Used:           signal.Used,
		QuotaLimit:     signal.QuotaLimit,
		RemainingRatio: signal.RemainingRatio,
		ResetAt:        cloneTimePtr(signal.ResetAt),
	}
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

type quotaFetchError string

func (e quotaFetchError) Error() string { return string(e) }

const (
	quotaFetchInvalidAccountID        quotaFetchError = "invalid account id"
	quotaFetchAccountNotFound         quotaFetchError = "account not found"
	quotaFetchProviderNotFound        quotaFetchError = "provider not found"
	quotaFetchCredentialLoadFailed    quotaFetchError = "failed to load account credential"
	quotaFetchCredentialRefreshFailed quotaFetchError = "quota fetch credential refresh failed"
	quotaFetchFailed                  quotaFetchError = "quota fetch failed"
)

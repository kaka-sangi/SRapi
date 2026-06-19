package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	healthrollupscontract "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	availabilityDefaultDays    = 7
	availabilityMaxDays        = 90
	availabilitySnapshotsLimit = 2000
)

func (s *Server) handleAdminAccountAvailability(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	if _, err := s.runtime.accounts.FindByID(r.Context(), accountID); err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	days := availabilityDefaultDays
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 || parsed > availabilityMaxDays {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid days parameter", requestID)
			return
		}
		days = parsed
	}

	snapshots, err := s.runtime.accounts.ListHealthSnapshotsByAccount(r.Context(), accountID, availabilitySnapshotsLimit)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load health snapshots", requestID)
		return
	}
	now := time.Now().UTC()
	samples := healthSnapshotsToSamples(snapshots, now.AddDate(0, 0, -(days-1)).Truncate(24*time.Hour))
	if _, err := s.runtime.healthRollups.RefreshAccount(r.Context(), accountID, samples, now); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to compute availability rollups", requestID)
		return
	}
	rollups, err := s.runtime.healthRollups.ListByAccount(r.Context(), accountID, days, now)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list availability rollups", requestID)
		return
	}

	data := make([]apiopenapi.AccountAvailabilityRollup, 0, len(rollups))
	for _, rollup := range rollups {
		data = append(data, toAPIAccountAvailabilityRollup(rollup))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountAvailabilityResponse{
		Data: struct {
			AccountId         int64                                  `json:"account_id"`
			DailyAvailability []apiopenapi.AccountAvailabilityRollup `json:"daily_availability"`
			OverallUptime     float32                                `json:"overall_uptime"`
			WindowDays        int64                                  `json:"window_days"`
		}{
			AccountId:         int64(accountID),
			DailyAvailability: data,
			OverallUptime:     availabilityOverall(rollups),
			WindowDays:        int64(days),
		},
		RequestId: requestID,
	})
}

// handleListAdminAccountsAvailability returns a current availability + uptime
// summary for every account, backing the admin monitoring page. It reads
// existing rollups (no per-account refresh) so the aggregate stays cheap; the
// per-account endpoint above is where a fresh recompute happens.
func (s *Server) handleListAdminAccountsAvailability(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	days := availabilityDefaultDays
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 || parsed > availabilityMaxDays {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid days parameter", requestID)
			return
		}
		days = parsed
	}

	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	now := time.Now().UTC()
	data := make([]apiopenapi.AccountAvailabilitySummary, 0, len(accounts))
	for _, account := range accounts {
		summary := apiopenapi.AccountAvailabilitySummary{
			AccountId:   int64(account.ID),
			AccountName: account.Name,
			ProviderId:  int64(account.ProviderID),
			Status:      string(account.Status),
			WindowDays:  int64(days),
		}
		active := strings.EqualFold(string(account.Status), "active")
		// Latest health probe drives the live status of an active account; a
		// disabled / suspended / dead account keeps its operational status.
		if snapshots, snapErr := s.runtime.accounts.ListHealthSnapshotsByAccount(r.Context(), account.ID, 1); snapErr == nil && len(snapshots) > 0 {
			latest := snapshots[0]
			checked := latest.SnapshotAt.UTC()
			summary.LastCheckedAt = &checked
			if active && strings.TrimSpace(latest.Status) != "" {
				summary.Status = latest.Status
			}
		} else if active {
			summary.Status = "unknown"
		}
		if rollups, rollErr := s.runtime.healthRollups.ListByAccount(r.Context(), account.ID, days, now); rollErr == nil {
			summary.OverallUptime = availabilityOverall(rollups)
		}
		data = append(data, summary)
	}

	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountsAvailabilitySummaryResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func healthSnapshotsToSamples(snapshots []accountcontract.AccountHealthSnapshot, since time.Time) []healthrollupscontract.Sample {
	samples := make([]healthrollupscontract.Sample, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.SnapshotAt.Before(since) {
			continue
		}
		samples = append(samples, healthrollupscontract.Sample{
			ProviderID:  snapshot.ProviderID,
			Healthy:     strings.EqualFold(snapshot.Status, "healthy"),
			SuccessRate: snapshot.SuccessRate,
			At:          snapshot.SnapshotAt,
		})
	}
	return samples
}

func toAPIAccountAvailabilityRollup(rollup healthrollupscontract.Rollup) apiopenapi.AccountAvailabilityRollup {
	return apiopenapi.AccountAvailabilityRollup{
		Date:              rollup.Date,
		ProviderId:        int64(rollup.ProviderID),
		TotalSamples:      int64(rollup.TotalSamples),
		HealthySamples:    int64(rollup.HealthySamples),
		AvailabilityRatio: rollup.AvailabilityRatio,
		AvgSuccessRate:    rollup.AvgSuccessRate,
		ComputedAt:        rollup.ComputedAt.UTC(),
	}
}

func availabilityOverall(rollups []healthrollupscontract.Rollup) float32 {
	total := 0
	healthy := 0
	for _, rollup := range rollups {
		total += rollup.TotalSamples
		healthy += rollup.HealthySamples
	}
	if total == 0 {
		return 0
	}
	return float32(healthy) / float32(total)
}

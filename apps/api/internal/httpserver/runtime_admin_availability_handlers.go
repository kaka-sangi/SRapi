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

type availabilityRollupPayload struct {
	Date              string    `json:"date"`
	ProviderID        int       `json:"provider_id"`
	TotalSamples      int       `json:"total_samples"`
	HealthySamples    int       `json:"healthy_samples"`
	AvailabilityRatio float32   `json:"availability_ratio"`
	AvgSuccessRate    float32   `json:"avg_success_rate"`
	ComputedAt        time.Time `json:"computed_at"`
}

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

	data := make([]availabilityRollupPayload, 0, len(rollups))
	for _, rollup := range rollups {
		data = append(data, availabilityRollupPayload{
			Date:              rollup.Date,
			ProviderID:        rollup.ProviderID,
			TotalSamples:      rollup.TotalSamples,
			HealthySamples:    rollup.HealthySamples,
			AvailabilityRatio: rollup.AvailabilityRatio,
			AvgSuccessRate:    rollup.AvgSuccessRate,
			ComputedAt:        rollup.ComputedAt.UTC(),
		})
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"account_id":         accountID,
			"window_days":        days,
			"overall_uptime":     availabilityOverall(rollups),
			"daily_availability": data,
		},
		"request_id": requestID,
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

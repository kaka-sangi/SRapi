package httpserver

import (
	"math/big"
	"net/http"
	"strconv"
	"time"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

const (
	// accountUsageDailyDefaultDays is the look-back window for usage-daily when
	// the ?days= query param is omitted.
	accountUsageDailyDefaultDays = 30
	// accountUsageDailyMaxDays caps the usage-daily look-back so a single read
	// cannot fan out into an unbounded number of day buckets.
	accountUsageDailyMaxDays = 365
	// accountUsageRecentWindow is the short "recent" rolling window reported by
	// usage-windows (now-5h .. now).
	accountUsageRecentWindow = 5 * time.Hour
	// accountUsageWeekWindow is the longer rolling window reported by
	// usage-windows (now-7d .. now).
	accountUsageWeekWindow = 7 * 24 * time.Hour
)

// usageWindowAccumulator collects per-window aggregates while scanning the
// account's usage logs once. Costs are summed as exact rationals and formatted
// at the end; tokens/requests are plain integer counters.
type usageWindowAccumulator struct {
	requests     int
	successCount int
	errorCount   int
	inputTokens  int
	outputTokens int
	totalTokens  int
	cost         *big.Rat
	currency     string
}

func newUsageWindowAccumulator() *usageWindowAccumulator {
	return &usageWindowAccumulator{cost: new(big.Rat)}
}

// add folds one usage log into the accumulator.
func (a *usageWindowAccumulator) add(log usagecontract.UsageLog) {
	a.requests++
	if log.Success {
		a.successCount++
	} else {
		a.errorCount++
	}
	a.inputTokens += log.InputTokens
	a.outputTokens += log.OutputTokens
	a.totalTokens += log.TotalTokens
	if rat, ok := money.DecimalRat(log.Cost); ok && rat != nil {
		a.cost.Add(a.cost, rat)
	}
	if a.currency == "" && log.Currency != "" {
		a.currency = log.Currency
	}
}

// currencyOrDefault returns the observed currency or the SRapi default when no
// log in the window carried one.
func (a *usageWindowAccumulator) currencyOrDefault() string {
	return money.NormalizeCurrency(a.currency)
}

// accountUsageLogs loads every usage log scoped to the given account id. It
// returns the filtered slice plus the count of total rows scanned (unused by
// callers beyond debugging, kept implicit). The whole table is loaded because
// the usage Store List contract has no account predicate; this mirrors the
// error-logs handler which also lists then filters in memory.
func (s *Server) accountUsageLogs(r *http.Request, accountID int) ([]usagecontract.UsageLog, error) {
	items, err := s.runtime.usage.List(r.Context())
	if err != nil {
		return nil, err
	}
	out := make([]usagecontract.UsageLog, 0, len(items))
	for _, item := range items {
		if item.AccountID != nil && *item.AccountID == accountID {
			out = append(out, item)
		}
	}
	return out, nil
}

// handleGetAdminAccountUsageWindows serves GET
// /api/v1/admin/accounts/{id}/usage-windows.
//
// It reports two rolling windows for the account: "5h" (now-5h .. now) and "7d"
// (now-7d .. now). For each window it sums requests, input/output/total tokens
// and cost, and splits the request count into success vs error by UsageLog
// Success. The 200 body is the inline {data, request_id} object the spec
// defines for this operation (no named wrapper was generated), so it is built
// with writeJSONAny.
func (s *Server) handleGetAdminAccountUsageWindows(w http.ResponseWriter, r *http.Request) {
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
	logs, err := s.accountUsageLogs(r, accountID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load account usage", requestID)
		return
	}

	now := time.Now().UTC()
	recentStart := now.Add(-accountUsageRecentWindow)
	weekStart := now.Add(-accountUsageWeekWindow)
	recent := newUsageWindowAccumulator()
	week := newUsageWindowAccumulator()
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if !created.Before(recentStart) && !created.After(now) {
			recent.add(log)
		}
		if !created.Before(weekStart) && !created.After(now) {
			week.add(log)
		}
	}

	result := apiopenapi.AccountUsageWindowsResult{
		AccountId: apiopenapi.Id(strconv.Itoa(accountID)),
		Windows: []apiopenapi.AccountUsageWindow{
			usageWindowDTO("5h", recent),
			usageWindowDTO("7d", week),
		},
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       result,
		"request_id": requestID,
	})
}

// usageWindowDTO maps one accumulated window onto the generated DTO. Cost is
// formatted to 2 decimal places as a plain decimal string.
func usageWindowDTO(label string, acc *usageWindowAccumulator) apiopenapi.AccountUsageWindow {
	return apiopenapi.AccountUsageWindow{
		Window:       label,
		Requests:     acc.requests,
		SuccessCount: acc.successCount,
		ErrorCount:   acc.errorCount,
		InputTokens:  acc.inputTokens,
		OutputTokens: acc.outputTokens,
		TotalTokens:  acc.totalTokens,
		Cost:         money.FormatRatFixed(acc.cost, 2),
		Currency:     acc.currencyOrDefault(),
	}
}

// handleGetAdminAccountUsageDaily serves GET
// /api/v1/admin/accounts/{id}/usage-daily.
//
// It groups the account's usage over the last N days (default 30, bounded by
// accountUsageDailyMaxDays) into one point per calendar day (UTC), summing
// requests, input/output tokens and cost. Days with no traffic are emitted as
// zero rows so the series is dense, oldest first.
func (s *Server) handleGetAdminAccountUsageDaily(w http.ResponseWriter, r *http.Request) {
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
	days := accountUsageDailyDefaultDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 || parsed > accountUsageDailyMaxDays {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid days parameter", requestID)
			return
		}
		days = parsed
	}
	logs, err := s.accountUsageLogs(r, accountID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load account usage", requestID)
		return
	}

	const dateLayout = "2006-01-02"
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// windowStart is the inclusive start-of-day for the oldest reported bucket.
	windowStart := todayStart.AddDate(0, 0, -(days - 1))

	type dailyBucket struct {
		requests     int
		inputTokens  int
		outputTokens int
		cost         *big.Rat
		currency     string
	}
	buckets := make(map[string]*dailyBucket, days)
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if created.Before(windowStart) || created.After(now) {
			continue
		}
		key := created.Format(dateLayout)
		bucket := buckets[key]
		if bucket == nil {
			bucket = &dailyBucket{cost: new(big.Rat)}
			buckets[key] = bucket
		}
		bucket.requests++
		bucket.inputTokens += log.InputTokens
		bucket.outputTokens += log.OutputTokens
		if rat, ok := money.DecimalRat(log.Cost); ok && rat != nil {
			bucket.cost.Add(bucket.cost, rat)
		}
		if bucket.currency == "" && log.Currency != "" {
			bucket.currency = log.Currency
		}
	}

	points := make([]apiopenapi.AccountUsageDailyPoint, 0, days)
	for day := windowStart; !day.After(todayStart); day = day.AddDate(0, 0, 1) {
		key := day.Format(dateLayout)
		point := apiopenapi.AccountUsageDailyPoint{
			Date:     key,
			Cost:     "0.00",
			Currency: money.NormalizeCurrency(""),
		}
		if bucket := buckets[key]; bucket != nil {
			point.Requests = bucket.requests
			point.InputTokens = bucket.inputTokens
			point.OutputTokens = bucket.outputTokens
			point.Cost = money.FormatRatFixed(bucket.cost, 2)
			point.Currency = money.NormalizeCurrency(bucket.currency)
		}
		points = append(points, point)
	}

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"account_id": apiopenapi.Id(strconv.Itoa(accountID)),
			"days":       days,
			"points":     points,
		},
		"request_id": requestID,
	})
}

// handleGetAdminAccountUsageToday serves GET
// /api/v1/admin/accounts/{id}/usage-today.
//
// It sums the account's usage from start-of-today (UTC) to now: requests,
// success/error counts, input/output/total tokens and cost, plus a success_rate
// (success/total) that is 0 when there has been no traffic today.
func (s *Server) handleGetAdminAccountUsageToday(w http.ResponseWriter, r *http.Request) {
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
	logs, err := s.accountUsageLogs(r, accountID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load account usage", requestID)
		return
	}

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	acc := newUsageWindowAccumulator()
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if created.Before(todayStart) || created.After(now) {
			continue
		}
		acc.add(log)
	}

	var successRate float32
	if acc.requests > 0 {
		successRate = float32(acc.successCount) / float32(acc.requests)
	}
	today := apiopenapi.AccountUsageToday{
		Requests:     acc.requests,
		SuccessCount: acc.successCount,
		ErrorCount:   acc.errorCount,
		SuccessRate:  successRate,
		InputTokens:  acc.inputTokens,
		OutputTokens: acc.outputTokens,
		TotalTokens:  acc.totalTokens,
		Cost:         money.FormatRatFixed(acc.cost, 2),
		Currency:     acc.currencyOrDefault(),
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       today,
		"request_id": requestID,
	})
}

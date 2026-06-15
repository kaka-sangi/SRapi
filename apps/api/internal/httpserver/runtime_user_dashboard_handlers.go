package httpserver

import (
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"time"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

const (
	// userDashboardThroughputWindow is the rolling window the throughput metric
	// (RPM/TPM plus per-minute peaks) is computed over.
	userDashboardThroughputWindow = 60 * time.Minute
	// userDashboardDefaultDays is the look-back window for the model-share and
	// trend metrics when the ?days= query param is omitted.
	userDashboardDefaultDays = 30
	// userDashboardMaxDays caps the look-back so a single read cannot fan out
	// into an unbounded number of buckets (mirrors the account-usage handlers).
	userDashboardMaxDays = 365
	// userDashboardCostPlaces is the fractional precision for the decimal cost
	// strings returned by these dashboard endpoints, matching the account-usage
	// handlers.
	userDashboardCostPlaces = 2
)

// currentUserUsageLogs resolves the current console user from the session the
// same way handleCurrentUserUsage does (requireConsoleSession, never
// requireAdminSession) and loads that user's usage logs. It returns the session
// user id alongside the logs so callers can build user-scoped responses.
//
// On any failure it writes the appropriate error response (401 for an
// unauthenticated request, 500 when the usage store read fails) and returns
// ok=false so the caller can return immediately.
func (s *Server) currentUserUsageLogs(w http.ResponseWriter, r *http.Request, requestID string) ([]usagecontract.UsageLog, bool) {
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return nil, false
	}
	logs, err := s.runtime.usage.ListByUser(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return nil, false
	}
	return logs, true
}

// userDashboardDays parses the optional ?days= query param shared by the
// model-share and trend endpoints, defaulting to userDashboardDefaultDays and
// rejecting non-positive or over-cap values.
func userDashboardDays(r *http.Request) (int, bool) {
	days := userDashboardDefaultDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > userDashboardMaxDays {
			return 0, false
		}
		days = parsed
	}
	return days, true
}

// handleGetCurrentUserUsageThroughput serves GET
// /api/v1/user/usage/dashboard/throughput.
//
// It reports the current user's request/token throughput over the last
// userDashboardThroughputWindow: rpm/tpm are the averaged per-minute rates and
// peak_rpm/peak_tpm are the busiest single minute observed in the window.
func (s *Server) handleGetCurrentUserUsageThroughput(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	logs, ok := s.currentUserUsageLogs(w, r, requestID)
	if !ok {
		return
	}

	now := time.Now().UTC()
	windowStart := now.Add(-userDashboardThroughputWindow)
	windowMinutes := int(userDashboardThroughputWindow / time.Minute)

	totalRequests := 0
	totalTokens := 0
	// Per-minute buckets keyed by the truncated minute so peaks reflect the
	// busiest single minute rather than the averaged rate.
	requestsByMinute := make(map[time.Time]int)
	tokensByMinute := make(map[time.Time]int)
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if created.Before(windowStart) || created.After(now) {
			continue
		}
		totalRequests++
		totalTokens += log.TotalTokens
		minute := created.Truncate(time.Minute)
		requestsByMinute[minute]++
		tokensByMinute[minute] += log.TotalTokens
	}

	peakRpm := 0
	for _, count := range requestsByMinute {
		if count > peakRpm {
			peakRpm = count
		}
	}
	peakTpm := 0
	for _, count := range tokensByMinute {
		if count > peakTpm {
			peakTpm = count
		}
	}

	data := apiopenapi.UsageThroughput{
		Rpm:           float32(totalRequests) / float32(windowMinutes),
		Tpm:           float32(totalTokens) / float32(windowMinutes),
		PeakRpm:       float32(peakRpm),
		PeakTpm:       float32(peakTpm),
		TotalRequests: totalRequests,
		TotalTokens:   totalTokens,
		WindowMinutes: windowMinutes,
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageThroughputResponse{
		Data:      data,
		RequestId: requestID,
	})
}

// handleGetCurrentUserUsageModels serves GET
// /api/v1/user/usage/dashboard/models.
//
// It groups the current user's usage over the last N days (default 30, bounded
// by userDashboardMaxDays) by model, summing requests, input/output/total
// tokens and cost. Rows are returned most-used first (by request count) so the
// dominant models surface at the top of the dashboard.
func (s *Server) handleGetCurrentUserUsageModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	days, ok := userDashboardDays(r)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid days parameter", requestID)
		return
	}
	logs, ok := s.currentUserUsageLogs(w, r, requestID)
	if !ok {
		return
	}

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	windowStart := todayStart.AddDate(0, 0, -(days - 1))

	type modelBucket struct {
		model        string
		requests     int
		inputTokens  int
		outputTokens int
		totalTokens  int
		cost         *big.Rat
		currency     string
	}
	buckets := make(map[string]*modelBucket)
	// order preserves first-seen model keys so the final sort is deterministic
	// for models with equal request counts.
	order := make([]string, 0)
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if created.Before(windowStart) || created.After(now) {
			continue
		}
		bucket := buckets[log.Model]
		if bucket == nil {
			bucket = &modelBucket{model: log.Model, cost: new(big.Rat)}
			buckets[log.Model] = bucket
			order = append(order, log.Model)
		}
		bucket.requests++
		bucket.inputTokens += log.InputTokens
		bucket.outputTokens += log.OutputTokens
		bucket.totalTokens += log.TotalTokens
		if rat, ok := money.DecimalRat(log.Cost); ok && rat != nil {
			bucket.cost.Add(bucket.cost, rat)
		}
		if bucket.currency == "" && log.Currency != "" {
			bucket.currency = log.Currency
		}
	}

	// Sort most-used first, breaking ties by total tokens then first-seen order
	// (the order slice is already insertion-ordered, and SliceStable keeps it for
	// full ties).
	sortedKeys := make([]string, len(order))
	copy(sortedKeys, order)
	sort.SliceStable(sortedKeys, func(i, j int) bool {
		a, b := buckets[sortedKeys[i]], buckets[sortedKeys[j]]
		if a.requests != b.requests {
			return a.requests > b.requests
		}
		return a.totalTokens > b.totalTokens
	})

	data := make([]apiopenapi.UsageModelShare, 0, len(sortedKeys))
	for _, key := range sortedKeys {
		b := buckets[key]
		data = append(data, apiopenapi.UsageModelShare{
			Model:        b.model,
			Requests:     b.requests,
			InputTokens:  b.inputTokens,
			OutputTokens: b.outputTokens,
			TotalTokens:  b.totalTokens,
			Cost:         money.FormatRatFixed(b.cost, userDashboardCostPlaces),
			Currency:     money.NormalizeCurrency(b.currency),
		})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageModelShareListResponse{
		Data:      data,
		RequestId: requestID,
	})
}

// handleGetCurrentUserUsageTrend serves GET /api/v1/user/usage/dashboard/trend.
//
// It groups the current user's usage over the last N days (default 30, bounded
// by userDashboardMaxDays) into dense, oldest-first buckets keyed by day or
// hour (?bucket=day|hour, default day), summing requests, input/output tokens
// and cost. Empty buckets are emitted as zero rows so charts render a
// continuous series.
func (s *Server) handleGetCurrentUserUsageTrend(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	days, ok := userDashboardDays(r)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid days parameter", requestID)
		return
	}
	bucketKind := r.URL.Query().Get("bucket")
	if bucketKind == "" {
		bucketKind = "day"
	}
	if bucketKind != "day" && bucketKind != "hour" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid bucket parameter", requestID)
		return
	}
	logs, ok := s.currentUserUsageLogs(w, r, requestID)
	if !ok {
		return
	}

	const (
		dayLayout  = "2006-01-02"
		hourLayout = "2006-01-02T15"
	)
	layout := dayLayout
	step := func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }
	truncate := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	if bucketKind == "hour" {
		layout = hourLayout
		step = func(t time.Time) time.Time { return t.Add(time.Hour) }
		truncate = func(t time.Time) time.Time { return t.Truncate(time.Hour).UTC() }
	}

	now := time.Now().UTC()
	endBucket := truncate(now)
	startBucket := truncate(now).AddDate(0, 0, -(days - 1))

	type trendBucket struct {
		requests     int
		inputTokens  int
		outputTokens int
		cost         *big.Rat
		currency     string
	}
	buckets := make(map[string]*trendBucket)
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if created.Before(startBucket) || created.After(now) {
			continue
		}
		key := truncate(created).Format(layout)
		bucket := buckets[key]
		if bucket == nil {
			bucket = &trendBucket{cost: new(big.Rat)}
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

	data := make([]apiopenapi.UsageTrendPoint, 0)
	for cursor := startBucket; !cursor.After(endBucket); cursor = step(cursor) {
		key := cursor.Format(layout)
		point := apiopenapi.UsageTrendPoint{
			Bucket:   key,
			Cost:     money.FormatRatFixed(new(big.Rat), userDashboardCostPlaces),
			Currency: money.NormalizeCurrency(""),
		}
		if bucket := buckets[key]; bucket != nil {
			point.Requests = bucket.requests
			point.InputTokens = bucket.inputTokens
			point.OutputTokens = bucket.outputTokens
			point.Cost = money.FormatRatFixed(bucket.cost, userDashboardCostPlaces)
			point.Currency = money.NormalizeCurrency(bucket.currency)
		}
		data = append(data, point)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageTrendPointListResponse{
		Data:      data,
		RequestId: requestID,
	})
}

// handleGetCurrentUserUsageCacheMetrics serves GET
// /api/v1/user/usage/dashboard/cache-metrics.
//
// It sums the current user's prompt-cache activity across all logged usage:
// cache-read tokens (UsageLog.CachedTokens), cache-creation tokens and total
// (non-cached) input tokens. cache_hit_rate is cache-read tokens divided by all
// prompt tokens seen (cache-read + input), and cache_cost_saved is the realized
// billing savings summed from each log's CacheReadCost.
func (s *Server) handleGetCurrentUserUsageCacheMetrics(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	logs, ok := s.currentUserUsageLogs(w, r, requestID)
	if !ok {
		return
	}

	cacheReadTokens := 0
	cacheCreationTokens := 0
	totalInputTokens := 0
	costSaved := new(big.Rat)
	currency := ""
	for _, log := range logs {
		cacheReadTokens += log.CachedTokens
		cacheCreationTokens += log.CacheCreationTokens
		totalInputTokens += log.InputTokens
		if rat, ok := money.DecimalRat(log.CacheReadCost); ok && rat != nil {
			costSaved.Add(costSaved, rat)
		}
		if currency == "" && log.Currency != "" {
			currency = log.Currency
		}
	}

	var cacheHitRate float32
	if promptTokens := cacheReadTokens + totalInputTokens; promptTokens > 0 {
		cacheHitRate = float32(cacheReadTokens) / float32(promptTokens)
	}

	data := apiopenapi.UsageCacheMetrics{
		CacheReadTokens:     cacheReadTokens,
		CacheCreationTokens: cacheCreationTokens,
		TotalInputTokens:    totalInputTokens,
		CacheHitRate:        cacheHitRate,
		CacheCostSaved:      money.FormatRatFixed(costSaved, userDashboardCostPlaces),
		Currency:            money.NormalizeCurrency(currency),
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageCacheMetricsResponse{
		Data:      data,
		RequestId: requestID,
	})
}

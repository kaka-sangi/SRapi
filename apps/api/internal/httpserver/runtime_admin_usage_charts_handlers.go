package httpserver

import (
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

const (
	// usageTrendDefaultLimit is the number of series kept (top-N by total
	// requests) when ?limit= is omitted.
	usageTrendDefaultLimit = 8
	// usageTrendMaxLimit caps ?limit= so a single read cannot fan out into an
	// unbounded number of series.
	usageTrendMaxLimit = 50
	// usageTrendDayLayout buckets a usage log to a calendar day (UTC).
	usageTrendDayLayout = "2006-01-02"
	// usageTrendHourLayout buckets a usage log to the start of an hour (UTC).
	usageTrendHourLayout = "2006-01-02T15"
	// usageTrendUnknownLabel is the series label used when a log carries no value
	// for the chosen dimension.
	usageTrendUnknownLabel = "unknown"
	// usageErrorUnknownClass is the bucket for failed logs with no error_class.
	usageErrorUnknownClass = "unknown"
)

// usageTrendBucket accumulates the per-(series,bucket) totals while scanning the
// usage logs once. Costs are summed as exact rationals and formatted at the end.
type usageTrendBucket struct {
	requests     int
	inputTokens  int
	outputTokens int
	cost         *big.Rat
	currency     string
}

func newUsageTrendBucket() *usageTrendBucket {
	return &usageTrendBucket{cost: new(big.Rat)}
}

func (b *usageTrendBucket) add(log usagecontract.UsageLog) {
	b.requests++
	b.inputTokens += log.InputTokens
	b.outputTokens += log.OutputTokens
	if rat, ok := money.DecimalRat(log.Cost); ok && rat != nil {
		b.cost.Add(b.cost, rat)
	}
	if b.currency == "" && log.Currency != "" {
		b.currency = log.Currency
	}
}

// usageTrendSeriesAcc tracks one dimension value's buckets plus its running
// total request count (used to pick the top-N series).
type usageTrendSeriesAcc struct {
	label         string
	totalRequests int
	buckets       map[string]*usageTrendBucket
}

// handleGetAdminUsageTrends serves GET /api/v1/admin/usage/trends.
//
// It buckets every usage log by day (default) or hour and groups the buckets by
// the chosen dimension (model, account or source_endpoint), then keeps only the
// top-N series by total requests (default 8). Each series is emitted as a dense,
// oldest-first run of points spanning the full observed bucket range so the
// frontend can render a continuous line per series (gaps are zero-filled).
//
// start/end accept RFC3339 or a bare YYYY-MM-DD date and bound CreatedAt
// inclusively, matching the admin usage-log filters. The 200 body is the inline
// {data, request_id} object built with writeJSONAny.
func (s *Server) handleGetAdminUsageTrends(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}

	query := r.URL.Query()

	bucketParam := apiopenapi.GetAdminUsageTrendsParamsBucketDay
	if raw := strings.TrimSpace(query.Get("bucket")); raw != "" {
		candidate := apiopenapi.GetAdminUsageTrendsParamsBucket(raw)
		if !candidate.Valid() {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid bucket parameter", requestID)
			return
		}
		bucketParam = candidate
	}

	dimensionParam := apiopenapi.GetAdminUsageTrendsParamsDimensionModel
	if raw := strings.TrimSpace(query.Get("dimension")); raw != "" {
		candidate := apiopenapi.GetAdminUsageTrendsParamsDimension(raw)
		if !candidate.Valid() {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid dimension parameter", requestID)
			return
		}
		dimensionParam = candidate
	}

	limit := usageTrendDefaultLimit
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 || parsed > usageTrendMaxLimit {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid limit parameter", requestID)
			return
		}
		limit = parsed
	}

	start, end, ok := parseUsageTrendRange(query.Get("start"), query.Get("end"))
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "start must be before end", requestID)
		return
	}

	logs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load usage", requestID)
		return
	}

	layout := usageTrendDayLayout
	if bucketParam == apiopenapi.GetAdminUsageTrendsParamsBucketHour {
		layout = usageTrendHourLayout
	}

	series := map[string]*usageTrendSeriesAcc{}
	bucketKeys := map[string]struct{}{}
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if !start.IsZero() && created.Before(start) {
			continue
		}
		if !end.IsZero() && created.After(end) {
			continue
		}
		label := usageTrendDimensionLabel(log, dimensionParam)
		bucketKey := created.Format(layout)
		bucketKeys[bucketKey] = struct{}{}

		acc := series[label]
		if acc == nil {
			acc = &usageTrendSeriesAcc{label: label, buckets: map[string]*usageTrendBucket{}}
			series[label] = acc
		}
		bucket := acc.buckets[bucketKey]
		if bucket == nil {
			bucket = newUsageTrendBucket()
			acc.buckets[bucketKey] = bucket
		}
		bucket.add(log)
		acc.totalRequests++
	}

	orderedBuckets := make([]string, 0, len(bucketKeys))
	for key := range bucketKeys {
		orderedBuckets = append(orderedBuckets, key)
	}
	sort.Strings(orderedBuckets)

	topSeries := topUsageTrendSeries(series, limit)
	result := apiopenapi.UsageTrendSeriesResult{
		Bucket:    string(bucketParam),
		Dimension: string(dimensionParam),
		Series:    make([]apiopenapi.UsageTrendSeries, 0, len(topSeries)),
	}
	for _, acc := range topSeries {
		points := make([]apiopenapi.UsageTrendSeriesPoint, 0, len(orderedBuckets))
		for _, bucketKey := range orderedBuckets {
			point := apiopenapi.UsageTrendSeriesPoint{
				Bucket:   bucketKey,
				Cost:     "0.00",
				Currency: money.NormalizeCurrency(""),
			}
			if bucket := acc.buckets[bucketKey]; bucket != nil {
				point.Requests = bucket.requests
				point.InputTokens = bucket.inputTokens
				point.OutputTokens = bucket.outputTokens
				point.Cost = money.FormatRatFixed(bucket.cost, 2)
				point.Currency = money.NormalizeCurrency(bucket.currency)
			}
			points = append(points, point)
		}
		result.Series = append(result.Series, apiopenapi.UsageTrendSeries{
			Label:  acc.label,
			Points: points,
		})
	}

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       result,
		"request_id": requestID,
	})
}

// handleGetAdminUsageErrorDistribution serves GET
// /api/v1/admin/usage/error-distribution.
//
// Among the usage logs that failed (Success == false) it groups by error_class
// (a nil class is bucketed as "unknown"), counts each class and computes its
// percentage share of the total error count. Buckets are sorted by count
// descending (ties broken by class name) so the largest contributors come first.
//
// start/end accept RFC3339 or a bare YYYY-MM-DD date and bound CreatedAt
// inclusively. The 200 body is the inline {data, request_id} object built with
// writeJSONAny.
func (s *Server) handleGetAdminUsageErrorDistribution(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}

	query := r.URL.Query()
	start, end, ok := parseUsageTrendRange(query.Get("start"), query.Get("end"))
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "start must be before end", requestID)
		return
	}

	logs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load usage", requestID)
		return
	}

	counts := map[string]int{}
	totalErrors := 0
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if !start.IsZero() && created.Before(start) {
			continue
		}
		if !end.IsZero() && created.After(end) {
			continue
		}
		if log.Success {
			continue
		}
		class := usageErrorUnknownClass
		if log.ErrorClass != nil {
			if trimmed := strings.TrimSpace(*log.ErrorClass); trimmed != "" {
				class = trimmed
			}
		}
		counts[class]++
		totalErrors++
	}

	buckets := make([]apiopenapi.UsageErrorBucket, 0, len(counts))
	for class, count := range counts {
		var percentage float32
		if totalErrors > 0 {
			percentage = float32(count) / float32(totalErrors) * 100
		}
		buckets = append(buckets, apiopenapi.UsageErrorBucket{
			Count:      count,
			ErrorClass: class,
			Percentage: percentage,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Count != buckets[j].Count {
			return buckets[i].Count > buckets[j].Count
		}
		return buckets[i].ErrorClass < buckets[j].ErrorClass
	})

	// data is a bare UsageErrorBucket array per the OpenAPI contract (each bucket
	// already carries its percentage; total is derivable by summing counts).
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       buckets,
		"request_id": requestID,
	})
}

// usageDistributionAcc accumulates one dimension value's totals while scanning
// the usage logs once. Cost is summed as an exact rational and formatted at the
// end; the share percentage is computed against the chosen metric total.
type usageDistributionAcc struct {
	label        string
	requests     int
	inputTokens  int
	outputTokens int
	totalTokens  int
	cost         *big.Rat
	currency     string
}

// metricValue returns this bucket's contribution under the chosen share metric
// (request count, total tokens, or summed cost as a float for ranking/share).
func (a *usageDistributionAcc) metricValue(metric apiopenapi.GetAdminUsageDistributionParamsMetric) float64 {
	switch metric {
	case apiopenapi.Tokens:
		return float64(a.totalTokens)
	case apiopenapi.Cost:
		v, _ := a.cost.Float64()
		return v
	default: // requests
		return float64(a.requests)
	}
}

// handleGetAdminUsageDistribution serves GET /api/v1/admin/usage/distribution.
//
// It groups every usage log by the chosen dimension (model, requested/upstream
// model, account, provider, api_key, source_endpoint, billing_mode or user) and
// reports each group's requests, input/output/total tokens and summed cost. Each
// bucket's percentage is its share of the chosen metric (requests, tokens or
// cost) across all groups. Buckets are sorted by that metric descending and
// capped at the top-N (default 8). start/end bound CreatedAt inclusively.
//
// This is the share-by-dimension complement to the time-series trends endpoint:
// a single call powers the model / endpoint / provider / billing-mode / api-key
// / per-user distribution charts. The 200 body is the inline {data, request_id}
// object built with writeJSONAny.
func (s *Server) handleGetAdminUsageDistribution(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}

	query := r.URL.Query()

	dimensionParam := apiopenapi.GetAdminUsageDistributionParamsDimensionModel
	if raw := strings.TrimSpace(query.Get("dimension")); raw != "" {
		candidate := apiopenapi.GetAdminUsageDistributionParamsDimension(raw)
		if !candidate.Valid() {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid dimension parameter", requestID)
			return
		}
		dimensionParam = candidate
	}

	metricParam := apiopenapi.Requests
	if raw := strings.TrimSpace(query.Get("metric")); raw != "" {
		candidate := apiopenapi.GetAdminUsageDistributionParamsMetric(raw)
		if !candidate.Valid() {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid metric parameter", requestID)
			return
		}
		metricParam = candidate
	}

	limit := usageTrendDefaultLimit
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 || parsed > usageTrendMaxLimit {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid limit parameter", requestID)
			return
		}
		limit = parsed
	}

	start, end, ok := parseUsageTrendRange(query.Get("start"), query.Get("end"))
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "start must be before end", requestID)
		return
	}

	logs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load usage", requestID)
		return
	}

	groups := map[string]*usageDistributionAcc{}
	var total float64
	for _, log := range logs {
		created := log.CreatedAt.UTC()
		if !start.IsZero() && created.Before(start) {
			continue
		}
		if !end.IsZero() && created.After(end) {
			continue
		}
		label := usageDistributionDimensionLabel(log, dimensionParam)
		acc := groups[label]
		if acc == nil {
			acc = &usageDistributionAcc{label: label, cost: new(big.Rat)}
			groups[label] = acc
		}
		acc.requests++
		acc.inputTokens += log.InputTokens
		acc.outputTokens += log.OutputTokens
		acc.totalTokens += log.TotalTokens
		if rat, costOK := money.DecimalRat(log.Cost); costOK && rat != nil {
			acc.cost.Add(acc.cost, rat)
		}
		if acc.currency == "" && log.Currency != "" {
			acc.currency = log.Currency
		}
	}

	ordered := make([]*usageDistributionAcc, 0, len(groups))
	for _, acc := range groups {
		ordered = append(ordered, acc)
		total += acc.metricValue(metricParam)
	}
	sort.Slice(ordered, func(i, j int) bool {
		vi, vj := ordered[i].metricValue(metricParam), ordered[j].metricValue(metricParam)
		if vi != vj {
			return vi > vj
		}
		return ordered[i].label < ordered[j].label
	})
	if limit > 0 && len(ordered) > limit {
		ordered = ordered[:limit]
	}

	buckets := make([]apiopenapi.UsageDistributionBucket, 0, len(ordered))
	for _, acc := range ordered {
		var percentage float32
		if total > 0 {
			percentage = float32(acc.metricValue(metricParam) / total * 100)
		}
		buckets = append(buckets, apiopenapi.UsageDistributionBucket{
			Label:        acc.label,
			Requests:     acc.requests,
			InputTokens:  acc.inputTokens,
			OutputTokens: acc.outputTokens,
			TotalTokens:  acc.totalTokens,
			Cost:         money.FormatRatFixed(acc.cost, 2),
			Currency:     money.NormalizeCurrency(acc.currency),
			Percentage:   percentage,
		})
	}

	result := apiopenapi.UsageDistributionResult{
		Dimension: string(dimensionParam),
		Metric:    string(metricParam),
		Buckets:   buckets,
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       result,
		"request_id": requestID,
	})
}

// usageDistributionDimensionLabel resolves the bucket label for one log under
// the chosen distribution dimension. Numeric foreign keys (account, provider,
// api_key, user) are stringified; a missing value falls back to "unknown" so
// every log lands in some bucket.
func usageDistributionDimensionLabel(log usagecontract.UsageLog, dimension apiopenapi.GetAdminUsageDistributionParamsDimension) string {
	var value string
	switch dimension {
	case apiopenapi.GetAdminUsageDistributionParamsDimensionRequestedModel:
		value = log.RequestedModel
	case apiopenapi.GetAdminUsageDistributionParamsDimensionUpstreamModel:
		value = log.UpstreamModel
	case apiopenapi.GetAdminUsageDistributionParamsDimensionAccount:
		if log.AccountID != nil {
			value = strconv.Itoa(*log.AccountID)
		}
	case apiopenapi.GetAdminUsageDistributionParamsDimensionProvider:
		if log.ProviderID != nil {
			value = strconv.Itoa(*log.ProviderID)
		}
	case apiopenapi.GetAdminUsageDistributionParamsDimensionApiKey:
		if log.APIKeyID > 0 {
			value = strconv.Itoa(log.APIKeyID)
		}
	case apiopenapi.GetAdminUsageDistributionParamsDimensionSourceEndpoint:
		value = log.SourceEndpoint
	case apiopenapi.GetAdminUsageDistributionParamsDimensionBillingMode:
		value = log.BillingMode
	case apiopenapi.GetAdminUsageDistributionParamsDimensionUser:
		if log.UserID > 0 {
			value = strconv.Itoa(log.UserID)
		}
	default: // model
		value = log.Model
	}
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return usageTrendUnknownLabel
}

// usageTrendDimensionLabel resolves the series label for one log under the
// chosen dimension. account uses the numeric account id as a string; model and
// source_endpoint use the corresponding UsageLog field. A missing value falls
// back to "unknown" so every log lands in some series.
func usageTrendDimensionLabel(log usagecontract.UsageLog, dimension apiopenapi.GetAdminUsageTrendsParamsDimension) string {
	var value string
	switch dimension {
	case apiopenapi.GetAdminUsageTrendsParamsDimensionAccount:
		if log.AccountID != nil {
			value = strconv.Itoa(*log.AccountID)
		}
	case apiopenapi.GetAdminUsageTrendsParamsDimensionSourceEndpoint:
		value = log.SourceEndpoint
	default: // model
		value = log.Model
	}
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return usageTrendUnknownLabel
}

// topUsageTrendSeries returns at most limit series ordered by total requests
// descending (ties broken by label) so the busiest dimensions are kept.
func topUsageTrendSeries(series map[string]*usageTrendSeriesAcc, limit int) []*usageTrendSeriesAcc {
	ordered := make([]*usageTrendSeriesAcc, 0, len(series))
	for _, acc := range series {
		ordered = append(ordered, acc)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].totalRequests != ordered[j].totalRequests {
			return ordered[i].totalRequests > ordered[j].totalRequests
		}
		return ordered[i].label < ordered[j].label
	})
	if limit > 0 && len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered
}

// parseUsageTrendRange parses the optional start/end query values (RFC3339 or a
// bare YYYY-MM-DD date, reusing parseUsageFilterTime). It returns ok == false
// only when both bounds are present and start is strictly after end; empty or
// unparseable values are treated as no bound (zero time).
func parseUsageTrendRange(startRaw, endRaw string) (time.Time, time.Time, bool) {
	start := parseUsageFilterTime(startRaw).UTC()
	end := parseUsageFilterTime(endRaw).UTC()
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

package httpserver

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type usageBucket struct {
	Start        time.Time
	RequestCount int
	ErrorCount   int
	TokenCount   int
	Cost         string
}

func (s *Server) handleAdminDashboardSnapshot(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	window := requestWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"), 24*time.Hour)
	snapshot, err := s.adminDashboardSnapshot(r, window)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to build dashboard snapshot", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminDashboardSnapshotResponse{Data: snapshot, RequestId: requestID})
}

func (s *Server) handleAdminOpsOverview(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	window := requestWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"), time.Hour)
	logs, err := s.runtime.usage.ListFiltered(r.Context(), usagecontract.QueryFilter{Start: &window.Start, End: &window.End})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsOverviewResponse{Data: opsOverview(logs, window), RequestId: requestID})
}

func (s *Server) handleAdminOpsThroughputTrend(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	window := requestWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"), 24*time.Hour)
	bucket := trendBucket(r.URL.Query().Get("bucket"))
	logs, err := s.runtime.usage.ListFiltered(r.Context(), usagecontract.QueryFilter{Start: &window.Start, End: &window.End})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	points := throughputTrendPoints(bucketUsageLogs(logs, bucket), bucket)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsThroughputTrendResponse{
		Data: apiopenapi.OpsThroughputTrend{
			Window: window,
			Bucket: apiopenapi.OpsThroughputTrendBucket(bucket),
			Points: points,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleAdminOpsErrorTrend(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	window := requestWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"), 24*time.Hour)
	bucket := trendBucket(r.URL.Query().Get("bucket"))
	logs, err := s.runtime.usage.ListFiltered(r.Context(), usagecontract.QueryFilter{Start: &window.Start, End: &window.End})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsErrorTrendResponse{
		Data: apiopenapi.OpsErrorTrend{
			Window: window,
			Bucket: apiopenapi.OpsErrorTrendBucket(bucket),
			Points: errorTrendPoints(bucketUsageLogs(logs, bucket)),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleAdminOpsErrorDistribution(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	window := requestWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"), 24*time.Hour)
	logs, err := s.runtime.usage.ListFiltered(r.Context(), usagecontract.QueryFilter{Start: &window.Start, End: &window.End})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsErrorDistributionResponse{
		Data:      apiopenapi.OpsErrorDistribution{Window: window, Items: errorDistribution(logs)},
		RequestId: requestID,
	})
}

func (s *Server) handleAdminOpsLatencyHistogram(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	window := requestWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"), 24*time.Hour)
	logs, err := s.runtime.usage.ListFiltered(r.Context(), usagecontract.QueryFilter{Start: &window.Start, End: &window.End})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsLatencyHistogramResponse{
		Data:      apiopenapi.OpsLatencyHistogram{Window: window, Buckets: latencyHistogram(logs)},
		RequestId: requestID,
	})
}

func (s *Server) handleAdminOpsConcurrency(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	active, err := s.runtime.realtime.ListActiveSlots(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list active realtime slots", requestID)
		return
	}
	byKey := make(map[string]int, len(active.ActiveByAPIKeyID))
	for id, count := range active.ActiveByAPIKeyID {
		byKey[strconv.Itoa(id)] = count
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsConcurrencyResponse{
		Data: apiopenapi.OpsConcurrency{
			// Live count of pending scheduler leases — the same gauge the
			// Prometheus exporter publishes as srapi_gateway_inflight_requests
			// (runtime_metrics.go:354). Previously hardcoded to 0, which made
			// the ops dashboard's "in-flight" tile permanently say zero.
			ActiveGatewayRequests: s.runtime.scheduler.ActiveLeaseCount(r.Context()),
			ActiveRealtimeSlots:   active.Snapshot.ActiveSlots,
			ActiveByApiKey:        byKey,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminOpsSystemLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	list, err := s.runtime.adminControl.ListSystemLogs(r.Context(), systemLogListOptionsFromRequest(r))
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsSystemLogListResponse{
		Data:       toAPIOpsSystemLogs(list.Items),
		Pagination: paginationWithRequest(r, list.Total),
		RequestId:  requestID,
	})
}

func (s *Server) handleCleanupAdminOpsSystemLogs(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.OpsSystemLogCleanupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid system log cleanup request", requestID)
		return
	}
	result, err := s.runtime.adminControl.CleanupSystemLogs(r.Context(), systemLogCleanupFilterFromAPI(body))
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_system_log.cleanup", "ops_system_log", "bulk", nil, systemLogCleanupAuditSnapshot(body, result)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsSystemLogCleanupResponse{
		Data:      toAPISystemLogCleanupResult(result),
		RequestId: requestID,
	})
}

func systemLogCleanupFilterFromAPI(body apiopenapi.OpsSystemLogCleanupRequest) admincontrol.SystemLogCleanupFilter {
	var level admincontrol.OpsSystemLogLevel
	if body.Level != nil {
		level = admincontrol.OpsSystemLogLevel(*body.Level)
	}
	var start, end *time.Time
	if body.Start != nil {
		startValue := (*body.Start).UTC()
		start = &startValue
	}
	if body.End != nil {
		endValue := (*body.End).UTC()
		end = &endValue
	}
	var maxDelete int
	if body.MaxDelete != nil {
		maxDelete = *body.MaxDelete
	}
	var dryRun bool
	if body.DryRun != nil {
		dryRun = *body.DryRun
	}
	return admincontrol.SystemLogCleanupFilter{
		Level:     level,
		Source:    optionalStringValue(body.Source),
		Query:     optionalStringValue(body.Q),
		Start:     start,
		End:       end,
		DryRun:    dryRun,
		MaxDelete: maxDelete,
	}
}

func toAPISystemLogCleanupResult(result admincontrol.SystemLogCleanupResult) apiopenapi.OpsSystemLogCleanupResult {
	return apiopenapi.OpsSystemLogCleanupResult{
		Deleted:   result.Deleted,
		DryRun:    result.DryRun,
		Limited:   result.Limited,
		Matched:   result.Matched,
		MaxDelete: result.MaxDelete,
	}
}

func systemLogCleanupAuditSnapshot(body apiopenapi.OpsSystemLogCleanupRequest, result admincontrol.SystemLogCleanupResult) map[string]any {
	snapshot := map[string]any{
		"dry_run":    result.DryRun,
		"limited":    result.Limited,
		"matched":    result.Matched,
		"deleted":    result.Deleted,
		"max_delete": result.MaxDelete,
	}
	if body.Level != nil {
		snapshot["level"] = string(*body.Level)
	}
	if source := strings.TrimSpace(optionalStringValue(body.Source)); source != "" {
		snapshot["source"] = source
	}
	if body.Start != nil {
		snapshot["start"] = (*body.Start).UTC()
	}
	if body.End != nil {
		snapshot["end"] = (*body.End).UTC()
	}
	return snapshot
}

func (s *Server) adminDashboardSnapshot(r *http.Request, window apiopenapi.TimeWindow) (apiopenapi.AdminDashboardSnapshot, error) {
	ctx := r.Context()
	apiKeys, err := s.runtime.apiKeys.List(ctx)
	if err != nil {
		return apiopenapi.AdminDashboardSnapshot{}, err
	}
	accounts, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return apiopenapi.AdminDashboardSnapshot{}, err
	}
	users, err := s.runtime.users.List(ctx, usersservice.ListRequest{})
	if err != nil {
		return apiopenapi.AdminDashboardSnapshot{}, err
	}
	logs, err := s.runtime.usage.List(ctx)
	if err != nil {
		return apiopenapi.AdminDashboardSnapshot{}, err
	}
	today := startOfDay(time.Now().UTC())
	windowLogs := filterLogsByWindow(logs, window.Start, window.End)
	todayLogs := filterLogsByWindow(logs, today, time.Now().UTC())
	return apiopenapi.AdminDashboardSnapshot{
		Window:            window,
		Inventory:         dashboardInventory(apiKeys, accounts),
		Traffic:           dashboardTraffic(logs, todayLogs),
		Users:             dashboardUsers(users, windowLogs, today),
		Tokens:            dashboardTokens(logs, todayLogs),
		Performance:       dashboardPerformance(logs, windowLogs),
		ModelDistribution: dashboardModelDistribution(windowLogs),
		TokenTrend:        dashboardTokenTrend(bucketUsageLogs(windowLogs, "hour")),
		UserUsageTrend:    dashboardUserUsageTrend(windowLogs, users),
		GeneratedAt:       time.Now().UTC(),
	}, nil
}

func requestWindow(startRaw, endRaw string, fallback time.Duration) apiopenapi.TimeWindow {
	end := time.Now().UTC()
	start := end.Add(-fallback)
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(startRaw)); err == nil {
		start = parsed.UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(endRaw)); err == nil {
		end = parsed.UTC()
	}
	if !end.After(start) {
		end = start.Add(fallback)
	}
	return apiopenapi.TimeWindow{Start: start, End: end}
}

func trendBucket(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "day") {
		return "day"
	}
	return "hour"
}

func dashboardInventory(keys []apikeycontract.APIKey, accounts []accountcontract.ProviderAccount) apiopenapi.AdminDashboardInventory {
	activeKeys := 0
	for _, key := range keys {
		if key.Status == apikeycontract.StatusActive {
			activeKeys++
		}
	}
	healthyAccounts := 0
	for _, account := range accounts {
		if account.Status == accountcontract.StatusActive {
			healthyAccounts++
		}
	}
	return apiopenapi.AdminDashboardInventory{
		TotalApiKeys:     len(keys),
		ActiveApiKeys:    activeKeys,
		TotalAccounts:    len(accounts),
		HealthyAccounts:  healthyAccounts,
		AbnormalAccounts: len(accounts) - healthyAccounts,
	}
}

func dashboardTraffic(allLogs, todayLogs []usagecontract.UsageLog) apiopenapi.AdminDashboardTraffic {
	success, errors := successErrorCounts(allLogs)
	return apiopenapi.AdminDashboardTraffic{
		TodayRequests:   len(todayLogs),
		TotalRequests:   len(allLogs),
		SuccessRequests: success,
		ErrorRequests:   errors,
	}
}

func dashboardUsers(users []userscontract.StoredUser, windowLogs []usagecontract.UsageLog, today time.Time) apiopenapi.AdminDashboardUsers {
	todayNew := 0
	for _, user := range users {
		if !user.CreatedAt.Before(today) {
			todayNew++
		}
	}
	return apiopenapi.AdminDashboardUsers{
		TodayNewUsers: todayNew,
		TotalUsers:    len(users),
		ActiveUsers:   len(distinctUsers(windowLogs)),
	}
}

func dashboardTokens(allLogs, todayLogs []usagecontract.UsageLog) apiopenapi.AdminDashboardTokens {
	totalCost, currency := costSum(allLogs)
	return apiopenapi.AdminDashboardTokens{
		TodayTokens:  tokenSum(todayLogs),
		TotalTokens:  tokenSum(allLogs),
		InputTokens:  inputTokenSum(allLogs),
		OutputTokens: outputTokenSum(allLogs),
		CachedTokens: cachedTokenSum(allLogs),
		Costs: apiopenapi.AdminDashboardCosts{
			ActualCost:   totalCost,
			AccountCost:  totalCost,
			StandardCost: totalCost,
			Currency:     currency,
		},
	}
}

func dashboardPerformance(allLogs, windowLogs []usagecontract.UsageLog) apiopenapi.AdminDashboardPerformance {
	current := filterLogsByWindow(allLogs, time.Now().UTC().Add(-time.Minute), time.Now().UTC())
	buckets := bucketUsageLogs(windowLogs, "hour")
	peakRPM, peakTPM := peakThroughput(buckets, "hour")
	return apiopenapi.AdminDashboardPerformance{
		CurrentRpm:       len(current),
		CurrentTpm:       tokenSum(current),
		PeakRpm:          peakRPM,
		PeakTpm:          peakTPM,
		AverageLatencyMs: averageLatency(windowLogs),
		P95LatencyMs:     latencyPercentile(windowLogs, 95),
	}
}

func opsOverview(logs []usagecontract.UsageLog, window apiopenapi.TimeWindow) apiopenapi.OpsOverview {
	success, errors := successErrorCounts(logs)
	minutes := windowMinutes(window)
	return apiopenapi.OpsOverview{
		Window:           window,
		RequestCount:     len(logs),
		SuccessCount:     success,
		ErrorCount:       errors,
		ErrorRate:        errorRate(len(logs), errors),
		LatencyP50Ms:     latencyPercentile(logs, 50),
		LatencyP95Ms:     latencyPercentile(logs, 95),
		LatencyP99Ms:     latencyPercentile(logs, 99),
		AverageLatencyMs: averageLatency(logs),
		Rpm:              int(float64(len(logs)) / minutes),
		Tpm:              int(float64(tokenSum(logs)) / minutes),
		ActiveUsers:      len(distinctUsers(logs)),
		GeneratedAt:      time.Now().UTC(),
	}
}

func filterLogsByWindow(logs []usagecontract.UsageLog, start, end time.Time) []usagecontract.UsageLog {
	out := make([]usagecontract.UsageLog, 0, len(logs))
	for _, log := range logs {
		if log.CreatedAt.Before(start) || !log.CreatedAt.Before(end) {
			continue
		}
		out = append(out, log)
	}
	return out
}

func bucketUsageLogs(logs []usagecontract.UsageLog, bucket string) []usageBucket {
	byStart := map[time.Time]*usageBucket{}
	for _, log := range logs {
		start := log.CreatedAt.UTC().Truncate(time.Hour)
		if bucket == "day" {
			start = startOfDay(log.CreatedAt.UTC())
		}
		item := byStart[start]
		if item == nil {
			item = &usageBucket{Start: start, Cost: "0.00000000"}
			byStart[start] = item
		}
		item.RequestCount++
		if !log.Success {
			item.ErrorCount++
		}
		item.TokenCount += log.TotalTokens
		item.Cost = addDecimalStrings(item.Cost, log.Cost)
	}
	starts := make([]time.Time, 0, len(byStart))
	for start := range byStart {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	out := make([]usageBucket, 0, len(starts))
	for _, start := range starts {
		out = append(out, *byStart[start])
	}
	return out
}

func throughputTrendPoints(buckets []usageBucket, bucket string) []apiopenapi.OpsThroughputTrendPoint {
	points := make([]apiopenapi.OpsThroughputTrendPoint, 0, len(buckets))
	for _, item := range buckets {
		minutes := float64(60)
		if bucket == "day" {
			minutes = 24 * 60
		}
		points = append(points, apiopenapi.OpsThroughputTrendPoint{
			BucketStart:  item.Start,
			RequestCount: item.RequestCount,
			TokenCount:   item.TokenCount,
			Rpm:          int(float64(item.RequestCount) / minutes),
			Tpm:          int(float64(item.TokenCount) / minutes),
			Cost:         item.Cost,
		})
	}
	return points
}

func errorTrendPoints(buckets []usageBucket) []apiopenapi.OpsErrorTrendPoint {
	points := make([]apiopenapi.OpsErrorTrendPoint, 0, len(buckets))
	for _, item := range buckets {
		points = append(points, apiopenapi.OpsErrorTrendPoint{
			BucketStart:  item.Start,
			RequestCount: item.RequestCount,
			ErrorCount:   item.ErrorCount,
			ErrorRate:    errorRate(item.RequestCount, item.ErrorCount),
		})
	}
	return points
}

func dashboardTokenTrend(buckets []usageBucket) []apiopenapi.DashboardTrendPoint {
	points := make([]apiopenapi.DashboardTrendPoint, 0, len(buckets))
	for _, item := range buckets {
		points = append(points, apiopenapi.DashboardTrendPoint{
			BucketStart:  item.Start,
			RequestCount: item.RequestCount,
			TokenCount:   item.TokenCount,
			Cost:         item.Cost,
		})
	}
	return points
}

func dashboardModelDistribution(logs []usagecontract.UsageLog) []apiopenapi.DashboardModelDistribution {
	type aggregate struct {
		requests int
		tokens   int
		cost     string
		currency string
	}
	byModel := map[string]*aggregate{}
	for _, log := range logs {
		model := strings.TrimSpace(log.Model)
		if model == "" {
			model = "unknown"
		}
		item := byModel[model]
		if item == nil {
			item = &aggregate{cost: "0.00000000", currency: "USD"}
			byModel[model] = item
		}
		item.requests++
		item.tokens += log.TotalTokens
		item.cost = addDecimalStrings(item.cost, log.Cost)
		if log.Currency != "" {
			item.currency = log.Currency
		}
	}
	models := make([]string, 0, len(byModel))
	for model := range byModel {
		models = append(models, model)
	}
	sort.Strings(models)
	out := make([]apiopenapi.DashboardModelDistribution, 0, len(models))
	for _, model := range models {
		item := byModel[model]
		out = append(out, apiopenapi.DashboardModelDistribution{
			Model:        model,
			RequestCount: item.requests,
			TokenCount:   item.tokens,
			Cost:         item.cost,
			Currency:     item.currency,
		})
	}
	return out
}

func dashboardUserUsageTrend(logs []usagecontract.UsageLog, users []userscontract.StoredUser) []apiopenapi.DashboardUserUsageTrend {
	emailByID := map[int]string{}
	for _, user := range users {
		emailByID[user.ID] = user.Email
	}
	type aggregate struct {
		requests int
		tokens   int
		cost     string
	}
	byUser := map[int]*aggregate{}
	for _, log := range logs {
		item := byUser[log.UserID]
		if item == nil {
			item = &aggregate{cost: "0.00000000"}
			byUser[log.UserID] = item
		}
		item.requests++
		item.tokens += log.TotalTokens
		item.cost = addDecimalStrings(item.cost, log.Cost)
	}
	ids := make([]int, 0, len(byUser))
	for id := range byUser {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]apiopenapi.DashboardUserUsageTrend, 0, len(ids))
	for _, id := range ids {
		item := byUser[id]
		email := emailByID[id]
		out = append(out, apiopenapi.DashboardUserUsageTrend{
			UserId:       apiopenapi.Id(strconv.Itoa(id)),
			Email:        ptrStringValue(email),
			RequestCount: item.requests,
			TokenCount:   item.tokens,
			Cost:         item.cost,
		})
	}
	return out
}

func errorDistribution(logs []usagecontract.UsageLog) []apiopenapi.OpsErrorDistributionItem {
	counts := map[string]int{}
	total := 0
	for _, log := range logs {
		if log.Success {
			continue
		}
		class := "unknown"
		if log.ErrorClass != nil && strings.TrimSpace(*log.ErrorClass) != "" {
			class = strings.TrimSpace(*log.ErrorClass)
		}
		owner := errorOwner(class)
		key := class + "\x00" + owner
		counts[key]++
		total++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]apiopenapi.OpsErrorDistributionItem, 0, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(key, "\x00", 2)
		out = append(out, apiopenapi.OpsErrorDistributionItem{
			ErrorClass: parts[0],
			Owner:      parts[1],
			Count:      counts[key],
			Share:      errorRate(total, counts[key]),
		})
	}
	return out
}

func latencyHistogram(logs []usagecontract.UsageLog) []apiopenapi.OpsLatencyBucket {
	bounds := []struct {
		label string
		lower int
		upper *int
	}{
		{"<100ms", 0, ptrInt(100)},
		{"100-250ms", 100, ptrInt(250)},
		{"250-500ms", 250, ptrInt(500)},
		{"500ms-1s", 500, ptrInt(1000)},
		{"1s-2.5s", 1000, ptrInt(2500)},
		{"2.5s-5s", 2500, ptrInt(5000)},
		{">=5s", 5000, nil},
	}
	out := make([]apiopenapi.OpsLatencyBucket, 0, len(bounds))
	for _, bound := range bounds {
		count := 0
		for _, log := range logs {
			if log.LatencyMS < bound.lower {
				continue
			}
			if bound.upper != nil && log.LatencyMS >= *bound.upper {
				continue
			}
			count++
		}
		out = append(out, apiopenapi.OpsLatencyBucket{
			Label:   bound.label,
			LowerMs: bound.lower,
			UpperMs: bound.upper,
			Count:   count,
			Share:   errorRate(len(logs), count),
		})
	}
	return out
}

func successErrorCounts(logs []usagecontract.UsageLog) (int, int) {
	success := 0
	errors := 0
	for _, log := range logs {
		if log.Success {
			success++
		} else {
			errors++
		}
	}
	return success, errors
}

func tokenSum(logs []usagecontract.UsageLog) int {
	total := 0
	for _, log := range logs {
		total += log.TotalTokens
	}
	return total
}

func inputTokenSum(logs []usagecontract.UsageLog) int {
	total := 0
	for _, log := range logs {
		total += log.InputTokens
	}
	return total
}

func outputTokenSum(logs []usagecontract.UsageLog) int {
	total := 0
	for _, log := range logs {
		total += log.OutputTokens
	}
	return total
}

func cachedTokenSum(logs []usagecontract.UsageLog) int {
	total := 0
	for _, log := range logs {
		total += log.CachedTokens
	}
	return total
}

func costSum(logs []usagecontract.UsageLog) (string, string) {
	total := "0.00000000"
	currency := "USD"
	for _, log := range logs {
		total = addDecimalStrings(total, log.Cost)
		if log.Currency != "" {
			currency = log.Currency
		}
	}
	return total, currency
}

func distinctUsers(logs []usagecontract.UsageLog) map[int]bool {
	users := map[int]bool{}
	for _, log := range logs {
		users[log.UserID] = true
	}
	return users
}

func peakThroughput(buckets []usageBucket, bucket string) (int, int) {
	var peakRPM, peakTPM int
	minutes := float64(60)
	if bucket == "day" {
		minutes = 24 * 60
	}
	for _, item := range buckets {
		rpm := int(float64(item.RequestCount) / minutes)
		tpm := int(float64(item.TokenCount) / minutes)
		if rpm > peakRPM {
			peakRPM = rpm
		}
		if tpm > peakTPM {
			peakTPM = tpm
		}
	}
	return peakRPM, peakTPM
}

func errorRate(total, errors int) float32 {
	if total <= 0 {
		return 0
	}
	return float32(errors) / float32(total)
}

func windowMinutes(window apiopenapi.TimeWindow) float64 {
	minutes := window.End.Sub(window.Start).Minutes()
	if minutes < 1 {
		return 1
	}
	return minutes
}

func startOfDay(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func errorOwner(errorClass string) string {
	switch {
	case strings.Contains(errorClass, "provider"), strings.Contains(errorClass, "upstream"):
		return "provider"
	case strings.Contains(errorClass, "rate"), strings.Contains(errorClass, "quota"):
		return "quota"
	case strings.Contains(errorClass, "auth"), strings.Contains(errorClass, "policy"):
		return "user"
	default:
		return "gateway"
	}
}

func listOptionsFromRequest(r *http.Request) admincontrol.ListOptions {
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	pageSize := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return admincontrol.ListOptions{
		Page:     page,
		PageSize: pageSize,
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Level:    strings.TrimSpace(r.URL.Query().Get("level")),
	}
}

func systemLogListOptionsFromRequest(r *http.Request) admincontrol.SystemLogListOptions {
	opts := listOptionsFromRequest(r)
	query := r.URL.Query()
	var start, end *time.Time
	if parsed, ok := parseOptionalRFC3339(query.Get("start")); ok {
		start = &parsed
	}
	if parsed, ok := parseOptionalRFC3339(query.Get("end")); ok {
		end = &parsed
	}
	return admincontrol.SystemLogListOptions{
		Page:     opts.Page,
		PageSize: opts.PageSize,
		Level:    admincontrol.OpsSystemLogLevel(strings.TrimSpace(query.Get("level"))),
		Source:   strings.TrimSpace(query.Get("source")),
		Query:    strings.TrimSpace(query.Get("q")),
		Start:    start,
		End:      end,
	}
}

func parseOptionalRFC3339(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

func paginationWithRequest(r *http.Request, total int) apiopenapi.Pagination {
	opts := listOptionsFromRequest(r)
	return apiopenapi.Pagination{
		Page:     opts.Page,
		PageSize: opts.PageSize,
		Total:    total,
		HasNext:  opts.Page*opts.PageSize < total,
	}
}

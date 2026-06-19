package httpserver

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	requestEvidenceDefaultPageSize   = 50
	requestEvidenceMaxPageSize       = 200
	requestEvidenceDefaultWindow     = time.Hour
	requestEvidenceOpsPageSize       = 200
	requestEvidenceMaxOpsRows        = 2000
	requestEvidenceSystemLogPageSize = 1000
	requestEvidenceMaxSystemLogRows  = 2000
	requestEvidenceDetailDumpLimit   = 50
	requestEvidenceSystemLogLimit    = 5
)

type requestEvidenceQuery struct {
	Page           int
	PageSize       int
	RequestID      string
	UserID         *int
	APIKeyID       *int
	AccountID      *int
	ProviderID     *int
	Model          string
	SourceEndpoint string
	ErrorClass     string
	Kind           string
	EvidenceSource string
	MinLatencyMS   *int
	MaxLatencyMS   *int
	Start          time.Time
	End            time.Time
	Sort           string
	Search         string
}

type requestEvidenceRow struct {
	Kind                       string
	EvidenceSource             string
	CreatedAt                  time.Time
	RequestID                  string
	UsageLogID                 *int
	OpsErrorLogID              *int64
	UserID                     *int
	APIKeyID                   *int
	AccountID                  *int
	ProviderID                 *int
	SourceProtocol             string
	SourceEndpoint             string
	TargetProtocol             string
	Model                      string
	StatusCode                 *int
	Success                    *bool
	ErrorClass                 string
	ErrorMessage               string
	ErrorPhase                 string
	ErrorOwner                 string
	ErrorSource                string
	UpstreamRequestID          string
	AttemptNo                  *int
	LatencyMS                  *int
	InputTokens                *int
	OutputTokens               *int
	TotalTokens                *int
	UsageEstimated             *bool
	Resolution                 string
	HasUsageLog                bool
	HasOpsErrorLog             bool
	HasRequestDump             bool
	HasSystemLog               bool
	RequestDumpCount           int
	RequestDumpErrorCount      int
	SystemLogCount             int
	SystemLogSearchText        string
	LatestRequestDumpName      string
	LatestRequestDumpCreatedAt *time.Time
}

type requestDumpEvidence struct {
	Count      int
	ErrorCount int
	Latest     rlfcontract.FileDescriptor
}

type requestEvidenceDetail struct {
	RequestID string
	Summary   requestEvidenceSummary
	Attempts  []requestEvidenceRow
	Dumps     []rlfcontract.FileDescriptor
	SystemLog requestEvidenceSystemLogEvidence
	FirstSeen time.Time
	LastSeen  time.Time
}

type requestEvidenceSummary struct {
	Kind                  string
	PrimarySource         string
	AttemptCount          int
	UsageLogCount         int
	OpsErrorLogCount      int
	RequestDumpCount      int
	RequestDumpErrorCount int
	HasUsageLog           bool
	HasOpsErrorLog        bool
	HasRequestDump        bool
	LatencyMS             *int
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	StatusCode            *int
	ErrorClass            string
	ErrorMessage          string
	ErrorPhase            string
	ErrorOwner            string
	ErrorSource           string
	UpstreamRequestID     string
}

type requestEvidenceSystemLogEvidence struct {
	Items       []operationscontract.OpsSystemLog
	Total       int
	LevelCounts map[operationscontract.OpsSystemLogLevel]int
	Latest      *operationscontract.OpsSystemLog
}

// handleListAdminOpsRequestEvidence serves GET /api/v1/admin/ops/request-evidence.
//
// It deliberately lives in the HTTP/admin layer: request evidence is a read-only
// operator projection over existing evidence stores, not a new write-side
// business concept. The response avoids raw dump bodies and headers; those stay
// behind the existing request-log-file preview/download endpoints.
func (s *Server) handleListAdminOpsRequestEvidence(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if s.runtime == nil || s.runtime.usageStore == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "usage logs unavailable", requestID)
		return
	}
	query, err := requestEvidenceQueryFromRequest(r, time.Now().UTC())
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, err.Error(), requestID)
		return
	}
	rows, err := s.requestEvidenceRows(r.Context(), query)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list request evidence", requestID)
		return
	}
	sortRequestEvidenceRows(rows, query.Sort)
	total := len(rows)
	paged := paginateRequestEvidenceRows(rows, query.Page, query.PageSize)
	data := make([]apiopenapi.RequestEvidenceRow, 0, len(paged))
	for _, row := range paged {
		data = append(data, requestEvidenceRowToAPI(row))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RequestEvidenceListResponse{
		Data:       data,
		Pagination: paginationFromTotal(total, query.Page, query.PageSize),
		RequestId:  requestID,
	})
}

// handleGetAdminOpsRequestEvidence returns an exact request-id drilldown for
// the request evidence feed. It is intentionally metadata-only: raw dump
// previews/downloads stay behind the existing request-log-file endpoints.
func (s *Server) handleGetAdminOpsRequestEvidence(w http.ResponseWriter, r *http.Request) {
	correlationID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", correlationID)
		return
	}
	if s.runtime == nil || s.runtime.usageStore == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "usage logs unavailable", correlationID)
		return
	}
	requestID := strings.TrimSpace(r.PathValue("request_id"))
	if requestID == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "request_id is required", correlationID)
		return
	}
	detail, err := s.requestEvidenceDetail(r.Context(), requestID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to get request evidence", correlationID)
		return
	}
	if detail.RequestID == "" {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "request evidence not found", correlationID)
		return
	}
	writeJSONAny(w, http.StatusOK, requestEvidenceDetailToAPI(detail, correlationID))
}

func (s *Server) requestEvidenceRows(ctx context.Context, query requestEvidenceQuery) ([]requestEvidenceRow, error) {
	usageLogs, err := requestEvidenceUsageLogs(ctx, s.runtime.usageStore, query)
	if err != nil {
		return nil, err
	}
	dumps, err := requestEvidenceDumps(ctx, s.runtime.requestLogFileReader(), query)
	if err != nil {
		return nil, err
	}
	systemLogs, err := s.requestEvidenceSystemLogRows(ctx, query)
	if err != nil {
		return nil, err
	}
	rowsByKey := make(map[string]*requestEvidenceRow, len(usageLogs))
	rowsByRequest := make(map[string][]*requestEvidenceRow)
	rows := make([]*requestEvidenceRow, 0, len(usageLogs))
	for _, log := range usageLogs {
		if !requestEvidenceUsageLogMatches(log, query) {
			continue
		}
		row := requestEvidenceRowFromUsage(log)
		rows = append(rows, row)
		rowsByKey[requestEvidenceKey(row.RequestID, row.AttemptNo, row.UsageLogID)] = row
		rowsByRequest[row.RequestID] = append(rowsByRequest[row.RequestID], row)
	}
	if s.runtime.opsErrorLogs != nil {
		entries, err := requestEvidenceOpsErrorLogs(ctx, s.runtime.opsErrorLogs, query)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !requestEvidenceOpsErrorLogMatches(entry, query) {
				continue
			}
			row := requestEvidenceRowFromOpsError(entry)
			key := requestEvidenceKey(row.RequestID, row.AttemptNo, nil)
			if existing := rowsByKey[key]; existing != nil {
				mergeRequestEvidenceOpsError(existing, entry)
				continue
			}
			rows = append(rows, row)
			rowsByKey[key] = row
			rowsByRequest[row.RequestID] = append(rowsByRequest[row.RequestID], row)
		}
	}
	for requestID, dump := range dumps {
		if existing := rowsByRequest[requestID]; len(existing) > 0 {
			for _, row := range existing {
				attachRequestDumpEvidence(row, dump)
			}
			continue
		}
		row := requestEvidenceRowFromDump(dump)
		rows = append(rows, row)
		rowsByRequest[row.RequestID] = append(rowsByRequest[row.RequestID], row)
	}
	for requestID, evidence := range systemLogs {
		if existing := rowsByRequest[requestID]; len(existing) > 0 {
			for _, row := range existing {
				attachRequestEvidenceSystemLog(row, evidence)
			}
			continue
		}
		row := requestEvidenceRowFromSystemLog(requestID, evidence)
		if row == nil || !requestEvidenceRowMatches(*row, query) {
			continue
		}
		rows = append(rows, row)
	}

	filtered := make([]requestEvidenceRow, 0, len(rows))
	for _, row := range rows {
		if requestEvidenceRowMatches(*row, query) {
			filtered = append(filtered, *row)
		}
	}
	return filtered, nil
}

func (s *Server) requestEvidenceDetail(ctx context.Context, requestID string) (requestEvidenceDetail, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return requestEvidenceDetail{}, nil
	}
	usageLogs, err := requestEvidenceUsageLogsByRequestID(ctx, s.runtime.usageStore, requestID)
	if err != nil {
		return requestEvidenceDetail{}, err
	}
	dumps, err := requestEvidenceDumpFilesByRequestID(ctx, s.runtime.requestLogFileReader(), requestID)
	if err != nil {
		return requestEvidenceDetail{}, err
	}
	var opsEntries []opserrorlogscontract.Entry
	if s.runtime.opsErrorLogs != nil {
		res, err := requestEvidenceOpsErrorLogsByRequestID(ctx, s.runtime.opsErrorLogs, requestID)
		if err != nil {
			return requestEvidenceDetail{}, err
		}
		opsEntries = res
	}
	dumpEvidence := requestDumpEvidenceFromDescriptors(dumps)
	rows := requestEvidenceRowsFromExactEvidence(requestID, usageLogs, opsEntries, dumpEvidence)
	systemLogEvidence, err := s.requestEvidenceSystemLogsByRequestID(ctx, requestID)
	if err != nil {
		return requestEvidenceDetail{}, err
	}
	if systemLogEvidence.Total > 0 {
		for i := range rows {
			attachRequestEvidenceSystemLog(&rows[i], systemLogEvidence)
		}
	}
	if len(rows) == 0 && len(dumps) == 0 && systemLogEvidence.Total == 0 {
		return requestEvidenceDetail{}, nil
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if intPtrValue(rows[i].AttemptNo) != intPtrValue(rows[j].AttemptNo) {
			return intPtrValue(rows[i].AttemptNo) < intPtrValue(rows[j].AttemptNo)
		}
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		}
		return intPtrValue(rows[i].UsageLogID) < intPtrValue(rows[j].UsageLogID)
	})
	detail := requestEvidenceDetail{
		RequestID: requestID,
		Attempts:  rows,
		Dumps:     dumps,
		SystemLog: systemLogEvidence,
	}
	detail.FirstSeen, detail.LastSeen = requestEvidenceDetailBounds(rows, dumps, systemLogEvidence.Items)
	detail.Summary = requestEvidenceSummaryFromDetail(rows, usageLogs, opsEntries, dumpEvidence, systemLogEvidence)
	return detail, nil
}

func requestEvidenceQueryFromRequest(r *http.Request, now time.Time) (requestEvidenceQuery, error) {
	q := r.URL.Query()
	page := 1
	if raw := strings.TrimSpace(q.Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return requestEvidenceQuery{}, errors.New("invalid page")
		}
		page = parsed
	}
	pageSize := requestEvidenceDefaultPageSize
	if raw := strings.TrimSpace(q.Get("page_size")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return requestEvidenceQuery{}, errors.New("invalid page_size")
		}
		if parsed > requestEvidenceMaxPageSize {
			parsed = requestEvidenceMaxPageSize
		}
		pageSize = parsed
	}
	end := now.UTC()
	if parsed, err := parseOptionalRFC3339(q.Get("end")); err != nil {
		return requestEvidenceQuery{}, errors.New("invalid end timestamp")
	} else if parsed != nil {
		end = *parsed
	}
	start := end.Add(-requestEvidenceDefaultWindow)
	if parsed, err := parseOptionalRFC3339(q.Get("start")); err != nil {
		return requestEvidenceQuery{}, errors.New("invalid start timestamp")
	} else if parsed != nil {
		start = *parsed
	}
	if !start.Before(end) {
		return requestEvidenceQuery{}, errors.New("start must be before end")
	}
	kind := strings.TrimSpace(q.Get("kind"))
	if kind == "" {
		kind = "all"
	}
	switch kind {
	case "all", "success", "error", "unknown":
	default:
		return requestEvidenceQuery{}, errors.New("invalid kind")
	}
	source := strings.TrimSpace(q.Get("evidence_source"))
	if source == "" {
		source = "all"
	}
	switch source {
	case "all", "usage", "ops_error", "request_dump", "system_log":
	default:
		return requestEvidenceQuery{}, errors.New("invalid evidence_source")
	}
	sortValue := strings.TrimSpace(q.Get("sort"))
	if sortValue == "" {
		sortValue = "created_at_desc"
	}
	switch sortValue {
	case "created_at_desc", "latency_desc":
	default:
		return requestEvidenceQuery{}, errors.New("invalid sort")
	}
	query := requestEvidenceQuery{
		Page:           page,
		PageSize:       pageSize,
		RequestID:      strings.TrimSpace(q.Get("request_id")),
		Model:          strings.TrimSpace(q.Get("model")),
		SourceEndpoint: strings.TrimSpace(q.Get("source_endpoint")),
		ErrorClass:     strings.TrimSpace(q.Get("error_class")),
		Kind:           kind,
		EvidenceSource: source,
		Start:          start,
		End:            end,
		Sort:           sortValue,
		Search:         strings.TrimSpace(q.Get("q")),
	}
	if id, ok, err := parseOptionalPositiveInt(q.Get("user_id"), "user_id"); err != nil {
		return requestEvidenceQuery{}, err
	} else if ok {
		query.UserID = &id
	}
	if id, ok, err := parseOptionalPositiveInt(q.Get("api_key_id"), "api_key_id"); err != nil {
		return requestEvidenceQuery{}, err
	} else if ok {
		query.APIKeyID = &id
	}
	if id, ok, err := parseOptionalPositiveInt(q.Get("account_id"), "account_id"); err != nil {
		return requestEvidenceQuery{}, err
	} else if ok {
		query.AccountID = &id
	}
	if id, ok, err := parseOptionalPositiveInt(q.Get("provider_id"), "provider_id"); err != nil {
		return requestEvidenceQuery{}, err
	} else if ok {
		query.ProviderID = &id
	}
	if value, ok, err := requestEvidenceOptionalNonNegativeInt(q.Get("min_latency_ms"), "min_latency_ms"); err != nil {
		return requestEvidenceQuery{}, err
	} else if ok {
		query.MinLatencyMS = &value
	}
	if value, ok, err := requestEvidenceOptionalNonNegativeInt(q.Get("max_latency_ms"), "max_latency_ms"); err != nil {
		return requestEvidenceQuery{}, err
	} else if ok {
		query.MaxLatencyMS = &value
	}
	if query.MinLatencyMS != nil && query.MaxLatencyMS != nil && *query.MinLatencyMS > *query.MaxLatencyMS {
		return requestEvidenceQuery{}, errors.New("min_latency_ms must be <= max_latency_ms")
	}
	return query, nil
}

func requestEvidenceUsageLogs(ctx context.Context, store usagecontract.Store, query requestEvidenceQuery) ([]usagecontract.UsageLog, error) {
	if reader, ok := store.(usagecontract.WindowReader); ok {
		return reader.ListWindow(ctx, usagecontract.QueryFilter{Start: &query.Start, End: &query.End}, 0)
	}
	items, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]usagecontract.UsageLog, 0, len(items))
	for _, item := range items {
		if !item.CreatedAt.Before(query.Start) && item.CreatedAt.Before(query.End) {
			out = append(out, item)
		}
	}
	return out, nil
}

func requestEvidenceUsageLogsByRequestID(ctx context.Context, store usagecontract.Store, requestID string) ([]usagecontract.UsageLog, error) {
	if reader, ok := store.(usagecontract.RequestReader); ok {
		return reader.ListByRequestID(ctx, requestID)
	}
	items, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]usagecontract.UsageLog, 0)
	for _, item := range items {
		if item.RequestID == requestID {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AttemptNo == out[j].AttemptNo {
			return out[i].ID < out[j].ID
		}
		return out[i].AttemptNo < out[j].AttemptNo
	})
	return out, nil
}

func requestEvidenceOpsErrorLogs(ctx context.Context, svc interface {
	List(context.Context, opserrorlogscontract.ListFilter) (opserrorlogscontract.ListResult, error)
}, query requestEvidenceQuery) ([]opserrorlogscontract.Entry, error) {
	filter := opserrorlogscontract.ListFilter{
		UserID:     query.UserID,
		AccountID:  query.AccountID,
		ProviderID: query.ProviderID,
		Model:      query.Model,
		ErrorClass: query.ErrorClass,
		From:       &query.Start,
		To:         &query.End,
		PageSize:   requestEvidenceOpsPageSize,
	}
	out := make([]opserrorlogscontract.Entry, 0, requestEvidenceOpsPageSize)
	for page := 1; ; page++ {
		filter.Page = page
		res, err := svc.List(ctx, filter)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Items...)
		if len(out) >= requestEvidenceMaxOpsRows || len(res.Items) == 0 || page*res.PageSize >= res.Total {
			break
		}
	}
	if len(out) > requestEvidenceMaxOpsRows {
		out = out[:requestEvidenceMaxOpsRows]
	}
	return out, nil
}

func requestEvidenceOpsErrorLogsByRequestID(ctx context.Context, svc interface {
	List(context.Context, opserrorlogscontract.ListFilter) (opserrorlogscontract.ListResult, error)
}, requestID string) ([]opserrorlogscontract.Entry, error) {
	filter := opserrorlogscontract.ListFilter{
		RequestID: strings.TrimSpace(requestID),
		PageSize:  requestEvidenceOpsPageSize,
	}
	out := make([]opserrorlogscontract.Entry, 0, requestEvidenceOpsPageSize)
	for page := 1; ; page++ {
		filter.Page = page
		res, err := svc.List(ctx, filter)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Items...)
		if len(out) >= requestEvidenceMaxOpsRows || len(res.Items) == 0 || page*res.PageSize >= res.Total {
			break
		}
	}
	if len(out) > requestEvidenceMaxOpsRows {
		out = out[:requestEvidenceMaxOpsRows]
	}
	return out, nil
}

func requestEvidenceDumpFilesByRequestID(ctx context.Context, reader rlfcontract.Reader, requestID string) ([]rlfcontract.FileDescriptor, error) {
	if reader == nil {
		return nil, nil
	}
	descs, err := reader.List(ctx, rlfcontract.ListFilter{
		RequestIDPrefix: requestID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]rlfcontract.FileDescriptor, 0, len(descs))
	for _, desc := range descs {
		if desc.RequestID == requestID {
			out = append(out, desc)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if len(out) > requestEvidenceDetailDumpLimit {
		out = out[len(out)-requestEvidenceDetailDumpLimit:]
	}
	return out, nil
}

func (s *Server) requestEvidenceSystemLogRows(ctx context.Context, query requestEvidenceQuery) (map[string]requestEvidenceSystemLogEvidence, error) {
	if s.runtime == nil || s.runtime.operations == nil {
		return map[string]requestEvidenceSystemLogEvidence{}, nil
	}
	opts := operationscontract.SystemLogListOptions{
		Start:    &query.Start,
		End:      &query.End,
		PageSize: requestEvidenceSystemLogPageSize,
	}
	out := make(map[string]requestEvidenceSystemLogEvidence)
	seen := 0
	for page := 1; ; page++ {
		opts.Page = page
		list, err := s.runtime.operations.ListSystemLogs(ctx, opts)
		if err != nil {
			return nil, err
		}
		for _, log := range list.Items {
			seen++
			if !requestEvidenceSystemLogMatches(log, query) {
				continue
			}
			evidence := out[log.RequestID]
			if evidence.LevelCounts == nil {
				evidence.LevelCounts = map[operationscontract.OpsSystemLogLevel]int{}
			}
			evidence.Items = append(evidence.Items, log)
			evidence.Total++
			evidence.LevelCounts[log.Level]++
			if evidence.Latest == nil || requestEvidenceSystemLogNewer(log, *evidence.Latest) {
				latest := log
				evidence.Latest = &latest
			}
			out[log.RequestID] = evidence
		}
		if seen >= requestEvidenceMaxSystemLogRows || len(list.Items) == 0 || page*requestEvidenceSystemLogPageSize >= list.Total {
			break
		}
	}
	return out, nil
}

func (s *Server) requestEvidenceSystemLogsByRequestID(ctx context.Context, requestID string) (requestEvidenceSystemLogEvidence, error) {
	if s.runtime == nil || s.runtime.operations == nil {
		return requestEvidenceSystemLogEvidence{LevelCounts: map[operationscontract.OpsSystemLogLevel]int{}}, nil
	}
	list, err := s.runtime.operations.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{
		RequestID: requestID,
		Page:      1,
		PageSize:  requestEvidenceSystemLogLimit,
	})
	if err != nil {
		return requestEvidenceSystemLogEvidence{}, err
	}
	evidence := requestEvidenceSystemLogEvidence{
		Items:       list.Items,
		Total:       list.Total,
		LevelCounts: map[operationscontract.OpsSystemLogLevel]int{},
	}
	for _, level := range []operationscontract.OpsSystemLogLevel{
		operationscontract.OpsSystemLogLevelDebug,
		operationscontract.OpsSystemLogLevelInfo,
		operationscontract.OpsSystemLogLevelWarn,
		operationscontract.OpsSystemLogLevelError,
	} {
		levelList, err := s.runtime.operations.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{
			RequestID: requestID,
			Level:     level,
			Page:      1,
			PageSize:  1,
		})
		if err != nil {
			return requestEvidenceSystemLogEvidence{}, err
		}
		if levelList.Total > 0 {
			evidence.LevelCounts[level] = levelList.Total
		}
	}
	if len(evidence.Items) > 0 {
		latest := evidence.Items[0]
		evidence.Latest = &latest
	}
	return evidence, nil
}

func requestEvidenceDumps(ctx context.Context, reader rlfcontract.Reader, query requestEvidenceQuery) (map[string]requestDumpEvidence, error) {
	if reader == nil {
		return map[string]requestDumpEvidence{}, nil
	}
	descs, err := reader.List(ctx, rlfcontract.ListFilter{
		RequestIDPrefix: query.RequestID,
		From:            &query.Start,
		To:              &query.End,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]requestDumpEvidence)
	for _, desc := range descs {
		if desc.RequestID == "" {
			continue
		}
		evidence := out[desc.RequestID]
		evidence.Count++
		if desc.IsErrorOnly || (desc.Success != nil && !*desc.Success) {
			evidence.ErrorCount++
		}
		if evidence.Latest.Name == "" || desc.CreatedAt.After(evidence.Latest.CreatedAt) {
			evidence.Latest = desc
		}
		out[desc.RequestID] = evidence
	}
	return out, nil
}

func requestDumpEvidenceFromDescriptors(descs []rlfcontract.FileDescriptor) map[string]requestDumpEvidence {
	out := make(map[string]requestDumpEvidence)
	for _, desc := range descs {
		if desc.RequestID == "" {
			continue
		}
		evidence := out[desc.RequestID]
		evidence.Count++
		if desc.IsErrorOnly || (desc.Success != nil && !*desc.Success) {
			evidence.ErrorCount++
		}
		if evidence.Latest.Name == "" || desc.CreatedAt.After(evidence.Latest.CreatedAt) {
			evidence.Latest = desc
		}
		out[desc.RequestID] = evidence
	}
	return out
}

func requestEvidenceRowsFromExactEvidence(requestID string, usageLogs []usagecontract.UsageLog, opsEntries []opserrorlogscontract.Entry, dumps map[string]requestDumpEvidence) []requestEvidenceRow {
	rowsByKey := make(map[string]*requestEvidenceRow, len(usageLogs))
	rows := make([]*requestEvidenceRow, 0, len(usageLogs)+len(opsEntries))
	for _, log := range usageLogs {
		if log.RequestID != requestID {
			continue
		}
		row := requestEvidenceRowFromUsage(log)
		rows = append(rows, row)
		rowsByKey[requestEvidenceKey(row.RequestID, row.AttemptNo, row.UsageLogID)] = row
	}
	for _, entry := range opsEntries {
		if entry.RequestID != requestID {
			continue
		}
		row := requestEvidenceRowFromOpsError(entry)
		key := requestEvidenceKey(row.RequestID, row.AttemptNo, nil)
		if existing := rowsByKey[key]; existing != nil {
			mergeRequestEvidenceOpsError(existing, entry)
			continue
		}
		rows = append(rows, row)
		rowsByKey[key] = row
	}
	if dump := dumps[requestID]; dump.Count > 0 {
		if len(rows) == 0 {
			rows = append(rows, requestEvidenceRowFromDump(dump))
		} else {
			for _, row := range rows {
				attachRequestDumpEvidence(row, dump)
			}
		}
	}
	out := make([]requestEvidenceRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	return out
}

func requestEvidenceRowFromUsage(log usagecontract.UsageLog) *requestEvidenceRow {
	id := log.ID
	userID := log.UserID
	apiKeyID := log.APIKeyID
	attemptNo := log.AttemptNo
	latency := log.LatencyMS
	inputTokens := log.InputTokens
	outputTokens := log.OutputTokens
	totalTokens := log.TotalTokens
	usageEstimated := log.UsageEstimated
	success := log.Success
	row := &requestEvidenceRow{
		Kind:              requestEvidenceKindFromSuccess(&success),
		EvidenceSource:    "usage",
		CreatedAt:         log.CreatedAt.UTC(),
		RequestID:         log.RequestID,
		UsageLogID:        &id,
		UserID:            &userID,
		APIKeyID:          &apiKeyID,
		AccountID:         log.AccountID,
		ProviderID:        log.ProviderID,
		SourceProtocol:    log.SourceProtocol,
		SourceEndpoint:    log.SourceEndpoint,
		TargetProtocol:    log.TargetProtocol,
		Model:             log.Model,
		Success:           &success,
		UpstreamRequestID: log.UpstreamRequestID,
		AttemptNo:         &attemptNo,
		LatencyMS:         &latency,
		InputTokens:       &inputTokens,
		OutputTokens:      &outputTokens,
		TotalTokens:       &totalTokens,
		UsageEstimated:    &usageEstimated,
		ErrorMessage:      strings.TrimSpace(log.ProviderErrorMessage),
		ErrorPhase:        log.ErrorPhase,
		ErrorOwner:        log.ErrorOwner,
		ErrorSource:       log.ErrorSource,
		HasUsageLog:       true,
	}
	if log.ErrorClass != nil {
		row.ErrorClass = strings.TrimSpace(*log.ErrorClass)
	}
	if log.StatusCode >= 100 && log.StatusCode <= 599 {
		status := log.StatusCode
		row.StatusCode = &status
	}
	return row
}

func requestEvidenceRowFromOpsError(entry opserrorlogscontract.Entry) *requestEvidenceRow {
	id := entry.ID
	attemptNo := entry.AttemptNo
	latency := entry.LatencyMS
	inputTokens := entry.InputTokens
	outputTokens := entry.OutputTokens
	totalTokens := entry.InputTokens + entry.OutputTokens
	usageEstimated := entry.UsageEstimated
	success := false
	row := &requestEvidenceRow{
		Kind:              "error",
		EvidenceSource:    "ops_error",
		CreatedAt:         entry.OccurredAt.UTC(),
		RequestID:         entry.RequestID,
		OpsErrorLogID:     &id,
		UserID:            entry.UserID,
		APIKeyID:          entry.APIKeyID,
		AccountID:         entry.AccountID,
		ProviderID:        entry.ProviderID,
		SourceProtocol:    entry.Platform,
		SourceEndpoint:    entry.SourceEndpoint,
		TargetProtocol:    entry.TargetProtocol,
		Model:             entry.Model,
		StatusCode:        entry.StatusCode,
		Success:           &success,
		ErrorClass:        entry.ErrorClass,
		ErrorMessage:      entry.ErrorMessage,
		ErrorPhase:        entry.ErrorPhase,
		ErrorOwner:        entry.ErrorOwner,
		ErrorSource:       entry.ErrorSource,
		UpstreamRequestID: entry.UpstreamRequestID,
		AttemptNo:         &attemptNo,
		LatencyMS:         &latency,
		InputTokens:       &inputTokens,
		OutputTokens:      &outputTokens,
		TotalTokens:       &totalTokens,
		UsageEstimated:    &usageEstimated,
		Resolution:        string(entry.Resolution),
		HasOpsErrorLog:    true,
	}
	return row
}

func requestEvidenceRowFromDump(dump requestDumpEvidence) *requestEvidenceRow {
	desc := dump.Latest
	row := &requestEvidenceRow{
		Kind:                  requestEvidenceKindFromSuccess(desc.Success),
		EvidenceSource:        "request_dump",
		CreatedAt:             desc.CreatedAt.UTC(),
		RequestID:             desc.RequestID,
		SourceProtocol:        desc.SourceProtocol,
		SourceEndpoint:        desc.SourceEndpoint,
		ErrorClass:            desc.ErrorClass,
		StatusCode:            desc.StatusCode,
		Success:               desc.Success,
		LatencyMS:             desc.LatencyMS,
		HasRequestDump:        true,
		RequestDumpCount:      dump.Count,
		RequestDumpErrorCount: dump.ErrorCount,
		LatestRequestDumpName: desc.Name,
	}
	if desc.UserID != "" {
		if id, err := strconv.Atoi(desc.UserID); err == nil && id > 0 {
			row.UserID = &id
		}
	}
	if desc.APIKeyID != "" {
		if id, err := strconv.Atoi(desc.APIKeyID); err == nil && id > 0 {
			row.APIKeyID = &id
		}
	}
	if desc.AccountID != "" {
		if id, err := strconv.Atoi(desc.AccountID); err == nil && id > 0 {
			row.AccountID = &id
		}
	}
	createdAt := desc.CreatedAt.UTC()
	row.LatestRequestDumpCreatedAt = &createdAt
	return row
}

func requestEvidenceRowFromSystemLog(requestID string, evidence requestEvidenceSystemLogEvidence) *requestEvidenceRow {
	if evidence.Latest == nil {
		return nil
	}
	latest := evidence.Latest
	return &requestEvidenceRow{
		Kind:                "unknown",
		EvidenceSource:      "system_log",
		CreatedAt:           latest.CreatedAt.UTC(),
		RequestID:           requestID,
		ErrorMessage:        strings.TrimSpace(latest.Message),
		ErrorSource:         strings.TrimSpace(latest.Source),
		HasSystemLog:        true,
		SystemLogCount:      evidence.Total,
		SystemLogSearchText: requestEvidenceSystemLogSearchText(evidence),
	}
}

func mergeRequestEvidenceOpsError(row *requestEvidenceRow, entry opserrorlogscontract.Entry) {
	id := entry.ID
	row.OpsErrorLogID = &id
	row.HasOpsErrorLog = true
	if row.EvidenceSource == "request_dump" {
		row.EvidenceSource = "ops_error"
	}
	success := false
	row.Success = &success
	row.Kind = "error"
	if row.ErrorClass == "" {
		row.ErrorClass = entry.ErrorClass
	}
	if row.ErrorMessage == "" {
		row.ErrorMessage = entry.ErrorMessage
	}
	if row.StatusCode == nil {
		row.StatusCode = entry.StatusCode
	}
	if row.ErrorPhase == "" {
		row.ErrorPhase = entry.ErrorPhase
	}
	if row.ErrorOwner == "" {
		row.ErrorOwner = entry.ErrorOwner
	}
	if row.ErrorSource == "" {
		row.ErrorSource = entry.ErrorSource
	}
	if row.UpstreamRequestID == "" {
		row.UpstreamRequestID = entry.UpstreamRequestID
	}
	if row.Resolution == "" {
		row.Resolution = string(entry.Resolution)
	}
}

func attachRequestDumpEvidence(row *requestEvidenceRow, dump requestDumpEvidence) {
	row.HasRequestDump = true
	row.RequestDumpCount = dump.Count
	row.RequestDumpErrorCount = dump.ErrorCount
	row.LatestRequestDumpName = dump.Latest.Name
	createdAt := dump.Latest.CreatedAt.UTC()
	row.LatestRequestDumpCreatedAt = &createdAt
}

func attachRequestEvidenceSystemLog(row *requestEvidenceRow, evidence requestEvidenceSystemLogEvidence) {
	if evidence.Total <= 0 {
		return
	}
	row.HasSystemLog = true
	row.SystemLogCount = evidence.Total
	row.SystemLogSearchText = requestEvidenceSystemLogSearchText(evidence)
	if evidence.Latest == nil {
		return
	}
}

func requestEvidenceUsageLogMatches(log usagecontract.UsageLog, query requestEvidenceQuery) bool {
	if query.RequestID != "" && !strings.HasPrefix(log.RequestID, query.RequestID) {
		return false
	}
	if query.UserID != nil && log.UserID != *query.UserID {
		return false
	}
	if query.APIKeyID != nil && log.APIKeyID != *query.APIKeyID {
		return false
	}
	if query.AccountID != nil && intPtrValue(log.AccountID) != *query.AccountID {
		return false
	}
	if query.ProviderID != nil && intPtrValue(log.ProviderID) != *query.ProviderID {
		return false
	}
	if query.Model != "" && !strings.Contains(strings.ToLower(log.Model), strings.ToLower(query.Model)) {
		return false
	}
	if query.SourceEndpoint != "" && !strings.Contains(strings.ToLower(log.SourceEndpoint), strings.ToLower(query.SourceEndpoint)) {
		return false
	}
	if query.ErrorClass != "" {
		if log.ErrorClass == nil || !strings.EqualFold(*log.ErrorClass, query.ErrorClass) {
			return false
		}
	}
	return true
}

func requestEvidenceOpsErrorLogMatches(entry opserrorlogscontract.Entry, query requestEvidenceQuery) bool {
	if query.RequestID != "" && !strings.HasPrefix(entry.RequestID, query.RequestID) {
		return false
	}
	if query.APIKeyID != nil && intPtrValue(entry.APIKeyID) != *query.APIKeyID {
		return false
	}
	if query.SourceEndpoint != "" && !strings.Contains(strings.ToLower(entry.SourceEndpoint), strings.ToLower(query.SourceEndpoint)) {
		return false
	}
	return true
}

func requestEvidenceSystemLogMatches(log operationscontract.OpsSystemLog, query requestEvidenceQuery) bool {
	if log.RequestID == "" {
		return false
	}
	if query.RequestID != "" && !strings.HasPrefix(log.RequestID, query.RequestID) {
		return false
	}
	createdAt := log.CreatedAt.UTC()
	if createdAt.Before(query.Start) || !createdAt.Before(query.End) {
		return false
	}
	return true
}

func requestEvidenceRowMatches(row requestEvidenceRow, query requestEvidenceQuery) bool {
	if query.RequestID != "" && !strings.HasPrefix(row.RequestID, query.RequestID) {
		return false
	}
	if query.UserID != nil && intPtrValue(row.UserID) != *query.UserID {
		return false
	}
	if query.APIKeyID != nil && intPtrValue(row.APIKeyID) != *query.APIKeyID {
		return false
	}
	if query.AccountID != nil && intPtrValue(row.AccountID) != *query.AccountID {
		return false
	}
	if query.ProviderID != nil && intPtrValue(row.ProviderID) != *query.ProviderID {
		return false
	}
	if query.Model != "" && !strings.Contains(strings.ToLower(row.Model), strings.ToLower(query.Model)) {
		return false
	}
	if query.SourceEndpoint != "" && !strings.Contains(strings.ToLower(row.SourceEndpoint), strings.ToLower(query.SourceEndpoint)) {
		return false
	}
	if query.ErrorClass != "" && !strings.EqualFold(row.ErrorClass, query.ErrorClass) {
		return false
	}
	if query.Kind != "all" && row.Kind != query.Kind {
		return false
	}
	if query.EvidenceSource != "all" {
		switch query.EvidenceSource {
		case "usage":
			if !row.HasUsageLog {
				return false
			}
		case "ops_error":
			if !row.HasOpsErrorLog {
				return false
			}
		case "request_dump":
			if !row.HasRequestDump {
				return false
			}
		case "system_log":
			if !row.HasSystemLog {
				return false
			}
		}
	}
	if query.MinLatencyMS != nil {
		if row.LatencyMS == nil || *row.LatencyMS < *query.MinLatencyMS {
			return false
		}
	}
	if query.MaxLatencyMS != nil {
		if row.LatencyMS == nil || *row.LatencyMS > *query.MaxLatencyMS {
			return false
		}
	}
	if query.Search != "" && !requestEvidenceRowContains(row, query.Search) {
		return false
	}
	return true
}

func requestEvidenceRowContains(row requestEvidenceRow, raw string) bool {
	needle := strings.ToLower(strings.TrimSpace(raw))
	if needle == "" {
		return true
	}
	fields := []string{
		row.RequestID,
		row.Model,
		row.SourceEndpoint,
		row.SourceProtocol,
		row.TargetProtocol,
		row.ErrorClass,
		row.ErrorMessage,
		row.ErrorPhase,
		row.ErrorOwner,
		row.ErrorSource,
		row.UpstreamRequestID,
		row.LatestRequestDumpName,
		row.SystemLogSearchText,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

func requestEvidenceSystemLogSearchText(evidence requestEvidenceSystemLogEvidence) string {
	if len(evidence.Items) == 0 && evidence.Latest == nil {
		return ""
	}
	var b strings.Builder
	for _, log := range evidence.Items {
		appendRequestEvidenceSearchField(&b, log.Message)
		appendRequestEvidenceSearchField(&b, log.Source)
		appendRequestEvidenceSearchField(&b, log.TraceID)
	}
	if evidence.Latest != nil {
		appendRequestEvidenceSearchField(&b, evidence.Latest.Message)
		appendRequestEvidenceSearchField(&b, evidence.Latest.Source)
		appendRequestEvidenceSearchField(&b, evidence.Latest.TraceID)
	}
	return b.String()
}

func appendRequestEvidenceSearchField(b *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(value)
}

func requestEvidenceSystemLogNewer(left, right operationscontract.OpsSystemLog) bool {
	leftAt := left.CreatedAt.UTC()
	rightAt := right.CreatedAt.UTC()
	if leftAt.Equal(rightAt) {
		return left.ID > right.ID
	}
	return leftAt.After(rightAt)
}

func sortRequestEvidenceRows(rows []requestEvidenceRow, sortValue string) {
	sort.SliceStable(rows, func(i, j int) bool {
		switch sortValue {
		case "latency_desc":
			left := intPtrValue(rows[i].LatencyMS)
			right := intPtrValue(rows[j].LatencyMS)
			if left != right {
				return left > right
			}
		}
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].RequestID > rows[j].RequestID
	})
}

func paginateRequestEvidenceRows(rows []requestEvidenceRow, page, pageSize int) []requestEvidenceRow {
	start := (page - 1) * pageSize
	if start >= len(rows) {
		return []requestEvidenceRow{}
	}
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

func requestEvidenceRowToAPI(row requestEvidenceRow) apiopenapi.RequestEvidenceRow {
	out := apiopenapi.RequestEvidenceRow{
		Kind:                  apiopenapi.RequestEvidenceKind(row.Kind),
		EvidenceSource:        apiopenapi.RequestEvidenceSource(row.EvidenceSource),
		CreatedAt:             row.CreatedAt.UTC(),
		RequestId:             row.RequestID,
		HasUsageLog:           row.HasUsageLog,
		HasOpsErrorLog:        row.HasOpsErrorLog,
		HasRequestDump:        row.HasRequestDump,
		HasSystemLog:          row.HasSystemLog,
		RequestDumpCount:      row.RequestDumpCount,
		RequestDumpErrorCount: row.RequestDumpErrorCount,
		SystemLogCount:        row.SystemLogCount,
	}
	if row.UsageLogID != nil {
		value := apiopenapi.Id(strconv.Itoa(*row.UsageLogID))
		out.UsageLogId = &value
	}
	if row.OpsErrorLogID != nil {
		value := apiopenapi.Id(strconv.FormatInt(*row.OpsErrorLogID, 10))
		out.OpsErrorLogId = &value
	}
	if row.UserID != nil {
		value := apiopenapi.Id(strconv.Itoa(*row.UserID))
		out.UserId = &value
	}
	if row.APIKeyID != nil {
		value := apiopenapi.Id(strconv.Itoa(*row.APIKeyID))
		out.ApiKeyId = &value
	}
	out.AccountId = optionalIDString(row.AccountID)
	out.ProviderId = optionalIDString(row.ProviderID)
	out.SourceProtocol = nonEmptyStringPtr(row.SourceProtocol)
	out.SourceEndpoint = nonEmptyStringPtr(row.SourceEndpoint)
	out.TargetProtocol = nonEmptyStringPtr(row.TargetProtocol)
	out.Model = nonEmptyStringPtr(row.Model)
	out.StatusCode = row.StatusCode
	out.Success = row.Success
	out.ErrorClass = nonEmptyStringPtr(row.ErrorClass)
	out.ErrorMessage = nonEmptyStringPtr(row.ErrorMessage)
	out.ErrorPhase = nonEmptyStringPtr(row.ErrorPhase)
	out.ErrorOwner = nonEmptyStringPtr(row.ErrorOwner)
	out.ErrorSource = nonEmptyStringPtr(row.ErrorSource)
	out.UpstreamRequestId = nonEmptyStringPtr(row.UpstreamRequestID)
	out.AttemptNo = row.AttemptNo
	out.LatencyMs = row.LatencyMS
	out.InputTokens = row.InputTokens
	out.OutputTokens = row.OutputTokens
	out.TotalTokens = row.TotalTokens
	out.UsageEstimated = row.UsageEstimated
	if row.Resolution != "" {
		out.Resolution = (*apiopenapi.RequestEvidenceRowResolution)(&row.Resolution)
	}
	out.LatestRequestDumpName = nonEmptyStringPtr(row.LatestRequestDumpName)
	out.LatestRequestDumpCreatedAt = row.LatestRequestDumpCreatedAt
	return out
}

func requestEvidenceDetailBounds(rows []requestEvidenceRow, dumps []rlfcontract.FileDescriptor, systemLogs []operationscontract.OpsSystemLog) (time.Time, time.Time) {
	var first time.Time
	var last time.Time
	observe := func(value time.Time) {
		if value.IsZero() {
			return
		}
		value = value.UTC()
		if first.IsZero() || value.Before(first) {
			first = value
		}
		if last.IsZero() || value.After(last) {
			last = value
		}
	}
	for _, row := range rows {
		observe(row.CreatedAt)
		if row.LatestRequestDumpCreatedAt != nil {
			observe(*row.LatestRequestDumpCreatedAt)
		}
	}
	for _, desc := range dumps {
		observe(desc.CreatedAt)
	}
	for _, log := range systemLogs {
		observe(log.CreatedAt)
	}
	return first, last
}

func requestEvidenceSummaryFromDetail(rows []requestEvidenceRow, usageLogs []usagecontract.UsageLog, opsEntries []opserrorlogscontract.Entry, dumps map[string]requestDumpEvidence, systemLogs requestEvidenceSystemLogEvidence) requestEvidenceSummary {
	summary := requestEvidenceSummary{
		Kind:             "unknown",
		PrimarySource:    "system_log",
		AttemptCount:     len(rows),
		UsageLogCount:    len(usageLogs),
		OpsErrorLogCount: len(opsEntries),
	}
	if len(rows) > 0 || len(dumps) > 0 {
		summary.PrimarySource = "request_dump"
	}
	for _, dump := range dumps {
		summary.RequestDumpCount += dump.Count
		summary.RequestDumpErrorCount += dump.ErrorCount
	}
	var latest *requestEvidenceRow
	for i := range rows {
		row := &rows[i]
		if row.HasUsageLog {
			summary.HasUsageLog = true
		}
		if row.HasOpsErrorLog {
			summary.HasOpsErrorLog = true
		}
		if row.HasRequestDump {
			summary.HasRequestDump = true
		}
		if row.LatencyMS != nil {
			value := intPtrValue(summary.LatencyMS) + *row.LatencyMS
			summary.LatencyMS = &value
		}
		summary.InputTokens += intPtrValue(row.InputTokens)
		summary.OutputTokens += intPtrValue(row.OutputTokens)
		summary.TotalTokens += intPtrValue(row.TotalTokens)
		if latest == nil || row.CreatedAt.After(latest.CreatedAt) {
			latest = row
		}
	}
	if latest != nil {
		summary.Kind = latest.Kind
		summary.PrimarySource = latest.EvidenceSource
		summary.StatusCode = latest.StatusCode
		summary.ErrorClass = latest.ErrorClass
		summary.ErrorMessage = latest.ErrorMessage
		summary.ErrorPhase = latest.ErrorPhase
		summary.ErrorOwner = latest.ErrorOwner
		summary.ErrorSource = latest.ErrorSource
		summary.UpstreamRequestID = latest.UpstreamRequestID
	}
	if summary.Kind != "error" {
		for _, row := range rows {
			if row.Kind == "error" {
				summary.Kind = "error"
				summary.StatusCode = row.StatusCode
				summary.ErrorClass = row.ErrorClass
				summary.ErrorMessage = row.ErrorMessage
				summary.ErrorPhase = row.ErrorPhase
				summary.ErrorOwner = row.ErrorOwner
				summary.ErrorSource = row.ErrorSource
				summary.UpstreamRequestID = row.UpstreamRequestID
				break
			}
		}
	}
	if latest == nil && systemLogs.Total > 0 {
		summary.PrimarySource = "system_log"
	}
	return summary
}

func requestEvidenceDetailToAPI(detail requestEvidenceDetail, correlationID string) apiopenapi.RequestEvidenceDetailResponse {
	attempts := make([]apiopenapi.RequestEvidenceRow, 0, len(detail.Attempts))
	for _, row := range detail.Attempts {
		attempts = append(attempts, requestEvidenceRowToAPI(row))
	}
	dumps := make([]apiopenapi.RequestEvidenceDumpDescriptor, 0, len(detail.Dumps))
	for _, desc := range detail.Dumps {
		dumps = append(dumps, requestEvidenceDumpDescriptorToAPI(desc))
	}
	systemLogs := toAPIOpsSystemLogs(detail.SystemLog.Items)
	out := apiopenapi.RequestEvidenceDetailResponse{
		RequestId:         correlationID,
		EvidenceRequestId: detail.RequestID,
		Summary:           requestEvidenceSummaryToAPI(detail.Summary),
		Attempts:          attempts,
		RequestDumps:      dumps,
		SystemLogSummary:  requestEvidenceSystemLogSummaryToAPI(detail.SystemLog),
		SystemLogs:        systemLogs,
	}
	if !detail.FirstSeen.IsZero() {
		out.FirstSeenAt = &detail.FirstSeen
	}
	if !detail.LastSeen.IsZero() {
		out.LastSeenAt = &detail.LastSeen
	}
	return out
}

func requestEvidenceSystemLogSummaryToAPI(evidence requestEvidenceSystemLogEvidence) apiopenapi.RequestEvidenceSystemLogSummary {
	levelCounts := make(map[string]int, len(evidence.LevelCounts))
	for level, count := range evidence.LevelCounts {
		levelCounts[string(level)] = count
	}
	out := apiopenapi.RequestEvidenceSystemLogSummary{
		TotalCount:  evidence.Total,
		LevelCounts: levelCounts,
	}
	if evidence.Latest != nil {
		level := apiopenapi.OpsSystemLogLevel(evidence.Latest.Level)
		out.LatestLevel = &level
		out.LatestMessage = nonEmptyStringPtr(evidence.Latest.Message)
		out.LatestSource = nonEmptyStringPtr(evidence.Latest.Source)
		latestAt := evidence.Latest.CreatedAt.UTC()
		out.LatestAt = &latestAt
	}
	return out
}

func requestEvidenceSummaryToAPI(summary requestEvidenceSummary) apiopenapi.RequestEvidenceSummary {
	out := apiopenapi.RequestEvidenceSummary{
		Kind:                  apiopenapi.RequestEvidenceKind(summary.Kind),
		PrimarySource:         apiopenapi.RequestEvidenceSource(summary.PrimarySource),
		AttemptCount:          summary.AttemptCount,
		UsageLogCount:         summary.UsageLogCount,
		OpsErrorLogCount:      summary.OpsErrorLogCount,
		RequestDumpCount:      summary.RequestDumpCount,
		RequestDumpErrorCount: summary.RequestDumpErrorCount,
		HasUsageLog:           summary.HasUsageLog,
		HasOpsErrorLog:        summary.HasOpsErrorLog,
		HasRequestDump:        summary.HasRequestDump,
	}
	out.LatencyMs = summary.LatencyMS
	if summary.InputTokens > 0 {
		out.InputTokens = &summary.InputTokens
	}
	if summary.OutputTokens > 0 {
		out.OutputTokens = &summary.OutputTokens
	}
	if summary.TotalTokens > 0 {
		out.TotalTokens = &summary.TotalTokens
	}
	out.StatusCode = summary.StatusCode
	out.ErrorClass = nonEmptyStringPtr(summary.ErrorClass)
	out.ErrorMessage = nonEmptyStringPtr(summary.ErrorMessage)
	out.ErrorPhase = nonEmptyStringPtr(summary.ErrorPhase)
	out.ErrorOwner = nonEmptyStringPtr(summary.ErrorOwner)
	out.ErrorSource = nonEmptyStringPtr(summary.ErrorSource)
	out.UpstreamRequestId = nonEmptyStringPtr(summary.UpstreamRequestID)
	return out
}

func requestEvidenceDumpDescriptorToAPI(desc rlfcontract.FileDescriptor) apiopenapi.RequestEvidenceDumpDescriptor {
	out := apiopenapi.RequestEvidenceDumpDescriptor{
		Name:           desc.Name,
		CreatedAt:      desc.CreatedAt.UTC(),
		SizeBytes:      desc.Size,
		IsErrorOnly:    desc.IsErrorOnly,
		RequestId:      desc.RequestID,
		AttemptCount:   desc.AttemptCount,
		ResponseCount:  desc.ResponseCount,
		HasSummary:     desc.HasSummary,
		SourceProtocol: nonEmptyStringPtr(desc.SourceProtocol),
		SourceEndpoint: nonEmptyStringPtr(desc.SourceEndpoint),
		ErrorClass:     nonEmptyStringPtr(desc.ErrorClass),
	}
	out.UserId = nonEmptyStringPtr(desc.UserID)
	out.ApiKeyId = nonEmptyStringPtr(desc.APIKeyID)
	out.AccountId = nonEmptyStringPtr(desc.AccountID)
	out.StartedAt = desc.StartedAt
	out.Success = desc.Success
	out.StatusCode = desc.StatusCode
	out.LatencyMs = desc.LatencyMS
	return out
}

func requestEvidenceKindFromSuccess(success *bool) string {
	if success == nil {
		return "unknown"
	}
	if *success {
		return "success"
	}
	return "error"
}

func requestEvidenceKey(requestID string, attemptNo *int, fallbackID *int) string {
	if attemptNo != nil && *attemptNo > 0 {
		return requestID + "#" + strconv.Itoa(*attemptNo)
	}
	if fallbackID != nil {
		return requestID + "#usage:" + strconv.Itoa(*fallbackID)
	}
	return requestID + "#0"
}

func requestEvidenceOptionalNonNegativeInt(raw string, field string) (int, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false, errors.New("invalid " + field)
	}
	return n, true, nil
}

func intPtrValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

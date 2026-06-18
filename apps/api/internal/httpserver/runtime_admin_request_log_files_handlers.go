package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleListAdminRequestLogFiles serves GET /api/v1/admin/request-log-files.
//
// Query params:
//
//	request_id  — prefix match against the captured request id
//	error_only  — when "true", restrict to error-* files
//	from / to   — RFC3339 timestamps bounding created_at
//	limit       — cap on the number of returned descriptors (default 100,
//	              max 500)
func (s *Server) handleListAdminRequestLogFiles(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	reader := s.runtime.requestLogFileReader()
	if reader == nil {
		writeJSONAny(w, http.StatusOK, apiopenapi.RequestLogFileListResponse{
			Data:       []apiopenapi.RequestLogFileDescriptor{},
			Pagination: requestLogFilePagination(100, 0),
			RequestId:  requestID,
		})
		return
	}

	from, err := parseRequestLogFileTimestamp(r.URL.Query().Get("from"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "invalid from timestamp", requestID)
		return
	}
	to, err := parseRequestLogFileTimestamp(r.URL.Query().Get("to"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "invalid to timestamp", requestID)
		return
	}
	if from != nil && to != nil && !from.Before(*to) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "from must be before to", requestID)
		return
	}
	limit, err := parseRequestLogFileLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "invalid limit", requestID)
		return
	}

	filter := rlfcontract.ListFilter{
		RequestIDPrefix: strings.TrimSpace(r.URL.Query().Get("request_id")),
		ErrorOnly:       strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("error_only")), "true"),
		From:            from,
		To:              to,
	}
	descs, err := reader.List(r.Context(), filter)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list request log files", requestID)
		return
	}
	total := len(descs)
	if len(descs) > limit {
		descs = descs[:limit]
	}
	data := make([]apiopenapi.RequestLogFileDescriptor, 0, len(descs))
	for _, d := range descs {
		data = append(data, descriptorToAPI(d))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RequestLogFileListResponse{
		Data:       data,
		Pagination: requestLogFilePagination(limit, total),
		RequestId:  requestID,
	})
}

// handleGetAdminRequestLogFile serves GET /api/v1/admin/request-log-files/{name}.
func (s *Server) handleGetAdminRequestLogFile(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	reader := s.runtime.requestLogFileReader()
	if reader == nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "request log file not found", requestID)
		return
	}
	name := r.PathValue("name")
	desc, err := reader.Get(r.Context(), name)
	if err != nil {
		writeRequestLogFileLookupError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RequestLogFileResponse{
		Data:      descriptorToAPI(desc),
		RequestId: requestID,
	})
}

// handleDownloadAdminRequestLogFile serves GET /api/v1/admin/request-log-files/{name}/download.
// The response is the raw file content with Content-Type text/plain so admins
// can preview the dump in a browser or wget it for triage.
func (s *Server) handleDownloadAdminRequestLogFile(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	reader := s.runtime.requestLogFileReader()
	if reader == nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "request log file not found", requestID)
		return
	}
	name := r.PathValue("name")
	body, err := reader.Open(r.Context(), name)
	if err != nil {
		writeRequestLogFileLookupError(w, err, requestID)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// handleDeleteAdminRequestLogFile serves DELETE /api/v1/admin/request-log-files/{name}.
func (s *Server) handleDeleteAdminRequestLogFile(w http.ResponseWriter, r *http.Request) {
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
	reader := s.runtime.requestLogFileReader()
	if reader == nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "request log file not found", requestID)
		return
	}
	name := r.PathValue("name")
	if err := reader.Delete(r.Context(), name); err != nil {
		writeRequestLogFileLookupError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.DeleteRequestLogFileResponse{
		Success:   true,
		RequestId: requestID,
	})
}

func descriptorToAPI(d rlfcontract.FileDescriptor) apiopenapi.RequestLogFileDescriptor {
	desc := apiopenapi.RequestLogFileDescriptor{
		Name:        d.Name,
		Size:        d.Size,
		CreatedAt:   d.CreatedAt.UTC(),
		RequestId:   d.RequestID,
		IsErrorOnly: d.IsErrorOnly,
	}
	if d.UserID != "" {
		desc.UserId = &d.UserID
	}
	if d.APIKeyID != "" {
		desc.ApiKeyId = &d.APIKeyID
	}
	if d.AccountID != "" {
		desc.AccountId = &d.AccountID
	}
	if d.SourceProtocol != "" {
		desc.SourceProtocol = &d.SourceProtocol
	}
	if d.SourceEndpoint != "" {
		desc.SourceEndpoint = &d.SourceEndpoint
	}
	if d.StartedAt != nil {
		startedAt := d.StartedAt.UTC()
		desc.StartedAt = &startedAt
	}
	desc.Success = d.Success
	desc.StatusCode = d.StatusCode
	if d.ErrorClass != "" {
		desc.ErrorClass = &d.ErrorClass
	}
	desc.LatencyMs = d.LatencyMS
	desc.AttemptCount = &d.AttemptCount
	desc.ResponseCount = &d.ResponseCount
	desc.HasSummary = &d.HasSummary
	return desc
}

func parseRequestLogFileTimestamp(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		parsed = parsed.UTC()
		return &parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		parsed = parsed.UTC()
		return &parsed, nil
	}
	return nil, errors.New("invalid request log file timestamp")
}

func parseRequestLogFileLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 100, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 500 {
		return 0, errors.New("invalid request log file limit")
	}
	return limit, nil
}

func requestLogFilePagination(limit, total int) apiopenapi.Pagination {
	return apiopenapi.Pagination{
		Page:     1,
		PageSize: limit,
		Total:    total,
		HasNext:  total > limit,
	}
}

// writeRequestLogFileLookupError converts the three failure modes of the
// reader (not found, invalid name, other I/O) into HTTP responses.
func writeRequestLogFileLookupError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, rlfcontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "request log file not found", requestID)
	case errors.Is(err, rlfcontract.ErrInvalidName):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "invalid request log file name", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to read request log file", requestID)
	}
}

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
			Pagination: paginationWithRequest(r, 0),
			RequestId:  requestID,
		})
		return
	}

	filter := rlfcontract.ListFilter{
		RequestIDPrefix: strings.TrimSpace(r.URL.Query().Get("request_id")),
		ErrorOnly:       strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("error_only")), "true"),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.From = &t
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.To = &t
		}
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	filter.Limit = limit

	descs, err := reader.List(r.Context(), filter)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list request log files", requestID)
		return
	}
	data := make([]apiopenapi.RequestLogFileDescriptor, 0, len(descs))
	for _, d := range descs {
		data = append(data, descriptorToAPI(d))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RequestLogFileListResponse{
		Data:       data,
		Pagination: paginationWithRequest(r, len(data)),
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
	return apiopenapi.RequestLogFileDescriptor{
		Name:        d.Name,
		Size:        d.Size,
		CreatedAt:   d.CreatedAt.UTC(),
		RequestId:   d.RequestID,
		IsErrorOnly: d.IsErrorOnly,
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

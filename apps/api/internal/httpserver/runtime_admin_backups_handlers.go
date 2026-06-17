package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	backupsnapcontract "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	defaultBackupSnapshotsLimit = 50
	maxBackupSnapshotsLimit     = 200
)

// handleListAdminBackupSnapshots returns the paginated backup-snapshot
// history. Offset/limit are the OFFSET-paginated variant used by the admin
// "Database backups" panel — the global Page/PageSize-paginated variant
// doesn't fit because the history is meant to be scanned from newest to
// oldest, not paged by 1-indexed page number.
func (s *Server) handleListAdminBackupSnapshots(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if s.runtime.backupSnapshots == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "backup history is not available in this storage backend", requestID)
		return
	}
	opts := backupSnapshotListOptionsFromRequest(r)
	result, err := s.runtime.backupSnapshots.ListBackupSnapshots(r.Context(), opts)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list backup snapshots", requestID)
		return
	}
	data := make([]apiopenapi.BackupSnapshot, 0, len(result.Items))
	for _, row := range result.Items {
		data = append(data, toAPIBackupSnapshot(row))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.BackupSnapshotListResponse{
		Data: data,
		Pagination: apiopenapi.BackupSnapshotPagination{
			Total:  result.Total,
			Offset: opts.Offset,
			Limit:  opts.Limit,
		},
		RequestId: requestID,
	})
}

// handleTriggerAdminBackupSnapshot kicks off a real backup synchronously
// and returns the resulting history row. The "Snapshot now" button calls
// this. Audit-logged as backup_snapshot.trigger so an operator-driven run
// shows up in the trail.
func (s *Server) handleTriggerAdminBackupSnapshot(w http.ResponseWriter, r *http.Request) {
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
	if s.runtime.backupSnapshots == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "backup history is not available in this storage backend", requestID)
		return
	}
	row, err := s.runtime.backupSnapshots.TriggerBackupNow(r.Context(), session.User.ID)
	if err != nil {
		// Treat "no snapshot created" (disabled, throttled, guard conflict)
		// as a 400 so the UI can show a meaningful message; treat real
		// pg_dump/IO errors as 500.
		if errors.Is(err, backupsnapcontract.ErrNotFound) {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "no backup snapshot was created (check Backup.Enabled / run guard / disk)", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "trigger backup snapshot failed: "+err.Error(), requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "backup_snapshot.trigger", "backup_snapshot", strconv.Itoa(row.ID), nil, backupSnapshotAuditSnapshot(row)))
	writeJSONAny(w, http.StatusOK, apiopenapi.BackupSnapshotResponse{
		Data:      toAPIBackupSnapshot(row),
		RequestId: requestID,
	})
}

// handleGetAdminBackupSnapshot returns a single history row.
func (s *Server) handleGetAdminBackupSnapshot(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if s.runtime.backupSnapshots == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "backup history is not available in this storage backend", requestID)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid backup snapshot id", requestID)
		return
	}
	row, err := s.runtime.backupSnapshots.GetBackupSnapshot(r.Context(), id)
	if err != nil {
		if errors.Is(err, backupsnapcontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "backup snapshot not found", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load backup snapshot", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.BackupSnapshotResponse{
		Data:      toAPIBackupSnapshot(row),
		RequestId: requestID,
	})
}

// handleDeleteAdminBackupSnapshot removes the file and drops the row.
// Audit-logged as backup_snapshot.delete.
func (s *Server) handleDeleteAdminBackupSnapshot(w http.ResponseWriter, r *http.Request) {
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
	if s.runtime.backupSnapshots == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "backup history is not available in this storage backend", requestID)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid backup snapshot id", requestID)
		return
	}
	// Capture the pre-delete row so the audit log carries the file_path and
	// size — important since the row will be gone after this call.
	before, err := s.runtime.backupSnapshots.GetBackupSnapshot(r.Context(), id)
	if err != nil {
		if errors.Is(err, backupsnapcontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "backup snapshot not found", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load backup snapshot", requestID)
		return
	}
	if err := s.runtime.backupSnapshots.DeleteBackupSnapshot(r.Context(), id, session.User.ID); err != nil {
		if errors.Is(err, backupsnapcontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "backup snapshot not found", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete backup snapshot: "+err.Error(), requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "backup_snapshot.delete", "backup_snapshot", strconv.Itoa(id), backupSnapshotAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, apiopenapi.DeleteAdminBackupSnapshotResponse{
		Data: apiopenapi.DeleteAdminBackupSnapshotResult{
			Id:      apiopenapi.Id(strconv.Itoa(id)),
			Deleted: true,
		},
		RequestId: requestID,
	})
}

// handleDownloadAdminBackupSnapshot streams the dump file back to the
// operator as application/octet-stream. Rejects rows whose file has been
// retention-wiped (status="superseded") or never completed.
func (s *Server) handleDownloadAdminBackupSnapshot(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if s.runtime.backupSnapshots == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "backup history is not available in this storage backend", requestID)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid backup snapshot id", requestID)
		return
	}
	row, err := s.runtime.backupSnapshots.GetBackupSnapshot(r.Context(), id)
	if err != nil {
		if errors.Is(err, backupsnapcontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "backup snapshot not found", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load backup snapshot", requestID)
		return
	}
	if row.Status == backupsnapcontract.StatusSuperseded {
		writeStandardError(w, http.StatusConflict, apiopenapi.INVALIDREQUEST, "backup snapshot file has been removed by retention", requestID)
		return
	}
	if row.Status != backupsnapcontract.StatusSuccess {
		writeStandardError(w, http.StatusConflict, apiopenapi.INVALIDREQUEST, "backup snapshot is not downloadable (status="+row.Status+")", requestID)
		return
	}
	open, err := s.runtime.backupSnapshots.OpenBackupFile(r.Context(), id)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to open backup snapshot file: "+err.Error(), requestID)
		return
	}
	defer open.Reader.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", open.FileName))
	if open.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(open.Size, 10))
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	// Stream the body. Don't wrap errors — the response is already in
	// flight, so the best we can do is stop writing.
	buf := make([]byte, 64*1024)
	for {
		n, readErr := open.Reader.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}

func backupSnapshotListOptionsFromRequest(r *http.Request) backupsnapcontract.ListOptions {
	q := r.URL.Query()
	offset := 0
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	limit := defaultBackupSnapshotsLimit
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxBackupSnapshotsLimit {
		limit = maxBackupSnapshotsLimit
	}
	return backupsnapcontract.ListOptions{
		Offset: offset,
		Limit:  limit,
		Status: strings.TrimSpace(q.Get("status")),
	}
}

func toAPIBackupSnapshot(row backupsnapcontract.BackupSnapshot) apiopenapi.BackupSnapshot {
	out := apiopenapi.BackupSnapshot{
		Id:                apiopenapi.Id(strconv.Itoa(row.ID)),
		Kind:              apiopenapi.BackupSnapshotKind(row.Kind),
		Status:            apiopenapi.BackupSnapshotStatus(row.Status),
		StartedAt:         row.StartedAt,
		SizeBytes:         row.SizeBytes,
		Sha256:            row.SHA256,
		FilePath:          row.FilePath,
		ErrorMessage:      row.ErrorMessage,
		TriggeredByUserId: row.TriggeredByUserID,
	}
	if row.CompletedAt != nil {
		completed := *row.CompletedAt
		out.CompletedAt = &completed
	}
	return out
}

func backupSnapshotAuditSnapshot(row backupsnapcontract.BackupSnapshot) map[string]any {
	out := map[string]any{
		"id":                   row.ID,
		"kind":                 row.Kind,
		"status":               row.Status,
		"started_at":           row.StartedAt,
		"size_bytes":           row.SizeBytes,
		"sha256":               row.SHA256,
		"file_path":            row.FilePath,
		"triggered_by_user_id": row.TriggeredByUserID,
	}
	if row.CompletedAt != nil {
		out["completed_at"] = *row.CompletedAt
	}
	if row.ErrorMessage != "" {
		out["error_message"] = row.ErrorMessage
	}
	return out
}

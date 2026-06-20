package httpserver

import (
	"net/http"
	"strconv"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminProxies(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	status := accountcontract.ProxyStatus(r.URL.Query().Get("status"))
	items, err := s.runtime.accounts.ListProxies(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list proxies", requestID)
		return
	}
	data := make([]apiopenapi.ProxyDefinition, 0, len(items))
	for _, item := range items {
		if status != "" && item.Status != status {
			continue
		}
		data = append(data, toAPIProxyDefinition(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ProxyDefinitionListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminProxy(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateProxyDefinitionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	backupProxyID, err := optionalAPIIDToInt(body.BackupProxyId)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid backup proxy id", requestID)
		return
	}
	proxy, err := s.runtime.accounts.CreateProxy(r.Context(), accountcontract.CreateProxyRequest{
		Name:          body.Name,
		Type:          accountcontract.ProxyType(body.Type),
		URL:           optionalStringValue(body.Url),
		Status:        toProxyStatusPtr(body.Status),
		Metadata:      jsonObjectToMap(body.Metadata),
		CountryCode:   body.CountryCode,
		CountryName:   body.CountryName,
		ExpiresAt:     body.ExpiresAt,
		FallbackMode:  toProxyFallbackModePtr(body.FallbackMode),
		BackupProxyID: backupProxyID,
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.create", "proxy", strconv.Itoa(proxy.ID), nil, proxyAuditSnapshot(proxy)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ProxyDefinitionResponse{
		Data:      toAPIProxyDefinition(proxy),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminProxy(w http.ResponseWriter, r *http.Request) {
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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindProxyByID(r.Context(), id)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "proxy not found", requestID)
		return
	}
	var body apiopenapi.UpdateProxyDefinitionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	backupProxyID, err := optionalAPIIDToInt(body.BackupProxyId)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid backup proxy id", requestID)
		return
	}
	updated, err := s.runtime.accounts.UpdateProxy(r.Context(), id, accountcontract.UpdateProxyRequest{
		Name:               body.Name,
		Type:               toProxyTypePtr(body.Type),
		URL:                body.Url,
		Status:             toProxyStatusPtr(body.Status),
		Metadata:           jsonObjectToMapPtr(body.Metadata),
		CountryCode:        body.CountryCode,
		CountryName:        body.CountryName,
		ExpiresAt:          body.ExpiresAt,
		ClearExpiresAt:     boolPtrValue(body.ClearExpiresAt),
		FallbackMode:       toProxyFallbackModePtr(body.FallbackMode),
		BackupProxyID:      backupProxyID,
		ClearBackupProxyID: boolPtrValue(body.ClearBackupProxyId),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.update", "proxy", strconv.Itoa(updated.ID), proxyAuditSnapshot(before), proxyAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProxyDefinitionResponse{
		Data:      toAPIProxyDefinition(updated),
		RequestId: requestID,
	})
}

// handleBatchCreateAdminProxies bulk-creates proxy definitions. Dedupes by
// name against existing proxies (and within the request itself) so importing
// a CSV that overlaps a previous batch is idempotent — duplicates surface in
// `skipped`, hard validation failures in `errors`, the rest still apply.
func (s *Server) handleBatchCreateAdminProxies(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchCreateProxiesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid batch proxy create request", requestID)
		return
	}
	if len(body.Proxies) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "proxies must be non-empty", requestID)
		return
	}
	reqs := make([]accountcontract.CreateProxyRequest, 0, len(body.Proxies))
	for _, p := range body.Proxies {
		backupProxyID, err := optionalAPIIDToInt(p.BackupProxyId)
		if err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid backup proxy id", requestID)
			return
		}
		reqs = append(reqs, accountcontract.CreateProxyRequest{
			Name:          p.Name,
			Type:          accountcontract.ProxyType(p.Type),
			URL:           optionalStringValue(p.Url),
			Status:        toProxyStatusPtr(p.Status),
			Metadata:      jsonObjectToMap(p.Metadata),
			CountryCode:   p.CountryCode,
			CountryName:   p.CountryName,
			ExpiresAt:     p.ExpiresAt,
			FallbackMode:  toProxyFallbackModePtr(p.FallbackMode),
			BackupProxyID: backupProxyID,
		})
	}
	results, err := s.runtime.accounts.BatchCreateProxies(r.Context(), reqs)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch proxy create request", requestID)
		return
	}
	created := make([]apiopenapi.ProxyDefinition, 0, len(results))
	skipped := make([]apiopenapi.BatchCreateProxiesSkippedRow, 0)
	errs := make([]apiopenapi.BatchCreateProxiesErrorRow, 0)
	for _, row := range results {
		switch {
		case row.Created != nil:
			created = append(created, toAPIProxyDefinition(*row.Created))
		case row.SkippedReason != "":
			skipped = append(skipped, apiopenapi.BatchCreateProxiesSkippedRow{
				Index:  row.Index,
				Name:   row.Name,
				Reason: row.SkippedReason,
			})
		case row.Err != nil:
			errs = append(errs, apiopenapi.BatchCreateProxiesErrorRow{
				Index:   row.Index,
				Message: row.Err.Error(),
			})
		}
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.batch_create", "proxy", "bulk", nil, map[string]any{
		"requested":     len(body.Proxies),
		"created_count": len(created),
		"skipped_count": len(skipped),
		"error_count":   len(errs),
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchCreateProxiesResponse{
		Data: apiopenapi.BatchCreateProxiesResult{
			CreatedCount: len(created),
			Created:      created,
			Skipped:      skipped,
			Errors:       errs,
		},
		RequestId: requestID,
	})
}

// handleBatchDeleteAdminProxies bulk soft-deletes proxies. Missing ids
// surface in `errors` without failing the call.
func (s *Server) handleBatchDeleteAdminProxies(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchDeleteProxiesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid batch proxy delete request", requestID)
		return
	}
	ids, err := apiIDsValueToInts(body.ProxyIds)
	if err != nil || len(ids) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy ids", requestID)
		return
	}
	results, err := s.runtime.accounts.BatchDeleteProxies(r.Context(), ids)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch proxy delete request", requestID)
		return
	}
	deletedIDs := make([]apiopenapi.Id, 0, len(results))
	errs := make([]apiopenapi.BatchDeleteProxiesErrorRow, 0)
	for _, row := range results {
		if row.Err == nil {
			deletedIDs = append(deletedIDs, apiopenapi.Id(strconv.Itoa(row.ID)))
		} else {
			errs = append(errs, apiopenapi.BatchDeleteProxiesErrorRow{
				Id:      apiopenapi.Id(strconv.Itoa(row.ID)),
				Message: row.Err.Error(),
			})
		}
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.batch_delete", "proxy", "bulk", nil, map[string]any{
		"requested":     len(ids),
		"deleted_count": len(deletedIDs),
		"error_count":   len(errs),
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchDeleteProxiesResponse{
		Data: apiopenapi.BatchDeleteProxiesResult{
			DeletedCount: len(deletedIDs),
			DeletedIds:   deletedIDs,
			Errors:       errs,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminProxy(w http.ResponseWriter, r *http.Request) {
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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindProxyByID(r.Context(), id)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "proxy not found", requestID)
		return
	}
	if err := s.runtime.accounts.DeleteProxy(r.Context(), id); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete proxy", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.delete", "proxy", strconv.Itoa(id), proxyAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

// handleBatchTestAdminProxies probes many proxies in one call. The body is
// {proxy_ids: Id[]}; the response is one ProxyBatchTestRow per input id in
// the same order. Server-side concurrency keeps the call fast even on
// 50-row selections — see Service.BatchTestProxies for the cap.
func (s *Server) handleBatchTestAdminProxies(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchTestProxiesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid batch test request", requestID)
		return
	}
	ids := make([]int, 0, len(body.ProxyIds))
	for _, raw := range body.ProxyIds {
		id, err := strconv.Atoi(string(raw))
		if err != nil || id <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy id in batch", requestID)
			return
		}
		ids = append(ids, id)
	}
	rows, err := s.runtime.accounts.BatchTestProxies(r.Context(), ids)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch test request", requestID)
		return
	}
	out := make([]apiopenapi.ProxyBatchTestRow, 0, len(rows))
	var okCount, failCount int
	for _, row := range rows {
		out = append(out, apiopenapi.ProxyBatchTestRow{
			ProxyId: apiopenapi.Id(strconv.Itoa(row.ProxyID)),
			Result: apiopenapi.ProxyTestResult{
				Ok:         row.Result.OK,
				LatencyMs:  row.Result.LatencyMS,
				StatusCode: row.Result.StatusCode,
				ErrorClass: row.Result.ErrorClass,
				TargetUrl:  row.Result.TargetURL,
			},
		})
		if row.Result.OK {
			okCount++
		} else {
			failCount++
		}
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.batch_test", "proxy", "bulk", nil, map[string]any{
		"requested": len(ids),
		"ok":        okCount,
		"failed":    failCount,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchTestProxiesResponse{
		Data:      out,
		RequestId: requestID,
	})
}

// handleTestAdminProxy issues a probe through the proxy and returns the
// outcome. Always 200 — categorized failures live in the body (ok=false,
// error_class set). Wraps the audit log so an operator-initiated probe
// shows up in the trail.
func (s *Server) handleTestAdminProxy(w http.ResponseWriter, r *http.Request) {
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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy id", requestID)
		return
	}
	// Body is optional — only target_url is read, and an absent body is fine.
	var body apiopenapi.TestProxyRequest
	if r.ContentLength > 0 {
		if err := s.decodeJSONBody(w, r, &body); err != nil {
			writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid proxy test request", requestID)
			return
		}
	}
	target := ""
	if body.TargetUrl != nil {
		target = *body.TargetUrl
	}
	result, err := s.runtime.accounts.TestProxy(r.Context(), id, target)
	if err != nil {
		// Service returns ErrNotFound when the proxy was deleted between the
		// list and the click — surface that explicitly so the UI can refresh.
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "proxy not found", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.test", "proxy", strconv.Itoa(id), nil, map[string]any{
		"ok":          result.OK,
		"latency_ms":  result.LatencyMS,
		"status_code": result.StatusCode,
		"error_class": result.ErrorClass,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProxyTestResultResponse{
		Data: apiopenapi.ProxyTestResult{
			Ok:         result.OK,
			LatencyMs:  result.LatencyMS,
			StatusCode: result.StatusCode,
			ErrorClass: result.ErrorClass,
			TargetUrl:  result.TargetURL,
		},
		RequestId: requestID,
	})
}

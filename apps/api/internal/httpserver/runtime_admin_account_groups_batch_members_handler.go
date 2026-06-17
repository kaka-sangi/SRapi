package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBatchAddAdminAccountGroupMembers + handleBatchRemoveAdminAccountGroupMembers
// live in their own file because runtime_admin_catalog_handlers.go is already
// past the architecture-test size limit and these endpoints are the user's
// most-requested batch surface (adding 1000 accounts to one group is the
// single biggest operator pain the sub2api comparison surfaced).
//
// Idempotent semantics on both: already-member rows on add and not-member
// rows on remove count as success. Operator-visible per-row failures
// (account not found, duplicate in batch, store conflict) surface in
// `errors[]` without aborting the batch.

func (s *Server) handleBatchAddAdminAccountGroupMembers(w http.ResponseWriter, r *http.Request) {
	s.handleBatchGroupMembers(w, r, batchGroupMembersAdd)
}

func (s *Server) handleBatchRemoveAdminAccountGroupMembers(w http.ResponseWriter, r *http.Request) {
	s.handleBatchGroupMembers(w, r, batchGroupMembersRemove)
}

type batchGroupMembersAction int

const (
	batchGroupMembersAdd batchGroupMembersAction = iota
	batchGroupMembersRemove
)

func (s *Server) handleBatchGroupMembers(w http.ResponseWriter, r *http.Request, action batchGroupMembersAction) {
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
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group id", requestID)
		return
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.Gateway.MaxBodySize+1))
	if err != nil || int64(len(bodyBytes)) > s.cfg.Gateway.MaxBodySize {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}
	var body apiopenapi.BatchAccountGroupMembersRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}

	ids := make([]int, 0, len(body.AccountIds))
	for _, raw := range body.AccountIds {
		id, parseErr := strconv.Atoi(string(raw))
		if parseErr != nil || id <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id in batch", requestID)
			return
		}
		ids = append(ids, id)
	}

	var (
		results   []contractAccountGroupBatchResultRow
		actionTag string
	)
	switch action {
	case batchGroupMembersAdd:
		actionTag = "add"
		serviceResults, err := s.runtime.accounts.BatchAddAccountsToGroup(r.Context(), groupID, ids)
		if err != nil {
			s.writeBatchGroupMembersOuterError(w, err, requestID)
			return
		}
		results = make([]contractAccountGroupBatchResultRow, len(serviceResults))
		for i, row := range serviceResults {
			results[i] = contractAccountGroupBatchResultRow{AccountID: row.AccountID, Error: row.Error}
		}
	case batchGroupMembersRemove:
		actionTag = "remove"
		serviceResults, err := s.runtime.accounts.BatchRemoveAccountsFromGroup(r.Context(), groupID, ids)
		if err != nil {
			s.writeBatchGroupMembersOuterError(w, err, requestID)
			return
		}
		results = make([]contractAccountGroupBatchResultRow, len(serviceResults))
		for i, row := range serviceResults {
			results[i] = contractAccountGroupBatchResultRow{AccountID: row.AccountID, Error: row.Error}
		}
	}

	appliedIDs := make([]apiopenapi.Id, 0, len(results))
	errorRows := make([]apiopenapi.BatchAccountGroupMembersErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			appliedIDs = append(appliedIDs, apiopenapi.Id(strconv.Itoa(row.AccountID)))
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchAccountGroupMembersErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.AccountID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.AccountID)
	}

	// Audit — bulk membership changes are operator-significant enough to
	// always record, but per-row credentials/secrets are not in scope here
	// (the rows are just account ids).
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"account_group.batch_members_"+actionTag,
		"account_group",
		strconv.Itoa(groupID),
		nil,
		map[string]any{
			"requested":  len(ids),
			"succeeded":  len(appliedIDs),
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchAccountGroupMembersResponse{
		Data: apiopenapi.BatchAccountGroupMembersResult{
			AppliedCount: len(appliedIDs),
			AppliedIds:   appliedIDs,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}

type contractAccountGroupBatchResultRow struct {
	AccountID int
	Error     string
}

func (s *Server) writeBatchGroupMembersOuterError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, accountservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch members request", requestID)
	default:
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, err.Error(), requestID)
	}
}

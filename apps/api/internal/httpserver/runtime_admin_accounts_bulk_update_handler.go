package httpserver

import (
	"net/http"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBulkUpdateAdminAccounts is the sub2api `BulkUpdateAccountsRequest`
// superset port. Single endpoint that updates an arbitrary subset of the
// scheduler / routing fields across many provider accounts in one call.
// Target selection: explicit `account_ids` OR server-side `filters` —
// the latter mirrors the GET /accounts list-query knobs so what the
// admin table shows IS what the bulk-edit hits ("Edit Filtered" without
// round-tripping IDs through the client).
//
// Filter-mode safety net: an empty filter set is treated as "match
// nothing" (refuses to operate on every account by accident). The list
// resolver reuses filterAccounts + the group-membership lookup that
// powers handleListAdminAccounts so the resolved selection is
// identical to what the UI is currently displaying.
//
// Per-row failures collect in `errors` without aborting the batch —
// same best-effort semantics as the existing batch-update endpoints.
// max_concurrency rides through account metadata (the same key the
// scheduler reads at admission, identical to
// `/admin/accounts/batch-concurrency`) so the new endpoint doesn't
// fragment per-row vs. uniform concurrency writes.
func (s *Server) handleBulkUpdateAdminAccounts(w http.ResponseWriter, r *http.Request) {
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

	var body apiopenapi.BulkUpdateProviderAccountsRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid bulk update request", requestID)
		return
	}

	accountIDs, err := s.resolveBulkUpdateTargets(r, body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, err.Error(), requestID)
		return
	}
	if len(accountIDs) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "no accounts matched the bulk-update selection", requestID)
		return
	}

	updateReq, hasFields := bulkUpdateAccountsRequestFromAPI(body)
	if !hasFields {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "bulk update requires at least one field", requestID)
		return
	}

	result := s.runtime.accounts.BatchUpdateFields(r.Context(), accountIDs, updateReq)

	updatedIDs := make([]apiopenapi.Id, 0, len(result.Updated))
	for _, updated := range result.Updated {
		updatedIDs = append(updatedIDs, apiopenapi.Id(strconv.Itoa(updated.ID)))
	}

	auditDelta := map[string]any{
		"account_ids":   accountIDs,
		"updated_ids":   updatedIDs,
		"updated_count": len(updatedIDs),
		"errors":        result.Errors,
		"fields":        bulkUpdateAuditFields(body),
	}
	if body.Filters != nil {
		auditDelta["filters"] = bulkUpdateAuditFilters(*body.Filters)
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.bulk_update", "provider_account", "bulk", nil, auditDelta))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchUpdateAccountsResponse{
		Data: apiopenapi.BatchUpdateAccountsResult{
			Errors:       result.Errors,
			UpdatedCount: len(updatedIDs),
			UpdatedIds:   updatedIDs,
		},
		RequestId: requestID,
	})
}

// resolveBulkUpdateTargets returns the account IDs the bulk-edit should
// hit. Explicit `account_ids` wins (matches sub2api). Otherwise filters
// are resolved through filterAccounts + the group-membership lookup so
// the resolved set matches what the admin table renders for the same
// query string. An empty filter object is rejected by the caller — see
// the empty-result guard in handleBulkUpdateAdminAccounts.
func (s *Server) resolveBulkUpdateTargets(r *http.Request, body apiopenapi.BulkUpdateProviderAccountsRequest) ([]int, error) {
	if body.AccountIds != nil && len(*body.AccountIds) > 0 {
		ids, err := apiIDsValueToInts(*body.AccountIds)
		if err != nil {
			return nil, err
		}
		return ids, nil
	}
	if body.Filters == nil {
		return nil, errBulkUpdateMissingSelection
	}
	if !bulkUpdateFiltersHaveContent(*body.Filters) {
		return nil, errBulkUpdateEmptyFilters
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		return nil, errBulkUpdateListFailed
	}
	statusFilter := optionalStringValue(body.Filters.Status)
	providerFilter := optionalStringValue(body.Filters.ProviderId)
	accounts = filterAccounts(accounts, statusFilter, providerFilter)
	if runtimeClass := optionalStringValue(body.Filters.RuntimeClass); runtimeClass != "" {
		filtered := make([]accountcontract.ProviderAccount, 0, len(accounts))
		for _, account := range accounts {
			if string(account.RuntimeClass) == runtimeClass {
				filtered = append(filtered, account)
			}
		}
		accounts = filtered
	}
	if search := strings.ToLower(strings.TrimSpace(optionalStringValue(body.Filters.Search))); search != "" {
		filtered := make([]accountcontract.ProviderAccount, 0, len(accounts))
		for _, account := range accounts {
			if strings.Contains(strings.ToLower(account.Name), search) {
				filtered = append(filtered, account)
			}
		}
		accounts = filtered
	}
	if groupRaw := strings.TrimSpace(optionalStringValue(body.Filters.GroupId)); groupRaw != "" {
		groupID, parseErr := strconv.Atoi(groupRaw)
		if parseErr != nil || groupID <= 0 {
			return nil, errBulkUpdateInvalidGroupID
		}
		members, memberErr := s.runtime.accounts.ListGroupMembers(r.Context(), groupID)
		if memberErr != nil {
			return nil, errBulkUpdateListFailed
		}
		inGroup := make(map[int]struct{}, len(members))
		for _, m := range members {
			inGroup[m.AccountID] = struct{}{}
		}
		filtered := make([]accountcontract.ProviderAccount, 0, len(accounts))
		for _, account := range accounts {
			if _, ok := inGroup[account.ID]; ok {
				filtered = append(filtered, account)
			}
		}
		accounts = filtered
	}
	ids := make([]int, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids, nil
}

func bulkUpdateFiltersHaveContent(f apiopenapi.BulkUpdateProviderAccountsFilters) bool {
	return optionalStringValue(f.Status) != "" ||
		optionalStringValue(f.ProviderId) != "" ||
		optionalStringValue(f.GroupId) != "" ||
		optionalStringValue(f.RuntimeClass) != "" ||
		strings.TrimSpace(optionalStringValue(f.Search)) != ""
}

// bulkUpdateAccountsRequestFromAPI maps the wire-format DTO onto the
// service-layer UpdateRequest. Each pointer field flows through only when
// the caller actually sent it — Go's nil-vs-zero distinction is exactly
// the "field present?" semantic the wire format uses. max_concurrency is
// merged into a metadata patch so it lands on the same metadata key the
// scheduler already reads. Returns (req, false) when the body contained no
// editable fields — caller surfaces a 400 instead of running a no-op
// loop over the selection.
func bulkUpdateAccountsRequestFromAPI(body apiopenapi.BulkUpdateProviderAccountsRequest) (accountcontract.UpdateRequest, bool) {
	var req accountcontract.UpdateRequest
	hasField := false
	if body.Name != nil {
		name := *body.Name
		req.Name = &name
		hasField = true
	}
	if body.RuntimeClass != nil {
		rc := accountcontract.RuntimeClass(*body.RuntimeClass)
		req.RuntimeClass = &rc
		hasField = true
	}
	if body.Status != nil {
		status := accountcontract.Status(*body.Status)
		req.Status = &status
		hasField = true
	}
	if body.Priority != nil {
		p := *body.Priority
		req.Priority = &p
		hasField = true
	}
	if body.Weight != nil {
		w := *body.Weight
		req.Weight = &w
		hasField = true
	}
	if body.RiskLevel != nil {
		rl := *body.RiskLevel
		req.RiskLevel = &rl
		hasField = true
	}
	if body.ProxyId != nil {
		v := *body.ProxyId
		// double-pointer signals "field present, optionally clearing"
		ptr := &v
		req.ProxyID = &ptr
		hasField = true
	}
	if body.UpstreamClient != nil {
		v := *body.UpstreamClient
		ptr := &v
		req.UpstreamClient = &ptr
		hasField = true
	}
	if body.MaxConcurrency != nil {
		patch := map[string]any{"max_concurrency": *body.MaxConcurrency}
		req.Metadata = &patch
		hasField = true
	}
	return req, hasField
}

func bulkUpdateAuditFields(body apiopenapi.BulkUpdateProviderAccountsRequest) map[string]any {
	fields := map[string]any{}
	if body.Name != nil {
		fields["name"] = *body.Name
	}
	if body.RuntimeClass != nil {
		fields["runtime_class"] = *body.RuntimeClass
	}
	if body.Status != nil {
		fields["status"] = string(*body.Status)
	}
	if body.Priority != nil {
		fields["priority"] = *body.Priority
	}
	if body.Weight != nil {
		fields["weight"] = *body.Weight
	}
	if body.RiskLevel != nil {
		fields["risk_level"] = *body.RiskLevel
	}
	if body.ProxyId != nil {
		fields["proxy_id"] = *body.ProxyId
	}
	if body.UpstreamClient != nil {
		fields["upstream_client"] = *body.UpstreamClient
	}
	if body.MaxConcurrency != nil {
		fields["max_concurrency"] = *body.MaxConcurrency
	}
	return fields
}

func bulkUpdateAuditFilters(f apiopenapi.BulkUpdateProviderAccountsFilters) map[string]any {
	out := map[string]any{}
	if s := optionalStringValue(f.Status); s != "" {
		out["status"] = s
	}
	if s := optionalStringValue(f.ProviderId); s != "" {
		out["provider_id"] = s
	}
	if s := optionalStringValue(f.GroupId); s != "" {
		out["group_id"] = s
	}
	if s := optionalStringValue(f.RuntimeClass); s != "" {
		out["runtime_class"] = s
	}
	if s := strings.TrimSpace(optionalStringValue(f.Search)); s != "" {
		out["search"] = s
	}
	return out
}

type bulkUpdateError string

func (e bulkUpdateError) Error() string { return string(e) }

const (
	errBulkUpdateMissingSelection bulkUpdateError = "either account_ids or filters is required"
	errBulkUpdateEmptyFilters     bulkUpdateError = "filters must include at least one constraint"
	errBulkUpdateInvalidGroupID   bulkUpdateError = "invalid group id in filters"
	errBulkUpdateListFailed       bulkUpdateError = "failed to resolve bulk update selection"
)

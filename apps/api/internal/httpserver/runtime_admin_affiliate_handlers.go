package httpserver

import (
	"net/http"
	"strconv"

	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminAffiliateInvites(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.affiliate.ListRelationships(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list affiliate invites", requestID)
		return
	}
	data := make([]apiopenapi.AffiliateInviteRecord, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIAffiliateInviteRecord(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateInviteRecordListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminAffiliateRebates(w http.ResponseWriter, r *http.Request) {
	s.handleListAdminAffiliateLedgers(w, r, "rebate")
}

func (s *Server) handleListAdminAffiliateTransfers(w http.ResponseWriter, r *http.Request) {
	s.handleListAdminAffiliateLedgers(w, r, "transfer")
}

func (s *Server) handleListAdminAffiliateLedgers(w http.ResponseWriter, r *http.Request, view string) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	userID, ok := optionalUserIDQuery(w, r, requestID)
	if !ok {
		return
	}
	items, err := s.runtime.affiliate.ListLedgers(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list affiliate ledger", requestID)
		return
	}
	data := make([]apiopenapi.AffiliateLedgerEntry, 0, len(items))
	for _, item := range items {
		if userID != nil && item.UserID != *userID {
			continue
		}
		if !affiliateLedgerMatchesView(item.Type, view) {
			continue
		}
		data = append(data, toAPIAffiliateLedgerEntry(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateLedgerEntryListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func optionalUserIDQuery(w http.ResponseWriter, r *http.Request, requestID string) (*int, bool) {
	raw := r.URL.Query().Get("user_id")
	if raw == "" {
		return nil, true
	}
	userID, err := strconv.Atoi(raw)
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return nil, false
	}
	return &userID, true
}

func affiliateLedgerMatchesView(value affiliatecontract.LedgerType, view string) bool {
	switch view {
	case "rebate":
		return value == affiliatecontract.LedgerTypeAccrue || value == affiliatecontract.LedgerTypeRefundCompensation
	case "transfer":
		return value == affiliatecontract.LedgerTypeSettle || value == affiliatecontract.LedgerTypeTransferToBalance || value == affiliatecontract.LedgerTypeWithdraw
	default:
		return false
	}
}

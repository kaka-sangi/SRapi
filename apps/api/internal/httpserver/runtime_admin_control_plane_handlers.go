package httpserver

import (
	"net/http"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"
	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleGetAdminSettings(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminSettingsResponse{Data: s.toAPIAdminSettings(settings), RequestId: requestID})
}

func (s *Server) handleUpdateAdminSettings(w http.ResponseWriter, r *http.Request) {
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
	before, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	var body apiopenapi.AdminSettings
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid settings request", requestID)
		return
	}
	mapped := adminSettingsFromAPI(body)
	// The copilot dedicated API key is write-only: encrypt a freshly supplied
	// key, otherwise carry over the existing ciphertext so a save never wipes it.
	if body.Copilot.DedicatedApiKey != nil && strings.TrimSpace(*body.Copilot.DedicatedApiKey) != "" {
		ciphertext, encErr := s.encryptCopilotSecret(strings.TrimSpace(*body.Copilot.DedicatedApiKey))
		if encErr != nil {
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to secure copilot key", requestID)
			return
		}
		mapped.Copilot.DedicatedAPIKeyCiphertext = ciphertext
	} else {
		mapped.Copilot.DedicatedAPIKeyCiphertext = before.Copilot.DedicatedAPIKeyCiphertext
	}
	updated, err := s.runtime.adminControl.UpdateAdminSettings(r.Context(), mapped, session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "admin_settings.update", "admin_settings", "system", adminControlAuditSnapshot(before), adminControlAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminSettingsResponse{Data: s.toAPIAdminSettings(updated), RequestId: requestID})
}

func (s *Server) handleUpdateAdminOpsSettings(w http.ResponseWriter, r *http.Request) {
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
	before, err := s.runtime.adminControl.GetOpsSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	var body apiopenapi.OpsSettings
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid ops settings request", requestID)
		return
	}
	updated, err := s.runtime.adminControl.UpdateOpsSettings(r.Context(), opsSettingsFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_settings.update", "ops_settings", "system", adminControlAuditSnapshot(before), adminControlAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsSettingsResponse{Data: toAPIOpsSettings(updated), RequestId: requestID})
}

func (s *Server) handleListAdminAnnouncements(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	list, err := s.runtime.adminControl.ListAnnouncements(r.Context(), listOptionsFromRequest(r))
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AnnouncementListResponse{Data: toAPIAnnouncements(list.Items), Pagination: paginationWithRequest(r, list.Total), RequestId: requestID})
}

func (s *Server) handleCreateAdminAnnouncement(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateAnnouncementRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid announcement request", requestID)
		return
	}
	item, err := s.runtime.adminControl.CreateAnnouncement(r.Context(), announcementRequestFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "announcement.create", "announcement", strconv.Itoa(item.ID), nil, adminControlAuditSnapshot(item)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.AnnouncementResponse{Data: toAPIAnnouncement(item), RequestId: requestID})
}

func (s *Server) handleUpdateAdminAnnouncement(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	var body apiopenapi.UpdateAnnouncementRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid announcement request", requestID)
		return
	}
	before := findAnnouncementForAudit(r, s, id)
	item, err := s.runtime.adminControl.UpdateAnnouncement(r.Context(), id, announcementRequestFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "announcement.update", "announcement", strconv.Itoa(item.ID), before, adminControlAuditSnapshot(item)))
	writeJSONAny(w, http.StatusOK, apiopenapi.AnnouncementResponse{Data: toAPIAnnouncement(item), RequestId: requestID})
}

func (s *Server) handleDeleteAdminAnnouncement(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	deleted, err := s.runtime.adminControl.DeleteAnnouncement(r.Context(), id, session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "announcement.delete", "announcement", strconv.Itoa(deleted.ID), adminControlAuditSnapshot(deleted), nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) handleListAdminRedeemCodes(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	list, err := s.runtime.adminControl.ListRedeemCodes(r.Context(), listOptionsFromRequest(r))
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RedeemCodeListResponse{Data: toAPIRedeemCodes(list.Items), Pagination: paginationWithRequest(r, list.Total), RequestId: requestID})
}

func (s *Server) handleCreateAdminRedeemCode(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateRedeemCodeRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid redeem code request", requestID)
		return
	}
	item, err := s.runtime.adminControl.CreateRedeemCode(r.Context(), redeemCodeRequestFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "redeem_code.create", "redeem_code", strconv.Itoa(item.ID), nil, adminControlAuditSnapshot(item)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.RedeemCodeResponse{Data: toAPIRedeemCode(item), RequestId: requestID})
}

func (s *Server) handleBatchGenerateAdminRedeemCodes(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchGenerateRedeemCodesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid redeem code batch request", requestID)
		return
	}
	items, err := s.runtime.adminControl.BatchGenerateRedeemCodes(r.Context(), redeemBatchRequestFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "redeem_code.batch_generate", "redeem_code", "bulk", nil, map[string]any{"count": len(items)}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.RedeemCodeListResponse{Data: toAPIRedeemCodes(items), Pagination: pagination(len(items)), RequestId: requestID})
}

func (s *Server) handleBatchDisableAdminRedeemCodes(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchDisableRedeemCodesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid redeem code batch request", requestID)
		return
	}
	ids, err := idsFromAPI(body.Ids)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	result, err := s.runtime.adminControl.BatchDisableRedeemCodes(r.Context(), ids, session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "redeem_code.batch_disable", "redeem_code", "bulk", nil, adminControlAuditSnapshot(result)))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchOperationResponse{Data: toAPIBatchOperationResult(result), RequestId: requestID})
}

func (s *Server) handleAdminRedeemCodeStats(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	stats, err := s.runtime.adminControl.RedeemCodeStats(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RedeemCodeStatsResponse{Data: toAPIRedeemCodeStats(stats), RequestId: requestID})
}

func (s *Server) handleListAdminPromoCodes(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	list, err := s.runtime.adminControl.ListPromoCodes(r.Context(), listOptionsFromRequest(r))
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PromoCodeListResponse{Data: toAPIPromoCodes(list.Items), Pagination: paginationWithRequest(r, list.Total), RequestId: requestID})
}

func (s *Server) handleCreateAdminPromoCode(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreatePromoCodeRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid promo code request", requestID)
		return
	}
	item, err := s.runtime.adminControl.CreatePromoCode(r.Context(), promoCodeRequestFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "promo_code.create", "promo_code", strconv.Itoa(item.ID), nil, adminControlAuditSnapshot(item)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.PromoCodeResponse{Data: toAPIPromoCode(item), RequestId: requestID})
}

func (s *Server) handleUpdateAdminPromoCode(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	var body apiopenapi.UpdatePromoCodeRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid promo code request", requestID)
		return
	}
	before := findPromoCodeForAudit(r, s, id)
	item, err := s.runtime.adminControl.UpdatePromoCode(r.Context(), id, promoCodeRequestFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "promo_code.update", "promo_code", strconv.Itoa(item.ID), before, adminControlAuditSnapshot(item)))
	writeJSONAny(w, http.StatusOK, apiopenapi.PromoCodeResponse{Data: toAPIPromoCode(item), RequestId: requestID})
}

func (s *Server) handleDeleteAdminPromoCode(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	deleted, err := s.runtime.adminControl.DeletePromoCode(r.Context(), id, session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "promo_code.delete", "promo_code", strconv.Itoa(deleted.ID), adminControlAuditSnapshot(deleted), nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) handleListAdminPromoCodeUsages(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	usages, err := s.runtime.adminControl.ListPromoCodeUsages(r.Context(), id, 200)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PromoCodeUsageListResponse{
		Data:      toAPIPromoCodeUsages(usages),
		RequestId: requestID,
	})
}

func (s *Server) handleGetAdminRiskControlConfig(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	config, err := s.runtime.adminControl.GetRiskConfig(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RiskControlConfigResponse{Data: toAPIRiskControlConfig(config), RequestId: requestID})
}

func (s *Server) handleUpdateAdminRiskControlConfig(w http.ResponseWriter, r *http.Request) {
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
	before, err := s.runtime.adminControl.GetRiskConfig(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	var body apiopenapi.RiskControlConfig
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid risk control request", requestID)
		return
	}
	updated, err := s.runtime.adminControl.UpdateRiskConfig(r.Context(), riskControlConfigFromAPI(body), session.User.ID)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "risk_control.update", "risk_control", "config", adminControlAuditSnapshot(before), adminControlAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.RiskControlConfigResponse{Data: toAPIRiskControlConfig(updated), RequestId: requestID})
}

func (s *Server) handleGetAdminRiskControlStatus(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	status, err := s.runtime.adminControl.RiskStatus(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RiskControlStatusResponse{Data: toAPIRiskControlStatus(status), RequestId: requestID})
}

func (s *Server) handleListAdminRiskControlLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	list, err := s.runtime.adminControl.ListRiskLogs(r.Context(), listOptionsFromRequest(r))
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RiskControlLogListResponse{Data: toAPIRiskControlLogs(list.Items), Pagination: paginationWithRequest(r, list.Total), RequestId: requestID})
}

func pathID(w http.ResponseWriter, r *http.Request, requestID string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid resource id", requestID)
		return 0, false
	}
	return id, true
}

func deleteResponse(deleted bool, requestID string) apiopenapi.DeleteResponse {
	resp := apiopenapi.DeleteResponse{RequestId: requestID}
	resp.Data.Deleted = deleted
	return resp
}

func findAnnouncementForAudit(r *http.Request, s *Server, id int) map[string]any {
	opts := listOptionsFromRequest(r)
	opts.Page = 1
	opts.PageSize = 1000
	list, err := s.runtime.adminControl.ListAnnouncements(r.Context(), opts)
	if err != nil {
		return nil
	}
	for _, item := range list.Items {
		if item.ID == id {
			return adminControlAuditSnapshot(item)
		}
	}
	return nil
}

func findPromoCodeForAudit(r *http.Request, s *Server, id int) map[string]any {
	opts := listOptionsFromRequest(r)
	opts.Page = 1
	opts.PageSize = 1000
	list, err := s.runtime.adminControl.ListPromoCodes(r.Context(), opts)
	if err != nil {
		return nil
	}
	for _, item := range list.Items {
		if item.ID == id {
			return adminControlAuditSnapshot(item)
		}
	}
	return nil
}

func toAPIAdminSettings(in admincontrol.AdminSettings) apiopenapi.AdminSettings {
	return apiopenapi.AdminSettings{
		Agreement: apiopenapi.AdminSettingsAgreement{
			PrivacyPolicy: in.Agreement.PrivacyPolicy,
			UserAgreement: in.Agreement.UserAgreement,
		},
		Backup: apiopenapi.AdminSettingsBackup{
			Enabled:       in.Backup.Enabled,
			LastBackupAt:  in.Backup.LastBackupAt,
			RetentionDays: in.Backup.RetentionDays,
		},
		Email: apiopenapi.AdminSettingsEmail{
			AccountQuotaNotifyEnabled:        boolPtrFromOptional(in.Email.AccountQuotaNotifyEnabled),
			AccountQuotaNotifyRemainingRatio: stringPtrValueForAPI(in.Email.AccountQuotaNotifyRemainingRatio),
			BalanceLowNotifyEnabled:          boolPtrFromOptional(in.Email.BalanceLowNotifyEnabled),
			BalanceLowNotifyRechargeUrl:      stringPtrValueForAPI(in.Email.BalanceLowNotifyRechargeURL),
			BalanceLowNotifyThreshold:        stringPtrValueForAPI(in.Email.BalanceLowNotifyThreshold),
			PublicBaseUrl:                    stringPtrValueForAPI(in.Email.PublicBaseURL),
			SmtpConfigured:                   in.Email.SMTPConfigured,
			SmtpFrom:                         emailPtrValueForAPI(in.Email.SMTPFrom),
			SmtpFromName:                     stringPtrValueForAPI(in.Email.SMTPFromName),
			SmtpHost:                         stringPtrValueForAPI(in.Email.SMTPHost),
			SmtpPasswordConfigured:           boolPtrValueForAPI(false),
			SmtpPort:                         intPtrValueForAPI(in.Email.SMTPPort),
			SmtpUsername:                     stringPtrValueForAPI(in.Email.SMTPUsername),
			SmtpUseTls:                       boolPtrValueForAPI(in.Email.SMTPUseTLS),
			SubscriptionExpiryNotifyEnabled:  boolPtrFromOptional(in.Email.SubscriptionExpiryNotifyEnabled),
			Templates:                        in.Email.Templates,
		},
		Features: apiopenapi.AdminSettingsFeatures{
			ChannelMonitoringEnabled: in.Features.ChannelMonitoringEnabled,
			EnabledChannels:          append([]string(nil), in.Features.EnabledChannels...),
			InvitationRebateEnabled:  in.Features.InvitationRebateEnabled,
			PaymentsEnabled:          in.Features.PaymentsEnabled,
		},
		Gateway: apiopenapi.AdminSettingsGateway{
			BetaStrategy:                         in.Gateway.BetaStrategy,
			OverloadCooldownSeconds:              in.Gateway.OverloadCooldownSeconds,
			RateLimitCooldownSeconds:             in.Gateway.RateLimitCooldownSeconds,
			RequestShaperEnabled:                 in.Gateway.RequestShaperEnabled,
			SchedulerStrategyRolloutApiKeyHashes: stringSlicePtr(in.Gateway.SchedulerStrategyRolloutAPIKeyHashes),
			SchedulerStrategyRolloutEnabled:      boolPtrValueForAPI(in.Gateway.SchedulerStrategyRolloutEnabled),
			SchedulerStrategyRolloutModels:       stringSlicePtr(in.Gateway.SchedulerStrategyRolloutModels),
			SchedulerStrategyRolloutPercent:      float32Ptr(float32(in.Gateway.SchedulerStrategyRolloutPercent)),
			SchedulerStrategyShadowStrategy:      apiSchedulerStrategyNamePtr(in.Gateway.SchedulerStrategyShadowStrategy),
			StreamTimeoutSeconds:                 in.Gateway.StreamTimeoutSeconds,
		},
		General: apiopenapi.AdminSettingsGeneral{
			CustomMenus:  mapsToJsonObjects(in.General.CustomMenus),
			LogoUrl:      in.General.LogoURL,
			SiteName:     in.General.SiteName,
			VersionLabel: in.General.VersionLabel,
		},
		Payment: apiopenapi.AdminSettingsPayment{
			Enabled:                  in.Payment.Enabled,
			Providers:                append([]string(nil), in.Payment.Providers...),
			SubscriptionPlansEnabled: in.Payment.SubscriptionPlansEnabled,
		},
		Security: apiopenapi.AdminSettingsSecurity{
			AdminApiKey:                      apiopenapi.SecretConfigured{Configured: in.Security.AdminAPIKey.Configured},
			OauthEnabled:                     in.Security.OAuthEnabled,
			OauthProviderConfigs:             toAPIOAuthProviderConfigs(in.Security.OAuthProviderConfigs),
			OauthProviders:                   append([]string(nil), in.Security.OAuthProviders...),
			RegistrationEmailSuffixAllowlist: append([]string(nil), in.Security.RegistrationEmailSuffixAllowlist...),
			RegistrationEnabled:              in.Security.RegistrationEnabled,
		},
		Users: apiopenapi.AdminSettingsUsers{
			DefaultBalance:        in.Users.DefaultBalance,
			DefaultGroup:          in.Users.DefaultGroup,
			RpmLimitDefault:       in.Users.RPMLimitDefault,
			UserSelfDeleteEnabled: in.Users.UserSelfDeleteEnabled,
		},
		Copilot: apiopenapi.AdminSettingsCopilot{
			Enabled:                   in.Copilot.Enabled,
			Source:                    apiopenapi.AdminSettingsCopilotSource(in.Copilot.Source),
			ProviderAccountId:         in.Copilot.ProviderAccountID,
			Model:                     in.Copilot.Model,
			Models:                    stringSlicePtr(in.Copilot.Models),
			DedicatedProtocol:         in.Copilot.DedicatedProtocol,
			DedicatedBaseUrl:          in.Copilot.DedicatedBaseURL,
			DedicatedApiKeyConfigured: strings.TrimSpace(in.Copilot.DedicatedAPIKeyCiphertext) != "",
			MaxSteps:                  in.Copilot.MaxSteps,
			OwnerOnly:                 in.Copilot.OwnerOnly,
			AutoRunReads:              in.Copilot.AutoRunReads,
		},
	}
}

func (s *Server) toAPIAdminSettings(in admincontrol.AdminSettings) apiopenapi.AdminSettings {
	settings := toAPIAdminSettings(in)
	settings.Email.SmtpPasswordConfigured = boolPtrValueForAPI(strings.TrimSpace(s.cfg.Email.SMTPPassword) != "")
	return settings
}

func adminSettingsFromAPI(in apiopenapi.AdminSettings) admincontrol.AdminSettings {
	return admincontrol.AdminSettings{
		Agreement: admincontrol.AdminSettingsAgreement{
			PrivacyPolicy: in.Agreement.PrivacyPolicy,
			UserAgreement: in.Agreement.UserAgreement,
		},
		Backup: admincontrol.AdminSettingsBackup{
			Enabled:       in.Backup.Enabled,
			LastBackupAt:  in.Backup.LastBackupAt,
			RetentionDays: in.Backup.RetentionDays,
		},
		Email: admincontrol.AdminSettingsEmail{
			SMTPConfigured:                   in.Email.SmtpConfigured,
			SMTPHost:                         stringFromPtr(in.Email.SmtpHost),
			SMTPPort:                         intFromPtr(in.Email.SmtpPort),
			SMTPUsername:                     stringFromPtr(in.Email.SmtpUsername),
			SMTPFrom:                         emailFromPtr(in.Email.SmtpFrom),
			SMTPFromName:                     stringFromPtr(in.Email.SmtpFromName),
			SMTPUseTLS:                       boolFromPtr(in.Email.SmtpUseTls),
			PublicBaseURL:                    stringFromPtr(in.Email.PublicBaseUrl),
			Templates:                        in.Email.Templates,
			BalanceLowNotifyEnabled:          boolOptionalFromPtr(in.Email.BalanceLowNotifyEnabled),
			BalanceLowNotifyThreshold:        stringFromPtr(in.Email.BalanceLowNotifyThreshold),
			BalanceLowNotifyRechargeURL:      stringFromPtr(in.Email.BalanceLowNotifyRechargeUrl),
			SubscriptionExpiryNotifyEnabled:  boolOptionalFromPtr(in.Email.SubscriptionExpiryNotifyEnabled),
			AccountQuotaNotifyEnabled:        boolOptionalFromPtr(in.Email.AccountQuotaNotifyEnabled),
			AccountQuotaNotifyRemainingRatio: stringFromPtr(in.Email.AccountQuotaNotifyRemainingRatio),
		},
		Features: admincontrol.AdminSettingsFeatures{
			ChannelMonitoringEnabled: in.Features.ChannelMonitoringEnabled,
			EnabledChannels:          append([]string(nil), in.Features.EnabledChannels...),
			InvitationRebateEnabled:  in.Features.InvitationRebateEnabled,
			PaymentsEnabled:          in.Features.PaymentsEnabled,
		},
		Gateway: admincontrol.AdminSettingsGateway{
			BetaStrategy:                         in.Gateway.BetaStrategy,
			OverloadCooldownSeconds:              in.Gateway.OverloadCooldownSeconds,
			RateLimitCooldownSeconds:             in.Gateway.RateLimitCooldownSeconds,
			RequestShaperEnabled:                 in.Gateway.RequestShaperEnabled,
			SchedulerStrategyRolloutAPIKeyHashes: stringSliceFromPtr(in.Gateway.SchedulerStrategyRolloutApiKeyHashes),
			SchedulerStrategyRolloutEnabled:      boolFromPtr(in.Gateway.SchedulerStrategyRolloutEnabled),
			SchedulerStrategyRolloutModels:       stringSliceFromPtr(in.Gateway.SchedulerStrategyRolloutModels),
			SchedulerStrategyRolloutPercent:      float64FromPtr(in.Gateway.SchedulerStrategyRolloutPercent),
			SchedulerStrategyShadowStrategy:      schedulerStrategyNameString(in.Gateway.SchedulerStrategyShadowStrategy),
			StreamTimeoutSeconds:                 in.Gateway.StreamTimeoutSeconds,
		},
		General: admincontrol.AdminSettingsGeneral{
			CustomMenus:  jsonObjectsToMaps(in.General.CustomMenus),
			LogoURL:      in.General.LogoUrl,
			SiteName:     in.General.SiteName,
			VersionLabel: in.General.VersionLabel,
		},
		Payment: admincontrol.AdminSettingsPayment{
			Enabled:                  in.Payment.Enabled,
			Providers:                append([]string(nil), in.Payment.Providers...),
			SubscriptionPlansEnabled: in.Payment.SubscriptionPlansEnabled,
		},
		Security: admincontrol.AdminSettingsSecurity{
			AdminAPIKey:                      admincontrol.SecretConfigured{Configured: in.Security.AdminApiKey.Configured},
			OAuthEnabled:                     in.Security.OauthEnabled,
			OAuthProviderConfigs:             oauthProviderConfigsFromAPI(in.Security.OauthProviderConfigs),
			OAuthProviders:                   append([]string(nil), in.Security.OauthProviders...),
			RegistrationEmailSuffixAllowlist: append([]string(nil), in.Security.RegistrationEmailSuffixAllowlist...),
			RegistrationEnabled:              in.Security.RegistrationEnabled,
		},
		Users: admincontrol.AdminSettingsUsers{
			DefaultBalance:        in.Users.DefaultBalance,
			DefaultGroup:          in.Users.DefaultGroup,
			RPMLimitDefault:       in.Users.RpmLimitDefault,
			UserSelfDeleteEnabled: in.Users.UserSelfDeleteEnabled,
		},
		Copilot: admincontrol.AdminSettingsCopilot{
			Enabled:           in.Copilot.Enabled,
			Source:            string(in.Copilot.Source),
			ProviderAccountID: in.Copilot.ProviderAccountId,
			Model:             in.Copilot.Model,
			Models:            stringSliceFromPtr(in.Copilot.Models),
			DedicatedProtocol: in.Copilot.DedicatedProtocol,
			DedicatedBaseURL:  in.Copilot.DedicatedBaseUrl,
			MaxSteps:          in.Copilot.MaxSteps,
			OwnerOnly:         in.Copilot.OwnerOnly,
			AutoRunReads:      in.Copilot.AutoRunReads,
			// DedicatedAPIKeyCiphertext is set by the handler (encrypt-new or
			// preserve-existing); never derived from the request body here.
		},
	}
}

func toAPIOAuthProviderConfigs(values []admincontrol.OAuthProviderConfig) []apiopenapi.OAuthProviderConfig {
	out := make([]apiopenapi.OAuthProviderConfig, 0, len(values))
	for _, value := range values {
		out = append(out, apiopenapi.OAuthProviderConfig{
			AuthorizeUrl:    value.AuthorizeURL,
			ClientId:        value.ClientID,
			DisplayName:     value.DisplayName,
			Provider:        apiopenapi.AuthIdentityProvider(value.Provider),
			ProviderKey:     value.ProviderKey,
			RedirectUri:     value.RedirectURI,
			Scopes:          append([]string(nil), value.Scopes...),
			TokenAuthMethod: oauthTokenAuthMethodPtrValueForAPI(value.TokenAuthMethod),
			TokenUrl:        stringPtrValueForAPI(value.TokenURL),
			UserinfoUrl:     stringPtrValueForAPI(value.UserInfoURL),
		})
	}
	return out
}

func oauthProviderConfigsFromAPI(values []apiopenapi.OAuthProviderConfig) []admincontrol.OAuthProviderConfig {
	out := make([]admincontrol.OAuthProviderConfig, 0, len(values))
	for _, value := range values {
		out = append(out, admincontrol.OAuthProviderConfig{
			Provider:        string(value.Provider),
			ProviderKey:     value.ProviderKey,
			DisplayName:     value.DisplayName,
			ClientID:        value.ClientId,
			AuthorizeURL:    value.AuthorizeUrl,
			TokenURL:        stringFromPtr(value.TokenUrl),
			UserInfoURL:     stringFromPtr(value.UserinfoUrl),
			TokenAuthMethod: oauthTokenAuthMethodString(value.TokenAuthMethod),
			RedirectURI:     value.RedirectUri,
			Scopes:          append([]string(nil), value.Scopes...),
		})
	}
	return out
}

func oauthTokenAuthMethodPtrValueForAPI(value string) *apiopenapi.OAuthProviderConfigTokenAuthMethod {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	method := apiopenapi.OAuthProviderConfigTokenAuthMethod(value)
	return &method
}

func oauthTokenAuthMethodString(value *apiopenapi.OAuthProviderConfigTokenAuthMethod) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSlicePtr(values []string) *[]string {
	out := append([]string(nil), values...)
	return &out
}

func stringSliceFromPtr(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
}

func boolFromPtr(value *bool) bool {
	return value != nil && *value
}

func boolOptionalFromPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func boolPtrFromOptional(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func boolPtrValueForAPI(value bool) *bool {
	return &value
}

func stringPtrValueForAPI(value string) *string {
	return &value
}

func emailPtrValueForAPI(value string) *openapi_types.Email {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	email := openapi_types.Email(value)
	return &email
}

func intPtrValueForAPI(value int) *int {
	return &value
}

func stringFromPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func emailFromPtr(value *openapi_types.Email) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func intFromPtr(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func float32Ptr(value float32) *float32 {
	return &value
}

func float64FromPtr(value *float32) float64 {
	if value == nil {
		return 0
	}
	return float64(*value)
}

func apiSchedulerStrategyNamePtr(value string) *apiopenapi.SchedulerStrategyName {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	out := apiopenapi.SchedulerStrategyName(trimmed)
	return &out
}

func schedulerStrategyNameString(value *apiopenapi.SchedulerStrategyName) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func toAPIOpsSettings(in admincontrol.OpsSettings) apiopenapi.OpsSettings {
	return apiopenapi.OpsSettings{
		AlertRetentionDays:     in.AlertRetentionDays,
		AutoRefreshEnabled:     in.AutoRefreshEnabled,
		ErrorRateThreshold:     in.ErrorRateThreshold,
		LatencyP95ThresholdMs:  in.LatencyP95ThresholdMS,
		RefreshIntervalSeconds: in.RefreshIntervalSeconds,
	}
}

func opsSettingsFromAPI(in apiopenapi.OpsSettings) admincontrol.OpsSettings {
	return admincontrol.OpsSettings{
		AlertRetentionDays:     in.AlertRetentionDays,
		AutoRefreshEnabled:     in.AutoRefreshEnabled,
		ErrorRateThreshold:     in.ErrorRateThreshold,
		LatencyP95ThresholdMS:  in.LatencyP95ThresholdMs,
		RefreshIntervalSeconds: in.RefreshIntervalSeconds,
	}
}

func toAPIAnnouncements(items []admincontrol.Announcement) []apiopenapi.Announcement {
	out := make([]apiopenapi.Announcement, 0, len(items))
	for _, item := range items {
		out = append(out, toAPIAnnouncement(item))
	}
	return out
}

func toAPIAnnouncement(in admincontrol.Announcement) apiopenapi.Announcement {
	return apiopenapi.Announcement{
		Audience:  apiopenapi.AnnouncementAudience(in.Audience),
		Content:   in.Content,
		CreatedAt: in.CreatedAt,
		EndsAt:    in.EndsAt,
		Id:        apiopenapi.Id(strconv.Itoa(in.ID)),
		Severity:  apiopenapi.AnnouncementSeverity(in.Severity),
		StartsAt:  in.StartsAt,
		Status:    apiopenapi.AnnouncementStatus(in.Status),
		Title:     in.Title,
		UpdatedAt: in.UpdatedAt,
	}
}

func toAPIUserAnnouncements(items []admincontrol.UserAnnouncement) []apiopenapi.UserAnnouncement {
	out := make([]apiopenapi.UserAnnouncement, 0, len(items))
	for _, item := range items {
		out = append(out, toAPIUserAnnouncement(item))
	}
	return out
}

func toAPIUserAnnouncement(in admincontrol.UserAnnouncement) apiopenapi.UserAnnouncement {
	item := apiopenapi.UserAnnouncement{
		Announcement: toAPIAnnouncement(in.Announcement),
		Read:         in.Read,
	}
	if in.ReadAt != nil {
		item.ReadAt = in.ReadAt
	}
	return item
}

func announcementRequestFromAPI(in apiopenapi.CreateAnnouncementRequest) admincontrol.AnnouncementRequest {
	req := admincontrol.AnnouncementRequest{
		Title:    in.Title,
		Content:  in.Content,
		StartsAt: in.StartsAt,
		EndsAt:   in.EndsAt,
	}
	if in.Status != nil {
		req.Status = admincontrol.AnnouncementStatus(*in.Status)
	}
	if in.Severity != nil {
		req.Severity = admincontrol.AnnouncementSeverity(*in.Severity)
	}
	if in.Audience != nil {
		req.Audience = admincontrol.AnnouncementAudience(*in.Audience)
	}
	return req
}

func toAPIRedeemCodes(items []admincontrol.RedeemCode) []apiopenapi.RedeemCode {
	out := make([]apiopenapi.RedeemCode, 0, len(items))
	for _, item := range items {
		out = append(out, toAPIRedeemCode(item))
	}
	return out
}

func toAPIRedeemCode(in admincontrol.RedeemCode) apiopenapi.RedeemCode {
	return apiopenapi.RedeemCode{
		Code:           in.Code,
		CreatedAt:      in.CreatedAt,
		Currency:       in.Currency,
		ExpiresAt:      in.ExpiresAt,
		Id:             apiopenapi.Id(strconv.Itoa(in.ID)),
		MaxRedemptions: in.MaxRedemptions,
		RedeemedCount:  in.RedeemedCount,
		Status:         apiopenapi.RedeemCodeStatus(in.Status),
		Type:           apiopenapi.RedeemCodeType(in.Type),
		UpdatedAt:      in.UpdatedAt,
		Value:          in.Value,
	}
}

func toAPIRedeemCodeRedemptionResult(in admincontrol.RedeemCodeRedemptionResult) apiopenapi.RedeemCodeRedemptionResult {
	return apiopenapi.RedeemCodeRedemptionResult{
		AlreadyRedeemed: in.AlreadyRedeemed,
		RedeemCode:      toAPIRedeemCode(in.RedeemCode),
		Redemption:      toAPIRedeemCodeRedemption(in.Redemption),
	}
}

func toAPIRedeemCodeRedemption(in admincontrol.RedeemCodeRedemption) apiopenapi.RedeemCodeRedemption {
	return apiopenapi.RedeemCodeRedemption{
		Amount:             in.Amount,
		BalanceAfter:       in.BalanceAfter,
		BalanceBefore:      in.BalanceBefore,
		BillingLedgerId:    optionalAPIID(in.BillingLedgerID),
		CreatedAt:          in.CreatedAt,
		Currency:           in.Currency,
		Id:                 apiopenapi.Id(strconv.Itoa(in.ID)),
		RedeemCodeId:       apiopenapi.Id(strconv.Itoa(in.RedeemCodeID)),
		RedeemedAt:         in.RedeemedAt,
		Type:               apiopenapi.RedeemCodeType(in.Type),
		UpdatedAt:          in.UpdatedAt,
		UserId:             apiopenapi.Id(strconv.Itoa(in.UserID)),
		UserSubscriptionId: optionalAPIID(in.UserSubscriptionID),
	}
}

func redeemCodeRequestFromAPI(in apiopenapi.CreateRedeemCodeRequest) admincontrol.CreateRedeemCodeRequest {
	req := admincontrol.CreateRedeemCodeRequest{
		Code:      in.Code,
		Type:      admincontrol.RedeemCodeType(in.Type),
		Value:     in.Value,
		ExpiresAt: in.ExpiresAt,
	}
	if in.Currency != nil {
		req.Currency = *in.Currency
	}
	if in.MaxRedemptions != nil {
		req.MaxRedemptions = *in.MaxRedemptions
	}
	return req
}

func redeemBatchRequestFromAPI(in apiopenapi.BatchGenerateRedeemCodesRequest) admincontrol.BatchGenerateRedeemCodesRequest {
	req := admincontrol.BatchGenerateRedeemCodesRequest{
		Count:     in.Count,
		Type:      admincontrol.RedeemCodeType(in.Type),
		Value:     in.Value,
		ExpiresAt: in.ExpiresAt,
	}
	if in.Prefix != nil {
		req.Prefix = *in.Prefix
	}
	if in.Currency != nil {
		req.Currency = *in.Currency
	}
	if in.MaxRedemptions != nil {
		req.MaxRedemptions = *in.MaxRedemptions
	}
	return req
}

func toAPIRedeemCodeStats(in admincontrol.RedeemCodeStats) apiopenapi.RedeemCodeStats {
	return apiopenapi.RedeemCodeStats{
		Active:   in.Active,
		Disabled: in.Disabled,
		Expired:  in.Expired,
		Redeemed: in.Redeemed,
		Total:    in.Total,
	}
}

func toAPIPromoCodeUsages(items []admincontrol.PromoCodeApplication) []apiopenapi.PromoCodeUsage {
	out := make([]apiopenapi.PromoCodeUsage, 0, len(items))
	for _, item := range items {
		out = append(out, apiopenapi.PromoCodeUsage{
			Id:             apiopenapi.Id(strconv.Itoa(item.ID)),
			UserId:         apiopenapi.Id(strconv.Itoa(item.UserID)),
			PromoCodeId:    apiopenapi.Id(strconv.Itoa(item.PromoCodeID)),
			PaymentOrderId: apiopenapi.Id(strconv.Itoa(item.PaymentOrderID)),
			OrderNo:        item.OrderNo,
			OriginalAmount: item.OriginalAmount,
			DiscountAmount: item.DiscountAmount,
			FinalAmount:    item.FinalAmount,
			Currency:       item.Currency,
			DiscountType:   apiopenapi.PromoDiscountType(item.DiscountType),
			AppliedAt:      item.AppliedAt,
		})
	}
	return out
}

func toAPIPromoCodes(items []admincontrol.PromoCode) []apiopenapi.PromoCode {
	out := make([]apiopenapi.PromoCode, 0, len(items))
	for _, item := range items {
		out = append(out, toAPIPromoCode(item))
	}
	return out
}

func toAPIPromoCode(in admincontrol.PromoCode) apiopenapi.PromoCode {
	return apiopenapi.PromoCode{
		Code:          in.Code,
		CreatedAt:     in.CreatedAt,
		Currency:      in.Currency,
		DiscountType:  apiopenapi.PromoDiscountType(in.DiscountType),
		DiscountValue: in.DiscountValue,
		ExpiresAt:     in.ExpiresAt,
		Id:            apiopenapi.Id(strconv.Itoa(in.ID)),
		MaxUses:       in.MaxUses,
		StartsAt:      in.StartsAt,
		Status:        apiopenapi.PromoCodeStatus(in.Status),
		UpdatedAt:     in.UpdatedAt,
		UsedCount:     in.UsedCount,
	}
}

func promoCodeRequestFromAPI(in apiopenapi.CreatePromoCodeRequest) admincontrol.PromoCodeRequest {
	req := admincontrol.PromoCodeRequest{
		Code:          in.Code,
		DiscountType:  admincontrol.PromoDiscountType(in.DiscountType),
		DiscountValue: in.DiscountValue,
		StartsAt:      in.StartsAt,
		ExpiresAt:     in.ExpiresAt,
	}
	if in.Status != nil {
		req.Status = admincontrol.PromoCodeStatus(*in.Status)
	}
	if in.Currency != nil {
		req.Currency = *in.Currency
	}
	if in.MaxUses != nil {
		req.MaxUses = *in.MaxUses
	}
	return req
}

func toAPIRiskControlConfig(in admincontrol.RiskControlConfig) apiopenapi.RiskControlConfig {
	blockedCountries := append([]string(nil), in.BlockedCountries...)
	if blockedCountries == nil {
		blockedCountries = []string{}
	}
	blockedIPs := append([]string(nil), in.BlockedIPs...)
	if blockedIPs == nil {
		blockedIPs = []string{}
	}
	return apiopenapi.RiskControlConfig{
		BlockedCountries:           blockedCountries,
		BlockedIps:                 blockedIPs,
		CooldownSeconds:            in.CooldownSeconds,
		Enabled:                    in.Enabled,
		MaxCostPerDay:              in.MaxCostPerDay,
		MaxFailedRequestsPerMinute: in.MaxFailedRequestsPerMinute,
		Mode:                       apiopenapi.RiskControlMode(in.Mode),
	}
}

func riskControlConfigFromAPI(in apiopenapi.RiskControlConfig) admincontrol.RiskControlConfig {
	return admincontrol.RiskControlConfig{
		BlockedCountries:           append([]string(nil), in.BlockedCountries...),
		BlockedIPs:                 append([]string(nil), in.BlockedIps...),
		CooldownSeconds:            in.CooldownSeconds,
		Enabled:                    in.Enabled,
		MaxCostPerDay:              in.MaxCostPerDay,
		MaxFailedRequestsPerMinute: in.MaxFailedRequestsPerMinute,
		Mode:                       admincontrol.RiskControlMode(in.Mode),
	}
}

func toAPIRiskControlStatus(in admincontrol.RiskControlStatus) apiopenapi.RiskControlStatus {
	return apiopenapi.RiskControlStatus{
		ActiveBlocks: in.ActiveBlocks,
		Enabled:      in.Enabled,
		EvaluatedAt:  in.EvaluatedAt,
		Mode:         apiopenapi.RiskControlMode(in.Mode),
		RecentEvents: in.RecentEvents,
	}
}

func toAPIRiskControlLogs(items []admincontrol.RiskControlLog) []apiopenapi.RiskControlLog {
	out := make([]apiopenapi.RiskControlLog, 0, len(items))
	for _, item := range items {
		out = append(out, apiopenapi.RiskControlLog{
			Action:    item.Action,
			CreatedAt: item.CreatedAt,
			Id:        apiopenapi.Id(strconv.Itoa(item.ID)),
			Level:     apiopenapi.RiskControlLogLevel(item.Level),
			Metadata:  mapToJsonObjectPtr(item.Metadata),
			Reason:    item.Reason,
			Subject:   item.Subject,
		})
	}
	return out
}

func toAPIOpsSystemLogs(items []admincontrol.OpsSystemLog) []apiopenapi.OpsSystemLog {
	out := make([]apiopenapi.OpsSystemLog, 0, len(items))
	for _, item := range items {
		log := apiopenapi.OpsSystemLog{
			CreatedAt: item.CreatedAt,
			Id:        apiopenapi.Id(strconv.Itoa(item.ID)),
			Level:     apiopenapi.OpsSystemLogLevel(item.Level),
			Message:   item.Message,
			Metadata:  mapToJsonObjectPtr(item.Metadata),
			Source:    item.Source,
		}
		log.RequestId = optionalString(item.RequestID)
		log.TraceId = optionalString(item.TraceID)
		out = append(out, log)
	}
	return out
}

func toAPIBatchOperationResult(in admincontrol.BatchOperationResult) apiopenapi.BatchOperationResult {
	out := apiopenapi.BatchOperationResult{
		Failed:    in.Failed,
		Requested: in.Requested,
		Succeeded: in.Succeeded,
	}
	if len(in.FailedIDs) > 0 {
		failedIDs := make([]apiopenapi.Id, 0, len(in.FailedIDs))
		for _, id := range in.FailedIDs {
			failedIDs = append(failedIDs, apiopenapi.Id(strconv.Itoa(id)))
		}
		out.FailedIds = &failedIDs
	}
	return out
}

func idsFromAPI(values []apiopenapi.Id) ([]int, error) {
	ids := make([]int, 0, len(values))
	for _, value := range values {
		id, err := strconv.Atoi(string(value))
		if err != nil || id <= 0 {
			return nil, admincontrol.ErrInvalidInput
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func mapsToJsonObjects(values []map[string]any) []apiopenapi.JsonObject {
	out := make([]apiopenapi.JsonObject, 0, len(values))
	for _, value := range values {
		out = append(out, apiopenapi.JsonObject(cloneAnyMap(value)))
	}
	return out
}

func jsonObjectsToMaps(values []apiopenapi.JsonObject) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, cloneAnyMap(map[string]any(value)))
	}
	return out
}

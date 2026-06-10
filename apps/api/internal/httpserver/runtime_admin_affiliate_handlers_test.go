package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	affiliatememory "github.com/srapi/srapi/apps/api/internal/modules/affiliate/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminAffiliateListsExposeRealDomainRecords(t *testing.T) {
	store := affiliatememory.New()
	now := time.Now().UTC()
	inviteCode, err := store.CreateInviteCode(t.Context(), affiliatecontract.InviteCode{
		UserID:    1,
		Code:      "AFF1",
		Status:    affiliatecontract.InviteCodeStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed invite code: %v", err)
	}
	if _, err := store.CreateRelationship(t.Context(), affiliatecontract.InviteRelationship{
		InviterUserID: 1,
		InviteeUserID: 2,
		InviteCodeID:  inviteCode.ID,
		Status:        affiliatecontract.RelationshipStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		FirstPaidAt:   &now,
	}); err != nil {
		t.Fatalf("seed invite relationship: %v", err)
	}
	paymentOrderID := 99
	if _, _, err := store.AppendLedger(t.Context(), affiliatecontract.AffiliateLedger{
		UserID:         1,
		RelatedUserID:  2,
		PaymentOrderID: &paymentOrderID,
		Type:           affiliatecontract.LedgerTypeAccrue,
		Amount:         "3.00000000",
		Currency:       "USD",
		Status:         affiliatecontract.LedgerStatusPending,
		ReferenceID:    "rebate-1",
		Metadata:       map[string]any{"order_no": "ord_1"},
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed rebate ledger: %v", err)
	}
	if _, _, err := store.AppendLedger(t.Context(), affiliatecontract.AffiliateLedger{
		UserID:        1,
		RelatedUserID: 0,
		Type:          affiliatecontract.LedgerTypeTransferToBalance,
		Amount:        "-1.50000000",
		Currency:      "USD",
		Status:        affiliatecontract.LedgerStatusSettled,
		ReferenceID:   "transfer-1",
		Metadata:      map[string]any{"balance_after": "1.50000000"},
		CreatedAt:     now,
		UpdatedAt:     now,
		SettledAt:     &now,
	}); err != nil {
		t.Fatalf("seed transfer ledger: %v", err)
	}

	handler := New(config.Load(), nil, WithAffiliateStore(store))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	invitesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/affiliates/invites", nil)
	invitesReq.AddCookie(sessionCookie)
	invitesRec := httptest.NewRecorder()
	handler.ServeHTTP(invitesRec, invitesReq)
	if invitesRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate invites 200, got %d body=%s", invitesRec.Code, invitesRec.Body.String())
	}
	var invitesResp apiopenapi.AffiliateInviteRecordListResponse
	if err := json.NewDecoder(invitesRec.Body).Decode(&invitesResp); err != nil {
		t.Fatalf("decode affiliate invites: %v", err)
	}
	if len(invitesResp.Data) != 1 || invitesResp.Data[0].InviterUserId != "1" || invitesResp.Data[0].InviteeUserId != "2" {
		t.Fatalf("unexpected affiliate invites: %+v", invitesResp.Data)
	}

	ruleCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliate-rules", strings.NewReader(`{"name":"Default rebate","trigger_type":"payment_paid","rate":"0.10000000","fixed_amount":"0","currency":"USD","max_rebate_amount":"5.00"}`))
	ruleCreateReq.Header.Set("Content-Type", "application/json")
	ruleCreateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	ruleCreateReq.AddCookie(sessionCookie)
	ruleCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(ruleCreateRec, ruleCreateReq)
	if ruleCreateRec.Code != http.StatusCreated {
		t.Fatalf("expected affiliate rule create 201, got %d body=%s", ruleCreateRec.Code, ruleCreateRec.Body.String())
	}
	var ruleCreateResp apiopenapi.AffiliateRuleResponse
	if err := json.NewDecoder(ruleCreateRec.Body).Decode(&ruleCreateResp); err != nil {
		t.Fatalf("decode affiliate rule create: %v", err)
	}
	if ruleCreateResp.Data.Rate != "0.10000000" || ruleCreateResp.Data.Currency != "USD" {
		t.Fatalf("unexpected affiliate rule create response: %+v", ruleCreateResp.Data)
	}

	ruleUpdateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/affiliate-rules/"+string(ruleCreateResp.Data.Id), strings.NewReader(`{"rate":"0.15000000","status":"active"}`))
	ruleUpdateReq.Header.Set("Content-Type", "application/json")
	ruleUpdateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	ruleUpdateReq.AddCookie(sessionCookie)
	ruleUpdateRec := httptest.NewRecorder()
	handler.ServeHTTP(ruleUpdateRec, ruleUpdateReq)
	if ruleUpdateRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate rule update 200, got %d body=%s", ruleUpdateRec.Code, ruleUpdateRec.Body.String())
	}
	var ruleUpdateResp apiopenapi.AffiliateRuleResponse
	if err := json.NewDecoder(ruleUpdateRec.Body).Decode(&ruleUpdateResp); err != nil {
		t.Fatalf("decode affiliate rule update: %v", err)
	}
	if ruleUpdateResp.Data.Rate != "0.15000000" {
		t.Fatalf("unexpected affiliate rule update response: %+v", ruleUpdateResp.Data)
	}

	rulesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/affiliate-rules", nil)
	rulesReq.AddCookie(sessionCookie)
	rulesRec := httptest.NewRecorder()
	handler.ServeHTTP(rulesRec, rulesReq)
	if rulesRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate rules 200, got %d body=%s", rulesRec.Code, rulesRec.Body.String())
	}
	var rulesResp apiopenapi.AffiliateRuleListResponse
	if err := json.NewDecoder(rulesRec.Body).Decode(&rulesResp); err != nil {
		t.Fatalf("decode affiliate rules: %v", err)
	}
	if len(rulesResp.Data) != 1 || rulesResp.Data[0].Rate != "0.15000000" {
		t.Fatalf("unexpected affiliate rules: %+v", rulesResp.Data)
	}

	rebatesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/affiliates/rebates?user_id=1", nil)
	rebatesReq.AddCookie(sessionCookie)
	rebatesRec := httptest.NewRecorder()
	handler.ServeHTTP(rebatesRec, rebatesReq)
	if rebatesRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate rebates 200, got %d body=%s", rebatesRec.Code, rebatesRec.Body.String())
	}
	var rebatesResp apiopenapi.AffiliateLedgerEntryListResponse
	if err := json.NewDecoder(rebatesRec.Body).Decode(&rebatesResp); err != nil {
		t.Fatalf("decode affiliate rebates: %v", err)
	}
	if len(rebatesResp.Data) != 1 || rebatesResp.Data[0].Type != apiopenapi.Accrue || rebatesResp.Data[0].Amount != "3.00000000" {
		t.Fatalf("unexpected affiliate rebates: %+v", rebatesResp.Data)
	}

	transfersReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/affiliates/transfers?user_id=1", nil)
	transfersReq.AddCookie(sessionCookie)
	transfersRec := httptest.NewRecorder()
	handler.ServeHTTP(transfersRec, transfersReq)
	if transfersRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate transfers 200, got %d body=%s", transfersRec.Code, transfersRec.Body.String())
	}
	var transfersResp apiopenapi.AffiliateLedgerEntryListResponse
	if err := json.NewDecoder(transfersRec.Body).Decode(&transfersResp); err != nil {
		t.Fatalf("decode affiliate transfers: %v", err)
	}
	if len(transfersResp.Data) != 1 || transfersResp.Data[0].Type != apiopenapi.TransferToBalance || transfersResp.Data[0].Amount != "-1.50000000" {
		t.Fatalf("unexpected affiliate transfers: %+v", transfersResp.Data)
	}

	manualReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliates/manual-adjustments", strings.NewReader(`{"user_id":"1","amount":"2.50","currency":"USD","reason":"support correction","reference_id":"manual-1"}`))
	manualReq.Header.Set("Content-Type", "application/json")
	manualReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	manualReq.AddCookie(sessionCookie)
	manualRec := httptest.NewRecorder()
	handler.ServeHTTP(manualRec, manualReq)
	if manualRec.Code != http.StatusCreated {
		t.Fatalf("expected manual adjustment 201, got %d body=%s", manualRec.Code, manualRec.Body.String())
	}
	var manualResp apiopenapi.AffiliateLedgerEntryResponse
	if err := json.NewDecoder(manualRec.Body).Decode(&manualResp); err != nil {
		t.Fatalf("decode manual adjustment: %v", err)
	}
	if manualResp.Data.Type != apiopenapi.ManualAdjustment || manualResp.Data.Amount != "2.50000000" {
		t.Fatalf("unexpected manual adjustment: %+v", manualResp.Data)
	}

	ledgers, err := store.ListLedgers(t.Context())
	if err != nil {
		t.Fatalf("list ledgers after manual adjustment: %v", err)
	}
	manualCount := 0
	for _, ledger := range ledgers {
		if ledger.Type == affiliatecontract.LedgerTypeManualAdjustment && ledger.UserID == 1 {
			manualCount++
		}
	}
	if manualCount != 1 {
		t.Fatalf("expected one manual adjustment ledger, got %d rows=%+v", manualCount, ledgers)
	}
}

func TestCurrentUserAffiliateSummaryLedgerAndTransfer(t *testing.T) {
	store := affiliatememory.New()
	now := time.Now().UTC()
	if _, _, err := store.AppendLedger(t.Context(), affiliatecontract.AffiliateLedger{
		UserID:        1,
		RelatedUserID: 2,
		Type:          affiliatecontract.LedgerTypeAccrue,
		Amount:        "10.00000000",
		Currency:      "USD",
		Status:        affiliatecontract.LedgerStatusPending,
		ReferenceID:   "current-user-rebate",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed current user rebate ledger: %v", err)
	}
	if _, _, err := store.AppendLedger(t.Context(), affiliatecontract.AffiliateLedger{
		UserID:        1,
		RelatedUserID: 2,
		Type:          affiliatecontract.LedgerTypeRefundCompensation,
		Amount:        "-2.00000000",
		Currency:      "USD",
		Status:        affiliatecontract.LedgerStatusCompensated,
		ReferenceID:   "current-user-compensation",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed current user compensation ledger: %v", err)
	}
	if _, _, err := store.AppendLedger(t.Context(), affiliatecontract.AffiliateLedger{
		UserID:        2,
		RelatedUserID: 1,
		Type:          affiliatecontract.LedgerTypeAccrue,
		Amount:        "99.00000000",
		Currency:      "USD",
		Status:        affiliatecontract.LedgerStatusPending,
		ReferenceID:   "other-user-rebate",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed other user rebate ledger: %v", err)
	}

	handler := New(config.Load(), nil, WithAffiliateStore(store))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createCodeReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/affiliate/invite-codes", strings.NewReader(`{"code":"my-code"}`))
	createCodeReq.Header.Set("Content-Type", "application/json")
	createCodeReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createCodeReq.AddCookie(sessionCookie)
	createCodeRec := httptest.NewRecorder()
	handler.ServeHTTP(createCodeRec, createCodeReq)
	if createCodeRec.Code != http.StatusCreated {
		t.Fatalf("expected invite code create 201, got %d body=%s", createCodeRec.Code, createCodeRec.Body.String())
	}
	var createCodeResp apiopenapi.AffiliateInviteCodeResponse
	if err := json.NewDecoder(createCodeRec.Body).Decode(&createCodeResp); err != nil {
		t.Fatalf("decode invite code create: %v", err)
	}
	if createCodeResp.Data.Code != "MY-CODE" || createCodeResp.Data.UserId != "1" {
		t.Fatalf("unexpected invite code create: %+v", createCodeResp.Data)
	}

	listCodesReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/affiliate/invite-codes", nil)
	listCodesReq.AddCookie(sessionCookie)
	listCodesRec := httptest.NewRecorder()
	handler.ServeHTTP(listCodesRec, listCodesReq)
	if listCodesRec.Code != http.StatusOK {
		t.Fatalf("expected invite code list 200, got %d body=%s", listCodesRec.Code, listCodesRec.Body.String())
	}
	var listCodesResp apiopenapi.AffiliateInviteCodeListResponse
	if err := json.NewDecoder(listCodesRec.Body).Decode(&listCodesResp); err != nil {
		t.Fatalf("decode invite code list: %v", err)
	}
	if len(listCodesResp.Data) != 1 || listCodesResp.Data[0].Code != "MY-CODE" {
		t.Fatalf("unexpected invite code list: %+v", listCodesResp.Data)
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/affiliate", nil)
	summaryReq.AddCookie(sessionCookie)
	summaryRec := httptest.NewRecorder()
	handler.ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate summary 200, got %d body=%s", summaryRec.Code, summaryRec.Body.String())
	}
	var summaryResp apiopenapi.AffiliateSummaryResponse
	if err := json.NewDecoder(summaryRec.Body).Decode(&summaryResp); err != nil {
		t.Fatalf("decode affiliate summary: %v", err)
	}
	if summaryResp.Data.UserId != "1" || len(summaryResp.Data.Balances) != 1 {
		t.Fatalf("unexpected affiliate summary shape: %+v", summaryResp.Data)
	}
	if len(summaryResp.Data.InviteCodes) != 1 || summaryResp.Data.InviteCodes[0].Code != "MY-CODE" {
		t.Fatalf("expected affiliate summary invite codes, got %+v", summaryResp.Data.InviteCodes)
	}
	balance := summaryResp.Data.Balances[0]
	if balance.AvailableBalance != "8.00000000" || balance.AccruedAmount != "10.00000000" || balance.RefundCompensatedAmount != "2.00000000" {
		t.Fatalf("unexpected affiliate summary balance: %+v", balance)
	}

	ledgerReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/affiliate/ledger", nil)
	ledgerReq.AddCookie(sessionCookie)
	ledgerRec := httptest.NewRecorder()
	handler.ServeHTTP(ledgerRec, ledgerReq)
	if ledgerRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate ledger 200, got %d body=%s", ledgerRec.Code, ledgerRec.Body.String())
	}
	var ledgerResp apiopenapi.AffiliateLedgerEntryListResponse
	if err := json.NewDecoder(ledgerRec.Body).Decode(&ledgerResp); err != nil {
		t.Fatalf("decode affiliate ledger: %v", err)
	}
	if len(ledgerResp.Data) != 2 || ledgerResp.Data[0].UserId != "1" || ledgerResp.Data[1].UserId != "1" {
		t.Fatalf("expected only current user ledger rows, got %+v", ledgerResp.Data)
	}

	missingKeyReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/affiliate/transfer-to-balance", strings.NewReader(`{"amount":"3.00","currency":"USD"}`))
	missingKeyReq.Header.Set("Content-Type", "application/json")
	missingKeyReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	missingKeyReq.AddCookie(sessionCookie)
	missingKeyRec := httptest.NewRecorder()
	handler.ServeHTTP(missingKeyRec, missingKeyReq)
	if missingKeyRec.Code != http.StatusBadRequest {
		t.Fatalf("expected missing idempotency key 400, got %d body=%s", missingKeyRec.Code, missingKeyRec.Body.String())
	}

	transferReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/affiliate/transfer-to-balance", strings.NewReader(`{"amount":"3.00","currency":"USD"}`))
	transferReq.Header.Set("Content-Type", "application/json")
	transferReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	transferReq.Header.Set("Idempotency-Key", "transfer-current-user")
	transferReq.AddCookie(sessionCookie)
	transferRec := httptest.NewRecorder()
	handler.ServeHTTP(transferRec, transferReq)
	if transferRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate transfer 200, got %d body=%s", transferRec.Code, transferRec.Body.String())
	}
	var transferResp apiopenapi.AffiliateTransferToBalanceResponse
	if err := json.NewDecoder(transferRec.Body).Decode(&transferResp); err != nil {
		t.Fatalf("decode affiliate transfer: %v", err)
	}
	if !transferResp.Data.Applied || transferResp.Data.AffiliateLedger.Amount != "-3.00000000" || transferResp.Data.BalanceAfter != "3.00000000" {
		t.Fatalf("unexpected affiliate transfer result: %+v", transferResp.Data)
	}

	duplicateReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/affiliate/transfer-to-balance", strings.NewReader(`{"amount":"3.00","currency":"USD"}`))
	duplicateReq.Header.Set("Content-Type", "application/json")
	duplicateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	duplicateReq.Header.Set("Idempotency-Key", "transfer-current-user")
	duplicateReq.AddCookie(sessionCookie)
	duplicateRec := httptest.NewRecorder()
	handler.ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusOK {
		t.Fatalf("expected duplicate affiliate transfer 200, got %d body=%s", duplicateRec.Code, duplicateRec.Body.String())
	}
	var duplicateResp apiopenapi.AffiliateTransferToBalanceResponse
	if err := json.NewDecoder(duplicateRec.Body).Decode(&duplicateResp); err != nil {
		t.Fatalf("decode duplicate affiliate transfer: %v", err)
	}
	if duplicateResp.Data.Applied || duplicateResp.Data.Reason == nil || *duplicateResp.Data.Reason != "duplicate_transfer" {
		t.Fatalf("expected duplicate transfer no-op, got %+v", duplicateResp.Data)
	}

	withdrawReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/affiliate/withdrawals", strings.NewReader(`{"amount":"2.00","currency":"USD","destination":"paypal:test@example.com"}`))
	withdrawReq.Header.Set("Content-Type", "application/json")
	withdrawReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	withdrawReq.Header.Set("Idempotency-Key", "withdraw-current-user")
	withdrawReq.AddCookie(sessionCookie)
	withdrawRec := httptest.NewRecorder()
	handler.ServeHTTP(withdrawRec, withdrawReq)
	if withdrawRec.Code != http.StatusCreated {
		t.Fatalf("expected affiliate withdraw 201, got %d body=%s", withdrawRec.Code, withdrawRec.Body.String())
	}
	var withdrawResp apiopenapi.AffiliateLedgerEntryResponse
	if err := json.NewDecoder(withdrawRec.Body).Decode(&withdrawResp); err != nil {
		t.Fatalf("decode affiliate withdraw: %v", err)
	}
	if withdrawResp.Data.Type != apiopenapi.Withdraw || withdrawResp.Data.Amount != "-2.00000000" || withdrawResp.Data.Status != apiopenapi.AffiliateLedgerEntryStatusPending {
		t.Fatalf("unexpected affiliate withdraw result: %+v", withdrawResp.Data)
	}

	afterWithdrawReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/affiliate", nil)
	afterWithdrawReq.AddCookie(sessionCookie)
	afterWithdrawRec := httptest.NewRecorder()
	handler.ServeHTTP(afterWithdrawRec, afterWithdrawReq)
	if afterWithdrawRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate summary after withdraw 200, got %d body=%s", afterWithdrawRec.Code, afterWithdrawRec.Body.String())
	}
	var afterWithdrawResp apiopenapi.AffiliateSummaryResponse
	if err := json.NewDecoder(afterWithdrawRec.Body).Decode(&afterWithdrawResp); err != nil {
		t.Fatalf("decode affiliate summary after withdraw: %v", err)
	}
	if len(afterWithdrawResp.Data.Balances) != 1 || afterWithdrawResp.Data.Balances[0].WithdrawnAmount != "2.00000000" || afterWithdrawResp.Data.Balances[0].AvailableBalance != "3.00000000" {
		t.Fatalf("unexpected affiliate summary after withdraw: %+v", afterWithdrawResp.Data.Balances)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliates/withdrawals/"+string(withdrawResp.Data.Id)+"/approve", strings.NewReader(`{"reason":"paid out"}`))
	approveReq.Header.Set("Content-Type", "application/json")
	approveReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	approveReq.AddCookie(sessionCookie)
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate withdraw approval 200, got %d body=%s", approveRec.Code, approveRec.Body.String())
	}
	var approveResp apiopenapi.AffiliateLedgerEntryResponse
	if err := json.NewDecoder(approveRec.Body).Decode(&approveResp); err != nil {
		t.Fatalf("decode affiliate withdraw approval: %v", err)
	}
	if approveResp.Data.Status != apiopenapi.AffiliateLedgerEntryStatusSettled || approveResp.Data.SettledAt == nil {
		t.Fatalf("unexpected affiliate withdraw approval result: %+v", approveResp.Data)
	}

	duplicateApproveReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliates/withdrawals/"+string(withdrawResp.Data.Id)+"/approve", strings.NewReader(`{}`))
	duplicateApproveReq.Header.Set("Content-Type", "application/json")
	duplicateApproveReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	duplicateApproveReq.AddCookie(sessionCookie)
	duplicateApproveRec := httptest.NewRecorder()
	handler.ServeHTTP(duplicateApproveRec, duplicateApproveReq)
	if duplicateApproveRec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate affiliate withdraw approval 409, got %d body=%s", duplicateApproveRec.Code, duplicateApproveRec.Body.String())
	}

	cancelWithdrawReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/affiliate/withdrawals", strings.NewReader(`{"amount":"1.00","currency":"USD","destination":"paypal:test@example.com"}`))
	cancelWithdrawReq.Header.Set("Content-Type", "application/json")
	cancelWithdrawReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	cancelWithdrawReq.Header.Set("Idempotency-Key", "withdraw-cancel-current-user")
	cancelWithdrawReq.AddCookie(sessionCookie)
	cancelWithdrawRec := httptest.NewRecorder()
	handler.ServeHTTP(cancelWithdrawRec, cancelWithdrawReq)
	if cancelWithdrawRec.Code != http.StatusCreated {
		t.Fatalf("expected second affiliate withdraw 201, got %d body=%s", cancelWithdrawRec.Code, cancelWithdrawRec.Body.String())
	}
	var cancelWithdrawResp apiopenapi.AffiliateLedgerEntryResponse
	if err := json.NewDecoder(cancelWithdrawRec.Body).Decode(&cancelWithdrawResp); err != nil {
		t.Fatalf("decode second affiliate withdraw: %v", err)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliates/withdrawals/"+string(cancelWithdrawResp.Data.Id)+"/cancel", strings.NewReader(`{"reason":"duplicate request"}`))
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	cancelReq.AddCookie(sessionCookie)
	cancelRec := httptest.NewRecorder()
	handler.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate withdraw cancellation 200, got %d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	var cancelResp apiopenapi.AffiliateLedgerEntryResponse
	if err := json.NewDecoder(cancelRec.Body).Decode(&cancelResp); err != nil {
		t.Fatalf("decode affiliate withdraw cancellation: %v", err)
	}
	if cancelResp.Data.Status != apiopenapi.AffiliateLedgerEntryStatusCanceled || cancelResp.Data.SettledAt != nil {
		t.Fatalf("unexpected affiliate withdraw cancellation result: %+v", cancelResp.Data)
	}

	afterCancelReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/affiliate", nil)
	afterCancelReq.AddCookie(sessionCookie)
	afterCancelRec := httptest.NewRecorder()
	handler.ServeHTTP(afterCancelRec, afterCancelReq)
	if afterCancelRec.Code != http.StatusOK {
		t.Fatalf("expected affiliate summary after cancel 200, got %d body=%s", afterCancelRec.Code, afterCancelRec.Body.String())
	}
	var afterCancelResp apiopenapi.AffiliateSummaryResponse
	if err := json.NewDecoder(afterCancelRec.Body).Decode(&afterCancelResp); err != nil {
		t.Fatalf("decode affiliate summary after cancel: %v", err)
	}
	if len(afterCancelResp.Data.Balances) != 1 || afterCancelResp.Data.Balances[0].WithdrawnAmount != "2.00000000" || afterCancelResp.Data.Balances[0].AvailableBalance != "3.00000000" {
		t.Fatalf("unexpected affiliate summary after cancel: %+v", afterCancelResp.Data.Balances)
	}
}

func TestAffiliateInviteRuleAndAccrualFlow(t *testing.T) {
	store := affiliatememory.New()
	handler := New(config.Load(), nil, WithAffiliateStore(store))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createCodeReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/affiliate/invite-codes", strings.NewReader(`{"code":"rebate-e2e"}`))
	createCodeReq.Header.Set("Content-Type", "application/json")
	createCodeReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createCodeReq.AddCookie(sessionCookie)
	createCodeRec := httptest.NewRecorder()
	handler.ServeHTTP(createCodeRec, createCodeReq)
	if createCodeRec.Code != http.StatusCreated {
		t.Fatalf("expected invite code create 201, got %d body=%s", createCodeRec.Code, createCodeRec.Body.String())
	}

	ruleReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliate-rules", strings.NewReader(`{"name":"E2E rebate","trigger_type":"payment_paid","rate":"0.10000000","fixed_amount":"0","currency":"USD","max_rebate_amount":"0"}`))
	ruleReq.Header.Set("Content-Type", "application/json")
	ruleReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	ruleReq.AddCookie(sessionCookie)
	ruleRec := httptest.NewRecorder()
	handler.ServeHTTP(ruleRec, ruleReq)
	if ruleRec.Code != http.StatusCreated {
		t.Fatalf("expected affiliate rule create 201, got %d body=%s", ruleRec.Code, ruleRec.Body.String())
	}

	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"rebate-e2e@srapi.local","name":"Rebate E2E","password":"password123","invite_code":"REBATE-E2E"}`))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	handler.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected invited register 201, got %d body=%s", registerRec.Code, registerRec.Body.String())
	}
	var registerResp apiopenapi.LoginResponse
	if err := json.NewDecoder(registerRec.Body).Decode(&registerResp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	inviteeUserID, err := strconv.Atoi(registerResp.Data.User.Id)
	if err != nil {
		t.Fatalf("parse invited user id: %v", err)
	}
	relationship, err := store.FindRelationshipByInvitee(t.Context(), inviteeUserID)
	if err != nil {
		t.Fatalf("expected invite relationship after register: %v", err)
	}
	if relationship.InviterUserID != 1 || relationship.InviteeUserID != inviteeUserID {
		t.Fatalf("unexpected invite relationship: %+v", relationship)
	}

	affiliateSvc, err := affiliateservice.New(store, affiliateservice.Dependencies{}, nil)
	if err != nil {
		t.Fatalf("new affiliate service: %v", err)
	}
	result, err := affiliateSvc.AccrueRebate(t.Context(), affiliatecontract.AccrueRebateRequest{
		OrderID:               12345,
		OrderNo:               "order_rebate_e2e",
		InviteeUserID:         inviteeUserID,
		Amount:                "50.00",
		Currency:              "USD",
		PaidAt:                time.Now().UTC(),
		ProviderTransactionID: "txn_rebate_e2e",
	})
	if err != nil {
		t.Fatalf("accrue rebate: %v", err)
	}
	if !result.Applied || result.Reason == "no_invite_relationship" || result.Reason == "no_effective_rule" {
		t.Fatalf("expected applied rebate, got %+v", result)
	}
	if len(result.Ledgers) != 1 || result.Ledgers[0].Type != affiliatecontract.LedgerTypeAccrue || result.Ledgers[0].Amount != "5.00000000" {
		t.Fatalf("unexpected accrue ledger: %+v", result.Ledgers)
	}

	ledgers, err := store.ListLedgers(t.Context())
	if err != nil {
		t.Fatalf("list ledgers: %v", err)
	}
	accrueCount := 0
	for _, ledger := range ledgers {
		if ledger.Type == affiliatecontract.LedgerTypeAccrue && ledger.UserID == 1 && ledger.RelatedUserID == inviteeUserID {
			accrueCount++
		}
	}
	if accrueCount != 1 {
		t.Fatalf("expected one accrue ledger for inviter, got %d rows=%+v", accrueCount, ledgers)
	}
}

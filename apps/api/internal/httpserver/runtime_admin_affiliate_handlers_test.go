package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
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
	_, sessionCookie := mustLoginAdmin(t, handler)

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
}

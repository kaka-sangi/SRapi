package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

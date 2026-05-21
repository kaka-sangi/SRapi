package service_test

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/usage/service"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
)

func TestRecordStoresSuccessfulGatewayUsageEvidence(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)}
	svc, err := service.New(usagememory.New(), clock)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	providerID := 11
	accountID := 22

	log, err := svc.Record(t.Context(), contract.RecordRequest{
		RequestID:             "req_usage_success",
		UserID:                1,
		APIKeyID:              2,
		ProviderID:            &providerID,
		AccountID:             &accountID,
		SourceProtocol:        "openai-compatible",
		SourceEndpoint:        "/v1/chat/completions",
		TargetProtocol:        "openai-compatible",
		Model:                 "gpt-4o-mini",
		InputTokens:           3,
		OutputTokens:          4,
		CachedTokens:          2,
		LatencyMS:             123,
		Success:               true,
		CompatibilityWarnings: []string{"vision_ignored"},
	})
	if err != nil {
		t.Fatalf("record usage: %v", err)
	}

	if log.ID == 0 || log.TotalTokens != 9 || log.Cost != "0.00000000" || log.Currency != "USD" || !log.CreatedAt.Equal(clock.now) {
		t.Fatalf("unexpected usage log defaults: %+v", log)
	}
	if log.ProviderID == nil || *log.ProviderID != providerID || log.AccountID == nil || *log.AccountID != accountID {
		t.Fatalf("expected provider/account evidence, got %+v", log)
	}
	if !log.Success || log.ErrorClass != nil || log.CompatibilityWarnings[0] != "vision_ignored" {
		t.Fatalf("unexpected successful usage evidence: %+v", log)
	}
}

func TestRecordStoresFailedGatewayUsageEvidence(t *testing.T) {
	svc, err := service.New(usagememory.New(), nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	errorClass := "no_available_account"

	log, err := svc.Record(t.Context(), contract.RecordRequest{
		RequestID:      "req_usage_failure",
		UserID:         1,
		APIKeyID:       2,
		SourceEndpoint: "/v1/responses",
		Model:          "missing-model",
		Success:        false,
		ErrorClass:     &errorClass,
		UsageEstimated: true,
	})
	if err != nil {
		t.Fatalf("record failed usage: %v", err)
	}

	if log.Success || log.ErrorClass == nil || *log.ErrorClass != errorClass {
		t.Fatalf("expected failed usage evidence, got %+v", log)
	}
	if log.SourceProtocol != "openai-compatible" || log.TotalTokens != 0 || !log.UsageEstimated {
		t.Fatalf("unexpected failed usage defaults: %+v", log)
	}
}

func TestRecordRejectsIncompleteGatewayUsageEvidence(t *testing.T) {
	svc, err := service.New(usagememory.New(), nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	if _, err := svc.Record(t.Context(), contract.RecordRequest{
		RequestID:      "req_invalid",
		UserID:         1,
		APIKeyID:       2,
		SourceEndpoint: "/v1/chat/completions",
	}); err == nil {
		t.Fatal("expected missing model to be rejected")
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

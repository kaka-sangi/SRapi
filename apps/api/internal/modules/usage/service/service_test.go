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

func TestAggregateAndExportUsage(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	svc, err := service.New(usagememory.New(), clock)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	accountID := 22
	for _, req := range []contract.RecordRequest{
		{
			RequestID:      "req_usage_1",
			UserID:         1,
			APIKeyID:       2,
			AccountID:      &accountID,
			SourceEndpoint: "/v1/chat/completions",
			Model:          "gpt-4o-mini",
			InputTokens:    3,
			OutputTokens:   4,
			Success:        true,
			Cost:           "0.10000000",
		},
		{
			RequestID:      "req_usage_2",
			UserID:         1,
			APIKeyID:       2,
			AccountID:      &accountID,
			SourceEndpoint: "/v1/chat/completions",
			Model:          "gpt-4o-mini",
			InputTokens:    5,
			OutputTokens:   6,
			Success:        false,
			Cost:           "0.20000000",
		},
	} {
		if _, err := svc.Record(t.Context(), req); err != nil {
			t.Fatalf("record usage: %v", err)
		}
	}

	aggregates, err := svc.Aggregate(t.Context(), contract.QueryFilter{}, contract.AggregateDimensionModel)
	if err != nil {
		t.Fatalf("aggregate usage: %v", err)
	}
	if len(aggregates) != 1 || aggregates[0].Key != "gpt-4o-mini" || aggregates[0].RequestCount != 2 || aggregates[0].SuccessCount != 1 || aggregates[0].ErrorCount != 1 || aggregates[0].TotalTokens != 18 || aggregates[0].TotalCost != "0.30000000" {
		t.Fatalf("unexpected aggregate: %+v", aggregates)
	}

	exported, err := svc.Export(t.Context(), contract.QueryFilter{})
	if err != nil {
		t.Fatalf("export usage: %v", err)
	}
	if !exported.GeneratedAt.Equal(clock.now) || len(exported.Logs) != 2 || len(exported.Daily) != 1 || len(exported.ByAccount) != 1 {
		t.Fatalf("unexpected export: %+v", exported)
	}
}

func TestSummarizeAPIKeyIsScopedAndAggregated(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	store := usagememory.New()
	svc, err := service.New(store, clock)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	oldCreatedAt := clock.now.AddDate(0, 0, -91)
	for _, log := range []contract.UsageLog{
		{
			RequestID:      "req_key_2_today",
			UserID:         1,
			APIKeyID:       2,
			SourceEndpoint: "/v1/chat/completions",
			Model:          "gpt-4o-mini",
			InputTokens:    3,
			OutputTokens:   4,
			CachedTokens:   1,
			Success:        true,
			Cost:           "0.10000000",
			Currency:       "USD",
			CreatedAt:      clock.now,
		},
		{
			RequestID:      "req_key_2_other_model",
			UserID:         1,
			APIKeyID:       2,
			SourceEndpoint: "/v1/responses",
			Model:          "gpt-4o",
			InputTokens:    5,
			OutputTokens:   6,
			Success:        false,
			Cost:           "0.25000000",
			Currency:       "USD",
			CreatedAt:      clock.now,
		},
		{
			RequestID:      "req_other_key",
			UserID:         1,
			APIKeyID:       3,
			SourceEndpoint: "/v1/chat/completions",
			Model:          "gpt-4o-mini",
			InputTokens:    100,
			OutputTokens:   100,
			TotalTokens:    200,
			Success:        true,
			Cost:           "0.00000000",
			Currency:       "USD",
			CreatedAt:      clock.now,
		},
		{
			RequestID:      "req_key_2_old",
			UserID:         1,
			APIKeyID:       2,
			SourceEndpoint: "/v1/chat/completions",
			Model:          "old-model",
			InputTokens:    100,
			OutputTokens:   100,
			TotalTokens:    200,
			Success:        true,
			Cost:           "0.00000000",
			Currency:       "USD",
			CreatedAt:      oldCreatedAt,
		},
	} {
		log.TotalTokens = log.InputTokens + log.OutputTokens + log.CachedTokens
		if _, err := store.Create(t.Context(), log); err != nil {
			t.Fatalf("seed usage log: %v", err)
		}
	}

	summary, err := svc.SummarizeAPIKey(t.Context(), 2, 30)
	if err != nil {
		t.Fatalf("summarize api key: %v", err)
	}
	if summary.APIKeyID != 2 || summary.RequestCount != 2 || summary.SuccessCount != 1 || summary.ErrorCount != 1 || summary.TotalTokens != 19 || summary.TotalCost != "0.35000000" {
		t.Fatalf("unexpected summary totals: %+v", summary)
	}
	if summary.Today.RequestCount != 2 || summary.Today.TotalTokens != 19 {
		t.Fatalf("unexpected today summary: %+v", summary.Today)
	}
	if len(summary.ModelStats) != 2 || summary.ModelStats[0].Key != "gpt-4o" || summary.ModelStats[0].TotalTokens != 11 {
		t.Fatalf("unexpected model stats: %+v", summary.ModelStats)
	}
	if len(summary.RecentLogs) != 2 || summary.RecentLogs[0].RequestID != "req_key_2_other_model" || summary.RecentLogs[1].RequestID != "req_key_2_today" {
		t.Fatalf("unexpected recent logs: %+v", summary.RecentLogs)
	}
}

func TestCleanupLogsBoundedDeleteAndDryRun(t *testing.T) {
	store := usagememory.New()
	svc, err := service.New(store, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	ctx := t.Context()
	old := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	seed := func(requestID, model string, when time.Time) {
		if _, err := store.Create(ctx, contract.UsageLog{
			RequestID: requestID, UserID: 1, APIKeyID: 1,
			SourceProtocol: "openai-compatible", SourceEndpoint: "/v1/chat/completions",
			Model: model, Cost: "0.00000000", Currency: "USD", CreatedAt: when,
		}); err != nil {
			t.Fatalf("seed usage: %v", err)
		}
	}
	seed("c1", "target", old)
	seed("c2", "target", old)
	seed("c3", "target", recent)
	seed("c4", "other", old)

	// A filter that matches nothing bounded is rejected.
	if _, err := svc.CleanupLogs(ctx, contract.CleanupFilter{MaxDelete: 5}); err != service.ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput without filter, got %v", err)
	}
	// start after end is rejected.
	start := recent
	end := old
	if _, err := svc.CleanupLogs(ctx, contract.CleanupFilter{Start: &start, End: &end}); err != service.ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for inverted range, got %v", err)
	}

	// Dry run counts the model matches without deleting.
	dry, err := svc.CleanupLogs(ctx, contract.CleanupFilter{Model: "TARGET", DryRun: true, MaxDelete: 5})
	if err != nil {
		t.Fatalf("dry run cleanup: %v", err)
	}
	if dry.Matched != 3 || dry.Deleted != 0 || !dry.DryRun || dry.Limited {
		t.Fatalf("dry run expected matched=3 deleted=0, got %+v", dry)
	}
	if logs, _ := store.List(ctx); len(logs) != 4 {
		t.Fatalf("dry run must not delete, got %d", len(logs))
	}

	// A capped delete removes only MaxDelete records (oldest first) and reports Limited.
	capped, err := svc.CleanupLogs(ctx, contract.CleanupFilter{Model: "target", MaxDelete: 1})
	if err != nil {
		t.Fatalf("capped cleanup: %v", err)
	}
	if capped.Matched != 3 || capped.Deleted != 1 || !capped.Limited {
		t.Fatalf("capped expected matched=3 deleted=1 limited, got %+v", capped)
	}

	// The remaining target matches clear; the other model survives.
	rest, err := svc.CleanupLogs(ctx, contract.CleanupFilter{Model: "target", MaxDelete: 100})
	if err != nil {
		t.Fatalf("final cleanup: %v", err)
	}
	if rest.Matched != 2 || rest.Deleted != 2 || rest.Limited {
		t.Fatalf("final expected matched=2 deleted=2, got %+v", rest)
	}
	logs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(logs) != 1 || logs[0].Model != "other" {
		t.Fatalf("expected only other-model survivor, got %+v", logs)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

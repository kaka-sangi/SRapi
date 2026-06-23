package httpserver

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	apikeymemory "github.com/srapi/srapi/apps/api/internal/modules/api_keys/store/memory"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsservice "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/service"
	opserrorlogsmemory "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usageservice "github.com/srapi/srapi/apps/api/internal/modules/usage/service"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

func TestWarnDefaultZeroGatewayPricing(t *testing.T) {
	var logs bytes.Buffer
	rt := &runtimeState{
		logger: slog.New(slog.NewTextHandler(&logs, nil)),
	}

	providerID := 42
	rt.warnDefaultZeroGatewayPricing(gatewayUsageRecord{
		RequestID:      "req_default_zero",
		SourceEndpoint: "/v1/chat/completions",
		RequestedModel: "claude-opus-4-1",
		UpstreamModel:  "gpt-5-preview",
		ProviderID:     &providerID,
	}, "zero-priced-model", gatewayPricingEvidence{PricingSource: "default_zero"})

	got := logs.String()
	if !strings.Contains(got, "gateway usage recorded with default zero pricing") {
		t.Fatalf("expected default zero pricing warning, got %q", got)
	}
	if !strings.Contains(got, "req_default_zero") || !strings.Contains(got, "zero-priced-model") {
		t.Fatalf("expected request and model fields in warning, got %q", got)
	}
	// Diagnostic context the operator needs to debug why their PricingRule
	// did not match: requested vs upstream model, provider id.
	for _, want := range []string{"claude-opus-4-1", "gpt-5-preview", "provider_id=42"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected diagnostic %q in warning, got %q", want, got)
		}
	}
}

func TestWarnDefaultZeroGatewayPricingIgnoresExplicitSources(t *testing.T) {
	var logs bytes.Buffer
	rt := &runtimeState{
		logger: slog.New(slog.NewTextHandler(&logs, nil)),
	}

	rt.warnDefaultZeroGatewayPricing(gatewayUsageRecord{
		RequestID:      "req_priced",
		SourceEndpoint: "/v1/chat/completions",
	}, "priced-model", gatewayPricingEvidence{PricingSource: "pricing_rule"})

	if got := logs.String(); got != "" {
		t.Fatalf("did not expect warning for explicit pricing source, got %q", got)
	}
}

func TestGatewayServiceTierNormalizesOpenAITiers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "fast alias", raw: " fast ", want: "priority"},
		{name: "official tier", raw: "AUTO", want: "auto"},
		{name: "unknown tier", raw: "turbo", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gatewayServiceTier(gatewaycontract.CanonicalRequest{
				RawBody: []byte(`{"service_tier":` + strconv.Quote(tt.raw) + `}`),
			})
			if got != tt.want {
				t.Fatalf("gatewayServiceTier() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRecordGatewaySystemLogDoesNotDependOnOpsErrorLogs(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	rt := &runtimeState{
		logger:     slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations: operations,
	}

	rt.recordGatewaySystemLog(ctx, gatewayUsageRecord{
		RequestID:                "req_gateway_system_log",
		SourceProtocol:           "openai-compatible",
		SourceEndpoint:           "/v1/responses",
		TargetProtocol:           "openai-compatible",
		Model:                    "codex-model",
		ProviderID:               ptrInt(11),
		AccountID:                ptrInt(22),
		AttemptNo:                2,
		Success:                  false,
		ErrorClass:               ptrStringValue("invalid_request"),
		StatusCode:               ptrInt(http.StatusBadRequest),
		ProviderErrorMessage:     "upstream rejected compact payload",
		ProviderErrorBodyExcerpt: `{"error":"bad request"}`,
	})

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelWarn || log.Source != "gateway" || log.RequestID != "req_gateway_system_log" {
		t.Fatalf("unexpected system log: %+v", log)
	}
	if log.Metadata["error_class"] != "invalid_request" ||
		metadataNumber(log.Metadata["upstream_status"]) != 400 ||
		metadataNumber(log.Metadata["provider_id"]) != 11 ||
		metadataNumber(log.Metadata["account_id"]) != 22 {
		t.Fatalf("unexpected system log metadata: %+v", log.Metadata)
	}
}

func TestRecordGatewayNoAvailableAccountCapturesSchedulerDiagnostics(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	opsStore := opserrorlogsmemory.New()
	opsLogs, err := opserrorlogsservice.New(opsStore, nil)
	if err != nil {
		t.Fatalf("new ops error log service: %v", err)
	}
	usageStore := usagememory.New()
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	billing, err := billingservice.New(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing service: %v", err)
	}
	rt := &runtimeState{
		logger:       slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations:   operations,
		opsErrorLogs: opsLogs,
		usage:        usage,
		events:       events,
		billing:      billing,
	}
	server := &Server{runtime: rt}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req = req.WithContext(ctx)

	server.recordGatewayNoAvailableAccount(req,
		apikeycontract.AuthResult{UserID: 42, Key: apikeycontract.APIKey{ID: 7, Prefix: "sk_live_test"}},
		gatewaycontract.CanonicalRequest{
			RequestID:        "req_no_account_diag",
			SourceProtocol:   gatewaycontract.ProtocolOpenAICompatible,
			SourceEndpoint:   string(gatewaycontract.EndpointResponses),
			Model:            "codex-mini",
			CanonicalModel:   "codex-mini",
			ResponseProtocol: gatewaycontract.ProtocolOpenAICompatible,
		},
		schedulercontract.ScheduleResult{Decision: schedulercontract.Decision{
			ID:                 77,
			RequestID:          "req_no_account_diag",
			AttemptNo:          2,
			TargetProtocol:     "openai-compatible",
			Model:              "codex-mini",
			CandidateCount:     3,
			RejectedCount:      3,
			RejectReasons:      map[string]any{"11": "capability_mismatch:responses", "12": "capability_mismatch:responses", "13": "cooldown_active"},
			SelectionRationale: "no account satisfied responses capability",
		}},
		gatewayAdmission{EstimatedUsage: gatewaycontract.Usage{InputTokens: 8, OutputTokens: 13}},
		time.Now().Add(-25*time.Millisecond),
	)

	systemLogs, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(systemLogs.Items) != 1 {
		t.Fatalf("expected one system log, got %+v", systemLogs.Items)
	}
	systemLog := systemLogs.Items[0]
	if systemLog.Level != operationscontract.OpsSystemLogLevelInfo || systemLog.Message == "" {
		t.Fatalf("unexpected system log: %+v", systemLog)
	}
	metadata := systemLog.Metadata
	if metadataNumber(metadata["scheduler_decision_id"]) != 77 ||
		metadataNumber(metadata["scheduler_candidate_count"]) != 3 ||
		metadataNumber(metadata["scheduler_rejected_count"]) != 3 ||
		metadata["scheduler_primary_reject_reason"] != "capability_mismatch:responses" ||
		metadataNumber(metadata["scheduler_primary_reject_count"]) != 2 ||
		metadata["scheduler_operator_action"] != "check_model_capabilities_or_mapping" ||
		metadataNumber(metadata["response_status"]) != http.StatusServiceUnavailable {
		t.Fatalf("unexpected scheduler diagnostic metadata: %+v", metadata)
	}
	reasonCounts, ok := metadata["scheduler_reject_reason_counts"].(map[string]any)
	if !ok {
		t.Fatalf("expected reject reason counts, got %#v", metadata["scheduler_reject_reason_counts"])
	}
	if metadataNumber(reasonCounts["capability_mismatch:responses"]) != 2 || metadataNumber(reasonCounts["cooldown_active"]) != 1 {
		t.Fatalf("unexpected reject reason counts: %+v", reasonCounts)
	}

	opsResult, err := opsStore.List(ctx, opserrorlogscontract.ListFilter{})
	if err != nil {
		t.Fatalf("list ops error logs: %v", err)
	}
	if len(opsResult.Items) != 1 {
		t.Fatalf("expected one ops error log, got %+v", opsResult.Items)
	}
	opsLog := opsResult.Items[0]
	if opsLog.ErrorClass != "no_available_account" ||
		opsLog.ErrorPhase != "routing" ||
		opsLog.ErrorOwner != "platform" ||
		opsLog.ErrorSource != "gateway" {
		t.Fatalf("unexpected ops error classification: %+v", opsLog)
	}
	if !strings.Contains(opsLog.ErrorMessage, "temporarily unavailable") ||
		!strings.Contains(opsLog.ErrorBodyExcerpt, `"scheduler_operator_action":"check_model_capabilities_or_mapping"`) ||
		!strings.Contains(opsLog.ErrorBodyExcerpt, `"capability_mismatch:responses":2`) {
		t.Fatalf("unexpected ops error evidence: message=%q body=%q", opsLog.ErrorMessage, opsLog.ErrorBodyExcerpt)
	}

	usageLogs, err := usageStore.List(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	if len(usageLogs) != 1 {
		t.Fatalf("expected one usage log, got %+v", usageLogs)
	}
	usageLog := usageLogs[0]
	if usageLog.ErrorClass == nil || *usageLog.ErrorClass != "no_available_account" ||
		usageLog.ErrorPhase != "routing" ||
		usageLog.ProviderErrorMessage != opsLog.ErrorMessage ||
		!strings.Contains(usageLog.ProviderErrorBodyExcerpt, `"scheduler_decision_id":77`) {
		t.Fatalf("unexpected usage evidence: %+v", usageLog)
	}
}

func TestRecordGatewayUsageWriteFailureCreatesSystemLog(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	usage, err := usageservice.New(failingUsageStore{err: errors.New("usage store down")}, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger:     slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations: operations,
		usage:      usage,
		events:     events,
	}

	rt.recordGatewayUsage(ctx, gatewayUsageRecord{
		RequestID:      "req_usage_write_failure",
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Model:          "broken-usage-model",
		ProviderID:     ptrInt(7),
		AccountID:      ptrInt(8),
		Success:        true,
		StatusCode:     ptrInt(http.StatusOK),
	})

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one usage failure system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelError || log.Source != "gateway.usage" || log.RequestID != "req_usage_write_failure" {
		t.Fatalf("unexpected usage failure system log: %+v", log)
	}
	if log.Message != "failed to record gateway usage log" || log.Metadata["error_class"] != "usage_log_write_failed" || log.Metadata["gateway_success"] != true {
		t.Fatalf("unexpected usage failure metadata: %+v", log.Metadata)
	}
}

func TestGatewayUsageBillingAggregationFailureCreatesSystemLog(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	rt := &runtimeState{
		logger:          slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations:      operations,
		usageAggregator: failingUsageAggregator{err: errors.New("aggregation store down")},
	}

	rt.recordGatewayUsageEffects(ctx, gatewayUsageRecord{
		RequestID:      "req_aggregation_failure",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		AccountID:      ptrInt(12),
		ProviderID:     ptrInt(34),
		AttemptNo:      2,
		Success:        true,
	}, "gpt-ops", gatewayPricingEvidence{}, 99)

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one aggregation failure system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelError ||
		log.Source != "gateway.usage" ||
		log.Message != "failed to aggregate gateway usage billing" ||
		log.RequestID != "req_aggregation_failure" {
		t.Fatalf("unexpected aggregation failure log: %+v", log)
	}
	if log.Metadata["effect"] != "billing_aggregation" ||
		log.Metadata["error_class"] != "billing_aggregation_failed" ||
		metadataNumber(log.Metadata["usage_log_id"]) != 99 ||
		metadataNumber(log.Metadata["account_id"]) != 12 ||
		metadataNumber(log.Metadata["provider_id"]) != 34 ||
		log.Metadata["gateway_success"] != true {
		t.Fatalf("unexpected aggregation failure metadata: %+v", log.Metadata)
	}
}

func TestGatewayUsageSchedulerFeedbackFailureCreatesSystemLog(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	schedulerStore := failingSchedulerFeedbackStore{
		Store: schedulermemory.New(),
		err:   errors.New("feedback store down"),
	}
	schedulerSvc, err := schedulerservice.New(schedulerStore, nil)
	if err != nil {
		t.Fatalf("new scheduler service: %v", err)
	}
	rt := &runtimeState{
		logger:     slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations: operations,
		scheduler:  schedulerSvc,
	}

	rt.recordGatewayUsageEffects(ctx, gatewayUsageRecord{
		RequestID:      "req_feedback_failure",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		AccountID:      ptrInt(21),
		ProviderID:     ptrInt(43),
		DecisionID:     88,
		AttemptNo:      1,
		Success:        true,
	}, "gpt-ops", gatewayPricingEvidence{}, 0)

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one scheduler feedback failure system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelError ||
		log.Source != "gateway.usage" ||
		log.Message != "failed to record scheduler feedback" ||
		log.RequestID != "req_feedback_failure" {
		t.Fatalf("unexpected feedback failure log: %+v", log)
	}
	if log.Metadata["effect"] != "scheduler_feedback" ||
		log.Metadata["error_class"] != "scheduler_feedback_failed" ||
		metadataNumber(log.Metadata["scheduler_decision_id"]) != 88 ||
		metadataNumber(log.Metadata["account_id"]) != 21 ||
		metadataNumber(log.Metadata["provider_id"]) != 43 ||
		log.Metadata["gateway_success"] != true {
		t.Fatalf("unexpected feedback failure metadata: %+v", log.Metadata)
	}
}

func TestGatewayUsageEventEnqueueFailureCreatesSystemLog(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	events, err := eventsservice.New(failingEventsStore{Store: eventsmemory.New(), err: errors.New("outbox down")}, nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger:     slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations: operations,
		events:     events,
	}

	rt.enqueueGatewayUsageEvent(ctx, usagecontract.UsageLog{
		ID:             77,
		RequestID:      "req_event_enqueue_failure",
		AttemptNo:      3,
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "gpt-ops",
		Success:        true,
		AccountID:      ptrInt(12),
		ProviderID:     ptrInt(34),
	})

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one event enqueue failure system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelError ||
		log.Source != "gateway.usage" ||
		log.Message != "failed to enqueue gateway usage event" ||
		log.RequestID != "req_event_enqueue_failure" {
		t.Fatalf("unexpected event enqueue failure log: %+v", log)
	}
	if log.Metadata["effect"] != "event_enqueue" ||
		log.Metadata["error_class"] != "gateway_usage_event_enqueue_failed" ||
		metadataNumber(log.Metadata["usage_log_id"]) != 77 ||
		metadataNumber(log.Metadata["attempt_no"]) != 3 ||
		metadataNumber(log.Metadata["account_id"]) != 12 ||
		metadataNumber(log.Metadata["provider_id"]) != 34 ||
		log.Metadata["gateway_success"] != true {
		t.Fatalf("unexpected event enqueue failure metadata: %+v", log.Metadata)
	}
}

func TestGatewayUsageFailureEventEnqueueFailureCreatesSystemLog(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	events, err := eventsservice.New(failingEventsStore{Store: eventsmemory.New(), err: errors.New("outbox down")}, nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger:     slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations: operations,
		events:     events,
	}

	rt.enqueueGatewayUsageFailureEvent(ctx, gatewayUsageRecord{
		RequestID:      "req_failure_event_enqueue_failure",
		AttemptNo:      2,
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Model:          "gpt-ops",
		Success:        false,
		AccountID:      ptrInt(21),
		ProviderID:     ptrInt(43),
		ErrorClass:     ptrStringValue("timeout"),
	}, "gpt-ops")

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one failure event enqueue system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelError ||
		log.Source != "gateway.usage" ||
		log.Message != "failed to enqueue gateway usage failure event" ||
		log.RequestID != "req_failure_event_enqueue_failure" {
		t.Fatalf("unexpected failure event enqueue log: %+v", log)
	}
	if log.Metadata["effect"] != "event_enqueue" ||
		log.Metadata["error_class"] != "gateway_usage_event_enqueue_failed" ||
		metadataNumber(log.Metadata["attempt_no"]) != 2 ||
		metadataNumber(log.Metadata["account_id"]) != 21 ||
		metadataNumber(log.Metadata["provider_id"]) != 43 ||
		log.Metadata["gateway_success"] != false {
		t.Fatalf("unexpected failure event enqueue metadata: %+v", log.Metadata)
	}
}

func TestGatewayUsageAccountSnapshotRefreshEnqueueFailureCreatesSystemLog(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	operations, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	events, err := eventsservice.New(failingEventsStore{Store: eventsmemory.New(), err: errors.New("outbox down")}, nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger:     slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		operations: operations,
		events:     events,
	}

	rt.enqueueGatewayAccountSnapshotRefresh(ctx, gatewayUsageRecord{
		RequestID:      "req_snapshot_refresh_enqueue_failure",
		AttemptNo:      4,
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "gpt-ops",
		Success:        true,
		AccountID:      ptrInt(52),
		ProviderID:     ptrInt(61),
	})

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one account snapshot refresh enqueue system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelError ||
		log.Source != "gateway.usage" ||
		log.Message != "failed to enqueue gateway account snapshot refresh" ||
		log.RequestID != "req_snapshot_refresh_enqueue_failure" {
		t.Fatalf("unexpected account snapshot refresh enqueue log: %+v", log)
	}
	if log.Metadata["effect"] != "account_snapshot_refresh_enqueue" ||
		log.Metadata["error_class"] != "account_snapshot_refresh_enqueue_failed" ||
		metadataNumber(log.Metadata["attempt_no"]) != 4 ||
		metadataNumber(log.Metadata["account_id"]) != 52 ||
		metadataNumber(log.Metadata["provider_id"]) != 61 ||
		log.Metadata["gateway_success"] != true {
		t.Fatalf("unexpected account snapshot refresh enqueue metadata: %+v", log.Metadata)
	}
}

func TestGatewayInvalidAPIKeyCreatesLowSensitiveSystemLog(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	handler := New(config.Load(), nil, WithOperationsStore(operationsStore))
	fullKey := "sk_aaaaaaaaaaaa_" + strings.Repeat("b", 64)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+fullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid key 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one auth failure system log, got %+v", list.Items)
	}
	log := list.Items[0]
	if log.Level != operationscontract.OpsSystemLogLevelWarn || log.Source != "gateway.auth" {
		t.Fatalf("unexpected auth failure system log: %+v", log)
	}
	if log.Message != "gateway API key authentication failed" {
		t.Fatalf("unexpected auth failure message %q", log.Message)
	}
	if log.Metadata["reason"] != "invalid_api_key" ||
		log.Metadata["source_endpoint"] != "/v1/models" ||
		log.Metadata["method"] != http.MethodGet ||
		log.Metadata["attempted_key_prefix"] != "sk_aaaaaaaaaaaa" {
		t.Fatalf("unexpected auth failure metadata: %+v", log.Metadata)
	}
	if strings.Contains(log.Message, fullKey) || metadataContainsString(log.Metadata, fullKey) {
		t.Fatalf("auth failure system log leaked full key: %+v", log)
	}
}

func TestGatewayDeletedAPIKeyCreatesTombstoneSystemLogEvidence(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	handler := New(config.Load(), nil, WithOperationsStore(operationsStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"deleted-gateway","scopes":["gateway:invoke"]}`)
	apiKey := keyResp.Data.PlaintextKey
	apiKeyID := string(keyResp.Data.ApiKey.Id)
	apiKeyIDInt, err := strconv.Atoi(apiKeyID)
	if err != nil {
		t.Fatalf("parse api key id: %v", err)
	}
	userIDInt, err := strconv.Atoi(string(loginResp.Data.User.Id))
	if err != nil {
		t.Fatalf("parse user id: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+apiKeyID, nil)
	deleteReq.AddCookie(sessionCookie)
	deleteReq.Header.Set(csrfHeaderName, loginResp.Data.CsrfToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected api key delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected deleted key 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	var authLog *operationscontract.OpsSystemLog
	for i := range list.Items {
		if list.Items[i].Source == "gateway.auth" {
			authLog = &list.Items[i]
			break
		}
	}
	if authLog == nil {
		t.Fatalf("expected gateway.auth system log, got %+v", list.Items)
	}
	if authLog.Metadata["reason"] != "invalid_api_key" ||
		authLog.Metadata["attempted_key_prefix"] != keyResp.Data.ApiKey.Prefix ||
		metadataNumber(authLog.Metadata["deleted_key_id"]) != apiKeyIDInt ||
		metadataNumber(authLog.Metadata["deleted_key_owner_user_id"]) != userIDInt ||
		authLog.Metadata["deleted_key_name"] != "deleted-gateway" {
		t.Fatalf("unexpected deleted key auth metadata: %+v", authLog.Metadata)
	}
	if strings.Contains(authLog.Message, apiKey) || metadataContainsString(authLog.Metadata, apiKey) {
		t.Fatalf("deleted key auth system log leaked full key: %+v", authLog)
	}
}

func TestGatewayMissingAPIKeyDoesNotCreateSystemLogNoise(t *testing.T) {
	ctx := context.Background()
	operationsStore := operationsmemory.New()
	handler := New(config.Load(), nil, WithOperationsStore(operationsStore))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing key 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	list, err := operationsStore.ListSystemLogs(ctx, operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("missing key should not create system log noise, got %+v", list.Items)
	}
}

func TestGatewayAttemptedKeyPrefixOnlyForKeyScopedAuthFailures(t *testing.T) {
	fullKey := "sk_aaaaaaaaaaaa_" + strings.Repeat("b", 64)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "invalid", err: apikeyservice.ErrInvalidKey, want: "sk_aaaaaaaaaaaa"},
		{name: "disabled", err: apikeyservice.ErrKeyDisabled, want: "sk_aaaaaaaaaaaa"},
		{name: "expired", err: apikeyservice.ErrKeyExpired, want: "sk_aaaaaaaaaaaa"},
		{name: "ip", err: errGatewayKeyIPNotAllowed, want: "sk_aaaaaaaaaaaa"},
		{name: "risk", err: errGatewayRiskControlBlocked, want: ""},
		{name: "concurrency", err: gatewayConcurrencyLimitError{}, want: ""},
		{name: "malformed", err: apikeyservice.ErrInvalidKey, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plaintext := fullKey
			if tt.name == "malformed" {
				plaintext = "not-a-srapi-key"
			}
			if got := gatewayAttemptedKeyPrefix(plaintext, tt.err); got != tt.want {
				t.Fatalf("gatewayAttemptedKeyPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGatewayConfiguredErrorCooldownRulesMatchCanonicalAndLegacyMetadata(t *testing.T) {
	tests := []struct {
		name            string
		metadata        map[string]any
		errorClass      string
		statusCode      int
		providerMessage string
		wantReason      string
		wantWindow      time.Duration
	}{
		{
			name: "canonical status class and keyword",
			metadata: map[string]any{
				"error_cooldown_rules": []any{
					map[string]any{
						"status_code":      float64(503),
						"error_class":      "provider_5xx",
						"keywords":         []any{"CAPACITY"},
						"cooldown_seconds": float64(90),
						"reason":           "Provider Capacity",
					},
				},
			},
			errorClass:      "provider_5xx",
			statusCode:      http.StatusServiceUnavailable,
			providerMessage: "capacity unavailable",
			wantReason:      "provider_capacity",
			wantWindow:      90 * time.Second,
		},
		{
			name: "legacy temp unschedulable metadata ignored",
			metadata: map[string]any{
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(401),
						"keywords":         []any{"unauthorized"},
						"duration_minutes": float64(10),
					},
				},
			},
			errorClass:      "auth_failed",
			statusCode:      http.StatusUnauthorized,
			providerMessage: "unauthorized",
			wantReason:      "auth_failed",
			wantWindow:      authFailureCooldownWindow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, ok := gatewayCooldownDecisionForFailure(tt.metadata, tt.errorClass, ptrInt(tt.statusCode), tt.providerMessage, nil)
			if !ok {
				t.Fatalf("expected configured cooldown decision")
			}
			if decision.Reason != tt.wantReason || decision.Window != tt.wantWindow || decision.LastErrorClass != tt.errorClass {
				t.Fatalf("unexpected cooldown decision: %+v", decision)
			}
		})
	}
}

func TestGatewayAccountFailureStatusHandled(t *testing.T) {
	tests := []struct {
		name       string
		metadata   map[string]any
		statusCode *int
		want       bool
	}{
		{
			name:       "unconfigured handles all",
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       true,
		},
		{
			name: "canonical list hit",
			metadata: map[string]any{
				"handled_error_status_codes": []any{float64(401), "429"},
			},
			statusCode: ptrInt(http.StatusTooManyRequests),
			want:       true,
		},
		{
			name: "canonical list miss",
			metadata: map[string]any{
				"handled_error_status_codes": []any{float64(401), "429"},
			},
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       false,
		},
		{
			name: "unrecognized account error status alias does not filter statuses",
			metadata: map[string]any{
				"account_error_status_codes": []any{float64(429)},
			},
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       true,
		},
		{
			name: "unrecognized custom codes metadata falls back to default",
			metadata: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(403)},
			},
			statusCode: ptrInt(http.StatusForbidden),
			want:       true,
		},
		{
			name: "unrecognized custom codes metadata does not filter statuses",
			metadata: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(429)},
			},
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       true,
		},
		{
			name: "pool mode skips without handled list",
			metadata: map[string]any{
				"pool_mode": true,
			},
			statusCode: ptrInt(http.StatusUnauthorized),
			want:       false,
		},
		{
			name: "pool mode ignores unrecognized custom codes metadata",
			metadata: map[string]any{
				"pool_mode":                  true,
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(401)},
			},
			statusCode: ptrInt(http.StatusUnauthorized),
			want:       false,
		},
		{
			name: "pool mode ignores unrecognized account error status alias",
			metadata: map[string]any{
				"pool_mode":                  true,
				"account_error_status_codes": []any{float64(401)},
			},
			statusCode: ptrInt(http.StatusUnauthorized),
			want:       false,
		},
		{
			name: "empty configured list handles all",
			metadata: map[string]any{
				"handled_error_status_codes": []any{},
			},
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       true,
		},
		{
			name:       "missing status handles",
			statusCode: nil,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gatewayAccountFailureStatusHandled(tt.metadata, tt.statusCode); got != tt.want {
				t.Fatalf("gatewayAccountFailureStatusHandled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecordGatewayUsagePersistsProviderQuotaSignals(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "quota-signal-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	usage, err := usageservice.New(usagememory.New(), nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	scheduler, err := schedulerservice.New(schedulermemory.New(), nil)
	if err != nil {
		t.Fatalf("new scheduler service: %v", err)
	}
	rt := &runtimeState{
		logger:    slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts:  accounts,
		usage:     usage,
		events:    events,
		scheduler: scheduler,
	}
	resetAt := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

	rt.recordGatewayUsage(ctx, gatewayUsageRecord{
		RequestID:      "req_quota_signal",
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		DecisionID:     1,
		AttemptNo:      1,
		ProviderID:     ptrInt(account.ProviderID),
		AccountID:      ptrInt(account.ID),
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "codex-model",
		Success:        true,
		ProviderQuotaSignals: []provideradaptercontract.QuotaSignal{{
			QuotaType:      "codex_5h_percent",
			Remaining:      "66",
			Used:           "34",
			QuotaLimit:     "100",
			RemainingRatio: 0.66,
			ResetAt:        &resetAt,
			SnapshotAt:     resetAt.Add(-time.Hour),
		}},
	})

	quotas, err := accounts.ListQuotaSnapshotsByAccount(ctx, account.ID, 10)
	if err != nil {
		t.Fatalf("list quota snapshots: %v", err)
	}
	if len(quotas) != 0 {
		t.Fatalf("expected quota snapshots to be deferred to outbox, got %+v", quotas)
	}
	outboxRows, err := events.ListOutbox(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	foundRefreshEvent := false
	for _, row := range outboxRows {
		if row.EventType == "GatewayAccountSnapshotRefreshRequested" {
			foundRefreshEvent = true
			if row.AggregateID != strconv.Itoa(account.ID) || row.Payload["account_id"] == nil {
				t.Fatalf("unexpected refresh event payload: %+v", row)
			}
		}
	}
	if !foundRefreshEvent {
		t.Fatalf("expected deferred account snapshot refresh event, got %+v", outboxRows)
	}
}

// asyncWriterFixture builds a runtime wired with the services the async usage
// effects touch (usage_log is the synchronous source of truth; the scheduler
// feedback and account-snapshot-refresh effects are dispatched asynchronously),
// plus a provider account to attribute records to, with async writing armed at
// the given concurrency.
func asyncWriterFixture(t *testing.T, concurrency int) (*runtimeState, accountcontract.ProviderAccount, *eventsservice.Service) {
	t.Helper()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(context.Background(), accountcontract.CreateRequest{
		ProviderID:   7,
		Name:         "async-writer-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	usage, err := usageservice.New(usagememory.New(), nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	scheduler, err := schedulerservice.New(schedulermemory.New(), nil)
	if err != nil {
		t.Fatalf("new scheduler service: %v", err)
	}
	rt := &runtimeState{
		logger:    slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts:  accounts,
		usage:     usage,
		events:    events,
		scheduler: scheduler,
	}
	rt.startUsageWriters(concurrency)
	return rt, account, events
}

func asyncWriterRecord(account accountcontract.ProviderAccount, requestID string) gatewayUsageRecord {
	return gatewayUsageRecord{
		RequestID:      requestID,
		AttemptNo:      1,
		DecisionID:     1,
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		ProviderID:     ptrInt(account.ProviderID),
		AccountID:      ptrInt(account.ID),
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Model:          "async-model",
		Success:        true,
	}
}

func countSnapshotRefreshEvents(t *testing.T, events *eventsservice.Service) int {
	t.Helper()
	rows, err := events.ListOutbox(context.Background())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	n := 0
	for _, row := range rows {
		if row.EventType == "GatewayAccountSnapshotRefreshRequested" {
			n++
		}
	}
	return n
}

// TestAsyncUsageWriterDrains exercises the asynchronous usage-effects path
// (startUsageWriters / dispatchUsageWrite / drainUsageWriters): with a small
// concurrency bound, more records than slots are fired so some are dispatched
// async and some fall back to inline under backpressure. After draining, the
// account-snapshot-refresh effect (which runs on the async path) must have
// landed for every record — proving the drain flushes in-flight writes and the
// backpressure fallback never drops them.
func TestAsyncUsageWriterDrains(t *testing.T) {
	ctx := context.Background()
	rt, account, events := asyncWriterFixture(t, 3)
	if rt.usageSem == nil {
		t.Fatal("expected async usage writers to be armed")
	}

	const total = 50
	for i := 0; i < total; i++ {
		rt.recordGatewayUsage(ctx, asyncWriterRecord(account, "req_async_"+strconv.Itoa(i)))
	}

	rt.drainUsageWriters(ctx)

	if got := countSnapshotRefreshEvents(t, events); got != total {
		t.Fatalf("after drain expected %d async snapshot-refresh events, got %d", total, got)
	}
}

// TestUsageWriterDrainRaceSafe fires usage records from many goroutines while a
// drain runs concurrently — the scenario where a slow graceful shutdown could
// have a handler dispatch a write as the drain's WaitGroup.Wait begins. With the
// draining gate this must neither panic (WaitGroup reuse) nor lose an effect:
// writes dispatched before the drain are flushed, and writes after it run inline.
// Run under -race to catch ordering violations.
func TestUsageWriterDrainRaceSafe(t *testing.T) {
	ctx := context.Background()
	rt, account, events := asyncWriterFixture(t, 4)

	const goroutines = 8
	const perGoroutine = 25
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < perGoroutine; k++ {
				rt.recordGatewayUsage(ctx, asyncWriterRecord(account, "req_race_"+strconv.Itoa(g)+"_"+strconv.Itoa(k)))
			}
		}(g)
	}
	// Drain concurrently with the in-flight dispatchers, then once more after
	// they all return to flush anything still async.
	rt.drainUsageWriters(ctx)
	wg.Wait()
	rt.drainUsageWriters(ctx)

	if want := goroutines * perGoroutine; countSnapshotRefreshEvents(t, events) != want {
		t.Fatalf("expected %d snapshot-refresh events (no loss across concurrent drain), got %d", want, countSnapshotRefreshEvents(t, events))
	}
}

func TestProviderQuotaSignalsFromErrorClonesSignals(t *testing.T) {
	resetAt := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	wantResetAt := resetAt
	snapshotAt := resetAt.Add(-time.Minute)
	err := provideradaptercontract.ProviderError{
		Class:      "rate_limit",
		StatusCode: http.StatusTooManyRequests,
		Message:    "too many requests",
		QuotaSignals: []provideradaptercontract.QuotaSignal{{
			QuotaType:      "codex_5h_percent",
			Remaining:      "0",
			Used:           "100",
			QuotaLimit:     "100",
			RemainingRatio: 0,
			ResetAt:        &resetAt,
			SnapshotAt:     snapshotAt,
		}},
	}

	signals := providerQuotaSignalsFromError(err)
	if len(signals) != 1 {
		t.Fatalf("expected one quota signal, got %+v", signals)
	}
	if signals[0].QuotaType != "codex_5h_percent" || signals[0].ResetAt == nil || !signals[0].ResetAt.Equal(resetAt) || !signals[0].SnapshotAt.Equal(snapshotAt) {
		t.Fatalf("unexpected quota signal cloned from provider error: %+v", signals[0])
	}
	*err.QuotaSignals[0].ResetAt = resetAt.Add(time.Hour)
	if !signals[0].ResetAt.Equal(wantResetAt) {
		t.Fatalf("expected cloned reset time to be independent, got %+v", signals[0].ResetAt)
	}
}

func TestRecordGatewayProviderAttemptFailureIncludesProviderQuotaSignals(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	usage, err := usageservice.New(usagememory.New(), nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	scheduler, err := schedulerservice.New(schedulermemory.New(), nil)
	if err != nil {
		t.Fatalf("new scheduler service: %v", err)
	}
	rt := &runtimeState{
		logger:    slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts:  accounts,
		usage:     usage,
		events:    events,
		scheduler: scheduler,
	}
	server := &Server{runtime: rt}
	resetAt := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	account := accountcontract.ProviderAccount{ID: 42, ProviderID: 7}
	providerErr := provideradaptercontract.ProviderError{
		Class:      "rate_limit",
		StatusCode: http.StatusTooManyRequests,
		Message:    "too many requests",
		QuotaSignals: []provideradaptercontract.QuotaSignal{{
			QuotaType:      "codex_5h_percent",
			Remaining:      "0",
			Used:           "100",
			QuotaLimit:     "100",
			RemainingRatio: 0,
			ResetAt:        &resetAt,
			SnapshotAt:     resetAt.Add(-time.Minute),
			Metadata: map[string]any{
				"codex_primary_over_secondary_percent": 117.5,
				"codex_usage_updated_at":               resetAt.Format(time.RFC3339),
			},
		}},
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/responses", nil)
	server.recordGatewayProviderAttemptFailure(
		req,
		apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		gatewaycontract.CanonicalRequest{
			RequestID:      "req_failed_quota_signal",
			SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
			SourceEndpoint: string(gatewaycontract.EndpointResponses),
			CanonicalModel: "codex-model",
		},
		schedulercontract.ScheduleResult{
			Decision: schedulercontract.Decision{ID: 99, AttemptNo: 1},
			Candidate: schedulercontract.Candidate{
				Account: account,
				Provider: providercontract.Provider{
					ID:       7,
					Protocol: "openai-compatible",
				},
				Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
			},
		},
		providerErr,
		"rate_limit",
		http.StatusTooManyRequests,
		12,
		gatewayAdmission{},
	)

	rows, err := events.ListOutbox(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	var refreshPayload map[string]any
	for _, row := range rows {
		if row.EventType == "GatewayAccountSnapshotRefreshRequested" {
			refreshPayload = row.Payload
			break
		}
	}
	if refreshPayload == nil {
		t.Fatalf("expected account snapshot refresh outbox event, got %+v", rows)
	}
	signals := quotaSignalPayloadsForTest(refreshPayload["quota_signals"])
	if len(signals) != 1 {
		t.Fatalf("expected quota signal payload, got %+v", refreshPayload["quota_signals"])
	}
	if signals[0]["quota_type"] != "codex_5h_percent" || signals[0]["used"] != "100" || signals[0]["remaining"] != "0" {
		t.Fatalf("unexpected quota signal payload: %+v", signals[0])
	}
	metadata, ok := signals[0]["metadata"].(map[string]any)
	if !ok ||
		metadata["codex_primary_over_secondary_percent"] != 117.5 ||
		metadata["codex_usage_updated_at"] != resetAt.Format(time.RFC3339) {
		t.Fatalf("unexpected quota signal metadata payload: %+v", signals[0]["metadata"])
	}
}

// TestCodexQuotaUsageMetadataUpdates ports sub2api's buildCodexUsageExtraUpdates
// field-copy semantics: the raw Codex primary/secondary used-percent +
// reset-after-seconds and the usage-updated-at marker are pulled out of the
// per-signal Metadata so the quota windows can be persisted (and survive
// offline). Only the named keys are mirrored, present fields keep their type
// (float for percents, int for seconds, string for the timestamp), absent
// fields are skipped, and an empty result is nil.
func TestCodexQuotaUsageMetadataUpdates(t *testing.T) {
	t.Run("copies named codex fields with their types", func(t *testing.T) {
		signals := []provideradaptercontract.QuotaSignal{{
			QuotaType: "codex_5h_percent",
			Metadata: map[string]any{
				"codex_primary_used_percent":           88.5,
				"codex_primary_reset_after_seconds":    86400,
				"codex_secondary_used_percent":         12.0,
				"codex_secondary_reset_after_seconds":  3600,
				"codex_usage_updated_at":               "2026-05-28T10:00:00Z",
				"codex_primary_window_minutes":         10080, // not mirrored
				"codex_primary_over_secondary_percent": 117.5, // not mirrored
			},
		}}

		updates := codexQuotaUsageMetadataUpdates(signals)
		if updates == nil {
			t.Fatal("expected non-nil updates")
		}
		if got := updates["codex_primary_used_percent"]; got != 88.5 {
			t.Fatalf("codex_primary_used_percent = %v (%T), want 88.5 float64", got, got)
		}
		if got := updates["codex_primary_reset_after_seconds"]; got != 86400 {
			t.Fatalf("codex_primary_reset_after_seconds = %v (%T), want 86400 int", got, got)
		}
		if got := updates["codex_secondary_used_percent"]; got != 12.0 {
			t.Fatalf("codex_secondary_used_percent = %v (%T), want 12 float64", got, got)
		}
		if got := updates["codex_secondary_reset_after_seconds"]; got != 3600 {
			t.Fatalf("codex_secondary_reset_after_seconds = %v (%T), want 3600 int", got, got)
		}
		if got := updates["codex_usage_updated_at"]; got != "2026-05-28T10:00:00Z" {
			t.Fatalf("codex_usage_updated_at = %v, want %s", got, "2026-05-28T10:00:00Z")
		}
		if _, ok := updates["codex_primary_window_minutes"]; ok {
			t.Fatalf("did not expect non-mirrored key codex_primary_window_minutes: %v", updates)
		}
		if _, ok := updates["codex_primary_over_secondary_percent"]; ok {
			t.Fatalf("did not expect non-mirrored key codex_primary_over_secondary_percent: %v", updates)
		}
	})

	t.Run("skips absent fields and signals without metadata", func(t *testing.T) {
		signals := []provideradaptercontract.QuotaSignal{
			{QuotaType: "codex_7d_percent"}, // no metadata
			{
				QuotaType: "codex_5h_percent",
				Metadata: map[string]any{
					"codex_primary_used_percent": 5.0,
					"codex_usage_updated_at":     "2026-05-28T11:00:00Z",
				},
			},
		}

		updates := codexQuotaUsageMetadataUpdates(signals)
		if len(updates) != 2 {
			t.Fatalf("expected exactly the two present fields, got %+v", updates)
		}
		if updates["codex_primary_used_percent"] != 5.0 || updates["codex_usage_updated_at"] != "2026-05-28T11:00:00Z" {
			t.Fatalf("unexpected updates: %+v", updates)
		}
		if _, ok := updates["codex_primary_reset_after_seconds"]; ok {
			t.Fatalf("did not expect absent reset-after seconds: %+v", updates)
		}
	})

	t.Run("no codex metadata yields nil", func(t *testing.T) {
		if got := codexQuotaUsageMetadataUpdates(nil); got != nil {
			t.Fatalf("expected nil for no signals, got %+v", got)
		}
		signals := []provideradaptercontract.QuotaSignal{{
			QuotaType: "codex_5h_percent",
			Metadata:  map[string]any{"unrelated": "x"},
		}}
		if got := codexQuotaUsageMetadataUpdates(signals); got != nil {
			t.Fatalf("expected nil when no mirrored fields present, got %+v", got)
		}
	})
}

// TestRecordProviderQuotaSignalsPersistsCodexUsageMetadata proves the quota
// signals' Codex usage fields are merged onto account.Metadata (so they survive
// offline) without clobbering unrelated metadata such as a live cooldown field.
func TestRecordProviderQuotaSignalsPersistsCodexUsageMetadata(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "codex-usage-meta-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
		Metadata: map[string]any{
			"cooldown_active": true,
			"cooldown_reason": "rate_limit",
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	rt := &runtimeState{
		logger:   slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts: accounts,
	}
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	resetAt := now.Add(time.Hour)
	signals := []provideradaptercontract.QuotaSignal{{
		QuotaType:      "codex_5h_percent",
		Remaining:      "66",
		Used:           "34",
		QuotaLimit:     "100",
		RemainingRatio: 0.66,
		ResetAt:        &resetAt,
		SnapshotAt:     now,
		Metadata: map[string]any{
			"codex_primary_used_percent":          88.0,
			"codex_primary_reset_after_seconds":   86400,
			"codex_secondary_used_percent":        34.0,
			"codex_secondary_reset_after_seconds": 3600,
			"codex_usage_updated_at":              now.Format(time.RFC3339),
		},
	}}

	rt.recordProviderQuotaSignals(ctx, account, signals, now)

	updated, err := accounts.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if metadataFloatValue(updated.Metadata, "codex_primary_used_percent") != 88.0 ||
		metadataInt(updated.Metadata, "codex_primary_reset_after_seconds") != 86400 ||
		metadataFloatValue(updated.Metadata, "codex_secondary_used_percent") != 34.0 ||
		metadataInt(updated.Metadata, "codex_secondary_reset_after_seconds") != 3600 ||
		metadataString(updated.Metadata, "codex_usage_updated_at") != now.Format(time.RFC3339) {
		t.Fatalf("expected codex usage metadata persisted, got %+v", updated.Metadata)
	}
	// The merge must not clobber unrelated metadata.
	if !metadataBool(updated.Metadata, "cooldown_active") || metadataString(updated.Metadata, "cooldown_reason") != "rate_limit" {
		t.Fatalf("expected pre-existing cooldown metadata preserved, got %+v", updated.Metadata)
	}
}

func quotaSignalPayloadsForTest(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				out = append(out, mapped)
			}
		}
		return out
	default:
		return nil
	}
}

func TestRecordGatewayAccountSnapshotsUsesBoundedAccountWindow(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "bounded-snapshot-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
		Metadata: map[string]any{
			"runtime_quota_window_seconds": 60,
			"cost_window_seconds":          60,
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	otherAccount, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "other-snapshot-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
	})
	if err != nil {
		t.Fatalf("create other account: %v", err)
	}
	usageStore := usagememory.New()
	now := time.Now().UTC()
	for _, log := range []struct {
		id        string
		accountID int
		tokens    int
		at        time.Time
	}{
		{id: "old", accountID: account.ID, tokens: 1000, at: now.Add(-10 * time.Minute)},
		{id: "inside", accountID: account.ID, tokens: 25, at: now.Add(-10 * time.Second)},
		{id: "other", accountID: otherAccount.ID, tokens: 5000, at: now.Add(-5 * time.Second)},
	} {
		accountID := log.accountID
		if _, err := usageStore.Create(ctx, usagecontract.UsageLog{
			RequestID:      log.id,
			UserID:         1,
			APIKeyID:       2,
			AccountID:      &accountID,
			ProviderID:     ptrInt(12),
			SourceEndpoint: "/v1/responses",
			Model:          "codex-model",
			Success:        true,
			TotalTokens:    log.tokens,
			BillableCost:   "0.01000000",
			CreatedAt:      log.at,
		}); err != nil {
			t.Fatalf("seed usage %s: %v", log.id, err)
		}
	}
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	rt := &runtimeState{
		logger:   slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts: accounts,
		usage:    usage,
	}
	rt.recordGatewayAccountSnapshots(ctx, gatewayUsageRecord{
		RequestID:      "req_bounded_snapshot",
		ProviderID:     ptrInt(account.ProviderID),
		AccountID:      ptrInt(account.ID),
		SourceEndpoint: "/v1/responses",
		Model:          "codex-model",
		Success:        true,
	})

	updated, err := accounts.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if metadataInt(updated.Metadata, "tpm_used") != 25 || metadataInt(updated.Metadata, "rpm_used") != 1 {
		t.Fatalf("expected bounded account window usage, got metadata %+v", updated.Metadata)
	}
	quotas, err := accounts.ListQuotaSnapshotsByAccount(ctx, account.ID, 10)
	if err != nil {
		t.Fatalf("list quota snapshots: %v", err)
	}
	var codexQuota *accountcontract.AccountQuotaSnapshot
	for i := range quotas {
		if quotas[i].QuotaType == accountcontract.QuotaTypeSyntheticMonthlyTokens {
			codexQuota = &quotas[i]
			break
		}
	}
	if codexQuota == nil {
		t.Fatalf("expected synthetic quota snapshot, got %+v", quotas)
	}
	if codexQuota.Used != "25" {
		t.Fatalf("expected synthetic quota to use bounded account window only, got %+v", *codexQuota)
	}
}

func TestAccountSchedulerRuntimeStateIgnoresSyntheticQuotaSnapshots(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "real-quota-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	if _, err := accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "codex_5h_percent",
		Remaining:      "42",
		Used:           "58",
		QuotaLimit:     "100",
		RemainingRatio: 0.42,
		SnapshotAt:     now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record real quota: %v", err)
	}
	if _, err := accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "codex_7d_percent",
		Remaining:      "75",
		Used:           "25",
		QuotaLimit:     "100",
		RemainingRatio: 0.75,
		SnapshotAt:     now.Add(-30 * time.Second),
	}); err != nil {
		t.Fatalf("record second real quota: %v", err)
	}
	if _, err := accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      accountcontract.QuotaTypeSyntheticMonthlyTokens,
		Remaining:      "unlimited",
		Used:           "1",
		QuotaLimit:     "unlimited",
		RemainingRatio: 1,
		SnapshotAt:     now,
	}); err != nil {
		t.Fatalf("record synthetic quota: %v", err)
	}
	rt := &runtimeState{accounts: accounts}

	candidates := []schedulercontract.Candidate{{Account: account}}
	rt.fillCandidateRuntimeStates(ctx, candidates)
	state := candidates[0].RuntimeState
	if state.QuotaRemainingRatio == nil || math.Abs(*state.QuotaRemainingRatio-0.42) > 0.000001 {
		t.Fatalf("expected scheduler to use real quota ratio 0.42, got %+v", state.QuotaRemainingRatio)
	}
	if state.QuotaExhausted {
		t.Fatalf("did not expect real quota to be exhausted: %+v", state)
	}
}

func TestAccountSchedulerRuntimeStateSkipsResetQuotaSnapshots(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "reset-quota-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	now := time.Now().UTC()
	resetAt := now.Add(-time.Minute)
	if _, err := accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "codex_5h_percent",
		Remaining:      "0",
		Used:           "100",
		QuotaLimit:     "100",
		RemainingRatio: 0,
		ResetAt:        &resetAt,
		SnapshotAt:     now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("record reset quota: %v", err)
	}
	rt := &runtimeState{accounts: accounts}

	candidates := []schedulercontract.Candidate{{Account: account}}
	rt.fillCandidateRuntimeStates(ctx, candidates)
	state := candidates[0].RuntimeState
	if state.QuotaExhausted || state.QuotaRemainingRatio != nil {
		t.Fatalf("expected reset quota snapshot to be ignored, got %+v", state)
	}
}

func TestAccountSchedulerRuntimeStateAutoPausesByQuotaThreshold(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   13,
		Name:         "auto-pause-quota-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
		Metadata: map[string]any{
			"auto_pause_5h_threshold": 0.90,
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	now := time.Now().UTC()
	resetAt := now.Add(time.Hour)
	if _, err := accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "codex_5h_percent",
		Remaining:      "5",
		Used:           "95",
		QuotaLimit:     "100",
		RemainingRatio: 0.05,
		ResetAt:        &resetAt,
		SnapshotAt:     now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record quota: %v", err)
	}
	rt := &runtimeState{accounts: accounts}

	candidates := []schedulercontract.Candidate{{Account: account}}
	rt.fillCandidateRuntimeStates(ctx, candidates)
	state := candidates[0].RuntimeState
	if !state.QuotaAutoPaused {
		t.Fatalf("expected quota auto pause, got %+v", state)
	}
	if state.QuotaExhausted {
		t.Fatalf("auto pause should not rewrite real exhaustion state, got %+v", state)
	}
}

func TestQuotaAutoPauseSkipsDisabledResetAndStaleSnapshots(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	futureReset := now.Add(time.Hour)
	pastReset := now.Add(-time.Minute)
	base := accountcontract.AccountQuotaSnapshot{
		AccountID:      1,
		ProviderID:     2,
		QuotaType:      "codex_5h_percent",
		RemainingRatio: 0.01,
		ResetAt:        &futureReset,
		SnapshotAt:     now.Add(-time.Minute),
	}

	if quotaAutoPausedByMetadata(map[string]any{"auto_pause_5h_threshold": 0.95, "auto_pause_5h_disabled": true}, []accountcontract.AccountQuotaSnapshot{base}, now) {
		t.Fatal("disabled 5h auto-pause should not pause")
	}

	resetSnapshot := base
	resetSnapshot.ResetAt = &pastReset
	if quotaAutoPausedByMetadata(map[string]any{"auto_pause_5h_threshold": 0.95}, []accountcontract.AccountQuotaSnapshot{resetSnapshot}, now) {
		t.Fatal("reset quota window should not pause")
	}

	staleSnapshot := base
	staleSnapshot.SnapshotAt = now.Add(-3 * time.Hour)
	if quotaAutoPausedByMetadata(map[string]any{"auto_pause_5h_threshold": 0.95}, []accountcontract.AccountQuotaSnapshot{staleSnapshot}, now) {
		t.Fatal("stale quota snapshot should not pause")
	}
}

func TestRecordGatewayUsageAppliesAccountGroupRateMultiplier(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   21,
		Name:         "discount-account",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	rateMultiplier := "0.80000000"
	group, err := accounts.CreateGroup(ctx, accountcontract.CreateGroupRequest{
		Name:           "discount-group",
		RateMultiplier: &rateMultiplier,
	})
	if err != nil {
		t.Fatalf("create account group: %v", err)
	}
	if _, err := accounts.AddAccountToGroup(ctx, account.ID, group.ID); err != nil {
		t.Fatalf("add account to group: %v", err)
	}
	usageStore := usagememory.New()
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger:   slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts: accounts,
		usage:    usage,
		events:   events,
	}

	rt.recordGatewayUsage(ctx, gatewayUsageRecord{
		RequestID:      "req_group_multiplier",
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		ProviderID:     ptrInt(account.ProviderID),
		AccountID:      ptrInt(account.ID),
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Model:          "claude-opus-test",
		Success:        true,
		Pricing:        gatewayPricingEvidence{Amount: "0.01000000", Currency: "USD", PricingSource: "pricing_rule"},
	})

	logs, err := usageStore.List(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one usage log, got %+v", logs)
	}
	log := logs[0]
	if log.Cost != "0.01000000" || log.ActualCost != "0.00800000" || log.BillableCost != "0.00800000" || log.RateMultiplier != "0.80000000" {
		t.Fatalf("expected multiplier snapshot and actual cost, got %+v", log)
	}
}

func TestRecordGatewayUsagePersistsCostBreakdownAndModelSnapshots(t *testing.T) {
	ctx := context.Background()
	usageStore := usagememory.New()
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		usage:  usage,
		events: events,
	}

	rt.recordGatewayUsage(ctx, gatewayUsageRecord{
		RequestID:      "req_breakdown_snapshot",
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		SourceEndpoint: "/v1/images/generations",
		Model:          "canonical-image-model",
		RequestedModel: "public-image-alias",
		UpstreamModel:  "upstream-image-model",
		Success:        true,
		Pricing: gatewayPricingEvidence{
			Amount:         "0.12000000",
			Currency:       "USD",
			BillingMode:    "image",
			InputCost:      "0.01000000",
			OutputCost:     "0.10000000",
			CacheReadCost:  "0.00500000",
			CacheWriteCost: "0.00500000",
			PricingSource:  "pricing_rule",
		},
	})

	logs, err := usageStore.List(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one usage log, got %+v", logs)
	}
	log := logs[0]
	sum := addUsageCostBreakdown(log.InputCost, log.OutputCost, log.CacheReadCost, log.CacheWriteCost)
	if sum != log.Cost {
		t.Fatalf("expected breakdown sum %s to equal total %s in %+v", sum, log.Cost, log)
	}
	if log.RequestedModel != "public-image-alias" || log.UpstreamModel != "upstream-image-model" || log.BillingMode != "image" {
		t.Fatalf("expected model and billing snapshots, got %+v", log)
	}
}

func TestRecordGatewayUsageMaterializedCostMatchesUsageLogBillableSum(t *testing.T) {
	ctx := context.Background()
	usageStore := usagememory.New()
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	billing, err := billingservice.New(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing service: %v", err)
	}
	subscriptions, err := subscriptionservice.New(subscriptionmemory.New(), nil)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	apiKeys, err := apikeyservice.New(apikeymemory.New(), strings.Repeat("k", 32), nil)
	if err != nil {
		t.Fatalf("new api key service: %v", err)
	}
	plan, err := subscriptions.CreatePlan(ctx, subscriptioncontract.CreatePlanRequest{
		Name:         "materialized-pro",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{"monthly_cost_quota": "1.00000000"},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := subscriptions.CreateUserSubscription(ctx, subscriptioncontract.CreateSubscriptionRequest{
		UserID: 1,
		PlanID: plan.ID,
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	createdKey, err := apiKeys.Create(ctx, apikeycontract.CreateRequest{UserID: 1, Name: "materialized-key"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	rt := &runtimeState{
		logger:        slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		usage:         usage,
		events:        events,
		billing:       billing,
		subscriptions: subscriptions,
		apiKeys:       apiKeys,
	}
	for i, amount := range []string{"0.01000000", "0.02500000", "0.00500000"} {
		rt.recordGatewayUsage(ctx, gatewayUsageRecord{
			RequestID:      "req_materialized_" + strconv.Itoa(i+1),
			Authed:         apikeycontract.AuthResult{UserID: 1, Key: createdKey.Key},
			SourceEndpoint: "/v1/chat/completions",
			TargetProtocol: "openai-compatible",
			Model:          "materialized-model",
			Success:        true,
			Pricing:        gatewayPricingEvidence{Amount: amount, Currency: "USD", PricingSource: "pricing_rule"},
		})
	}

	logs, err := usageStore.List(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	sum := "0.00000000"
	for _, log := range logs {
		sum = money.AddMoney(sum, log.BillableCost)
	}
	if sum != "0.04000000" {
		t.Fatalf("unexpected billable cost sum %s from logs %+v", sum, logs)
	}
	materialized, err := subscriptions.MaterializedUsageForUser(ctx, 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("read materialized usage: %v", err)
	}
	if materialized.MonthlyUsageUSD != sum || materialized.DailyUsageUSD != sum || materialized.WeeklyUsageUSD != sum {
		t.Fatalf("expected materialized usage to match log sum %s, got %+v", sum, materialized)
	}
	key, err := apiKeys.GetByID(ctx, createdKey.Key.ID)
	if err != nil {
		t.Fatalf("find api key: %v", err)
	}
	if key.CostUsed != sum || key.CostUsed5h != sum || key.CostUsed1d != sum || key.CostUsed7d != sum {
		t.Fatalf("expected api key materialized cost to match log sum %s, got %+v", sum, key)
	}
}

func addUsageCostBreakdown(input, output, cacheRead, cacheWrite string) string {
	sum := money.AddMoney(input, output)
	sum = money.AddMoney(sum, cacheRead)
	sum = money.AddMoney(sum, cacheWrite)
	return sum
}

// TestGatewayErrorClassUsesCooldownNetworkError pins the sub2api transport-error
// parity: a transport/network failure (dead proxy / DNS-failed account) must take
// a SHORT account cooldown instead of being immediately reschedulable and hammered
// in a tight loop, while unrelated classes (e.g. a client-side invalid request)
// stay uncooled.
func TestGatewayErrorClassUsesCooldownNetworkError(t *testing.T) {
	if !gatewayErrorClassUsesCooldown("network_error") {
		t.Fatalf("expected gatewayErrorClassUsesCooldown(\"network_error\") == true")
	}
	if gatewayErrorClassUsesCooldown("invalid_request") {
		t.Fatalf("expected gatewayErrorClassUsesCooldown(\"invalid_request\") == false")
	}

	// network_error must derive the SHORT transport cooldown, not the long
	// auth/overload window.
	if got := gatewayCooldownWindow("network_error"); got != networkErrorCooldownWindow {
		t.Fatalf("expected network_error cooldown window %s, got %s", networkErrorCooldownWindow, got)
	}
	if networkErrorCooldownWindow >= authFailureCooldownWindow || networkErrorCooldownWindow >= overloadCooldownWindow {
		t.Fatalf("expected network_error cooldown (%s) to be shorter than auth (%s) and overload (%s) windows",
			networkErrorCooldownWindow, authFailureCooldownWindow, overloadCooldownWindow)
	}
	if networkErrorCooldownWindow <= 0 || networkErrorCooldownWindow > 15*time.Minute {
		t.Fatalf("expected network_error cooldown to be a few minutes, got %s", networkErrorCooldownWindow)
	}

	// End-to-end: a network_error failure with no configured rule yields a
	// cooldown decision carrying the short window.
	decision, ok := gatewayCooldownDecisionForFailure(nil, "network_error", nil, "dial tcp: lookup proxy.example: no such host", nil)
	if !ok {
		t.Fatalf("expected network_error to produce a cooldown decision")
	}
	if decision.Reason != "network_error" || decision.LastErrorClass != "network_error" {
		t.Fatalf("unexpected network_error cooldown decision: %+v", decision)
	}
	if decision.Window != networkErrorCooldownWindow {
		t.Fatalf("expected network_error decision window %s, got %s", networkErrorCooldownWindow, decision.Window)
	}

	// An unrelated class still yields no cooldown decision.
	if _, ok := gatewayCooldownDecisionForFailure(nil, "invalid_request", nil, "", nil); ok {
		t.Fatalf("expected invalid_request to produce no cooldown decision")
	}
}

type failingUsageStore struct {
	err error
}

func (s failingUsageStore) Create(context.Context, usagecontract.UsageLog) (usagecontract.UsageLog, error) {
	return usagecontract.UsageLog{}, s.err
}

func (s failingUsageStore) List(context.Context) ([]usagecontract.UsageLog, error) {
	return nil, s.err
}

func (s failingUsageStore) ListByUser(context.Context, int) ([]usagecontract.UsageLog, error) {
	return nil, s.err
}

func (s failingUsageStore) ListByAccountWindow(context.Context, usagecontract.AccountWindowFilter) ([]usagecontract.UsageLog, error) {
	return nil, s.err
}

func (s failingUsageStore) SummarizeUserWindow(context.Context, usagecontract.UserWindowFilter) (usagecontract.UserWindowSummary, error) {
	return usagecontract.UserWindowSummary{}, s.err
}

func (s failingUsageStore) CleanupLogs(context.Context, usagecontract.CleanupFilter) (usagecontract.CleanupResult, error) {
	return usagecontract.CleanupResult{}, s.err
}

type failingUsageAggregator struct {
	err error
}

func (a failingUsageAggregator) ApplyAggregation(context.Context, int) (bool, error) {
	return false, a.err
}

type failingSchedulerFeedbackStore struct {
	*schedulermemory.Store
	err error
}

func (s failingSchedulerFeedbackStore) CreateFeedback(context.Context, schedulercontract.Feedback) (schedulercontract.Feedback, error) {
	return schedulercontract.Feedback{}, s.err
}

type failingEventsStore struct {
	*eventsmemory.Store
	err error
}

func (s failingEventsStore) CreateOutbox(context.Context, eventscontract.OutboxEvent) (eventscontract.OutboxEvent, error) {
	return eventscontract.OutboxEvent{}, s.err
}

func metadataNumber(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func metadataContainsString(value any, needle string) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, needle)
	case map[string]any:
		for _, item := range v {
			if metadataContainsString(item, needle) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if metadataContainsString(item, needle) {
				return true
			}
		}
	}
	return false
}

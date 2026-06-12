package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/metacoerce"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

func (h domainEventHandler) refreshGatewayAccountSnapshot(ctx context.Context, event eventscontract.OutboxEvent) error {
	if h.accounts == nil || h.usage == nil {
		return nil
	}
	accountID := payloadInt(event.Payload, "account_id")
	providerID := payloadInt(event.Payload, "provider_id")
	if accountID <= 0 || providerID <= 0 {
		return nil
	}
	account, err := h.accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	logs, err := h.accountSnapshotUsageLogs(ctx, account, now)
	if err != nil {
		return err
	}
	if err := h.updateAccountRuntimeQuotaMetadata(ctx, account, logs, now); err != nil {
		return err
	}
	signals := gatewayQuotaSignalsFromPayload(event.Payload["quota_signals"])
	if len(signals) > 0 {
		updated, err := h.accounts.FindByID(ctx, accountID)
		if err != nil {
			return err
		}
		account = updated
		if _, err := h.accounts.ApplyQuotaReport(ctx, account, provideradaptercontract.QuotaReport{
			Supported:    true,
			Source:       "headers",
			QuotaSignals: signals,
			FetchedAt:    now,
		}); err != nil {
			return err
		}
	}
	if _, err := h.accounts.RecordHealthSnapshot(ctx, buildAccountHealthSnapshot(account, logs, now)); err != nil {
		return err
	}
	_, err = h.accounts.RecordQuotaSnapshot(ctx, buildAccountQuotaSnapshot(account, logs, now))
	return err
}

func (h domainEventHandler) accountSnapshotUsageLogs(ctx context.Context, account accountcontract.ProviderAccount, now time.Time) ([]usagecontract.UsageLog, error) {
	window := accountRuntimeQuotaWindow(account.Metadata)
	if costWindow := accountCostWindow(account.Metadata); costWindow > window {
		window = costWindow
	}
	return h.usage.ListByAccountWindow(ctx, usagecontract.AccountWindowFilter{
		AccountID: account.ID,
		Start:     now.UTC().Add(-window),
		End:       now.UTC().Add(time.Nanosecond),
		Limit:     accountSnapshotUsageLogLimit(account.Metadata),
	})
}

func (h domainEventHandler) updateAccountRuntimeQuotaMetadata(ctx context.Context, account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) error {
	window := accountRuntimeQuotaWindow(account.Metadata)
	windowStart := now.Add(-window)
	rpmUsed := 0
	tpmUsed := 0
	for _, log := range logs {
		if log.CreatedAt.Before(windowStart) {
			continue
		}
		rpmUsed++
		tpmUsed += log.TotalTokens
	}
	costWindow := accountCostWindow(account.Metadata)
	costWindowStart := now.Add(-costWindow)
	costUsed := new(big.Rat)
	for _, log := range logs {
		if log.CreatedAt.Before(costWindowStart) {
			continue
		}
		amount := strings.TrimSpace(log.BillableCost)
		if amount == "" {
			amount = strings.TrimSpace(log.Cost)
		}
		if parsed, ok := money.DecimalRat(amount); ok {
			costUsed.Add(costUsed, parsed)
		}
	}
	metadata := cloneMetadata(account.Metadata)
	windowSeconds := int(window / time.Second)
	resetAt := now.Add(window).Format(time.RFC3339)
	metadata["rpm_used"] = rpmUsed
	metadata["tpm_used"] = tpmUsed
	metadata["rpm_window_seconds"] = windowSeconds
	metadata["tpm_window_seconds"] = windowSeconds
	metadata["rpm_reset_at"] = resetAt
	metadata["tpm_reset_at"] = resetAt
	metadata["cost_window_used"] = money.FormatRatFixed(costUsed, 8)
	metadata["cost_window_seconds"] = int(costWindow / time.Second)
	metadata["cost_window_reset_at"] = now.Add(costWindow).Format(time.RFC3339)
	metadata["runtime_quota_updated_at"] = now.Format(time.RFC3339)
	_, err := h.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Metadata: &metadata})
	return err
}

func buildAccountHealthSnapshot(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) accountcontract.AccountHealthSnapshot {
	successRate := usageSuccessRate(logs)
	return accountcontract.AccountHealthSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		Status:         accountHealthStatus(account, logs),
		SuccessRate:    successRate,
		ErrorRate:      1 - successRate,
		LatencyP50MS:   latencyPercentile(logs, 50),
		LatencyP95MS:   latencyPercentile(logs, 95),
		RateLimitCount: errorClassCount(logs, "rate_limit"),
		TimeoutCount:   errorClassCount(logs, "timeout"),
		CooldownUntil:  metadataOptionalTime(account.Metadata, "cooldown_until"),
		CircuitState:   accountCircuitState(account),
		SnapshotAt:     now,
	}
}

func buildAccountQuotaSnapshot(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) accountcontract.AccountQuotaSnapshot {
	usedTokens := 0
	for _, log := range logs {
		usedTokens += log.TotalTokens
	}
	return accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      accountcontract.QuotaTypeSyntheticMonthlyTokens,
		Remaining:      "unlimited",
		Used:           strconv.Itoa(usedTokens),
		QuotaLimit:     "unlimited",
		RemainingRatio: 1,
		SnapshotAt:     now,
	}
}

func gatewayQuotaSignalsFromPayload(value any) []provideradaptercontract.QuotaSignal {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]provideradaptercontract.QuotaSignal, 0, len(items))
	for _, item := range items {
		mapped, ok := item.(map[string]any)
		if !ok {
			continue
		}
		signal := provideradaptercontract.QuotaSignal{
			QuotaType:      payloadMapString(mapped, "quota_type"),
			Remaining:      payloadMapString(mapped, "remaining"),
			Used:           payloadMapString(mapped, "used"),
			QuotaLimit:     payloadMapString(mapped, "quota_limit"),
			RemainingRatio: float32(payloadMapFloat(mapped, "remaining_ratio")),
			ResetAt:        payloadMapTimePtr(mapped, "reset_at"),
			SnapshotAt:     payloadMapTime(mapped, "snapshot_at"),
		}
		if strings.TrimSpace(signal.QuotaType) != "" {
			out = append(out, signal)
		}
	}
	return out
}

func accountRuntimeQuotaWindow(metadata map[string]any) time.Duration {
	seconds := metadataInt(metadata, "runtime_quota_window_seconds", "quota_window_seconds", "rpm_window_seconds", "tpm_window_seconds", "window_seconds")
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func accountCostWindow(metadata map[string]any) time.Duration {
	seconds := metadataInt(metadata, "cost_window_seconds")
	if seconds <= 0 {
		seconds = 5 * 60 * 60
	}
	return time.Duration(seconds) * time.Second
}

func accountSnapshotUsageLogLimit(metadata map[string]any) int {
	limit := metadataInt(metadata, "runtime_snapshot_usage_limit")
	if limit <= 0 {
		return 5000
	}
	return limit
}

func usageSuccessRate(logs []usagecontract.UsageLog) float32 {
	if len(logs) == 0 {
		return 1
	}
	success := 0
	for _, log := range logs {
		if log.Success {
			success++
		}
	}
	return float32(success) / float32(len(logs))
}

func latencyPercentile(logs []usagecontract.UsageLog, percentile int) int {
	if len(logs) == 0 {
		return 0
	}
	values := make([]int, 0, len(logs))
	for _, log := range logs {
		values = append(values, log.LatencyMS)
	}
	sort.Ints(values)
	index := (len(values)*percentile + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > len(values) {
		index = len(values)
	}
	return values[index-1]
}

func errorClassCount(logs []usagecontract.UsageLog, errorClass string) int {
	count := 0
	for _, log := range logs {
		if log.ErrorClass != nil && *log.ErrorClass == errorClass {
			count++
		}
	}
	return count
}

func accountHealthStatus(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog) string {
	if account.Status != accountcontract.StatusActive {
		return string(account.Status)
	}
	if len(logs) > 0 && usageSuccessRate(logs) < 0.5 {
		return "degraded"
	}
	return "healthy"
}

func accountCircuitState(account accountcontract.ProviderAccount) string {
	if account.Status == accountcontract.StatusActive {
		return "closed"
	}
	return "open"
}

func metadataOptionalTime(metadata map[string]any, key string) *time.Time {
	value := payloadMapString(metadata, key)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func metadataInt(metadata map[string]any, keys ...string) int {
	value, ok := metacoerce.Value(metadata, keys...)
	if !ok {
		return 0
	}
	parsed, ok := metacoerce.Int(value)
	if !ok {
		return 0
	}
	return parsed
}

func cloneMetadata(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}

func payloadMapString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func payloadMapFloat(payload map[string]any, key string) float64 {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
		return parsed
	}
}

func payloadMapTime(payload map[string]any, key string) time.Time {
	value := payloadMapString(payload, key)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC()
	}
	parsed, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func payloadMapTimePtr(payload map[string]any, key string) *time.Time {
	parsed := payloadMapTime(payload, key)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

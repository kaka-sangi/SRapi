package httpserver

import (
	"math"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func gatewayUsageResponse(key apikeycontract.APIKey, user userscontract.User, summary usagecontract.APIKeyUsageSummary) apiopenapi.GatewayUsageResponse {
	planName := "wallet_balance"
	resp := apiopenapi.GatewayUsageResponse{
		ApiKeyId:       apiopenapi.Id(strconv.Itoa(key.ID)),
		ApiKeyName:     key.Name,
		Balance:        user.Balance,
		DailyUsage:     gatewayUsageDaily(summary.DailyUsage),
		GeneratedAt:    summary.GeneratedAt,
		IsValid:        key.Status == apikeycontract.StatusActive,
		Mode:           gatewayUsageMode(key),
		ModelStats:     gatewayUsageModels(summary.ModelStats),
		Object:         apiopenapi.Usage,
		PlanName:       &planName,
		RecentRequests: gatewayUsageLogs(summary.RecentLogs),
		Remaining:      user.Balance,
		Status:         string(key.Status),
		Today:          gatewayUsageWindow(summary.Today),
		Unit:           normalizeGatewayUsageCurrency(user.Currency),
		Usage:          gatewayUsageTotals(summary),
		WindowDays:     summary.WindowDays,
	}
	if limits := gatewayUsageLimits(key); len(limits) > 0 {
		resp.Limits = &limits
	}
	if len(key.AllowedModels) > 0 {
		resp.AllowedModels = ptrStringSlice(append([]string(nil), key.AllowedModels...))
	}
	if key.ExpiresAt != nil {
		resp.ExpiresAt = cloneTimePtr(key.ExpiresAt)
		resp.DaysUntilExpiry = ptrInt(daysUntil(*key.ExpiresAt, summary.GeneratedAt))
	}
	return resp
}

func gatewayUsageMode(key apikeycontract.APIKey) apiopenapi.GatewayUsageResponseMode {
	if key.RPMLimit != nil || key.TPMLimit != nil || key.ConcurrencyLimit != nil || len(key.AllowedModels) > 0 {
		return apiopenapi.QuotaLimited
	}
	return apiopenapi.Unrestricted
}

func gatewayUsageTotals(summary usagecontract.APIKeyUsageSummary) apiopenapi.GatewayUsageTotals {
	return apiopenapi.GatewayUsageTotals{
		CachedTokens: summary.CachedTokens,
		Cost:         summary.TotalCost,
		Currency:     normalizeGatewayUsageCurrency(summary.Currency),
		ErrorCount:   summary.ErrorCount,
		InputTokens:  summary.InputTokens,
		OutputTokens: summary.OutputTokens,
		Requests:     summary.RequestCount,
		SuccessCount: summary.SuccessCount,
		TotalTokens:  summary.TotalTokens,
	}
}

func gatewayUsageWindow(summary usagecontract.UsageWindowSummary) apiopenapi.GatewayUsageWindow {
	return apiopenapi.GatewayUsageWindow{
		CachedTokens: summary.CachedTokens,
		Cost:         summary.TotalCost,
		Currency:     normalizeGatewayUsageCurrency(summary.Currency),
		Date:         gatewayUsageDate(summary.Date),
		ErrorCount:   summary.ErrorCount,
		InputTokens:  summary.InputTokens,
		OutputTokens: summary.OutputTokens,
		Requests:     summary.RequestCount,
		SuccessCount: summary.SuccessCount,
		TotalTokens:  summary.TotalTokens,
	}
}

func gatewayUsageDaily(values []usagecontract.UsageDailySummary) []apiopenapi.GatewayUsageWindow {
	out := make([]apiopenapi.GatewayUsageWindow, 0, len(values))
	for _, item := range values {
		out = append(out, gatewayUsageWindow(usagecontract.UsageWindowSummary(item)))
	}
	return out
}

func gatewayUsageModels(values []usagecontract.UsageModelSummary) []apiopenapi.GatewayUsageModel {
	out := make([]apiopenapi.GatewayUsageModel, 0, len(values))
	for _, item := range values {
		out = append(out, apiopenapi.GatewayUsageModel{
			CachedTokens: item.CachedTokens,
			Cost:         item.TotalCost,
			Currency:     normalizeGatewayUsageCurrency(item.Currency),
			ErrorCount:   item.ErrorCount,
			InputTokens:  item.InputTokens,
			Model:        item.Model,
			OutputTokens: item.OutputTokens,
			Requests:     item.RequestCount,
			SuccessCount: item.SuccessCount,
			TotalTokens:  item.TotalTokens,
		})
	}
	return out
}

func gatewayUsageLogs(logs []usagecontract.UsageLog) []apiopenapi.GatewayUsageRequest {
	out := make([]apiopenapi.GatewayUsageRequest, 0, len(logs))
	for _, log := range logs {
		out = append(out, apiopenapi.GatewayUsageRequest{
			AttemptNo:      log.AttemptNo,
			CachedTokens:   log.CachedTokens,
			Cost:           log.Cost,
			CreatedAt:      log.CreatedAt,
			Currency:       normalizeGatewayUsageCurrency(log.Currency),
			ErrorClass:     log.ErrorClass,
			InputTokens:    log.InputTokens,
			LatencyMs:      log.LatencyMS,
			Model:          log.Model,
			OutputTokens:   log.OutputTokens,
			RequestId:      apiopenapi.RequestId(log.RequestID),
			SourceEndpoint: log.SourceEndpoint,
			SourceProtocol: log.SourceProtocol,
			Success:        log.Success,
			TargetProtocol: optionalString(log.TargetProtocol),
			TotalTokens:    log.TotalTokens,
			UsageEstimated: log.UsageEstimated,
		})
	}
	return out
}

func gatewayUsageLimits(key apikeycontract.APIKey) map[string]interface{} {
	limits := map[string]interface{}{}
	if key.RPMLimit != nil {
		limits["rpm"] = *key.RPMLimit
	}
	if key.TPMLimit != nil {
		limits["tpm"] = *key.TPMLimit
	}
	if key.ConcurrencyLimit != nil {
		limits["concurrency"] = *key.ConcurrencyLimit
	}
	return limits
}

func gatewayUsageDate(value string) openapi_types.Date {
	parsed, err := time.Parse(openapi_types.DateFormat, value)
	if err != nil {
		return openapi_types.Date{Time: time.Time{}}
	}
	return openapi_types.Date{Time: parsed}
}

func normalizeGatewayUsageCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "USD"
	}
	return value
}

func daysUntil(expiresAt time.Time, now time.Time) int {
	if !expiresAt.After(now) {
		return 0
	}
	return int(math.Ceil(expiresAt.Sub(now).Hours() / 24))
}

func ptrStringSlice(values []string) *[]string {
	return &values
}

package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	"github.com/srapi/srapi/apps/api/internal/pkg/usagewindow"
)

var errGatewayRiskControlBlocked = errors.New("gateway request blocked by risk control")

type gatewayRiskDecision struct {
	blocked bool
	reason  string
	subject string
	meta    map[string]any
}

func (rt *runtimeState) enforceGatewayRiskControl(ctx context.Context, authed apikeycontract.AuthResult, clientIP string) error {
	if rt == nil || rt.adminControl == nil {
		return nil
	}
	config, err := rt.adminControl.GetRiskConfig(ctx)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return nil
	}
	decision, err := rt.evaluateGatewayRiskControl(ctx, config, authed, clientIP, time.Now().UTC())
	if err != nil {
		return err
	}
	if decision.reason == "" {
		return nil
	}
	rt.recordGatewayRiskControl(ctx, config, decision)
	if config.Mode == admincontrolcontract.RiskControlModeEnforce && decision.blocked {
		return fmt.Errorf("%w: %s", errGatewayRiskControlBlocked, decision.reason)
	}
	return nil
}

func (rt *runtimeState) evaluateGatewayRiskControl(ctx context.Context, config admincontrolcontract.RiskControlConfig, authed apikeycontract.AuthResult, clientIP string, now time.Time) (gatewayRiskDecision, error) {
	normalizedIP := strings.TrimSpace(clientIP)
	if riskIPBlocked(config.BlockedIPs, normalizedIP) {
		return gatewayRiskDecision{
			blocked: true,
			reason:  "blocked_ip",
			subject: normalizedIP,
			meta: map[string]any{
				"user_id":    authed.UserID,
				"api_key_id": authed.Key.ID,
				"ip":         normalizedIP,
			},
		}, nil
	}
	if exceeded, used, limit, err := rt.gatewayDailyRiskCostExceeded(ctx, config, authed.UserID, now); err != nil {
		return gatewayRiskDecision{}, err
	} else if exceeded {
		return gatewayRiskDecision{
			blocked: true,
			reason:  "daily_cost_limit_exceeded",
			subject: fmt.Sprintf("user:%d", authed.UserID),
			meta: map[string]any{
				"user_id":       authed.UserID,
				"api_key_id":    authed.Key.ID,
				"used_cost":     used,
				"max_cost":      limit,
				"window":        "day",
				"window_start":  usagewindow.StartOfDayUTC(now).Format(time.RFC3339),
				"cooldown_secs": config.CooldownSeconds,
			},
		}, nil
	}
	if exceeded, count, limit, err := rt.gatewayRiskFailuresExceeded(ctx, config, authed.UserID, now); err != nil {
		return gatewayRiskDecision{}, err
	} else if exceeded {
		return gatewayRiskDecision{
			blocked: true,
			reason:  "failed_request_limit_exceeded",
			subject: fmt.Sprintf("user:%d", authed.UserID),
			meta: map[string]any{
				"user_id":       authed.UserID,
				"api_key_id":    authed.Key.ID,
				"failed_count":  count,
				"limit":         limit,
				"window":        "minute",
				"cooldown_secs": config.CooldownSeconds,
			},
		}, nil
	}
	return gatewayRiskDecision{}, nil
}

func (rt *runtimeState) recordGatewayRiskFailure(ctx context.Context, rec gatewayUsageRecord) {
	if rt == nil || rt.adminControl == nil || rec.Success || rec.Authed.UserID <= 0 {
		return
	}
	config, err := rt.adminControl.GetRiskConfig(ctx)
	if err != nil || !config.Enabled || config.MaxFailedRequestsPerMinute <= 0 {
		return
	}
	now := time.Now().UTC()
	errorClass := ""
	if rec.ErrorClass != nil {
		errorClass = strings.TrimSpace(*rec.ErrorClass)
	}
	subject := fmt.Sprintf("user:%d", rec.Authed.UserID)
	_, err = rt.adminControl.RecordRiskLog(ctx, admincontrolcontract.RecordRiskLogRequest{
		Level:   admincontrolcontract.RiskControlLogLevelWarn,
		Action:  "gateway.failure",
		Reason:  "gateway_request_failed",
		Subject: &subject,
		Metadata: map[string]any{
			"user_id":         rec.Authed.UserID,
			"api_key_id":      rec.Authed.Key.ID,
			"request_id":      rec.RequestID,
			"source_endpoint": rec.SourceEndpoint,
			"model":           rec.Model,
			"error_class":     errorClass,
			"status_code":     rec.StatusCode,
		},
		CreatedAt: now,
	})
	if err != nil {
		rt.logger.Warn("failed to record risk control failure", "error", err, "request_id", rec.RequestID)
	}
}

func (rt *runtimeState) recordGatewayRiskControl(ctx context.Context, config admincontrolcontract.RiskControlConfig, decision gatewayRiskDecision) {
	if decision.reason == "" || rt == nil || rt.adminControl == nil {
		return
	}
	level := admincontrolcontract.RiskControlLogLevelWarn
	action := "gateway.risk_detected"
	if config.Mode == admincontrolcontract.RiskControlModeEnforce && decision.blocked {
		level = admincontrolcontract.RiskControlLogLevelBlock
		action = "gateway.block"
	}
	var subject *string
	if strings.TrimSpace(decision.subject) != "" {
		trimmed := strings.TrimSpace(decision.subject)
		subject = &trimmed
	}
	meta := cloneAnyMap(decision.meta)
	meta["mode"] = string(config.Mode)
	if _, err := rt.adminControl.RecordRiskLog(ctx, admincontrolcontract.RecordRiskLogRequest{
		Level:    level,
		Action:   action,
		Reason:   decision.reason,
		Subject:  subject,
		Metadata: meta,
	}); err != nil {
		rt.logger.Warn("failed to record risk control decision", "error", err, "reason", decision.reason)
	}
}

func (rt *runtimeState) gatewayDailyRiskCostExceeded(ctx context.Context, config admincontrolcontract.RiskControlConfig, userID int, now time.Time) (bool, string, string, error) {
	limit, ok := money.DecimalRat(config.MaxCostPerDay)
	if !ok || limit.Sign() <= 0 || rt == nil || rt.usage == nil || userID <= 0 {
		return false, "0.00000000", money.NormalizeAmount(config.MaxCostPerDay), nil
	}
	summary, err := rt.usage.SummarizeUserWindow(ctx, usagecontract.UserWindowFilter{
		UserID:      userID,
		Start:       usagewindow.StartOfDayUTC(now),
		End:         now.UTC().Add(time.Nanosecond),
		SuccessOnly: true,
	})
	if err != nil {
		return false, "", "", err
	}
	used, ok := money.DecimalRat(summary.BillableCost)
	if !ok {
		return false, money.NormalizeAmount(summary.BillableCost), money.NormalizeAmount(config.MaxCostPerDay), nil
	}
	return used.Cmp(limit) >= 0, money.FormatRatFixed(used, 8), money.FormatRatFixed(limit, 8), nil
}

func (rt *runtimeState) gatewayRiskFailuresExceeded(ctx context.Context, config admincontrolcontract.RiskControlConfig, userID int, now time.Time) (bool, int, int, error) {
	limit := config.MaxFailedRequestsPerMinute
	if limit <= 0 || rt == nil || rt.adminControl == nil || userID <= 0 {
		return false, 0, limit, nil
	}
	logs, err := rt.adminControl.ListRiskLogs(ctx, admincontrolcontract.ListOptions{
		Page:     1,
		PageSize: 1000,
	})
	if err != nil {
		return false, 0, limit, err
	}
	cutoff := now.UTC().Add(-time.Minute)
	subject := fmt.Sprintf("user:%d", userID)
	var count int
	for _, item := range logs.Items {
		if item.CreatedAt.Before(cutoff) {
			break
		}
		if item.Action == "gateway.failure" && item.Subject != nil && *item.Subject == subject {
			count++
		}
	}
	return count >= limit, count, limit, nil
}

func riskIPBlocked(blocked []string, clientIP string) bool {
	ip, err := netip.ParseAddr(strings.TrimSpace(clientIP))
	if err != nil {
		return false
	}
	for _, value := range blocked {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(value); err == nil {
			if prefix.Contains(ip) {
				return true
			}
			continue
		}
		if blockedIP, err := netip.ParseAddr(value); err == nil && blockedIP == ip {
			return true
		}
	}
	return false
}

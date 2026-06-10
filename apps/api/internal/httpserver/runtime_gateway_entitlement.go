package httpserver

import (
	"context"
	"net/http"
	"strings"

	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

// gatewayBalanceGate enforces a synchronous positive-balance requirement for
// balance-billed requests, closing the window in which the deferred
// balance_charger worker could let a user overspend before their account is
// disabled. It is opt-in (GATEWAY_REQUIRE_POSITIVE_BALANCE) and only applies to
// requests that actually draw down balance: pure pay-go users and allowance-mode
// subscription overage. hard_cap subscription users never bill to balance and so
// are never blocked here (they may legitimately carry a zero balance).
func (rt *runtimeState) gatewayBalanceGate(ctx context.Context, userID int, entitlement subscriptioncontract.EntitlementDecision, pricing gatewayPricingEvidence) (bool, error) {
	if !rt.cfg.Gateway.RequirePositiveBalance {
		return false, nil
	}
	if !gatewayEntitlementBalanceBilled(entitlement) {
		return false, nil
	}
	if userID <= 0 {
		return false, nil
	}
	user, err := rt.users.FindByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return !gatewayBalanceCoversRequest(user.Balance, pricing.Amount), nil
}

// gatewayEntitlementBalanceBilled reports whether a request that passed the
// subscription entitlement check will draw down user balance.
func gatewayEntitlementBalanceBilled(decision subscriptioncontract.EntitlementDecision) bool {
	// No active subscription entitlement → pure pay-go, always billed to balance.
	if strings.TrimSpace(decision.Reason) == "system_default" {
		return true
	}
	// Allowance-mode subscriptions absorb cost up to the included allowance and
	// bill the overage to balance; hard_cap subscriptions never touch balance.
	return strings.EqualFold(strings.TrimSpace(decision.CostQuotaMode), "allowance")
}

// gatewayBalanceCoversRequest reports whether balance can cover the request: the
// balance must be strictly positive and, when an estimated cost is known, at
// least that estimate. A zero or negative balance never covers a balance-billed
// request.
func gatewayBalanceCoversRequest(balance string, estimatedCost string) bool {
	balanceRat, ok := money.DecimalRat(money.NormalizeAmount(balance))
	if !ok {
		// Unparseable balance is treated as covering the request so a data
		// anomaly never hard-blocks traffic; the deferred charger remains the
		// backstop.
		return true
	}
	if balanceRat.Sign() <= 0 {
		return false
	}
	costRat, ok := money.DecimalRat(money.NormalizeAmount(estimatedCost))
	if !ok || costRat.Sign() <= 0 {
		return true
	}
	return balanceRat.Cmp(costRat) >= 0
}

func gatewayEntitlementErrorClass(decision subscriptioncontract.EntitlementDecision) string {
	switch strings.TrimSpace(decision.Reason) {
	case "model_not_allowed":
		return "entitlement_model_not_allowed"
	case "monthly_token_quota_exceeded":
		return "monthly_token_quota_exceeded"
	case "monthly_cost_quota_exceeded":
		return "monthly_cost_quota_exceeded"
	case "insufficient_balance":
		return "insufficient_balance"
	case "rpm_limit_exceeded":
		return "rpm_limit_exceeded"
	case "tpm_limit_exceeded":
		return "tpm_limit_exceeded"
	case "rate_limit_exceeded":
		return "rate_limit_exceeded"
	case "content_safety_blocked":
		return "content_safety_blocked"
	default:
		return "entitlement_denied"
	}
}

func gatewayEntitlementHTTPStatus(errorClass string) int {
	switch errorClass {
	case "insufficient_balance":
		return http.StatusPaymentRequired
	case "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded", "rpm_limit_exceeded", "tpm_limit_exceeded", "rate_limit_exceeded":
		return http.StatusTooManyRequests
	case "content_safety_blocked":
		return http.StatusForbidden
	default:
		return http.StatusForbidden
	}
}

func gatewayEntitlementErrorType(errorClass string) apiopenapi.GatewayErrorObjectType {
	switch errorClass {
	case "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded", "rpm_limit_exceeded", "tpm_limit_exceeded", "rate_limit_exceeded":
		return apiopenapi.RateLimitError
	default:
		return apiopenapi.PermissionError
	}
}

func gatewayEntitlementMessage(errorClass string) string {
	switch errorClass {
	case "entitlement_model_not_allowed":
		return "model not allowed by subscription entitlement"
	case "monthly_token_quota_exceeded":
		return "monthly token quota exceeded"
	case "monthly_cost_quota_exceeded":
		return "monthly cost quota exceeded"
	case "insufficient_balance":
		return "insufficient balance"
	case "rpm_limit_exceeded":
		return "API key RPM limit exceeded"
	case "tpm_limit_exceeded":
		return "API key TPM limit exceeded"
	case "rate_limit_exceeded":
		return "API key rate limit exceeded"
	case "content_safety_blocked":
		return "request blocked by content safety policy"
	default:
		return "request not allowed by subscription entitlement"
	}
}

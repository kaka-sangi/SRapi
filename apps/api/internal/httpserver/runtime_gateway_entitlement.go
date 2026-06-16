package httpserver

import (
	"context"
	"net/http"
	"strings"

	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

// gatewayBalanceGate enforces a positive-balance requirement for
// balance-billed requests. Layered defence:
//
//  1. The cheap read-only check (gatewayBalanceCoversRequest) rejects users
//     whose persisted balance can't even nominally cover the request.
//  2. When the atomic reservation store is wired (Redis-backed) and an
//     idempotencyKey is available, additionally reserve the estimated cost
//     against the user's in-flight reservation pool — closing the
//     concurrent-overspend race where N concurrent requests each see the
//     same balance and all pass.
//
// Opt-in via GATEWAY_REQUIRE_POSITIVE_BALANCE. Only applies to requests that
// actually draw down balance: pure pay-go users and allowance-mode
// subscription overage. hard_cap subscription users never bill to balance
// and so are never blocked here.
//
// Returns (denied bool, error). denied=true → write 402; admit otherwise.
func (rt *runtimeState) gatewayBalanceGate(ctx context.Context, user userscontract.StoredUser, entitlement subscriptioncontract.EntitlementDecision, pricing gatewayPricingEvidence, idempotencyKey string) (bool, error) {
	if !rt.cfg.Gateway.RequirePositiveBalance {
		return false, nil
	}
	if !gatewayEntitlementBalanceBilled(entitlement) {
		return false, nil
	}
	if user.ID <= 0 {
		return false, nil
	}
	if !gatewayBalanceCoversRequest(user.Balance, pricing.Amount) {
		return true, nil
	}
	// Atomic reservation closes the race the read-only check above can't:
	// two concurrent requests with the same balance each see it covers their
	// individual cost, but together they overdraft. Skip when not wired
	// (single-instance dev / no Redis) — the read-only check still rejects
	// $0-balance users, which is the original goal of the gate. Also skip
	// when no idempotency key is available (a programmer bug — log loudly
	// rather than fail closed; the read-only check above is the safety net).
	if rt.balanceReservation == nil || strings.TrimSpace(idempotencyKey) == "" {
		return false, nil
	}
	ok, err := rt.balanceReservation.Reserve(ctx, user.ID, idempotencyKey, user.Balance, pricing.Amount, 0)
	if err != nil {
		// Fail open: a Redis outage shouldn't 402 every paying user. The
		// read-only check above already filtered $0-balance users; the worst
		// case is a brief return to the pre-reservation race window.
		rt.logger.Warn("balance reservation gate failed; admitting on read-only check", "error", err, "user_id", user.ID, "idempotency_key", idempotencyKey)
		return false, nil
	}
	if !ok {
		// Atomic gate rejected — concurrent in-flight requests already
		// reserved this user's available balance.
		return true, nil
	}
	return false, nil
}

// releaseGatewayReservation returns a previously-claimed reservation slot
// back to the pool. Called from recordGatewayUsage on every request (success
// or failure), so the reservation lives only as long as the actual upstream
// call. Idempotent on the store — releasing a never-reserved key is fine.
func (rt *runtimeState) releaseGatewayReservation(ctx context.Context, userID int, idempotencyKey string) {
	if rt.balanceReservation == nil || userID <= 0 || strings.TrimSpace(idempotencyKey) == "" {
		return
	}
	if err := rt.balanceReservation.Release(ctx, userID, idempotencyKey); err != nil {
		rt.logger.Warn("failed to release balance reservation", "error", err, "user_id", userID, "idempotency_key", idempotencyKey)
	}
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
		// Fail CLOSED: if we cannot determine the balance we must not draw down a
		// real upstream on credit. (Previously this failed open, which let a user
		// with a corrupted/unparseable balance run unbounded paid requests.)
		return false
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

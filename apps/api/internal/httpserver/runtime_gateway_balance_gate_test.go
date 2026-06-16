package httpserver

import (
	"testing"

	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
)

func TestGatewayEntitlementBalanceBilled(t *testing.T) {
	cases := []struct {
		name     string
		decision subscriptioncontract.EntitlementDecision
		want     bool
	}{
		{
			name:     "pay-go (no subscription) bills to balance",
			decision: subscriptioncontract.EntitlementDecision{Reason: "system_default"},
			want:     true,
		},
		{
			name:     "allowance-mode subscription bills overage to balance",
			decision: subscriptioncontract.EntitlementDecision{Reason: "allowed", CostQuotaMode: "allowance"},
			want:     true,
		},
		{
			name:     "allowance-mode is case-insensitive",
			decision: subscriptioncontract.EntitlementDecision{Reason: "allowed", CostQuotaMode: "Allowance"},
			want:     true,
		},
		{
			name:     "hard_cap subscription never bills to balance",
			decision: subscriptioncontract.EntitlementDecision{Reason: "allowed", CostQuotaMode: "hard_cap"},
			want:     false,
		},
		{
			name:     "subscription with no cost-quota mode does not bill to balance",
			decision: subscriptioncontract.EntitlementDecision{Reason: "allowed"},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := gatewayEntitlementBalanceBilled(tc.decision); got != tc.want {
				t.Fatalf("gatewayEntitlementBalanceBilled(%+v) = %v, want %v", tc.decision, got, tc.want)
			}
		})
	}
}

func TestGatewayBalanceCoversRequest(t *testing.T) {
	cases := []struct {
		name      string
		balance   string
		estCost   string
		wantCover bool
	}{
		{name: "positive balance, zero estimate covers", balance: "10.00000000", estCost: "0.00000000", wantCover: true},
		{name: "positive balance above estimate covers", balance: "10.00000000", estCost: "1.50000000", wantCover: true},
		{name: "balance equal to estimate covers", balance: "1.50000000", estCost: "1.50000000", wantCover: true},
		{name: "balance below estimate does not cover", balance: "1.00000000", estCost: "1.50000000", wantCover: false},
		{name: "zero balance never covers", balance: "0.00000000", estCost: "0.00000000", wantCover: false},
		{name: "negative balance never covers", balance: "-0.00000001", estCost: "0.00000000", wantCover: false},
		{name: "empty balance treated as zero, does not cover", balance: "", estCost: "0.00000000", wantCover: false},
		{name: "unparseable balance fails closed (does not cover)", balance: "not-a-number", estCost: "1.00000000", wantCover: false},
		{name: "positive balance with unknown estimate covers", balance: "0.00000001", estCost: "", wantCover: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := gatewayBalanceCoversRequest(tc.balance, tc.estCost); got != tc.wantCover {
				t.Fatalf("gatewayBalanceCoversRequest(%q, %q) = %v, want %v", tc.balance, tc.estCost, got, tc.wantCover)
			}
		})
	}
}

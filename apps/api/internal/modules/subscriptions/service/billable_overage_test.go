package service_test

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
)

func TestBillableOverage(t *testing.T) {
	cases := []struct {
		name       string
		cost       string
		usedBefore string
		allowance  string
		want       string
	}{
		{name: "fully covered under allowance", cost: "1.00000000", usedBefore: "0.00000000", allowance: "10.00000000", want: "0.00000000"},
		{name: "exact boundary fully covered", cost: "2.00000000", usedBefore: "8.00000000", allowance: "10.00000000", want: "0.00000000"},
		{name: "partial overage at boundary", cost: "5.00000000", usedBefore: "8.00000000", allowance: "10.00000000", want: "3.00000000"},
		{name: "allowance exhausted fully billable", cost: "2.00000000", usedBefore: "10.00000000", allowance: "10.00000000", want: "2.00000000"},
		{name: "used beyond allowance fully billable", cost: "2.00000000", usedBefore: "15.00000000", allowance: "10.00000000", want: "2.00000000"},
		{name: "zero allowance everything billable", cost: "2.00000000", usedBefore: "0.00000000", allowance: "0.00000000", want: "2.00000000"},
		{name: "zero cost returns cost", cost: "0.00000000", usedBefore: "0.00000000", allowance: "10.00000000", want: "0.00000000"},
		{name: "unparseable allowance returns full cost", cost: "2.00000000", usedBefore: "0.00000000", allowance: "abc", want: "2.00000000"},
		{name: "fractional partial overage", cost: "0.00010000", usedBefore: "0.99995000", allowance: "1.00000000", want: "0.00005000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.BillableOverage(tc.cost, tc.usedBefore, tc.allowance)
			if got != tc.want {
				t.Fatalf("BillableOverage(%q,%q,%q) = %q, want %q", tc.cost, tc.usedBefore, tc.allowance, got, tc.want)
			}
		})
	}
}

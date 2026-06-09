package money

import (
	"math/big"
	"testing"
)

func TestDecimalRatBlankIsZero(t *testing.T) {
	got, ok := DecimalRat(" ")
	if !ok {
		t.Fatal("blank amount did not parse")
	}
	if got.Sign() != 0 {
		t.Fatalf("blank amount = %s, want zero", got.RatString())
	}
}

func TestDecimalRatRejectsScientificNotation(t *testing.T) {
	if _, ok := DecimalRat("1e-8"); ok {
		t.Fatal("scientific notation parsed, want rejection")
	}
}

func TestFormatRatFixedEightPlaces(t *testing.T) {
	rat, ok := new(big.Rat).SetString("1.234567891")
	if !ok {
		t.Fatal("invalid test rat")
	}
	if got := FormatRatFixed(rat, 8); got != "1.23456789" {
		t.Fatalf("format = %q, want 1.23456789", got)
	}
}

func TestFormatRatFixedRoundHalfAwayFromZero(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "positive half", value: "1.234567895", want: "1.23456790"},
		{name: "positive below half", value: "1.234567894", want: "1.23456789"},
		{name: "negative half", value: "-1.234567895", want: "-1.23456790"},
		{name: "negative below half", value: "-1.234567894", want: "-1.23456789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rat, ok := new(big.Rat).SetString(tt.value)
			if !ok {
				t.Fatalf("invalid test rat %q", tt.value)
			}
			if got := FormatRatFixed(rat, 8); got != tt.want {
				t.Fatalf("format = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddNormalizesInvalidOperandsAsZero(t *testing.T) {
	if got := AddMoney("1.20000000", "0.34567891"); got != "1.54567891" {
		t.Fatalf("add = %q, want 1.54567891", got)
	}
	if got := AddMoney("invalid", "0.00000001"); got != "0.00000001" {
		t.Fatalf("add invalid operand = %q, want 0.00000001", got)
	}
}

func TestNormalizeCurrency(t *testing.T) {
	if got := NormalizeCurrency(" usd "); got != DefaultCurrency {
		t.Fatalf("currency = %q, want %q", got, DefaultCurrency)
	}
	if got := NormalizeCurrency(""); got != DefaultCurrency {
		t.Fatalf("blank currency = %q, want %q", got, DefaultCurrency)
	}
}

package money

import (
	"math/big"
	"strings"
)

const (
	// DefaultCurrency is the fallback ISO-like currency code used across SRapi billing.
	DefaultCurrency = "USD"
	// ZeroAmount is SRapi's canonical 8-place zero money string.
	ZeroAmount = "0.00000000"
)

// DecimalRat parses a decimal money string into a rational number. Blank values
// are treated as zero so callers can normalize optional money fields without
// adding their own empty-string branch.
func DecimalRat(value string) (*big.Rat, bool) {
	return decimalRat(value)
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return big.NewRat(0, 1), true
	}
	if strings.ContainsAny(value, "eE") {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(value)
	return rat, ok
}

// ParseMoneyRat is an alias for DecimalRat kept for call sites that read more
// naturally when parsing optional money input.
func ParseMoneyRat(value string) (*big.Rat, bool) {
	return decimalRat(value)
}

// RequiredDecimalRat parses a required decimal money string. Blank values are
// rejected so callers can keep validation semantics for mandatory amount fields.
func RequiredDecimalRat(value string) (*big.Rat, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, false
	}
	return decimalRat(value)
}

// FormatRatFixed returns value with exactly places fractional digits.
//
// The rounding rule is half-away-from-zero, matching big.Rat.FloatString for
// non-negative values while preserving the same tie behavior for negative
// values used by the existing SRapi money helpers.
func FormatRatFixed(value *big.Rat, places int) string {
	return formatRatFixed(value, places)
}

func formatRatFixed(value *big.Rat, places int) string {
	if value == nil {
		value = new(big.Rat)
	}
	if places < 0 {
		places = 0
	}
	sign := value.Sign()
	if sign < 0 {
		value = new(big.Rat).Neg(value)
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(multiplier))
	numerator := new(big.Int).Set(scaled.Num())
	denominator := new(big.Int).Set(scaled.Denom())
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	doubleRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	if doubleRemainder.Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	raw := quotient.String()
	if places > 0 {
		for len(raw) <= places {
			raw = "0" + raw
		}
		raw = raw[:len(raw)-places] + "." + raw[len(raw)-places:]
	}
	if sign < 0 && raw != "0" && !strings.HasPrefix(raw, "0.") {
		return "-" + raw
	}
	if sign < 0 && strings.Trim(raw, "0.") != "" {
		return "-" + raw
	}
	return raw
}

// AddMoney returns the canonical 8-place sum of two money strings. Invalid operands
// are treated as zero to preserve existing billing helper semantics.
func AddMoney(left string, right string) string {
	return addMoney(left, right)
}

func addMoney(left string, right string) string {
	leftRat, ok := DecimalRat(NormalizeAmount(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := DecimalRat(NormalizeAmount(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return formatRatFixed(leftRat.Add(leftRat, rightRat), 8)
}

// NormalizeAmount returns ZeroAmount for blank input and the trimmed value otherwise.
func NormalizeAmount(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ZeroAmount
	}
	return value
}

// NormalizeCurrency uppercases a currency code and falls back to DefaultCurrency.
func NormalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return DefaultCurrency
	}
	return value
}

package money

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

type Currency string

const (
	USD Currency = "USD"
	KRW Currency = "KRW"
	AED Currency = "AED"
	JPY Currency = "JPY"
)

var Currencies = []Currency{USD, KRW, AED, JPY}

const rateDecimals = 4

type format struct {
	symbol   string
	decimals int
}

var formats = map[Currency]format{
	USD: {symbol: "USD ", decimals: 2},
	KRW: {symbol: "KRW ", decimals: 0},
	AED: {symbol: "AED ", decimals: 2},
	JPY: {symbol: "JPY ", decimals: 0},
}

var amountInput = regexp.MustCompile(`^-?(\d{1,3}(,\d{3})+|\d+)(\.\d+)?$`)

func (c Currency) IsValid() bool {
	_, ok := formats[c]
	return ok
}

// Parse converts text like "1,234.56" to an amount in the currency's minor unit.
// It rejects more decimal places than the currency has.
func Parse(c Currency, text string) (int64, bool) {
	f, ok := formats[c]
	if !ok {
		return 0, false
	}
	text = strings.TrimSpace(text)
	if !amountInput.MatchString(text) {
		return 0, false
	}
	sign, text := "", strings.ReplaceAll(text, ",", "")
	if rest, ok := strings.CutPrefix(text, "-"); ok {
		sign, text = "-", rest
	}
	whole, fraction, _ := strings.Cut(text, ".")
	if len(fraction) > f.decimals {
		return 0, false
	}
	amount, err := strconv.ParseInt(sign+whole+fraction+strings.Repeat("0", f.decimals-len(fraction)), 10, 64)
	if err != nil {
		return 0, false
	}
	return amount, true
}

func Format(c Currency, amount int64) string {
	return formats[c].symbol + Input(c, amount)
}

// Input formats amount for a form field so that Parse reads it back unchanged.
func Input(c Currency, amount int64) string {
	sign, whole, fraction := split(c, amount)
	return join(sign+group(whole), fraction)
}

// FormatRate formats an exchange rate with at most four decimal places, as in "1,368.605".
func FormatRate(rate *big.Rat) string {
	whole, fraction, _ := strings.Cut(rate.FloatString(rateDecimals), ".")
	return join(group(whole), strings.TrimRight(fraction, "0"))
}

// ToUSD converts amount in c's minor unit to USD cents, rounding half away from zero.
// perUSD is the number of units of c that one USD buys.
func ToUSD(c Currency, amount int64, perUSD *big.Rat) int64 {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(formats[c].decimals)), nil)
	cents := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(amount), big.NewInt(100)), scale)
	return round(cents.Quo(cents, perUSD))
}

func round(r *big.Rat) int64 {
	quo, rem := new(big.Int).QuoRem(new(big.Int).Abs(r.Num()), r.Denom(), new(big.Int))
	if rem.Lsh(rem, 1).Cmp(r.Denom()) >= 0 {
		quo.Add(quo, big.NewInt(1))
	}
	if r.Sign() < 0 {
		quo.Neg(quo)
	}
	return quo.Int64()
}

func split(c Currency, amount int64) (sign, whole, fraction string) {
	decimals := formats[c].decimals
	digits := strconv.FormatInt(amount, 10)
	if rest, ok := strings.CutPrefix(digits, "-"); ok {
		sign, digits = "-", rest
	}
	if len(digits) <= decimals {
		digits = strings.Repeat("0", decimals-len(digits)+1) + digits
	}
	return sign, digits[:len(digits)-decimals], digits[len(digits)-decimals:]
}

func group(digits string) string {
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func join(whole, fraction string) string {
	if fraction == "" {
		return whole
	}
	return whole + "." + fraction
}

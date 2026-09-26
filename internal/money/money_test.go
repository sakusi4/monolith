package money

import (
	"math/big"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		currency Currency
		text     string
		want     int64
		wantOK   bool
	}{
		{USD, "12", 1200, true},
		{USD, "12.3", 1230, true},
		{USD, "12.34", 1234, true},
		{USD, "0.29", 29, true},
		{USD, "1,234.56", 123456, true},
		{USD, " 7 ", 700, true},
		{KRW, "1,234,000", 1234000, true},
		{KRW, "500", 500, true},
		{JPY, "12,000", 12000, true},
		{USD, "-0.29", -29, true},
		{USD, "--5", 0, false},
		{USD, "", 0, false},
		{USD, "abc", 0, false},
		{USD, "12.345", 0, false},
		{USD, "1,23", 0, false},
		{USD, "$5", 0, false},
		{USD, "99999999999999999999", 0, false},
		{"EUR", "5", 0, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.currency)+" "+tt.text, func(t *testing.T) {
			got, ok := Parse(tt.currency, tt.text)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Parse(%s, %q) = %d, %v, want %d, %v", tt.currency, tt.text, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		currency Currency
		amount   int64
		want     string
	}{
		{USD, 0, "USD 0.00"},
		{USD, 29, "USD 0.29"},
		{USD, 123456, "USD 1,234.56"},
		{USD, -5, "USD -0.05"},
		{KRW, 1234000, "KRW 1,234,000"},
		{JPY, 12000, "JPY 12,000"},
	}
	for _, tt := range tests {
		if got := Format(tt.currency, tt.amount); got != tt.want {
			t.Errorf("Format(%s, %d) = %q, want %q", tt.currency, tt.amount, got, tt.want)
		}
	}
}

func TestInput_RoundTrip(t *testing.T) {
	for _, c := range Currencies {
		for _, amount := range []int64{0, 5, 29, 123456} {
			got, ok := Parse(c, Input(c, amount))
			if !ok || got != amount {
				t.Errorf("Parse(%s, Input(%s, %d)) = %d, %v, want %d, true", c, c, amount, got, ok, amount)
			}
		}
	}
}

func TestToUSD(t *testing.T) {
	tests := []struct {
		currency Currency
		amount   int64
		perUSD   string
		want     int64
	}{
		{AED, 367250, "3.6725", 100000},
		{KRW, 5, "1000", 1},
		{KRW, 4, "1000", 0},
		{KRW, -5, "1000", -1},
	}
	for _, tt := range tests {
		perUSD, _ := new(big.Rat).SetString(tt.perUSD)
		if got := ToUSD(tt.currency, tt.amount, perUSD); got != tt.want {
			t.Errorf("ToUSD(%s, %d, %s) = %d, want %d", tt.currency, tt.amount, tt.perUSD, got, tt.want)
		}
	}
}

func TestFormatRate(t *testing.T) {
	tests := []struct {
		rate string
		want string
	}{
		{"1374.61", "1,374.61"},
		{"1368.604976", "1,368.605"},
		{"3.6725", "3.6725"},
	}
	for _, tt := range tests {
		rate, _ := new(big.Rat).SetString(tt.rate)
		if got := FormatRate(rate); got != tt.want {
			t.Errorf("FormatRate(%s) = %q, want %q", tt.rate, got, tt.want)
		}
	}
}

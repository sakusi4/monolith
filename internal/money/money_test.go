package money

import "testing"

func TestParseUSD(t *testing.T) {
	tests := []struct {
		text   string
		want   int64
		wantOK bool
	}{
		{"12", 1200, true},
		{"12.3", 1230, true},
		{"12.34", 1234, true},
		{"0.29", 29, true},
		{"1,234.56", 123456, true},
		{" 7 ", 700, true},
		{"", 0, false},
		{"abc", 0, false},
		{"12.345", 0, false},
		{"-5", 0, false},
		{"1,23", 0, false},
		{"$5", 0, false},
		{"99999999999999999999", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, ok := ParseUSD(tt.text)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ParseUSD(%q) = %d, %v, want %d, %v", tt.text, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFormatUSD(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{0, "$0.00"},
		{29, "$0.29"},
		{123456, "$1,234.56"},
		{123456789, "$1,234,567.89"},
	}
	for _, tt := range tests {
		if got := FormatUSD(tt.cents); got != tt.want {
			t.Errorf("FormatUSD(%d) = %q, want %q", tt.cents, got, tt.want)
		}
	}
}

func TestInputUSD_RoundTrip(t *testing.T) {
	for _, cents := range []int64{0, 5, 29, 123456} {
		got, ok := ParseUSD(InputUSD(cents))
		if !ok || got != cents {
			t.Errorf("ParseUSD(InputUSD(%d)) = %d, %v, want %d, true", cents, got, ok, cents)
		}
	}
}

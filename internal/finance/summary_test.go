package finance

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func TestSummarize(t *testing.T) {
	krw := big.NewRat(1000, 1)
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	cur := Snapshot{Items: []SnapshotItem{
		{Asset: Asset{Currency: money.USD}, Amount: 600000},
		{Asset: Asset{Currency: money.KRW}, Amount: 900000000, PerUSD: krw},
		{Asset: Asset{Currency: money.KRW}, Amount: -600000000, PerUSD: krw},
	}}
	tests := []struct {
		name    string
		cur     Snapshot
		prev    Snapshot
		hasPrev bool
		want    summary
	}{
		{
			name:    "loans subtract and the change compares with the previous month",
			cur:     cur,
			prev:    Snapshot{Month: august, Items: []SnapshotItem{{Asset: Asset{Currency: money.USD}, Amount: 32000000}}},
			hasPrev: true,
			want: summary{
				Total: 30600000, Loans: -60000000,
				HasChange: true, Change: -1400000, ChangePercent: "-4.4%", PreviousMonth: august,
			},
		},
		{
			name:    "growth from nothing has no percentage",
			cur:     Snapshot{Items: []SnapshotItem{{Asset: Asset{Currency: money.USD}, Amount: 100}}},
			prev:    Snapshot{Month: august},
			hasPrev: true,
			want:    summary{Total: 100, HasChange: true, Change: 100, PreviousMonth: august},
		},
		{
			name: "missing rate leaves only the total and the currency",
			cur:  Snapshot{Items: []SnapshotItem{{Asset: Asset{Currency: money.USD}, Amount: 5}, {Asset: Asset{Currency: money.AED}, Amount: 100}}},
			want: summary{Total: 5, Missing: []money.Currency{money.AED}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarize(tt.cur, tt.prev, tt.hasPrev); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("summarize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPercent(t *testing.T) {
	tests := []struct {
		part, whole int64
		want        string
	}{
		{1, 3, "33.3%"},
		{2, 3, "66.7%"},
		{-1, 3, "-33.3%"},
		{1, 2000, "0.1%"},
		{1, 0, ""},
	}
	for _, tt := range tests {
		if got := percent(tt.part, tt.whole); got != tt.want {
			t.Errorf("percent(%d, %d) = %q, want %q", tt.part, tt.whole, got, tt.want)
		}
	}
}

func TestAppliedRates(t *testing.T) {
	september := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	item := func(c money.Currency, rateMonth time.Time, perUSD string) SnapshotItem {
		it := SnapshotItem{Asset: Asset{Currency: c}, RateMonth: rateMonth}
		if perUSD != "" {
			it.PerUSD, _ = new(big.Rat).SetString(perUSD)
		}
		return it
	}
	s := Snapshot{Month: september, Items: []SnapshotItem{
		item(money.AED, august, "3.6725"),
		item(money.USD, time.Time{}, ""),
		item(money.KRW, september, "1374.61"),
		item(money.KRW, september, "1374.61"),
		item(money.JPY, time.Time{}, ""),
	}}
	want := []appliedRate{
		{Currency: money.KRW, PerUSD: "1,374.61", Month: september},
		{Currency: money.AED, PerUSD: "3.6725", Month: august, OtherMonth: true},
	}
	if got := appliedRates(s); !reflect.DeepEqual(got, want) {
		t.Errorf("appliedRates() = %+v, want %+v without USD and the currency missing a rate", got, want)
	}
}

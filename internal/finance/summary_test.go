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
	tests := []struct {
		name string
		s    Snapshot
		want summary
	}{
		{
			name: "loans subtract from the net worth",
			s: Snapshot{Items: []SnapshotItem{
				{Currency: money.USD, Amount: 600000},
				{Currency: money.KRW, Amount: 900000000, PerUSD: krw},
				{Currency: money.KRW, Amount: -600000000, PerUSD: krw},
			}},
			want: summary{Total: 30600000, Loans: -60000000},
		},
		{
			name: "missing rate leaves only the total and the currency",
			s:    Snapshot{Items: []SnapshotItem{{Currency: money.USD, Amount: 5}, {Currency: money.AED, Amount: 100}}},
			want: summary{Total: 5, Missing: []money.Currency{money.AED}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarize(tt.s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("summarize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAppliedRates(t *testing.T) {
	september := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	rate := func(s string) *big.Rat {
		r, _ := new(big.Rat).SetString(s)
		return r
	}
	rates := []itemRate{
		{Currency: money.AED, Month: august, PerUSD: rate("3.6725")},
		{Currency: money.USD},
		{Currency: money.KRW, Month: september, PerUSD: rate("1374.61")},
		{Currency: money.KRW, Month: september, PerUSD: rate("1374.61")},
		{Currency: money.JPY},
	}
	want := []appliedRate{
		{Currency: money.KRW, PerUSD: "1,374.61", Month: september},
		{Currency: money.AED, PerUSD: "3.6725", Month: august, OtherMonth: true},
	}
	if got := appliedRates(september, rates); !reflect.DeepEqual(got, want) {
		t.Errorf("appliedRates() = %+v, want %+v without USD and the currency missing a rate", got, want)
	}
}

package finance

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func TestNewNetWorthChart(t *testing.T) {
	month := func(m time.Month) time.Time { return time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC) }
	usd := func(amount int64) SnapshotItem { return SnapshotItem{Currency: money.USD, Amount: amount} }
	snapshots := []Snapshot{
		{ID: 3, Month: month(time.September), Items: []SnapshotItem{usd(500), usd(-200)}},
		{ID: 2, Month: month(time.August), Items: []SnapshotItem{usd(400), {Currency: money.AED, Amount: 1}}},
		{ID: 1, Month: month(time.July), Items: []SnapshotItem{{Currency: money.KRW, Amount: 1000, PerUSD: big.NewRat(10, 1)}}},
	}
	want := netWorthChart{
		Labels:        []string{"Jul 2026", "Sep 2026"},
		NetWorthCents: []int64{10000, 300},
		LoansCents:    []int64{0, 200},
	}
	if got := newNetWorthChart(snapshots); !reflect.DeepEqual(got, want) {
		t.Errorf("newNetWorthChart() = %+v, want %+v, oldest first without the August month missing a rate", got, want)
	}
	if empty := newNetWorthChart(nil); empty.Labels == nil || empty.NetWorthCents == nil || empty.LoansCents == nil {
		t.Errorf("newNetWorthChart(nil) = %+v, want empty slices that encode as []", empty)
	}
}

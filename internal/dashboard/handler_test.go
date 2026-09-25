package dashboard

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/finance"
	"github.com/sakusi4/monolith/internal/money"
)

func TestNewPage(t *testing.T) {
	month := func(m time.Month) time.Time { return time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC) }
	usd := func(amount int64) finance.SnapshotItem {
		return finance.SnapshotItem{Currency: money.USD, Amount: amount}
	}
	snapshots := []finance.Snapshot{
		{ID: 3, Month: month(time.September), Items: []finance.SnapshotItem{usd(500), usd(-200)}},
		{ID: 2, Month: month(time.August), Items: []finance.SnapshotItem{usd(400), {Currency: money.AED, Amount: 1}}},
		{ID: 1, Month: month(time.July), Items: []finance.SnapshotItem{{Currency: money.KRW, Amount: 1000, PerUSD: big.NewRat(10, 1)}}},
	}

	got := newPage(snapshots)

	if got.Latest.ID != 3 || !reflect.DeepEqual(got.Totals, finance.Totals{NetWorth: 300, Loans: -200}) {
		t.Errorf("newPage() latest = %d %+v, want snapshot 3 worth 300", got.Latest.ID, got.Totals)
	}
	wantChart := totalsChart{
		Labels:        []string{"Jul 2026", "Sep 2026"},
		NetWorthCents: []int64{10000, 300},
		LoansCents:    []int64{0, 200},
	}
	if !reflect.DeepEqual(got.Chart, wantChart) {
		t.Errorf("newPage() chart = %+v, want %+v without the August month missing a rate", got.Chart, wantChart)
	}
	if empty := newPage(nil); empty.Latest.ID != 0 || empty.Chart.Labels == nil {
		t.Errorf("newPage(nil) = %+v, want no latest snapshot and an empty chart", empty)
	}
}

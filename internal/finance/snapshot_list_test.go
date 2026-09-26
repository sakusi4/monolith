package finance

import (
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

func TestParseAssetQuery(t *testing.T) {
	values, err := url.ParseQuery("month=2026-08&type=loan&currency=KRW&order=name&direction=asc")
	if err != nil {
		t.Fatal(err)
	}
	want := assetQuery{Month: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), Type: AssetLoan, Currency: money.KRW, Order: orderName, Direction: web.Asc}
	if got, err := parseAssetQuery(values); err != nil || got != want {
		t.Errorf("parseAssetQuery() = %+v, %v, want %+v", got, err, want)
	}
}

func TestAssetQuery_Apply(t *testing.T) {
	rows := []assetRow{
		{Item: SnapshotItem{Name: "bravo", Type: AssetStock, Currency: money.USD}, USD: 300, HasUSD: true},
		{Item: SnapshotItem{Name: "Alpha", Type: AssetLoan, Currency: money.KRW}, USD: -500, HasUSD: true},
		{Item: SnapshotItem{Name: "charlie", Type: AssetCash, Currency: money.AED}},
		{Item: SnapshotItem{Name: "delta", Type: AssetCash, Currency: money.USD}},
		{Item: SnapshotItem{Name: "echo", Type: AssetCash, Currency: money.USD}, USD: 100, HasUSD: true},
	}
	tests := []struct {
		name string
		q    assetQuery
		want []string
	}{
		{"USD high first, rows without USD last", assetQuery{Order: orderUSD, Direction: web.Desc}, []string{"bravo", "echo", "Alpha", "charlie", "delta"}},
		{"USD low first, rows without USD still last", assetQuery{Order: orderUSD, Direction: web.Asc}, []string{"Alpha", "echo", "bravo", "charlie", "delta"}},
		{"name ignores case", assetQuery{Order: orderName, Direction: web.Asc}, []string{"Alpha", "bravo", "echo", "charlie", "delta"}},
		{"currency filter", assetQuery{Currency: money.USD, Order: orderName, Direction: web.Desc}, []string{"echo", "bravo", "delta"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, r := range tt.q.apply(rows) {
				got = append(got, r.Item.Name)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("apply() = %v, want %v", got, tt.want)
			}
		})
	}
}

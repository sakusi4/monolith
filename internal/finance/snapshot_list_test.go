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
	tests := []struct {
		name    string
		query   string
		want    assetQuery
		wantErr bool
	}{
		{"empty query uses defaults", "", assetQuery{Order: orderUSD, Direction: web.Desc}, false},
		{
			"every parameter",
			"month=2026-08&type=loan&currency=KRW&order=name&direction=asc",
			assetQuery{Month: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), Type: AssetLoan, Currency: money.KRW, Order: orderName, Direction: web.Asc},
			false,
		},
		{"unknown order", "order=amount", assetQuery{}, true},
		{"unknown direction", "direction=up", assetQuery{}, true},
		{"unknown type", "type=gold", assetQuery{}, true},
		{"unknown currency", "currency=EUR", assetQuery{}, true},
		{"malformed month", "month=2026-8", assetQuery{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseAssetQuery(values)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseAssetQuery(%q) error = %v, want error %v", tt.query, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseAssetQuery(%q) = %+v, want %+v", tt.query, got, tt.want)
			}
		})
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
		{"type filter", assetQuery{Type: AssetCash, Order: orderUSD, Direction: web.Desc}, []string{"echo", "charlie", "delta"}},
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

func TestTotalOf(t *testing.T) {
	usd := func(amount int64) assetRow {
		return assetRow{Item: SnapshotItem{Currency: money.USD, Amount: amount}, USD: amount, HasUSD: true}
	}
	tests := []struct {
		name string
		rows []assetRow
		want rowTotal
	}{
		{"one currency sums amounts", []assetRow{usd(100), usd(-30)}, rowTotal{Amount: 70, Currency: money.USD, HasAmount: true, USD: 70, HasUSD: true}},
		{"mixed currencies sum only USD", []assetRow{usd(100), {Item: SnapshotItem{Currency: money.KRW, Amount: 5000}, USD: 500, HasUSD: true}}, rowTotal{Amount: 5100, Currency: money.USD, USD: 600, HasUSD: true}},
		{"a row without USD hides the USD total", []assetRow{usd(100), {Item: SnapshotItem{Currency: money.AED, Amount: 1}}}, rowTotal{Amount: 101, Currency: money.USD, USD: 100}},
		{"no rows", nil, rowTotal{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := totalOf(tt.rows); got != tt.want {
				t.Errorf("totalOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAssetQuery_ListURL(t *testing.T) {
	q := assetQuery{Type: AssetLoan, Order: orderName, Direction: web.Asc}
	month := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	if got, want := q.listURL(month), "/finance/snapshots?direction=asc&month=2026-09&order=name&type=loan"; got != want {
		t.Errorf("listURL() = %q, want %q", got, want)
	}
	if got, want := q.itemURL(month, "items/7/edit"), "/finance/snapshots/2026-09/items/7/edit?direction=asc&order=name&type=loan"; got != want {
		t.Errorf("itemURL() = %q, want %q", got, want)
	}
}

package finance

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

const monthLabelLayout = "Jan 2006"

type assetOrder string

const (
	orderUSD  assetOrder = "usd"
	orderName assetOrder = "name"
	orderType assetOrder = "type"
)

var assetOrders = []assetOrder{orderUSD, orderName, orderType}

func (o assetOrder) Label() string {
	switch o {
	case orderUSD:
		return "USD"
	case orderName:
		return "Name"
	case orderType:
		return "Type"
	}
	return string(o)
}

// assetQuery is the snapshot list state in the URL. A zero Month means the current
// month, and an empty Type or Currency means all of them.
type assetQuery struct {
	Month     time.Time
	Type      AssetType
	Currency  money.Currency
	Order     assetOrder
	Direction web.Direction
}

type assetRow struct {
	Asset  Asset
	Amount int64
	USD    int64
	HasUSD bool
}

// assetTotal sums rows. Amount is set only when they share one currency,
// and USD only when every one of them has a USD value.
type assetTotal struct {
	Amount    int64
	Currency  money.Currency
	HasAmount bool
	USD       int64
	HasUSD    bool
}

func parseAssetQuery(v url.Values) (assetQuery, error) {
	var q assetQuery
	var monthErr error
	if month := v.Get("month"); month != "" {
		q.Month, monthErr = time.Parse(monthLayout, month)
		if monthErr != nil {
			monthErr = fmt.Errorf("invalid month %q", month)
		}
	}
	var typeErr, currencyErr, orderErr, directionErr error
	q.Type, typeErr = web.ParseChoice("type", v.Get("type"), assetTypes, "")
	q.Currency, currencyErr = web.ParseChoice("currency", v.Get("currency"), money.Currencies, "")
	q.Order, orderErr = web.ParseChoice("order", v.Get("order"), assetOrders, orderUSD)
	q.Direction, directionErr = web.ParseChoice("direction", v.Get("direction"), web.Directions, web.Desc)
	if err := errors.Join(monthErr, typeErr, currencyErr, orderErr, directionErr); err != nil {
		return assetQuery{}, err
	}
	return q, nil
}

func monthFilter(months []time.Time, shown time.Time) web.Filter {
	options := make([]web.Option, len(months))
	for i, m := range months {
		options[i] = web.Option{Value: m.Format(monthLayout), Label: m.Format(monthLabelLayout), Selected: m.Equal(shown)}
	}
	return web.Filter{Name: "month", Label: "Month", Options: options}
}

func (q assetQuery) filters() []web.Filter {
	currencyLabel := func(c money.Currency) string { return string(c) }
	return []web.Filter{
		{Name: "type", Label: "Type", Options: append([]web.Option{{Label: "All"}}, web.Options(assetTypes, AssetType.Label, q.Type)...)},
		{Name: "currency", Label: "Currency", Options: append([]web.Option{{Label: "All"}}, web.Options(money.Currencies, currencyLabel, q.Currency)...)},
		{Name: "order", Label: "Sort by", Options: web.Options(assetOrders, assetOrder.Label, q.Order)},
		{Name: "direction", Label: "Direction", Options: web.Options(web.Directions, web.Direction.Label, q.Direction)},
	}
}

func (q assetQuery) matches(a Asset) bool {
	return (q.Type == "" || a.Type == q.Type) && (q.Currency == "" || a.Currency == q.Currency)
}

// apply filters and sorts rows. Rows without a USD value always come last.
func (q assetQuery) apply(rows []assetRow) []assetRow {
	rows = slices.DeleteFunc(slices.Clone(rows), func(r assetRow) bool { return !q.matches(r.Asset) })
	slices.SortStableFunc(rows, func(a, b assetRow) int {
		if a.HasUSD != b.HasUSD {
			if a.HasUSD {
				return -1
			}
			return 1
		}
		c := q.compare(a, b)
		if q.Direction == web.Desc {
			return -c
		}
		return c
	})
	return rows
}

func (q assetQuery) compare(a, b assetRow) int {
	switch q.Order {
	case orderUSD:
		return cmp.Compare(a.USD, b.USD)
	case orderName:
		return strings.Compare(strings.ToLower(a.Asset.Name), strings.ToLower(b.Asset.Name))
	case orderType:
		return strings.Compare(a.Asset.Type.Label(), b.Asset.Type.Label())
	}
	return 0
}

func assetRows(s Snapshot) []assetRow {
	rows := make([]assetRow, len(s.Items))
	for i, it := range s.Items {
		rows[i] = assetRow{Asset: it.Asset, Amount: it.Amount}
		rows[i].USD, rows[i].HasUSD = it.USD()
	}
	return rows
}

func totalOf(rows []assetRow) assetTotal {
	if len(rows) == 0 {
		return assetTotal{}
	}
	t := assetTotal{Currency: rows[0].Asset.Currency, HasAmount: true, HasUSD: true}
	for _, r := range rows {
		t.HasAmount = t.HasAmount && r.Asset.Currency == t.Currency
		t.HasUSD = t.HasUSD && r.HasUSD
		t.Amount += r.Amount
		t.USD += r.USD
	}
	return t
}

// listURL is the snapshot list of month with q's filters.
func (q assetQuery) listURL(month time.Time) string {
	v := q.values()
	v.Set("month", month.Format(monthLayout))
	return "/finance/snapshots?" + v.Encode()
}

// itemURL is an action on the month's items that returns to the list with q's filters.
func (q assetQuery) itemURL(month time.Time, action string) string {
	return "/finance/snapshots/" + month.Format(monthLayout) + "/" + action + "?" + q.values().Encode()
}

func (q assetQuery) values() url.Values {
	v := url.Values{}
	if q.Type != "" {
		v.Set("type", string(q.Type))
	}
	if q.Currency != "" {
		v.Set("currency", string(q.Currency))
	}
	v.Set("order", string(q.Order))
	v.Set("direction", string(q.Direction))
	return v
}

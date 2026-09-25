package finance

import (
	"math/big"
	"slices"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

type summary struct {
	Total   int64
	Loans   int64
	Missing []money.Currency
}

// summarize values s in USD. Only Total and Missing are set when a currency has no rate.
func summarize(s Snapshot) summary {
	t := s.Totals()
	if len(t.Missing) > 0 {
		return summary{Total: t.NetWorth, Missing: t.Missing}
	}
	return summary{Total: t.NetWorth, Loans: t.Loans}
}

// appliedRate is the exchange rate that values a month's items in one currency.
// OtherMonth reports that the rate comes from Month because the month has none.
type appliedRate struct {
	Currency   money.Currency
	PerUSD     string
	Month      time.Time
	OtherMonth bool
}

// itemRate is the exchange rate that values one item: the rate of Month, the month with a rate
// closest to the item's month. PerUSD is nil when the currency has no rate.
type itemRate struct {
	Currency money.Currency
	Month    time.Time
	PerUSD   *big.Rat
}

// appliedRates lists rates once per currency in the order of money.Currencies, leaving out
// the currencies without a rate, such as USD. OtherMonth compares each rate's month with month.
func appliedRates(month time.Time, rates []itemRate) []appliedRate {
	var applied []appliedRate
	for _, c := range money.Currencies {
		i := slices.IndexFunc(rates, func(r itemRate) bool { return r.Currency == c && r.PerUSD != nil })
		if i < 0 {
			continue
		}
		r := rates[i]
		applied = append(applied, appliedRate{
			Currency:   c,
			PerUSD:     money.FormatRate(r.PerUSD),
			Month:      r.Month,
			OtherMonth: !r.Month.Equal(month),
		})
	}
	return applied
}

func snapshotRates(s Snapshot) []itemRate {
	rates := make([]itemRate, len(s.Items))
	for i, it := range s.Items {
		rates[i] = itemRate{Currency: it.Currency, Month: it.RateMonth, PerUSD: it.PerUSD}
	}
	return rates
}

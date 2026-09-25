package finance

import (
	"math/big"
	"slices"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

type summary struct {
	Total         int64
	Loans         int64
	Missing       []money.Currency
	HasChange     bool
	Change        int64
	ChangePercent string
	PreviousMonth time.Time
}

// summarize values cur in USD and compares it with prev. Only Total and Missing are set
// when a currency has no rate.
func summarize(cur, prev Snapshot, hasPrev bool) summary {
	t := cur.Totals()
	if len(t.Missing) > 0 {
		return summary{Total: t.NetWorth, Missing: t.Missing}
	}
	s := summary{Total: t.NetWorth, Loans: t.Loans}
	if !hasPrev {
		return s
	}
	p := prev.Totals()
	if len(p.Missing) > 0 {
		return s
	}
	prevTotal := p.NetWorth
	s.HasChange, s.Change, s.PreviousMonth = true, s.Total-prevTotal, prev.Month
	s.ChangePercent = percent(s.Change, max(prevTotal, -prevTotal))
	if s.Change > 0 && s.ChangePercent != "" {
		s.ChangePercent = "+" + s.ChangePercent
	}
	return s
}

// percent formats part/whole as a percentage with one decimal, rounding half away from zero.
// It returns an empty string when whole is zero.
func percent(part, whole int64) string {
	if whole == 0 {
		return ""
	}
	return new(big.Rat).SetFrac64(part*100, whole).FloatString(1) + "%"
}

// appliedRate is the exchange rate that values a snapshot's items in one currency.
// OtherMonth reports that the rate comes from Month because the snapshot month has none.
type appliedRate struct {
	Currency   money.Currency
	PerUSD     string
	Month      time.Time
	OtherMonth bool
}

// appliedRates lists the rates that value s in the order of money.Currencies,
// leaving out the currencies without a rate, such as USD.
func appliedRates(s Snapshot) []appliedRate {
	var rates []appliedRate
	for _, c := range money.Currencies {
		i := slices.IndexFunc(s.Items, func(it SnapshotItem) bool { return it.Asset.Currency == c && it.PerUSD != nil })
		if i < 0 {
			continue
		}
		it := s.Items[i]
		rates = append(rates, appliedRate{
			Currency:   c,
			PerUSD:     money.FormatRate(it.PerUSD),
			Month:      it.RateMonth,
			OtherMonth: !it.RateMonth.Equal(s.Month),
		})
	}
	return rates
}

package finance

import (
	"cmp"
	"slices"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

// expenseSummary values a month's expenses in USD. Total leaves out the currencies in Missing,
// which have no rate.
type expenseSummary struct {
	Total   int64
	Missing []money.Currency
}

type expenseRow struct {
	Expense Expense
	USD     int64
	HasUSD  bool
}

func summarizeExpenses(expenses []Expense) expenseSummary {
	var s expenseSummary
	for _, e := range expenses {
		usd, ok := e.USD()
		if !ok {
			if !slices.Contains(s.Missing, e.Currency) {
				s.Missing = append(s.Missing, e.Currency)
			}
			continue
		}
		s.Total += usd
	}
	return s
}

// expenseRows orders expenses by date, newest first, and within a date by USD value, largest first,
// with those without a USD value last.
func expenseRows(expenses []Expense) []expenseRow {
	rows := make([]expenseRow, len(expenses))
	for i, e := range expenses {
		rows[i] = expenseRow{Expense: e}
		rows[i].USD, rows[i].HasUSD = e.USD()
	}
	slices.SortStableFunc(rows, func(a, b expenseRow) int {
		if c := b.Expense.Date.Compare(a.Expense.Date); c != 0 {
			return c
		}
		if a.HasUSD != b.HasUSD {
			if a.HasUSD {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.USD, a.USD)
	})
	return rows
}

func expenseRates(expenses []Expense) []itemRate {
	rates := make([]itemRate, len(expenses))
	for i, e := range expenses {
		rates[i] = itemRate{Currency: e.Currency, Month: e.RateMonth, PerUSD: e.PerUSD}
	}
	return rates
}

// defaultExpenseDate is the date the add form starts with: today in loc when month is the
// current month, else month's first day.
func defaultExpenseDate(now time.Time, loc *time.Location, month time.Time) time.Time {
	if !currentMonth(now, loc).Equal(month) {
		return month
	}
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func inMonth(date, month time.Time) bool {
	return !date.Before(month) && date.Before(month.AddDate(0, 1, 0))
}

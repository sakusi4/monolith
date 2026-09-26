package finance

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func TestNewSpendingChart(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	expenses := []Expense{
		usdExpense(day(time.September, 30), 1, 500),
		{ID: 2, Date: day(time.July, 3), Currency: money.KRW, Amount: 1000, PerUSD: big.NewRat(10, 1)},
		usdExpense(day(time.August, 1), 3, 400),
		{ID: 4, Date: day(time.August, 9), Currency: money.JPY, Amount: 1},
		usdExpense(day(time.September, 1), 5, -200),
		usdExpense(day(time.July, 31), 6, 50),
	}
	want := spendingChart{
		Labels:     []string{"Jul 2026", "Sep 2026"},
		TotalCents: []int64{10050, 300},
	}
	if got := newSpendingChart(expenses); !reflect.DeepEqual(got, want) {
		t.Errorf("newSpendingChart() = %+v, want %+v, oldest first without the August month missing a rate", got, want)
	}
}

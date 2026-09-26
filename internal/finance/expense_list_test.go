package finance

import (
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func usdExpense(month time.Time, id, amount int64) Expense {
	return Expense{ID: id, Date: month, Currency: money.USD, Amount: amount}
}

func TestExpenseRows(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, time.July, d, 0, 0, 0, 0, time.UTC) }
	jpy := Expense{ID: 9, Date: day(20), Currency: money.JPY, Amount: 100}
	rows := expenseRows([]Expense{usdExpense(day(5), 1, 100), jpy, usdExpense(day(20), 2, 300), usdExpense(day(20), 3, -50), usdExpense(day(5), 4, 900)})
	var got []int64
	for _, r := range rows {
		got = append(got, r.Expense.ID)
	}
	if want := []int64{2, 3, 9, 4, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("expenseRows() order = %v, want %v: newest date first, then by USD with the row without a rate last in its date", got, want)
	}
}

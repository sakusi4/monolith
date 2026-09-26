package finance

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func expenseMonth(m time.Month) time.Time {
	return time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)
}

func usdExpense(month time.Time, id, amount int64) Expense {
	return Expense{ID: id, Date: month, Currency: money.USD, Amount: amount}
}

func TestSummarizeExpenses(t *testing.T) {
	july := expenseMonth(time.July)
	krw := Expense{ID: 3, Date: july, Currency: money.KRW, Amount: 200000, PerUSD: big.NewRat(1000, 1)}
	jpy := Expense{ID: 4, Date: july, Currency: money.JPY, Amount: 100}
	tests := []struct {
		name     string
		expenses []Expense
		want     expenseSummary
	}{
		{"a refund lowers the total", []Expense{usdExpense(july, 1, 30000), usdExpense(july, 2, -5000), krw}, expenseSummary{Total: 45000}},
		{"a currency without a rate leaves only the total and the currency", []Expense{usdExpense(july, 1, 100), jpy}, expenseSummary{Total: 100, Missing: []money.Currency{money.JPY}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeExpenses(tt.expenses); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("summarizeExpenses() = %+v, want %+v", got, tt.want)
			}
		})
	}
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

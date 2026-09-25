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
	july := expenseMonth(time.July)
	jpy := Expense{ID: 9, Date: july, Currency: money.JPY, Amount: 100}
	rows := expenseRows([]Expense{usdExpense(july, 1, 100), jpy, usdExpense(july, 2, 300), usdExpense(july, 3, -50)})
	var got []int64
	for _, r := range rows {
		got = append(got, r.Expense.ID)
	}
	if want := []int64{2, 1, 3, 9}; !reflect.DeepEqual(got, want) {
		t.Errorf("expenseRows() order = %v, want %v by USD with the row without a rate last", got, want)
	}
}

func TestExpenseTotalOf(t *testing.T) {
	july := expenseMonth(time.July)
	krw := Expense{ID: 3, Date: july, Currency: money.KRW, Amount: 5000, PerUSD: big.NewRat(1000, 1)}
	jpy := Expense{ID: 4, Date: july, Currency: money.JPY, Amount: 1}
	tests := []struct {
		name     string
		expenses []Expense
		want     rowTotal
	}{
		{"one currency sums amounts", []Expense{usdExpense(july, 1, 100), usdExpense(july, 2, -30)}, rowTotal{Amount: 70, Currency: money.USD, HasAmount: true, USD: 70, HasUSD: true}},
		{"mixed currencies sum only USD", []Expense{usdExpense(july, 1, 1000), krw}, rowTotal{Amount: 6000, Currency: money.USD, USD: 1500, HasUSD: true}},
		{"a row without USD hides the USD total", []Expense{usdExpense(july, 1, 100), jpy}, rowTotal{Amount: 101, Currency: money.USD, USD: 100}},
		{"no rows", nil, rowTotal{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expenseTotalOf(expenseRows(tt.expenses)); got != tt.want {
				t.Errorf("expenseTotalOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

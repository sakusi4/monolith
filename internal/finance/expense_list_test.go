package finance

import (
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

func usdExpense(month time.Time, id, amount int64) Expense {
	return Expense{ID: id, Date: month, Currency: money.USD, Amount: amount}
}

func TestParseExpenseQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    expenseQuery
		wantErr bool
	}{
		{
			"every parameter",
			"month=2026-08&category=food&order=usd&direction=asc&page=3",
			expenseQuery{Month: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), Category: CategoryFood, Order: expenseOrderUSD, Direction: web.Asc, Page: 3},
			false,
		},
		{"page below 1", "page=0", expenseQuery{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseExpenseQuery(values)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("parseExpenseQuery(%q) = %+v, %v, want %+v, error %v", tt.query, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestExpenseQuery_Sort(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, time.July, d, 0, 0, 0, 0, time.UTC) }
	spent := func(id int64, d int, c ExpenseCategory, cents int64) Expense {
		return Expense{ID: id, Date: day(d), Category: c, Currency: money.USD, Amount: cents}
	}
	rows := expenseRows([]Expense{
		spent(1, 5, CategoryFood, 100),
		spent(2, 20, CategoryTravel, 300),
		spent(3, 20, CategoryFood, -50),
		{ID: 9, Date: day(20), Category: CategoryFood, Currency: money.JPY, Amount: 100},
		spent(4, 5, CategoryTravel, 900),
		spent(5, 10, CategoryFood, 300),
	})
	tests := []struct {
		name string
		q    expenseQuery
		want []int64
	}{
		{"newest first, a date by USD with rows without USD last", expenseQuery{Order: expenseOrderDate, Direction: web.Desc}, []int64{2, 3, 9, 5, 4, 1}},
		{"oldest first", expenseQuery{Order: expenseOrderDate, Direction: web.Asc}, []int64{4, 1, 5, 2, 3, 9}},
		{"USD low first, equal USD newest first, rows without USD last", expenseQuery{Order: expenseOrderUSD, Direction: web.Asc}, []int64{3, 1, 2, 5, 4, 9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int64
			for _, r := range tt.q.sort(rows) {
				got = append(got, r.Expense.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("sort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpenseQuery_Paginate(t *testing.T) {
	july := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]expenseRow, 45)
	for i := range rows {
		rows[i].Expense.ID = int64(i + 1)
	}
	list := "/finance/expenses?direction=desc&month=2026-07&order=date"
	tests := []struct {
		name      string
		page      int
		wantFirst int64
		want      expensePager
	}{
		{"a middle page links both ways", 2, 21, expensePager{From: 21, To: 40, Total: 45, PrevURL: list, NextURL: list + "&page=3"}},
		{"a page past the last shows the last", 9, 41, expensePager{From: 41, To: 45, Total: 45, PrevURL: list + "&page=2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := expenseQuery{Month: july, Order: expenseOrderDate, Direction: web.Desc, Page: tt.page}
			shown, pager := q.paginate(rows)
			if len(shown) == 0 || shown[0].Expense.ID != tt.wantFirst || pager != tt.want {
				t.Errorf("paginate() = %d rows from %v, %+v, want from %d, %+v", len(shown), shown, pager, tt.wantFirst, tt.want)
			}
		})
	}
}

package finance

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

const expensesPerPage = 20

type expenseOrder string

const (
	expenseOrderDate expenseOrder = "date"
	expenseOrderUSD  expenseOrder = "usd"
)

var expenseOrders = []expenseOrder{expenseOrderDate, expenseOrderUSD}

func (o expenseOrder) Label() string {
	switch o {
	case expenseOrderDate:
		return "Date"
	case expenseOrderUSD:
		return "USD"
	}
	return string(o)
}

// expenseQuery is the expense list state in the URL. A zero Month means the current month,
// an empty Category means all of them, and Page counts from 1.
type expenseQuery struct {
	Month     time.Time
	Category  ExpenseCategory
	Order     expenseOrder
	Direction web.Direction
	Page      int
}

// expensePager places the page shown among the Total rows that match the filters.
// PrevURL and NextURL are empty on the first and the last page.
type expensePager struct {
	From    int
	To      int
	Total   int
	PrevURL string
	NextURL string
}

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

func expenseRows(expenses []Expense) []expenseRow {
	rows := make([]expenseRow, len(expenses))
	for i, e := range expenses {
		rows[i] = expenseRow{Expense: e}
		rows[i].USD, rows[i].HasUSD = e.USD()
	}
	return rows
}

func parseExpenseQuery(v url.Values) (expenseQuery, error) {
	q := expenseQuery{Page: 1}
	var monthErr, pageErr error
	if month := v.Get("month"); month != "" {
		q.Month, monthErr = time.Parse(monthLayout, month)
		if monthErr != nil {
			monthErr = fmt.Errorf("invalid month %q", month)
		}
	}
	if page := v.Get("page"); page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n < 1 {
			pageErr = fmt.Errorf("invalid page %q", page)
		}
		q.Page = n
	}
	var categoryErr, orderErr, directionErr error
	q.Category, categoryErr = web.ParseChoice("category", v.Get("category"), expenseCategories, "")
	q.Order, orderErr = web.ParseChoice("order", v.Get("order"), expenseOrders, expenseOrderDate)
	q.Direction, directionErr = web.ParseChoice("direction", v.Get("direction"), web.Directions, web.Desc)
	if err := errors.Join(monthErr, pageErr, categoryErr, orderErr, directionErr); err != nil {
		return expenseQuery{}, err
	}
	return q, nil
}

func (q expenseQuery) filters() []web.Filter {
	return []web.Filter{
		{Name: "category", Label: "Category", Options: append([]web.Option{{Label: "All"}}, web.Options(expenseCategories, ExpenseCategory.Label, q.Category)...)},
		{Name: "order", Label: "Sort by", Options: web.Options(expenseOrders, expenseOrder.Label, q.Order)},
		{Name: "direction", Label: "Direction", Options: web.Options(web.Directions, web.Direction.Label, q.Direction)},
	}
}

func (q expenseQuery) matching(expenses []Expense) []Expense {
	return slices.DeleteFunc(slices.Clone(expenses), func(e Expense) bool {
		return q.Category != "" && e.Category != q.Category
	})
}

// sort orders rows by date or by USD value. By date, rows of the same date follow USD value,
// largest first. By USD, equal values follow the date, newest first. Rows without a USD value
// come after the others they tie with by date, and last of all by USD.
func (q expenseQuery) sort(rows []expenseRow) []expenseRow {
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, q.compare)
	return rows
}

func (q expenseQuery) compare(a, b expenseRow) int {
	byDate := a.Expense.Date.Compare(b.Expense.Date)
	switch q.Order {
	case expenseOrderDate:
		if byDate == 0 {
			return compareUSD(a, b, web.Desc)
		}
		if q.Direction == web.Desc {
			return -byDate
		}
		return byDate
	case expenseOrderUSD:
		if c := compareUSD(a, b, q.Direction); c != 0 {
			return c
		}
		return -byDate
	}
	return 0
}

// compareUSD orders rows by USD value in direction, with rows without a USD value last.
func compareUSD(a, b expenseRow, direction web.Direction) int {
	if a.HasUSD != b.HasUSD {
		if a.HasUSD {
			return -1
		}
		return 1
	}
	c := cmp.Compare(a.USD, b.USD)
	if direction == web.Desc {
		return -c
	}
	return c
}

// paginate returns the rows of q.Page, or of the last page when q.Page is past it.
func (q expenseQuery) paginate(rows []expenseRow) ([]expenseRow, expensePager) {
	last := max(1, (len(rows)+expensesPerPage-1)/expensesPerPage)
	page := min(q.Page, last)
	start := (page - 1) * expensesPerPage
	end := min(start+expensesPerPage, len(rows))
	pager := expensePager{From: start + 1, To: end, Total: len(rows)}
	if page > 1 {
		prev := q
		prev.Page = page - 1
		pager.PrevURL = prev.listURL(q.Month)
	}
	if page < last {
		next := q
		next.Page = page + 1
		pager.NextURL = next.listURL(q.Month)
	}
	return rows[start:end], pager
}

// listURL is the expense list of month with q's filters and page.
func (q expenseQuery) listURL(month time.Time) string {
	v := q.values()
	v.Set("month", month.Format(monthLayout))
	return "/finance/expenses?" + v.Encode()
}

// itemURL is an action on the month's expenses that returns to the list with q's filters and page.
func (q expenseQuery) itemURL(month time.Time, action string) string {
	return "/finance/expenses/" + month.Format(monthLayout) + "/" + action + "?" + q.values().Encode()
}

func (q expenseQuery) values() url.Values {
	v := url.Values{}
	if q.Category != "" {
		v.Set("category", string(q.Category))
	}
	v.Set("order", string(q.Order))
	v.Set("direction", string(q.Direction))
	if q.Page > 1 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	return v
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

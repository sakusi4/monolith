package finance

import (
	"maps"
	"slices"
	"time"
)

// spendingChart is the chart data that spending_chart.js reads from the page as JSON, oldest
// month first.
type spendingChart struct {
	Labels     []string `json:"labels"`
	TotalCents []int64  `json:"totalCents"`
}

// newSpendingChart totals expenses by month. It leaves out months with a missing exchange rate.
func newSpendingChart(expenses []Expense) spendingChart {
	byMonth := map[time.Time][]Expense{}
	for _, e := range expenses {
		month := time.Date(e.Date.Year(), e.Date.Month(), 1, 0, 0, 0, 0, time.UTC)
		byMonth[month] = append(byMonth[month], e)
	}
	var c spendingChart
	for _, month := range slices.SortedFunc(maps.Keys(byMonth), time.Time.Compare) {
		s := summarizeExpenses(byMonth[month])
		if len(s.Missing) == 0 {
			c.Labels = append(c.Labels, month.Format(monthLabelLayout))
			c.TotalCents = append(c.TotalCents, s.Total)
		}
	}
	return c
}

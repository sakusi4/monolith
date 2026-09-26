package finance

import "slices"

// netWorthChart is the chart data that net_worth_chart.js reads from the page as JSON, oldest
// month first. LoansCents holds the loan balances as positive amounts.
type netWorthChart struct {
	Labels        []string `json:"labels"`
	NetWorthCents []int64  `json:"netWorthCents"`
	LoansCents    []int64  `json:"loansCents"`
}

// newNetWorthChart expects snapshots newest first, as Store.Snapshots returns them.
// It leaves out months with a missing exchange rate.
func newNetWorthChart(snapshots []Snapshot) netWorthChart {
	var c netWorthChart
	for _, s := range slices.Backward(snapshots) {
		t := s.Totals()
		if len(t.Missing) == 0 {
			c.Labels = append(c.Labels, s.Month.Format(monthLabelLayout))
			c.NetWorthCents = append(c.NetWorthCents, t.NetWorth)
			c.LoansCents = append(c.LoansCents, -t.Loans)
		}
	}
	return c
}

package dashboard

import (
	"net/http"
	"slices"

	"github.com/sakusi4/monolith/internal/finance"
	"github.com/sakusi4/monolith/web"
)

const monthLabelLayout = "Jan 2006"

type handler struct {
	finance *finance.Store
}

type page struct {
	Latest finance.Snapshot
	Totals finance.Totals
	Chart  totalsChart
}

// totalsChart is the chart data that dashboard.js reads from the page as JSON, oldest month first.
// LoansCents holds the loan balances as positive amounts.
type totalsChart struct {
	Labels        []string `json:"labels"`
	NetWorthCents []int64  `json:"netWorthCents"`
	LoansCents    []int64  `json:"loansCents"`
}

func NewHandler(financeStore *finance.Store) http.Handler {
	h := &handler{finance: financeStore}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dashboard", h.showDashboard)
	return mux
}

func (h *handler) showDashboard(w http.ResponseWriter, r *http.Request) {
	snapshots, err := h.finance.Snapshots(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.Render(w, r, http.StatusOK, "dashboard", newPage(snapshots))
}

// newPage expects snapshots newest first, as finance.Store.Snapshots returns them.
// The chart leaves out months with a missing exchange rate.
func newPage(snapshots []finance.Snapshot) page {
	p := page{Chart: totalsChart{Labels: []string{}, NetWorthCents: []int64{}, LoansCents: []int64{}}}
	if len(snapshots) == 0 {
		return p
	}
	p.Latest, p.Totals = snapshots[0], snapshots[0].Totals()
	for _, s := range slices.Backward(snapshots) {
		t := s.Totals()
		if len(t.Missing) == 0 {
			p.Chart.Labels = append(p.Chart.Labels, s.Month.Format(monthLabelLayout))
			p.Chart.NetWorthCents = append(p.Chart.NetWorthCents, t.NetWorth)
			p.Chart.LoansCents = append(p.Chart.LoansCents, -t.Loans)
		}
	}
	return p
}

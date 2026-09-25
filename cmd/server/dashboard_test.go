package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDashboard(t *testing.T) {
	s := newTestServer(t)
	session := s.login(t)

	t.Run("home redirects to the dashboard", func(t *testing.T) {
		wantRedirect(t, s.get(t, "/", session), "/dashboard")
	})

	t.Run("without snapshots it asks for the first one", func(t *testing.T) {
		rec := s.get(t, "/dashboard", session)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Add your first snapshot") {
			t.Errorf("GET /dashboard = %d, want 200 with an empty state", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `href="/dashboard" aria-current="page"`) {
			t.Errorf("GET /dashboard does not mark Dashboard as the current page")
		}
	})

	t.Run("shows the latest totals and the monthly chart data", func(t *testing.T) {
		for _, row := range []url.Values{
			{"month": {"2025-08"}, "name": {"Wallet"}, "type": {"cash"}, "currency": {"USD"}, "amount": {"900.00"}},
			{"month": {"2025-09"}, "name": {"Wallet"}, "type": {"cash"}, "currency": {"USD"}, "amount": {"1,500.00"}},
			{"month": {"2025-09"}, "name": {"Car loan"}, "type": {"loan"}, "currency": {"USD"}, "amount": {"-400.00"}},
		} {
			rec := s.post(t, "/finance/snapshots/"+row.Get("month")+"/items/new", row, session)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("add %v = %d, want 303", row, rec.Code)
			}
		}

		rec := s.get(t, "/dashboard", session)
		body := rec.Body.String()
		for _, want := range []string{"USD 1,100.00", "USD -400.00", "as of Sep 2025", `{"labels":["Aug 2025","Sep 2025"],"netWorthCents":[90000,110000],"loansCents":[0,40000]}`} {
			if !strings.Contains(body, want) {
				t.Errorf("GET /dashboard does not contain %q:\n%s", want, body)
			}
		}
	})
}

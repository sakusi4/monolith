package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestFinance(t *testing.T) {
	s := newTestServer(t)
	session := s.login(t)
	body := func(t *testing.T, path string) string {
		t.Helper()
		rec := s.get(t, path, session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		return rec.Body.String()
	}
	wantContains := func(t *testing.T, path string, texts ...string) {
		t.Helper()
		got := body(t, path)
		for _, text := range texts {
			if !strings.Contains(got, text) {
				t.Errorf("GET %s does not contain %q:\n%s", path, text, got)
			}
		}
	}
	wantStatus := func(t *testing.T, path string, form url.Values, want int) {
		t.Helper()
		if rec := s.post(t, path, form, session); rec.Code != want {
			t.Errorf("POST %s %v = %d, want %d", path, form, rec.Code, want)
		}
	}
	add := func(t *testing.T, month, name, typ, currency, amount string) {
		t.Helper()
		form := url.Values{"name": {name}, "type": {typ}, "currency": {currency}, "amount": {amount}}
		wantRedirect(t, s.post(t, "/finance/snapshots/"+month+"/items/new", form, session), "/finance/snapshots?direction=desc&month="+month+"&order=usd")
	}
	assetID := func(t *testing.T, name string) string {
		t.Helper()
		var id int64
		if err := s.db.QueryRowContext(t.Context(), `SELECT id FROM assets WHERE name = $1`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return strconv.FormatInt(id, 10)
	}
	count := func(t *testing.T, query string) int {
		t.Helper()
		var n int
		if err := s.db.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("the current month starts empty", func(t *testing.T) {
		wantContains(t, "/finance/snapshots", "yet.", "Add to ")
	})

	if _, err := s.db.ExecContext(t.Context(), `
		INSERT INTO exchange_rates (month, currency, per_usd)
		VALUES ('2024-12-01', 'KRW', 1000), ('2025-03-01', 'KRW', 2000)`); err != nil {
		t.Fatal(err)
	}

	t.Run("added rows are valued with the closest rate", func(t *testing.T) {
		add(t, "2025-01", " Brokerage ", "stock", "USD", "10,000.00")
		add(t, "2025-01", "Savings", "deposit", "KRW", "20,000,000")
		add(t, "2025-01", "Mortgage", "loan", "KRW", "-5,000,000")
		wantContains(t, "/finance/snapshots?month=2025-01", "USD 25,000.00", "KRW -5,000,000", "USD -5,000.00", "1 USD = KRW 1,000 (Dec 2024)")
	})

	t.Run("invalid rows show the form again", func(t *testing.T) {
		for _, form := range []url.Values{
			{"name": {"  "}, "type": {"cash"}, "currency": {"USD"}, "amount": {"1"}},
			{"name": {"Euro cash"}, "type": {"cash"}, "currency": {"EUR"}, "amount": {"1"}},
			{"name": {"Won cash"}, "type": {"cash"}, "currency": {"KRW"}, "amount": {"12.5"}},
			{"name": {"brokerage"}, "type": {"stock"}, "currency": {"USD"}, "amount": {"1"}},
		} {
			wantStatus(t, "/finance/snapshots/2025-01/items/new", form, http.StatusUnprocessableEntity)
		}
	})

	t.Run("a month with assets keeps a note", func(t *testing.T) {
		wantRedirect(t, s.post(t, "/finance/snapshots/2025-01/note", url.Values{"note": {" Paid off the car loan "}}, session), "/finance/snapshots?direction=desc&month=2025-01&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-01", `value="Paid off the car loan"`)
		wantStatus(t, "/finance/snapshots/2025-04/note", url.Values{"note": {"Empty month"}}, http.StatusNotFound)
	})

	t.Run("an existing name takes the asset's type and currency", func(t *testing.T) {
		rec := s.post(t, "/finance/snapshots/2025-02/items/new", url.Values{"name": {"savings"}, "amount": {"1.5"}}, session)
		if body := rec.Body.String(); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, `<select name="type" disabled>`) || !strings.Contains(body, `<option value="KRW" selected>`) {
			t.Errorf("existing asset with an invalid amount = %d, want 422 with its type and currency fixed:\n%s", rec.Code, body)
		}
		add(t, "2025-02", "brokerage", "", "", "12.50")
		wantContains(t, "/finance/snapshots?month=2025-02", "Brokerage", "USD 12.50")
		if n := count(t, `SELECT count(*) FROM assets`); n != 3 {
			t.Errorf("assets = %d, want 3", n)
		}
	})

	t.Run("an empty month copies the previous snapshot once", func(t *testing.T) {
		wantContains(t, "/finance/snapshots?month=2025-03", "Copy Feb 2025 (1 assets)")
		wantRedirect(t, s.post(t, "/finance/snapshots/2025-03/copy", nil, session), "/finance/snapshots?direction=desc&month=2025-03&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-03", "Brokerage", "USD 12.50")
		wantStatus(t, "/finance/snapshots/2025-03/copy", nil, http.StatusUnprocessableEntity)
		wantStatus(t, "/finance/snapshots/2024-02/copy", nil, http.StatusUnprocessableEntity)
	})

	t.Run("edit turns the row into inputs and saves it", func(t *testing.T) {
		mortgage := assetID(t, "Mortgage")
		edit := "/finance/snapshots/2025-01/items/" + mortgage + "/edit"
		wantContains(t, edit, `form="edit-item"`, `value="-5,000,000"`, `value="Mortgage"`)
		wantRedirect(t, s.post(t, edit, url.Values{"name": {"Home loan"}, "type": {"loan"}, "amount": {"-6,000,000"}}, session), "/finance/snapshots?direction=desc&month=2025-01&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-01", "Home loan", "USD 24,000.00")
		wantStatus(t, edit, url.Values{"name": {"savings"}, "type": {"loan"}, "amount": {"1"}}, http.StatusUnprocessableEntity)
		wantStatus(t, edit, url.Values{"name": {"Home loan"}, "type": {"loan"}, "amount": {"1.5"}}, http.StatusUnprocessableEntity)
		if rec := s.get(t, "/finance/snapshots/2025-02/items/"+mortgage+"/edit", session); rec.Code != http.StatusNotFound {
			t.Errorf("edit of an asset outside the month = %d, want 404", rec.Code)
		}
	})

	t.Run("delete removes the row, the empty month, and the unused asset", func(t *testing.T) {
		brokerage := assetID(t, "Brokerage")
		wantRedirect(t, s.post(t, "/finance/snapshots/2025-02/items/"+brokerage+"/delete", nil, session), "/finance/snapshots?direction=desc&month=2025-02&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-02", "No assets in Feb 2025 yet.")
		if n := count(t, `SELECT count(*) FROM snapshots WHERE month = '2025-02-01'`); n != 0 {
			t.Errorf("snapshots in Feb 2025 = %d, want 0", n)
		}
		wantStatus(t, "/finance/snapshots/2025-02/items/"+brokerage+"/delete", nil, http.StatusNotFound)

		homeLoan := assetID(t, "Home loan")
		wantRedirect(t, s.post(t, "/finance/snapshots/2025-01/items/"+homeLoan+"/delete", nil, session), "/finance/snapshots?direction=desc&month=2025-01&order=usd")
		if n := count(t, `SELECT count(*) FROM assets WHERE name = 'Home loan'`); n != 0 {
			t.Errorf("Home loan is still an asset after leaving its only month")
		}
	})

	t.Run("filters narrow the rows but not the net worth", func(t *testing.T) {
		got := body(t, "/finance/snapshots?month=2025-01&type=deposit")
		if strings.Contains(got, "<td>Brokerage</td>") || !strings.Contains(got, `<th scope="row">Total</th>`) {
			t.Errorf("deposit filter does not show only Savings with a total:\n%s", got)
		}
		wantContains(t, "/finance/snapshots?month=2025-01&type=deposit", "USD 30,000.00", "KRW 20,000,000")
	})

	t.Run("filters and months outside the range are rejected", func(t *testing.T) {
		for path, want := range map[string]int{
			"/finance/snapshots?order=amount":                      http.StatusBadRequest,
			"/finance/snapshots?month=2024-01":                     http.StatusNotFound,
			"/finance/snapshots?month=2099-01":                     http.StatusNotFound,
			"/finance/snapshots/2099-01/items/1/edit":              http.StatusNotFound,
			"/finance/snapshots/2025-01/items/1/edit?order=amount": http.StatusBadRequest,
		} {
			if rec := s.get(t, path, session); rec.Code != want {
				t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
			}
		}
		wantStatus(t, "/finance/snapshots/2099-01/items/new", url.Values{"name": {"x"}, "type": {"cash"}, "currency": {"USD"}, "amount": {"1"}}, http.StatusNotFound)
	})

	t.Run("htmx requests get the same page", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/finance/snapshots?month=2025-01", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("HX-Request", "true")
		if rec := s.do(t, req, session); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<main hx-target="main"`) {
			t.Errorf("htmx GET = %d, want 200 with the whole page", rec.Code)
		}
	})
}

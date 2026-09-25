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
	itemID := func(t *testing.T, month, name, currency string) string {
		t.Helper()
		var id int64
		query := `
			SELECT i.id FROM snapshot_items i JOIN snapshots s ON s.id = i.snapshot_id
			WHERE s.month = $1 AND i.name = $2 AND i.currency = $3`
		if err := s.db.QueryRowContext(t.Context(), query, month+"-01", name, currency).Scan(&id); err != nil {
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
		} {
			wantStatus(t, "/finance/snapshots/2025-01/items/new", form, http.StatusUnprocessableEntity)
		}
	})

	t.Run("a month with assets keeps a note", func(t *testing.T) {
		wantRedirect(t, s.post(t, "/finance/snapshots/2025-01/note", url.Values{"note": {" Paid off the car loan "}}, session), "/finance/snapshots?direction=desc&month=2025-01&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-01", `value="Paid off the car loan"`)
		wantStatus(t, "/finance/snapshots/2025-04/note", url.Values{"note": {"Empty month"}}, http.StatusNotFound)
	})

	t.Run("names can repeat and earlier names are suggested", func(t *testing.T) {
		add(t, "2025-02", "Brokerage", "stock", "USD", "12.50")
		add(t, "2025-02", "Brokerage", "stock", "KRW", "1,000,000")
		got := body(t, "/finance/snapshots?month=2025-02")
		for _, want := range []string{"USD 12.50", "KRW 1,000,000", `<option value="Savings" data-type="deposit" data-currency="KRW">`} {
			if !strings.Contains(got, want) {
				t.Errorf("GET 2025-02 does not contain %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, `<option value="Brokerage"`) {
			t.Errorf("GET 2025-02 suggests Brokerage, which the month already has")
		}
	})

	t.Run("an empty month copies the previous snapshot once", func(t *testing.T) {
		wantContains(t, "/finance/snapshots?month=2025-03", "Copy last recorded month")
		wantRedirect(t, s.post(t, "/finance/snapshots/2025-03/copy", nil, session), "/finance/snapshots?direction=desc&month=2025-03&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-03", "Brokerage", "USD 12.50")
		wantRedirect(t, s.post(t, "/finance/snapshots/2025-05/copy", nil, session), "/finance/snapshots?direction=desc&month=2025-05&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-05", "Brokerage", "USD 12.50")
		wantStatus(t, "/finance/snapshots/2025-03/copy", nil, http.StatusUnprocessableEntity)
		wantStatus(t, "/finance/snapshots/2024-02/copy", nil, http.StatusUnprocessableEntity)
	})

	t.Run("edit changes the row in its month only", func(t *testing.T) {
		mortgage := itemID(t, "2025-01", "Mortgage", "KRW")
		edit := "/finance/snapshots/2025-01/items/" + mortgage + "/edit"
		wantContains(t, edit, `form="edit-item"`, `value="-5,000,000"`, `value="Mortgage"`)
		saved := url.Values{"name": {"Home loan"}, "type": {"loan"}, "currency": {"KRW"}, "amount": {"-6,000,000"}}
		wantRedirect(t, s.post(t, edit, saved, session), "/finance/snapshots?direction=desc&month=2025-01&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-01", "Home loan", "USD 24,000.00")
		wantStatus(t, edit, url.Values{"name": {"Home loan"}, "type": {"loan"}, "currency": {"KRW"}, "amount": {"1.5"}}, http.StatusUnprocessableEntity)
		wantStatus(t, edit, url.Values{"name": {" "}, "type": {"loan"}, "currency": {"KRW"}, "amount": {"1"}}, http.StatusUnprocessableEntity)
		other := "/finance/snapshots/2025-02/items/" + mortgage + "/edit"
		if rec := s.get(t, other, session); rec.Code != http.StatusNotFound {
			t.Errorf("GET edit of a row outside the month = %d, want 404", rec.Code)
		}
		wantStatus(t, other, saved, http.StatusNotFound)
	})

	t.Run("delete removes only the row, and the month once it is empty", func(t *testing.T) {
		usd, krw := itemID(t, "2025-02", "Brokerage", "USD"), itemID(t, "2025-02", "Brokerage", "KRW")
		del := func(id string) string { return "/finance/snapshots/2025-02/items/" + id + "/delete" }
		wantRedirect(t, s.post(t, del(usd), nil, session), "/finance/snapshots?direction=desc&month=2025-02&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-02", "KRW 1,000,000")
		wantRedirect(t, s.post(t, del(krw), nil, session), "/finance/snapshots?direction=desc&month=2025-02&order=usd")
		wantContains(t, "/finance/snapshots?month=2025-02", "No assets in Feb 2025 yet.")
		if n := count(t, `SELECT count(*) FROM snapshots WHERE month = '2025-02-01'`); n != 0 {
			t.Errorf("snapshots in Feb 2025 = %d, want 0", n)
		}
		wantStatus(t, del(krw), nil, http.StatusNotFound)
	})

	t.Run("filters narrow the rows but not the net worth", func(t *testing.T) {
		got := body(t, "/finance/snapshots?month=2025-01&type=deposit")
		if strings.Contains(got, "<td>Brokerage</td>") || !strings.Contains(got, `<th scope="row">Total</th>`) {
			t.Errorf("deposit filter does not show only Savings with a total:\n%s", got)
		}
		wantContains(t, "/finance/snapshots?month=2025-01&type=deposit", "USD 24,000.00", "KRW 20,000,000")
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

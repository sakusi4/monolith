package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestExpenses(t *testing.T) {
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
	add := func(t *testing.T, date, name, category, currency, amount string) {
		t.Helper()
		month := date[:len("2006-01")]
		form := url.Values{"date": {date}, "name": {name}, "category": {category}, "currency": {currency}, "amount": {amount}}
		wantRedirect(t, s.post(t, "/finance/expenses/"+month+"/items/new", form, session), "/finance/expenses?month="+month)
	}
	expenseID := func(t *testing.T, month, name, currency string) string {
		t.Helper()
		var id int64
		query := `SELECT id FROM expenses WHERE date_trunc('month', spent_on) = $1::date AND name = $2 AND currency = $3`
		if err := s.db.QueryRowContext(t.Context(), query, month+"-01", name, currency).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return strconv.FormatInt(id, 10)
	}

	t.Run("the sidebar links the empty current month, which offers no copy", func(t *testing.T) {
		got := body(t, "/finance/expenses")
		for _, want := range []string{`href="/finance/expenses" aria-current="page"`, "<h1>Expenses</h1>", `<h2 id="records">Records</h2>`, "No expenses in ", "Add to "} {
			if !strings.Contains(got, want) {
				t.Errorf("GET /finance/expenses does not contain %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "Copy") || strings.Contains(got, `<h2 id="trend">`) {
			t.Errorf("GET /finance/expenses offers to copy a month or shows a trend without expenses:\n%s", got)
		}
		i := strings.Index(got, "<legend>Add to")
		if i < 0 {
			t.Fatalf("GET /finance/expenses has no add form:\n%s", got)
		}
		form := got[i:]
		if d, c, n := strings.Index(form, "<label>Date"), strings.Index(form, "<label>Category"), strings.Index(form, "<label>Name"); d < 0 || d > c || c > n {
			t.Errorf("add form fields are not in the order Date, Category, Name:\n%s", form)
		}
	})

	if _, err := s.db.ExecContext(t.Context(), `
		INSERT INTO exchange_rates (month, currency, per_usd)
		VALUES ('2024-12-01', 'KRW', 1000), ('2024-12-01', 'AED', 4)`); err != nil {
		t.Fatal(err)
	}

	t.Run("expenses are valued in USD and a refund lowers the total", func(t *testing.T) {
		add(t, "2025-01-01", "Rent", "housing", "AED", "2,000.00")
		add(t, "2025-01-15", "Groceries", "food", "KRW", "300,000")
		add(t, "2025-01-20", "Netflix", "bills", "USD", "15.49")
		add(t, "2025-01-31", "Refund", "other", "USD", "-5.49")
		wantContains(t, "/finance/expenses?month=2025-01",
			"<th>Date</th><th>Category</th><th>Name</th>", "<td>Housing</td>", "<td>Food</td>", "<td>Jan 15</td>", "<td>Jan 31</td>", "AED 2,000.00", "USD 500.00", "KRW 300,000", "USD 300.00", "USD -5.49", "USD 810.00",
			"1 USD = KRW 1,000 (Dec 2024) · AED 4 (Dec 2024)")
	})

	t.Run("the table has no total row", func(t *testing.T) {
		if got := body(t, "/finance/expenses?month=2025-01"); strings.Contains(got, `<th scope="row">Total</th>`) {
			t.Errorf("GET 2025-01 shows a total row under the table:\n%s", got)
		}
	})

	t.Run("a name can repeat with its own currency and earlier names are suggested", func(t *testing.T) {
		add(t, "2025-02-03", "Groceries", "food", "KRW", "100,000")
		add(t, "2025-02-17", "Groceries", "food", "AED", "40.00")
		got := body(t, "/finance/expenses?month=2025-02")
		for _, want := range []string{"KRW 100,000", "AED 40.00", "USD 110.00", `<option value="Rent" data-category="housing" data-currency="AED">`} {
			if !strings.Contains(got, want) {
				t.Errorf("GET 2025-02 does not contain %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, `<option value="Groceries"`) {
			t.Errorf("GET 2025-02 suggests Groceries, which the month already has")
		}
	})

	t.Run("the trend charts each month's total, oldest first, whatever month is shown", func(t *testing.T) {
		wantContains(t, "/finance/expenses?month=2025-03", `<h2 id="trend">Trend</h2>`,
			`<script type="application/json" id="spending-data">{"labels":["Jan 2025","Feb 2025"],"totalCents":[81000,11000]}</script>`)
	})

	t.Run("invalid expenses show the form again", func(t *testing.T) {
		for _, form := range []url.Values{
			{"date": {"2025-01-05"}, "name": {"  "}, "category": {"other"}, "currency": {"USD"}, "amount": {"1"}},
			{"date": {"2025-01-05"}, "name": {"Gym"}, "category": {"sports"}, "currency": {"USD"}, "amount": {"1"}},
			{"date": {"2025-01-05"}, "name": {"Gym"}, "category": {"other"}, "currency": {"EUR"}, "amount": {"1"}},
			{"date": {"2025-01-05"}, "name": {"Gym"}, "category": {"other"}, "currency": {"KRW"}, "amount": {"1.5"}},
			{"date": {"2025-02-01"}, "name": {"Gym"}, "category": {"other"}, "currency": {"USD"}, "amount": {"1"}},
			{"date": {"2025-01-32"}, "name": {"Gym"}, "category": {"other"}, "currency": {"USD"}, "amount": {"1"}},
			{"name": {"Gym"}, "category": {"other"}, "currency": {"USD"}, "amount": {"1"}},
		} {
			wantStatus(t, "/finance/expenses/2025-01/items/new", form, http.StatusUnprocessableEntity)
		}
	})

	t.Run("the add form starts on the month's first day outside the current month", func(t *testing.T) {
		wantContains(t, "/finance/expenses?month=2025-03", `name="date" value="2025-03-01" min="2025-03-01" max="2025-03-31"`)
	})

	t.Run("expenses are not copied from another month", func(t *testing.T) {
		wantStatus(t, "/finance/expenses/2025-04/copy", nil, http.StatusNotFound)
		add(t, "2025-04-02", "Groceries", "food", "KRW", "100,000")
		add(t, "2025-04-09", "Groceries", "food", "AED", "40.00")
	})

	t.Run("edit changes the name, currency, and amount of its month only", func(t *testing.T) {
		id := expenseID(t, "2025-04", "Groceries", "AED")
		edit := "/finance/expenses/2025-04/items/" + id + "/edit"
		wantContains(t, edit, `form="edit-item"`, `value="40.00"`, `value="Groceries"`, `<option value="food" selected>`, `value="2025-04-09"`)
		wantRedirect(t, s.post(t, edit, url.Values{"date": {"2025-04-30"}, "name": {"Dining"}, "category": {"travel"}, "currency": {"USD"}, "amount": {"12.00"}}, session), "/finance/expenses?month=2025-04")
		wantContains(t, "/finance/expenses?month=2025-04", "Dining", "<td>Travel</td>", "<td>Apr 30</td>", "USD 12.00")
		wantStatus(t, edit, url.Values{"date": {"2025-05-01"}, "name": {"Dining"}, "category": {"travel"}, "currency": {"USD"}, "amount": {"12.00"}}, http.StatusUnprocessableEntity)
		wantContains(t, "/finance/expenses?month=2025-02", "AED 40.00")
		wantStatus(t, edit, url.Values{"date": {"2025-04-30"}, "name": {"Dining"}, "category": {"travel"}, "currency": {"KRW"}, "amount": {"1.5"}}, http.StatusUnprocessableEntity)
		wantStatus(t, edit, url.Values{"date": {"2025-04-30"}, "name": {" "}, "category": {"travel"}, "currency": {"USD"}, "amount": {"1"}}, http.StatusUnprocessableEntity)
		wantStatus(t, edit, url.Values{"date": {"2025-04-30"}, "name": {"Dining"}, "category": {"sports"}, "currency": {"USD"}, "amount": {"1"}}, http.StatusUnprocessableEntity)
		other := "/finance/expenses/2025-02/items/" + id + "/edit"
		if rec := s.get(t, other, session); rec.Code != http.StatusNotFound {
			t.Errorf("GET edit of an expense outside the month = %d, want 404", rec.Code)
		}
		wantStatus(t, other, url.Values{"date": {"2025-02-10"}, "name": {"Dining"}, "category": {"travel"}, "currency": {"USD"}, "amount": {"1"}}, http.StatusNotFound)
	})

	t.Run("delete removes only the row", func(t *testing.T) {
		id := expenseID(t, "2025-04", "Dining", "USD")
		del := "/finance/expenses/2025-04/items/" + id + "/delete"
		wantRedirect(t, s.post(t, del, nil, session), "/finance/expenses?month=2025-04")
		got := body(t, "/finance/expenses?month=2025-04")
		if strings.Contains(got, "Dining") || !strings.Contains(got, "KRW 100,000") {
			t.Errorf("GET 2025-04 after delete still shows Dining or lost the other row:\n%s", got)
		}
		wantStatus(t, del, nil, http.StatusNotFound)
	})

	t.Run("months outside the range are rejected", func(t *testing.T) {
		for path, want := range map[string]int{
			"/finance/expenses?month=2024-01":        http.StatusNotFound,
			"/finance/expenses?month=2099-01":        http.StatusNotFound,
			"/finance/expenses?month=january":        http.StatusBadRequest,
			"/finance/expenses/2099-01/items/1/edit": http.StatusNotFound,
		} {
			if rec := s.get(t, path, session); rec.Code != want {
				t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
			}
		}
		wantStatus(t, "/finance/expenses/2099-01/items/new", url.Values{"date": {"2099-01-01"}, "name": {"x"}, "category": {"other"}, "currency": {"USD"}, "amount": {"1"}}, http.StatusNotFound)
	})
}

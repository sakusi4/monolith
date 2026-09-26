package finance

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

type expenseListPage struct {
	Chart       spendingChart
	Month       time.Time
	MonthOpts   web.Filter
	HasExpenses bool
	Summary     expenseSummary
	Rates       []appliedRate
	Filters     web.FilterBar
	Rows        []expenseListRow
	Pager       expensePager
	Edit        itemForm
	Add         itemForm
	Suggestions []expenseSuggestion
	Categories  []ExpenseCategory
	DateMin     string
	DateMax     string
	Currencies  []money.Currency
	Error       string
}

type expenseListRow struct {
	Row       expenseRow
	Editing   bool
	EditURL   string
	DeleteURL string
}

func (h *handler) listExpenses(w http.ResponseWriter, r *http.Request) {
	q, err := parseExpenseQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid filter.", http.StatusBadRequest)
		return
	}
	h.renderExpenses(w, r, http.StatusOK, q, listView{})
}

// renderExpenses shows the expenses of q.Month, or of the current month when it is zero.
func (h *handler) renderExpenses(w http.ResponseWriter, r *http.Request, status int, q expenseQuery, view listView) {
	months := h.months()
	if q.Month.IsZero() {
		q.Month = months[0]
	}
	month := q.Month
	if !slices.ContainsFunc(months, month.Equal) {
		http.NotFound(w, r)
		return
	}
	suggestions, err := h.store.expenseSuggestions(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	expenses, err := h.store.Expenses(r.Context(), month, month.AddDate(0, 1, 0))
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page, ok := newExpenseListPage(q, months, suggestions, expenses, defaultExpenseDate(time.Now(), h.loc, month), view)
	if !ok {
		http.NotFound(w, r)
		return
	}
	all, err := h.store.Expenses(r.Context(), firstMonth, months[0].AddDate(0, 1, 0))
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page.Chart = newSpendingChart(all)
	web.Render(w, r, status, "expense_list", page)
}

// newExpenseListPage starts the add form on date unless view has the user's input.
// It returns false when view edits an expense that is not on the page shown.
func newExpenseListPage(q expenseQuery, months []time.Time, suggestions []expenseSuggestion, expenses []Expense, date time.Time, view listView) (expenseListPage, bool) {
	month := q.Month
	matching := q.matching(expenses)
	rows, pager := q.paginate(q.sort(expenseRows(matching)))
	tableRows, edit, ok := editableExpenseRows(q, rows, view)
	if !ok {
		return expenseListPage{}, false
	}
	page := expenseListPage{
		Month:       month,
		MonthOpts:   monthFilter(months, month),
		HasExpenses: len(expenses) > 0,
		Filters:     web.FilterBar{Action: "/finance/expenses", Filters: q.filters()},
		Rows:        tableRows,
		Pager:       pager,
		Edit:        edit,
		Add:         view.Add,
		Suggestions: unusedExpenseSuggestions(suggestions, expenses),
		Categories:  expenseCategories,
		DateMin:     month.Format(time.DateOnly),
		DateMax:     month.AddDate(0, 1, -1).Format(time.DateOnly),
		Currencies:  money.Currencies,
		Error:       view.Error,
	}
	page.Add.URL = q.itemURL(month, "items/new")
	if !page.Add.Submitted {
		page.Add.Date = date.Format(time.DateOnly)
	}
	if len(expenses) > 0 {
		page.Summary = summarizeExpenses(matching)
		page.Rates = appliedRates(month, expenseRates(matching))
	}
	return page, true
}

// editableExpenseRows adds the row actions to rows and returns the form of the row that view
// edits: the input the user submitted, or else the row's current values. It returns false when
// view edits an expense that is not in rows.
func editableExpenseRows(q expenseQuery, rows []expenseRow, view listView) ([]expenseListRow, itemForm, bool) {
	var tableRows []expenseListRow
	edit := view.Edit
	found := view.EditID == 0
	for _, row := range rows {
		e := row.Expense
		id := strconv.FormatInt(e.ID, 10)
		lr := expenseListRow{
			Row:       row,
			Editing:   e.ID == view.EditID,
			EditURL:   q.itemURL(q.Month, "items/"+id+"/edit"),
			DeleteURL: q.itemURL(q.Month, "items/"+id+"/delete"),
		}
		if lr.Editing {
			found = true
			if !edit.Submitted {
				edit = itemForm{Date: e.Date.Format(time.DateOnly), Name: e.Name, Category: e.Category, Currency: e.Currency, Amount: money.Input(e.Currency, e.Amount)}
			}
			edit.URL, edit.CancelURL = lr.EditURL, q.listURL(q.Month)
		}
		tableRows = append(tableRows, lr)
	}
	return tableRows, edit, found
}

// unusedExpenseSuggestions leaves out the names that expenses already have.
func unusedExpenseSuggestions(suggestions []expenseSuggestion, expenses []Expense) []expenseSuggestion {
	var out []expenseSuggestion
	for _, sg := range suggestions {
		if !slices.ContainsFunc(expenses, func(e Expense) bool { return strings.EqualFold(e.Name, sg.Name) }) {
			out = append(out, sg)
		}
	}
	return out
}

func (h *handler) createExpense(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	form := itemForm{
		Date:      r.PostFormValue("date"),
		Name:      r.PostFormValue("name"),
		Category:  ExpenseCategory(r.PostFormValue("category")),
		Currency:  money.Currency(r.PostFormValue("currency")),
		Amount:    r.PostFormValue("amount"),
		Submitted: true,
	}
	in, problem := expenseInput(form, month)
	if problem != "" {
		form.Error = problem
		h.renderExpenses(w, r, http.StatusUnprocessableEntity, q, listView{Add: form})
		return
	}
	err := h.store.AddExpense(r.Context(), in)
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name, category, and currency."
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, q.listURL(month), http.StatusSeeOther)
		return
	}
	h.renderExpenses(w, r, http.StatusUnprocessableEntity, q, listView{Add: form})
}

func (h *handler) editExpense(w http.ResponseWriter, r *http.Request) {
	q, _, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderExpenses(w, r, http.StatusOK, q, listView{EditID: id})
}

func (h *handler) updateExpense(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	form := itemForm{
		Date:      r.PostFormValue("date"),
		Name:      r.PostFormValue("name"),
		Category:  ExpenseCategory(r.PostFormValue("category")),
		Currency:  money.Currency(r.PostFormValue("currency")),
		Amount:    r.PostFormValue("amount"),
		Submitted: true,
	}
	view := listView{EditID: id}
	in, problem := expenseInput(form, month)
	if problem != "" {
		form.Error = problem
		view.Edit = form
		h.renderExpenses(w, r, http.StatusUnprocessableEntity, q, view)
		return
	}
	err := h.store.UpdateExpense(r.Context(), month, id, in)
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name, category, and currency."
	case errors.Is(err, ErrItemNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, q.listURL(month), http.StatusSeeOther)
		return
	}
	view.Edit = form
	h.renderExpenses(w, r, http.StatusUnprocessableEntity, q, view)
}

func (h *handler) deleteExpense(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.store.DeleteExpense(r.Context(), month, id)
	switch {
	case errors.Is(err, ErrItemNotFound):
		http.NotFound(w, r)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, q.listURL(month), http.StatusSeeOther)
	}
}

// expenseInput parses form for an expense in month. It returns the problem to show instead when
// the amount or the date is invalid.
func expenseInput(form itemForm, month time.Time) (ExpenseInput, string) {
	amount, ok := money.Parse(form.Currency, form.Amount)
	if !ok {
		return ExpenseInput{}, amountExample
	}
	date, err := time.Parse(time.DateOnly, form.Date)
	if err != nil || !inMonth(date, month) {
		return ExpenseInput{}, fmt.Sprintf("Pick a date in %s.", month.Format(monthLabelLayout))
	}
	return ExpenseInput{Date: date, Name: form.Name, Category: form.Category, Currency: form.Currency, Amount: amount}, ""
}

// expenseRequest reads the list filters and the month of a request on a month's expenses.
// It writes the error response and returns false when either is invalid.
func (h *handler) expenseRequest(w http.ResponseWriter, r *http.Request) (expenseQuery, time.Time, bool) {
	q, err := parseExpenseQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid filter.", http.StatusBadRequest)
		return expenseQuery{}, time.Time{}, false
	}
	month, err := time.Parse(monthLayout, r.PathValue("month"))
	if err != nil || !h.hasMonth(month) {
		http.NotFound(w, r)
		return expenseQuery{}, time.Time{}, false
	}
	q.Month = month
	return q, month, true
}

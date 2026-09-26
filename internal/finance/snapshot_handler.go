package finance

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

const (
	monthLayout   = "2006-01"
	amountExample = "Enter the amount like -1,234.56. KRW and JPY have no decimals."
)

type snapshotListPage struct {
	Month       time.Time
	Snapshot    Snapshot
	Summary     summary
	Rates       []appliedRate
	NoteURL     string
	MonthOpts   web.Filter
	Filters     web.FilterBar
	Rows        []snapshotRow
	Chart       netWorthChart
	Edit        itemForm
	Add         itemForm
	Suggestions []itemSuggestion
	Types       []AssetType
	Currencies  []money.Currency
	CopyURL     string
	Error       string
}

type snapshotRow struct {
	Row       assetRow
	Editing   bool
	EditURL   string
	DeleteURL string
}

// itemForm holds a row's form values as the user typed them, with the error to show next to them.
// Type belongs to snapshot rows, and Category and Date to expenses.
type itemForm struct {
	Name      string
	Type      AssetType
	Category  ExpenseCategory
	Date      string
	Currency  money.Currency
	Amount    string
	Error     string
	Submitted bool
	URL       string
	CancelURL string
}

// listView is what a request adds to a month's list of snapshot items or expenses: a row being edited, rejected input, or an error.
type listView struct {
	EditID int64
	Edit   itemForm
	Add    itemForm
	Error  string
}

func (h *handler) listSnapshots(w http.ResponseWriter, r *http.Request) {
	q, err := parseAssetQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid filter.", http.StatusBadRequest)
		return
	}
	h.renderList(w, r, http.StatusOK, q, listView{})
}

func (h *handler) renderList(w http.ResponseWriter, r *http.Request, status int, q assetQuery, view listView) {
	months := h.months()
	if q.Month.IsZero() {
		q.Month = months[0]
	}
	if !slices.ContainsFunc(months, q.Month.Equal) {
		http.NotFound(w, r)
		return
	}
	suggestions, err := h.store.itemSuggestions(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	snapshot, err := h.store.Snapshot(r.Context(), q.Month)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page, ok := newSnapshotListPage(q, months, suggestions, snapshot, view)
	if !ok {
		http.NotFound(w, r)
		return
	}
	snapshots, err := h.store.Snapshots(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page.Chart = newNetWorthChart(snapshots)
	web.Render(w, r, status, "snapshot_list", page)
}

// newSnapshotListPage returns false when view edits a row that is not in q.Month.
func newSnapshotListPage(q assetQuery, months []time.Time, suggestions []itemSuggestion, cur Snapshot, view listView) (snapshotListPage, bool) {
	rows := q.apply(assetRows(cur))
	tableRows, edit, ok := editableRows(q, rows, view)
	if !ok {
		return snapshotListPage{}, false
	}
	page := snapshotListPage{
		Month:       q.Month,
		Snapshot:    cur,
		MonthOpts:   monthFilter(months, q.Month),
		Filters:     web.FilterBar{Action: "/finance/assets", Filters: q.filters()},
		Rows:        tableRows,
		Edit:        edit,
		Add:         view.Add,
		Suggestions: unusedSuggestions(suggestions, cur),
		Types:       assetTypes,
		Currencies:  money.Currencies,
		Error:       view.Error,
	}
	page.Add.URL = q.itemURL(q.Month, "items/new")
	if cur.ID != 0 {
		page.Summary = summarize(cur)
		page.Rates = appliedRates(cur.Month, snapshotRates(cur))
		page.NoteURL = q.itemURL(q.Month, "note")
	} else {
		page.CopyURL = q.itemURL(q.Month, "copy")
	}
	return page, true
}

// editableRows adds the row actions to rows and returns the form of the row that view edits:
// the input the user submitted, or else the row's current values. It returns false when
// view edits a row that is not in rows.
func editableRows(q assetQuery, rows []assetRow, view listView) ([]snapshotRow, itemForm, bool) {
	var tableRows []snapshotRow
	edit := view.Edit
	found := view.EditID == 0
	for _, row := range rows {
		id := strconv.FormatInt(row.Item.ID, 10)
		sr := snapshotRow{
			Row:       row,
			Editing:   row.Item.ID == view.EditID,
			EditURL:   q.itemURL(q.Month, "items/"+id+"/edit"),
			DeleteURL: q.itemURL(q.Month, "items/"+id+"/delete"),
		}
		if sr.Editing {
			found = true
			if !edit.Submitted {
				it := row.Item
				edit = itemForm{Name: it.Name, Type: it.Type, Currency: it.Currency, Amount: money.Input(it.Currency, it.Amount)}
			}
			edit.URL, edit.CancelURL = sr.EditURL, q.listURL(q.Month)
		}
		tableRows = append(tableRows, sr)
	}
	return tableRows, edit, found
}

// unusedSuggestions leaves out the names that s already has.
func unusedSuggestions(suggestions []itemSuggestion, s Snapshot) []itemSuggestion {
	var out []itemSuggestion
	for _, sg := range suggestions {
		if !slices.ContainsFunc(s.Items, func(it SnapshotItem) bool { return strings.EqualFold(it.Name, sg.Name) }) {
			out = append(out, sg)
		}
	}
	return out
}

func (h *handler) createItem(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.itemRequest(w, r)
	if !ok {
		return
	}
	form := itemForm{
		Name:      r.PostFormValue("name"),
		Type:      AssetType(r.PostFormValue("type")),
		Currency:  money.Currency(r.PostFormValue("currency")),
		Amount:    r.PostFormValue("amount"),
		Submitted: true,
	}
	amount, ok := money.Parse(form.Currency, form.Amount)
	if !ok {
		form.Error = amountExample
		h.renderList(w, r, http.StatusUnprocessableEntity, q, listView{Add: form})
		return
	}
	err := h.store.AddItem(r.Context(), month, ItemInput{Name: form.Name, Type: form.Type, Currency: form.Currency, Amount: amount})
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name, type, and currency."
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, q.listURL(month), http.StatusSeeOther)
		return
	}
	h.renderList(w, r, http.StatusUnprocessableEntity, q, listView{Add: form})
}

func (h *handler) editItem(w http.ResponseWriter, r *http.Request) {
	q, _, ok := h.itemRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderList(w, r, http.StatusOK, q, listView{EditID: id})
}

func (h *handler) updateItem(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.itemRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	form := itemForm{
		Name:      r.PostFormValue("name"),
		Type:      AssetType(r.PostFormValue("type")),
		Currency:  money.Currency(r.PostFormValue("currency")),
		Amount:    r.PostFormValue("amount"),
		Submitted: true,
	}
	view := listView{EditID: id}
	amount, ok := money.Parse(form.Currency, form.Amount)
	if !ok {
		form.Error = amountExample
		view.Edit = form
		h.renderList(w, r, http.StatusUnprocessableEntity, q, view)
		return
	}
	err := h.store.UpdateItem(r.Context(), month, id, ItemInput{Name: form.Name, Type: form.Type, Currency: form.Currency, Amount: amount})
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name, type, and currency."
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
	h.renderList(w, r, http.StatusUnprocessableEntity, q, view)
}

func (h *handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.itemRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.store.DeleteItem(r.Context(), month, id)
	switch {
	case errors.Is(err, ErrItemNotFound):
		http.NotFound(w, r)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, q.listURL(month), http.StatusSeeOther)
	}
}

func (h *handler) copySnapshot(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.itemRequest(w, r)
	if !ok {
		return
	}
	err := h.store.CopyPreviousSnapshot(r.Context(), month)
	switch {
	case errors.Is(err, ErrMonthNotEmpty):
		h.renderList(w, r, http.StatusUnprocessableEntity, q, listView{Error: "This month already has assets."})
	case errors.Is(err, ErrNothingToCopy):
		h.renderList(w, r, http.StatusUnprocessableEntity, q, listView{Error: "There is no earlier month to copy."})
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, q.listURL(month), http.StatusSeeOther)
	}
}

func (h *handler) updateNote(w http.ResponseWriter, r *http.Request) {
	q, month, ok := h.itemRequest(w, r)
	if !ok {
		return
	}
	err := h.store.SetNote(r.Context(), month, r.PostFormValue("note"))
	switch {
	case errors.Is(err, ErrSnapshotNotFound):
		http.NotFound(w, r)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, q.listURL(month), http.StatusSeeOther)
	}
}

// itemRequest reads the list filters and the month of a request on a month's snapshot.
// It writes the error response and returns false when either is invalid.
func (h *handler) itemRequest(w http.ResponseWriter, r *http.Request) (assetQuery, time.Time, bool) {
	q, err := parseAssetQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid filter.", http.StatusBadRequest)
		return assetQuery{}, time.Time{}, false
	}
	month, err := time.Parse(monthLayout, r.PathValue("month"))
	if err != nil || !h.hasMonth(month) {
		http.NotFound(w, r)
		return assetQuery{}, time.Time{}, false
	}
	q.Month = month
	return q, month, true
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

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

const (
	monthLayout   = "2006-01"
	amountExample = "Enter the amount like -1,234.56. KRW and JPY have no decimals."
)

type snapshotListPage struct {
	Month      time.Time
	Snapshot   Snapshot
	Summary    summary
	Rates      []appliedRate
	NoteURL    string
	MonthOpts  web.Filter
	Filters    web.FilterBar
	Rows       []snapshotRow
	Total      assetTotal
	Edit       itemForm
	Add        itemForm
	Candidates []Asset
	Types      []AssetType
	Currencies []money.Currency
	Copy       copyOffer
	Error      string
}

type snapshotRow struct {
	Row       assetRow
	Editing   bool
	EditURL   string
	DeleteURL string
}

// itemForm holds a row's form values as the user typed them, with the error to show next to them.
// Existing marks an add form whose name is an existing asset, which fixes Type and Currency.
type itemForm struct {
	AssetID   int64
	Name      string
	Type      AssetType
	Currency  money.Currency
	Amount    string
	Existing  bool
	Error     string
	Submitted bool
	URL       string
	CancelURL string
}

type copyOffer struct {
	From  time.Time
	Count int
	URL   string
}

// listView is what a request adds to the snapshot list: a row being edited, rejected input, or an error.
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
	assets, err := h.store.Assets(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	snapshots, err := h.store.Snapshots(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page, ok := newSnapshotListPage(q, months, assets, snapshots, view)
	if !ok {
		http.NotFound(w, r)
		return
	}
	web.Render(w, r, status, "snapshot_list", page)
}

// newSnapshotListPage returns false when view edits an asset that is not in q.Month.
func newSnapshotListPage(q assetQuery, months []time.Time, assets []Asset, snapshots []Snapshot, view listView) (snapshotListPage, bool) {
	cur, prev := monthSnapshots(snapshots, q.Month)
	rows := q.apply(assetRows(cur))
	tableRows, edit, ok := editableRows(q, rows, view)
	if !ok {
		return snapshotListPage{}, false
	}
	page := snapshotListPage{
		Month:      q.Month,
		Snapshot:   cur,
		MonthOpts:  monthFilter(months, q.Month),
		Filters:    web.FilterBar{Action: "/finance/snapshots", Filters: q.filters()},
		Rows:       tableRows,
		Total:      totalOf(rows),
		Edit:       edit,
		Add:        view.Add,
		Candidates: candidates(assets, cur),
		Types:      assetTypes,
		Currencies: money.Currencies,
		Error:      view.Error,
	}
	page.Add.URL = q.itemURL(q.Month, "items/new")
	switch {
	case cur.ID != 0:
		page.Summary = summarize(cur, prev, prev.ID != 0)
		page.Rates = appliedRates(cur)
		page.NoteURL = q.itemURL(q.Month, "note")
	case prev.ID != 0:
		page.Copy = copyOffer{From: prev.Month, Count: len(prev.Items), URL: q.itemURL(q.Month, "copy")}
	}
	return page, true
}

// monthSnapshots finds the snapshot of month and the latest one before it in snapshots,
// which are newest first. A zero ID means there is none.
func monthSnapshots(snapshots []Snapshot, month time.Time) (cur, prev Snapshot) {
	for _, s := range snapshots {
		switch {
		case s.Month.Equal(month):
			cur = s
		case s.Month.Before(month) && prev.ID == 0:
			prev = s
		}
	}
	return cur, prev
}

// editableRows adds the row actions to rows and returns the form of the row that view edits:
// the input the user submitted, or else the row's current values. It returns false when
// view edits an asset that is not in rows.
func editableRows(q assetQuery, rows []assetRow, view listView) ([]snapshotRow, itemForm, bool) {
	var tableRows []snapshotRow
	edit := view.Edit
	found := view.EditID == 0
	for _, row := range rows {
		id := strconv.FormatInt(row.Asset.ID, 10)
		sr := snapshotRow{
			Row:       row,
			Editing:   row.Asset.ID == view.EditID,
			EditURL:   q.itemURL(q.Month, "items/"+id+"/edit"),
			DeleteURL: q.itemURL(q.Month, "items/"+id+"/delete"),
		}
		if sr.Editing {
			found = true
			if !edit.Submitted {
				edit = itemForm{Name: row.Asset.Name, Type: row.Asset.Type, Amount: money.Input(row.Asset.Currency, row.Amount)}
			}
			edit.AssetID, edit.Currency = row.Asset.ID, row.Asset.Currency
			edit.URL, edit.CancelURL = sr.EditURL, q.listURL(q.Month)
		}
		tableRows = append(tableRows, sr)
	}
	return tableRows, edit, found
}

// candidates lists the assets that s does not have yet, for the add form to suggest.
func candidates(assets []Asset, s Snapshot) []Asset {
	var out []Asset
	for _, a := range assets {
		if !slices.ContainsFunc(s.Items, func(it SnapshotItem) bool { return it.Asset.ID == a.ID }) {
			out = append(out, a)
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
	assets, err := h.store.Assets(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if a, ok := assetNamed(assets, form.Name); ok {
		form.Type, form.Currency, form.Existing = a.Type, a.Currency, true
	}
	amount, ok := money.Parse(form.Currency, form.Amount)
	if !ok {
		form.Error = amountExample
		h.renderList(w, r, http.StatusUnprocessableEntity, q, listView{Add: form})
		return
	}
	err = h.store.AddItem(r.Context(), month, ItemInput{Name: form.Name, Type: form.Type, Currency: form.Currency, Amount: amount})
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name, type, and currency."
	case errors.Is(err, ErrItemExists):
		form.Error = fmt.Sprintf("%s is already in %s.", strings.TrimSpace(form.Name), month.Format("Jan 2006"))
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
	assets, err := h.store.Assets(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	i := slices.IndexFunc(assets, func(a Asset) bool { return a.ID == id })
	if i < 0 {
		http.NotFound(w, r)
		return
	}
	form := itemForm{
		Name:      r.PostFormValue("name"),
		Type:      AssetType(r.PostFormValue("type")),
		Amount:    r.PostFormValue("amount"),
		Submitted: true,
	}
	view := listView{EditID: id}
	amount, ok := money.Parse(assets[i].Currency, form.Amount)
	if !ok {
		form.Error = amountExample
		view.Edit = form
		h.renderList(w, r, http.StatusUnprocessableEntity, q, view)
		return
	}
	err = h.store.UpdateItem(r.Context(), month, id, ItemUpdate{Name: form.Name, Type: form.Type, Amount: amount})
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name and type."
	case errors.Is(err, ErrNameTaken):
		form.Error = fmt.Sprintf("Another asset is already named %s.", strings.TrimSpace(form.Name))
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

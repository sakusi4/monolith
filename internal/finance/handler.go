package finance

import (
	"net/http"
	"slices"
	"time"
)

type handler struct {
	store *Store
	loc   *time.Location
}

// NewHandler serves the finance pages. loc decides which month "this month" is.
func NewHandler(store *Store, loc *time.Location) http.Handler {
	h := &handler{store: store, loc: loc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /finance/assets", h.listSnapshots)
	mux.HandleFunc("POST /finance/assets/{month}/items/new", h.createItem)
	mux.HandleFunc("GET /finance/assets/{month}/items/{id}/edit", h.editItem)
	mux.HandleFunc("POST /finance/assets/{month}/items/{id}/edit", h.updateItem)
	mux.HandleFunc("POST /finance/assets/{month}/items/{id}/delete", h.deleteItem)
	mux.HandleFunc("POST /finance/assets/{month}/copy", h.copySnapshot)
	mux.HandleFunc("POST /finance/assets/{month}/note", h.updateNote)
	mux.HandleFunc("GET /finance/expenses", h.listExpenses)
	mux.HandleFunc("POST /finance/expenses/{month}/items/new", h.createExpense)
	mux.HandleFunc("GET /finance/expenses/{month}/items/{id}/edit", h.editExpense)
	mux.HandleFunc("POST /finance/expenses/{month}/items/{id}/edit", h.updateExpense)
	mux.HandleFunc("POST /finance/expenses/{month}/items/{id}/delete", h.deleteExpense)
	return mux
}

// months lists the months that snapshots and expenses can be recorded for, newest first.
func (h *handler) months() []time.Time {
	return monthsUntil(currentMonth(time.Now(), h.loc))
}

func (h *handler) hasMonth(month time.Time) bool {
	return slices.ContainsFunc(h.months(), month.Equal)
}

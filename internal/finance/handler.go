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
	mux.HandleFunc("GET /finance/snapshots", h.listSnapshots)
	mux.HandleFunc("POST /finance/snapshots/{month}/items/new", h.createItem)
	mux.HandleFunc("GET /finance/snapshots/{month}/items/{id}/edit", h.editItem)
	mux.HandleFunc("POST /finance/snapshots/{month}/items/{id}/edit", h.updateItem)
	mux.HandleFunc("POST /finance/snapshots/{month}/items/{id}/delete", h.deleteItem)
	mux.HandleFunc("POST /finance/snapshots/{month}/copy", h.copySnapshot)
	mux.HandleFunc("POST /finance/snapshots/{month}/note", h.updateNote)
	return mux
}

// months lists the months a snapshot can have, newest first.
func (h *handler) months() []time.Time {
	return monthsUntil(currentMonth(time.Now(), h.loc))
}

func (h *handler) hasMonth(month time.Time) bool {
	return slices.ContainsFunc(h.months(), month.Equal)
}

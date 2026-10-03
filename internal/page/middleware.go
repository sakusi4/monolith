package page

import (
	"net/http"

	"github.com/sakusi4/monolith/web"
)

// Sidebar adds the pages of their own at the top level to each request, for the sidebar of the
// Pages area to list.
func Sidebar(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entries, err := store.TopPages(r.Context())
			if err != nil {
				web.ServerError(w, r, err)
				return
			}
			links := make([]web.Link, len(entries))
			for i, e := range entries {
				links[i] = web.Link{Title: e.Title, URL: e.URL}
			}
			next.ServeHTTP(w, r.WithContext(web.WithSidebarPages(r.Context(), links)))
		})
	}
}

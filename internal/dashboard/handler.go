package dashboard

import (
	"net/http"

	"github.com/sakusi4/monolith/web"
)

func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dashboard", showDashboard)
	return mux
}

func showDashboard(w http.ResponseWriter, r *http.Request) {
	web.Render(w, r, http.StatusOK, "dashboard", nil)
}

package main

import (
	"database/sql"
	"net/http"

	"github.com/sakusi4/monolith/internal/auth"
	"github.com/sakusi4/monolith/internal/finance"
	"github.com/sakusi4/monolith/web"
)

func routes(db *sql.DB) http.Handler {
	authStore := auth.NewStore(db)
	requireAuth := auth.Require(authStore)

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/finance/assets", http.StatusSeeOther))
	mux.Handle("GET /static/", web.Static())
	mux.Handle("/auth/", auth.NewHandler(authStore))
	mux.Handle("/finance/", requireAuth(finance.NewHandler(finance.NewStore(db))))
	return http.NewCrossOriginProtection().Handler(mux)
}

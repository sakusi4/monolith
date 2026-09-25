package main

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/sakusi4/monolith/internal/auth"
	"github.com/sakusi4/monolith/internal/dashboard"
	"github.com/sakusi4/monolith/internal/finance"
	"github.com/sakusi4/monolith/web"
)

func routes(db *sql.DB, loc *time.Location) http.Handler {
	authStore := auth.NewStore(db)
	requireAuth := auth.Require(authStore)
	financeStore := finance.NewStore(db)

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/dashboard", http.StatusSeeOther))
	mux.Handle("GET /static/", web.Static())
	mux.Handle("/auth/", auth.NewHandler(authStore))
	mux.Handle("/dashboard", requireAuth(dashboard.NewHandler(financeStore)))
	mux.Handle("/finance/", requireAuth(finance.NewHandler(financeStore, loc)))
	return http.NewCrossOriginProtection().Handler(mux)
}

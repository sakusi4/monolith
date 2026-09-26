package main

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/sakusi4/monolith/internal/auth"
	"github.com/sakusi4/monolith/internal/dashboard"
	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/internal/finance"
	"github.com/sakusi4/monolith/web"
)

func routes(db *sql.DB, loc *time.Location, driveStore *drive.Store, maxUpload int64) http.Handler {
	authStore := auth.NewStore(db)
	requireAuth := auth.Require(authStore)
	financeStore := finance.NewStore(db)
	driveHandler := requireAuth(drive.NewHandler(driveStore, loc, maxUpload))

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/dashboard", http.StatusSeeOther))
	mux.Handle("GET /static/", web.Static())
	mux.Handle("/auth/", auth.NewHandler(authStore))
	mux.Handle("/dashboard", requireAuth(dashboard.NewHandler()))
	mux.Handle("/drive", driveHandler)
	mux.Handle("/drive/", driveHandler)
	mux.Handle("/finance/", requireAuth(finance.NewHandler(financeStore, loc)))
	return http.NewCrossOriginProtection().Handler(mux)
}

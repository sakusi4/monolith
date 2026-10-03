package main

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/sakusi4/monolith/internal/auth"
	"github.com/sakusi4/monolith/internal/dashboard"
	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/internal/finance"
	"github.com/sakusi4/monolith/internal/page"
	"github.com/sakusi4/monolith/internal/task"
	"github.com/sakusi4/monolith/web"
)

func routes(db *sql.DB, loc *time.Location, driveStore *drive.Store, maxUpload int64) http.Handler {
	authStore := auth.NewStore(db)
	pageStore := page.NewStore(db, driveStore)
	requireAuth := auth.Require(authStore)
	sidebar := page.Sidebar(pageStore)
	pagesArea := func(h http.Handler) http.Handler { return requireAuth(sidebar(h)) }
	financeStore := finance.NewStore(db)
	driveHandler := requireAuth(drive.NewHandler(driveStore, loc, maxUpload))
	taskStore := task.NewStore(db, driveStore, pageStore)
	pageHandler := pagesArea(page.NewHandler(pageStore, loc, maxUpload))

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/dashboard", http.StatusSeeOther))
	mux.Handle("GET /static/", web.Static())
	mux.Handle("/auth/", auth.NewHandler(authStore))
	mux.Handle("/dashboard", requireAuth(dashboard.NewHandler()))
	mux.Handle("/drive", driveHandler)
	mux.Handle("/drive/", driveHandler)
	mux.Handle("/page", pageHandler)
	mux.Handle("/page/", pageHandler)
	mux.Handle("/task/", pagesArea(task.NewHandler(taskStore, loc, maxUpload)))
	mux.Handle("/finance/", requireAuth(finance.NewHandler(financeStore, loc)))
	return http.NewCrossOriginProtection().Handler(mux)
}

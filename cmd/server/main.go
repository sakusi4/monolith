package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sakusi4/monolith/internal/auth"
	"github.com/sakusi4/monolith/internal/postgres"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	db, err := postgres.Open(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := postgres.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	if cfg.adminEmail != "" {
		if err := auth.NewStore(db).SetUser(ctx, cfg.adminEmail, cfg.adminPassword); err != nil {
			return fmt.Errorf("set admin user: %w", err)
		}
	}

	return serve(ctx, &http.Server{
		Addr:              cfg.listenAddr,
		Handler:           routes(db),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	})
}

func serve(ctx context.Context, srv *http.Server) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.ListenAndServe()
	}()
	slog.InfoContext(ctx, "server started", slog.String("addr", srv.Addr))

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	<-serveErr
	return nil
}

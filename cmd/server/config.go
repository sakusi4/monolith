package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	defaultListenAddr = ":8080"
	defaultTimezone   = "UTC"
)

type config struct {
	listenAddr    string
	databaseURL   string
	adminEmail    string
	adminPassword string
	location      *time.Location
}

func loadConfig() (config, error) {
	cfg := config{
		listenAddr:    cmp.Or(os.Getenv("LISTEN_ADDR"), defaultListenAddr),
		databaseURL:   os.Getenv("DATABASE_URL"),
		adminEmail:    os.Getenv("ADMIN_EMAIL"),
		adminPassword: os.Getenv("ADMIN_PASSWORD"),
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is not set")
	}
	if (cfg.adminEmail == "") != (cfg.adminPassword == "") {
		return config{}, errors.New("ADMIN_EMAIL and ADMIN_PASSWORD must be set together")
	}
	loc, err := time.LoadLocation(cmp.Or(os.Getenv("TIMEZONE"), defaultTimezone))
	if err != nil {
		return config{}, fmt.Errorf("TIMEZONE: %w", err)
	}
	cfg.location = loc
	return cfg, nil
}

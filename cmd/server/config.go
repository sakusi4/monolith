package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultListenAddr = ":8080"
	defaultTimezone   = "UTC"

	defaultFilesDir    = "files_data"
	defaultMaxUploadGB = 10
	bytesPerGB         = 1 << 30
)

type config struct {
	listenAddr    string
	databaseURL   string
	adminEmail    string
	adminPassword string
	location      *time.Location
	filesDir      string
	maxUpload     int64
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
	cfg.filesDir = cmp.Or(os.Getenv("FILES_DIR"), defaultFilesDir)
	maxUploadGB := int64(defaultMaxUploadGB)
	if v := os.Getenv("DRIVE_MAX_UPLOAD_GB"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			return config{}, fmt.Errorf("DRIVE_MAX_UPLOAD_GB: %q is not a positive number", v)
		}
		maxUploadGB = n
	}
	cfg.maxUpload = maxUploadGB * bytesPerGB
	return cfg, nil
}

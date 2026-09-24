package postgrestest

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"testing"

	"github.com/sakusi4/monolith/internal/postgres"
)

func New(t *testing.T) *sql.DB {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}

	admin, err := postgres.Open(t.Context(), base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})

	name := fmt.Sprintf("test_%d", rand.Uint64())
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})

	u.Path = "/" + name
	db, err := postgres.Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	if err := postgres.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

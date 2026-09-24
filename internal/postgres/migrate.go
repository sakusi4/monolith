package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Migrate(ctx context.Context, db *sql.DB) error {
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	current, err := schemaVersion(ctx, db)
	if err != nil {
		return err
	}
	if current > len(names) {
		return fmt.Errorf("database schema version %d is newer than latest migration %d", current, len(names))
	}
	for i, name := range names {
		version := i + 1
		if want := fmt.Sprintf("migrations/%04d.sql", version); name != want {
			return fmt.Errorf("migration %s: want %s", name, want)
		}
		if version <= current {
			continue
		}
		if err := apply(ctx, db, name, version); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func schemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('schema_version') IS NOT NULL`).Scan(&exists); err != nil {
		return 0, fmt.Errorf("check schema_version: %w", err)
	}
	if !exists {
		return 0, nil
	}
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func apply(ctx context.Context, db *sql.DB, name string, version int) error {
	query, err := migrations.ReadFile(name)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(query)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

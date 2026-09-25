package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

var ErrInvalidItem = errors.New("invalid item")

var ErrItemNotFound = errors.New("item not found")

var ErrNothingToCopy = errors.New("no earlier month to copy")

var ErrMonthNotEmpty = errors.New("month is not empty")

// ItemInput is one row of a month's snapshot, as added or edited.
type ItemInput struct {
	Name     string
	Type     AssetType
	Currency money.Currency
	Amount   int64
}

func (in ItemInput) Clean() (ItemInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		return ItemInput{}, fmt.Errorf("%w: name is empty", ErrInvalidItem)
	case !in.Type.valid():
		return ItemInput{}, fmt.Errorf("%w: unknown type %q", ErrInvalidItem, in.Type)
	case !in.Currency.IsValid():
		return ItemInput{}, fmt.Errorf("%w: unknown currency %q", ErrInvalidItem, in.Currency)
	}
	return in, nil
}

// AddItem adds in to month, creating the month's snapshot when it does not exist.
func (s *Store) AddItem(ctx context.Context, month time.Time, in ItemInput) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	var snapshotID int64
	query := `
		INSERT INTO snapshots (month) VALUES ($1)
		ON CONFLICT (month) DO UPDATE SET updated_at = now()
		RETURNING id`
	if err := tx.QueryRowContext(ctx, query, month).Scan(&snapshotID); err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	query = `INSERT INTO snapshot_items (snapshot_id, name, type, currency, amount) VALUES ($1, $2, $3, $4, $5)`
	if _, err := tx.ExecContext(ctx, query, snapshotID, in.Name, in.Type, in.Currency, in.Amount); err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// UpdateItem replaces the row id of month. It returns ErrItemNotFound when month has no such row.
func (s *Store) UpdateItem(ctx context.Context, month time.Time, id int64, in ItemInput) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	query := `
		UPDATE snapshot_items i SET name = $3, type = $4, currency = $5, amount = $6
		FROM snapshots s
		WHERE i.snapshot_id = s.id AND s.month = $1 AND i.id = $2`
	res, err := s.db.ExecContext(ctx, query, month, id, in.Name, in.Type, in.Currency, in.Amount)
	if err != nil {
		return fmt.Errorf("update item: %w", err)
	}
	return requireRow(res, ErrItemNotFound)
}

// DeleteItem removes the row id from month, and the month's snapshot with its note when it
// becomes empty. It returns ErrItemNotFound when month has no such row.
func (s *Store) DeleteItem(ctx context.Context, month time.Time, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	query := `
		DELETE FROM snapshot_items i USING snapshots s
		WHERE i.snapshot_id = s.id AND s.month = $1 AND i.id = $2`
	res, err := tx.ExecContext(ctx, query, month, id)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	if err := requireRow(res, ErrItemNotFound); err != nil {
		return err
	}
	query = `
		DELETE FROM snapshots s WHERE s.month = $1
		AND NOT EXISTS (SELECT 1 FROM snapshot_items i WHERE i.snapshot_id = s.id)`
	if _, err := tx.ExecContext(ctx, query, month); err != nil {
		return fmt.Errorf("delete empty snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// CopyPreviousSnapshot fills an empty month with copies of the rows of the latest earlier snapshot.
// It returns ErrMonthNotEmpty or ErrNothingToCopy when it cannot.
func (s *Store) CopyPreviousSnapshot(ctx context.Context, month time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	var snapshotID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO snapshots (month) VALUES ($1) RETURNING id`, month).Scan(&snapshotID)
	if isPgError(err, uniqueViolation) {
		return ErrMonthNotEmpty
	}
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	query := `
		INSERT INTO snapshot_items (snapshot_id, name, type, currency, amount)
		SELECT $1, i.name, i.type, i.currency, i.amount
		FROM snapshot_items i
		WHERE i.snapshot_id = (SELECT id FROM snapshots WHERE month < $2 ORDER BY month DESC LIMIT 1)
		ORDER BY i.id`
	res, err := tx.ExecContext(ctx, query, snapshotID, month)
	if err != nil {
		return fmt.Errorf("copy items: %w", err)
	}
	if err := requireRow(res, ErrNothingToCopy); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func requireRow(res sql.Result, errNone error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return errNone
	}
	return nil
}

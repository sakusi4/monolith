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

var ErrItemExists = errors.New("asset is already in the month")

var ErrNameTaken = errors.New("asset name is taken")

var ErrNothingToCopy = errors.New("no earlier snapshot to copy")

var ErrMonthNotEmpty = errors.New("month already has a snapshot")

// ItemInput adds an asset to a month. Type and Currency apply only when Name is a new asset.
type ItemInput struct {
	Name     string
	Type     AssetType
	Currency money.Currency
	Amount   int64
}

// ItemUpdate changes an item of a month. Name and Type belong to the asset, so they change it in every month.
type ItemUpdate struct {
	Name   string
	Type   AssetType
	Amount int64
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

func (in ItemUpdate) Clean() (ItemUpdate, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		return ItemUpdate{}, fmt.Errorf("%w: name is empty", ErrInvalidItem)
	case !in.Type.valid():
		return ItemUpdate{}, fmt.Errorf("%w: unknown type %q", ErrInvalidItem, in.Type)
	}
	return in, nil
}

// AddItem records in for month, creating the month's snapshot and the asset when they do not exist.
// It returns ErrItemExists when the asset is already in the month.
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
	var assetID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM assets WHERE lower(name) = lower($1)`, in.Name).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		query := `INSERT INTO assets (type, name, currency) VALUES ($1, $2, $3) RETURNING id`
		err = tx.QueryRowContext(ctx, query, in.Type, in.Name, in.Currency).Scan(&assetID)
	}
	if err != nil {
		return fmt.Errorf("find or insert asset: %w", err)
	}
	var snapshotID int64
	query := `
		INSERT INTO snapshots (month) VALUES ($1)
		ON CONFLICT (month) DO UPDATE SET updated_at = now()
		RETURNING id`
	if err := tx.QueryRowContext(ctx, query, month).Scan(&snapshotID); err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	query = `INSERT INTO snapshot_items (snapshot_id, asset_id, amount) VALUES ($1, $2, $3)`
	_, err = tx.ExecContext(ctx, query, snapshotID, assetID, in.Amount)
	if isPgError(err, uniqueViolation) {
		return ErrItemExists
	}
	if err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// UpdateItem returns ErrNameTaken when another asset has the new name.
func (s *Store) UpdateItem(ctx context.Context, month time.Time, assetID int64, in ItemUpdate) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	query := `
		UPDATE snapshot_items i SET amount = $3
		FROM snapshots s
		WHERE i.snapshot_id = s.id AND s.month = $1 AND i.asset_id = $2`
	res, err := tx.ExecContext(ctx, query, month, assetID, in.Amount)
	if err != nil {
		return fmt.Errorf("update item: %w", err)
	}
	if err := requireRow(res, ErrItemNotFound); err != nil {
		return err
	}
	query = `UPDATE assets SET name = $2, type = $3, updated_at = now() WHERE id = $1`
	_, err = tx.ExecContext(ctx, query, assetID, in.Name, in.Type)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("update asset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// DeleteItem removes the asset from month. It also removes the month's snapshot when it
// becomes empty and the asset when no month records it any more.
func (s *Store) DeleteItem(ctx context.Context, month time.Time, assetID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	query := `
		DELETE FROM snapshot_items i USING snapshots s
		WHERE i.snapshot_id = s.id AND s.month = $1 AND i.asset_id = $2`
	res, err := tx.ExecContext(ctx, query, month, assetID)
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
	query = `
		DELETE FROM assets a WHERE a.id = $1
		AND NOT EXISTS (SELECT 1 FROM snapshot_items i WHERE i.asset_id = a.id)`
	if _, err := tx.ExecContext(ctx, query, assetID); err != nil {
		return fmt.Errorf("delete unused asset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// CopyPreviousSnapshot fills an empty month with the items of the latest earlier snapshot.
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
		INSERT INTO snapshot_items (snapshot_id, asset_id, amount)
		SELECT $1, i.asset_id, i.amount
		FROM snapshot_items i
		WHERE i.snapshot_id = (SELECT id FROM snapshots WHERE month < $2 ORDER BY month DESC LIMIT 1)`
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

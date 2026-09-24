package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type AssetType string

const (
	AssetCash       AssetType = "cash"
	AssetDeposit    AssetType = "deposit"
	AssetStock      AssetType = "stock"
	AssetCrypto     AssetType = "crypto"
	AssetRealEstate AssetType = "real_estate"
	AssetOther      AssetType = "other"
)

var ErrAssetNotFound = errors.New("asset not found")

var ErrInvalidAsset = errors.New("invalid asset")

type Asset struct {
	ID          int64
	Type        AssetType
	Name        string
	AmountCents int64
}

type AssetInput struct {
	Type        AssetType
	Name        string
	AmountCents int64
}

func (in AssetInput) Clean() (AssetInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case !in.Type.valid():
		return AssetInput{}, fmt.Errorf("%w: unknown type %q", ErrInvalidAsset, in.Type)
	case in.Name == "":
		return AssetInput{}, fmt.Errorf("%w: name is empty", ErrInvalidAsset)
	case in.AmountCents < 0:
		return AssetInput{}, fmt.Errorf("%w: amount is negative", ErrInvalidAsset)
	}
	return in, nil
}

var assetTypes = []AssetType{AssetCash, AssetDeposit, AssetStock, AssetCrypto, AssetRealEstate, AssetOther}

func (t AssetType) valid() bool {
	return slices.Contains(assetTypes, t)
}

func (t AssetType) Label() string {
	switch t {
	case AssetCash:
		return "Cash"
	case AssetDeposit:
		return "Savings"
	case AssetStock:
		return "Stock"
	case AssetCrypto:
		return "Crypto"
	case AssetRealEstate:
		return "Real estate"
	case AssetOther:
		return "Other"
	}
	return string(t)
}

const assetColumns = `id, type, name, amount_cents`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAsset(row rowScanner) (Asset, error) {
	var a Asset
	err := row.Scan(&a.ID, &a.Type, &a.Name, &a.AmountCents)
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, ErrAssetNotFound
	}
	return a, err
}

func (s *Store) Assets(ctx context.Context) ([]Asset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+assetColumns+` FROM assets ORDER BY amount_cents DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	defer rows.Close()
	assets := []Asset{}
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	return assets, nil
}

func (s *Store) Asset(ctx context.Context, id int64) (Asset, error) {
	a, err := scanAsset(s.db.QueryRowContext(ctx, `SELECT `+assetColumns+` FROM assets WHERE id = $1`, id))
	if err != nil {
		return Asset{}, fmt.Errorf("get asset %d: %w", id, err)
	}
	return a, nil
}

func (s *Store) CreateAsset(ctx context.Context, in AssetInput) (Asset, error) {
	in, err := in.Clean()
	if err != nil {
		return Asset{}, err
	}
	query := `INSERT INTO assets (type, name, amount_cents) VALUES ($1, $2, $3) RETURNING ` + assetColumns
	a, err := scanAsset(s.db.QueryRowContext(ctx, query, in.Type, in.Name, in.AmountCents))
	if err != nil {
		return Asset{}, fmt.Errorf("insert asset: %w", err)
	}
	return a, nil
}

func (s *Store) UpdateAsset(ctx context.Context, id int64, in AssetInput) (Asset, error) {
	in, err := in.Clean()
	if err != nil {
		return Asset{}, err
	}
	query := `
		UPDATE assets SET type = $2, name = $3, amount_cents = $4, updated_at = now()
		WHERE id = $1
		RETURNING ` + assetColumns
	a, err := scanAsset(s.db.QueryRowContext(ctx, query, id, in.Type, in.Name, in.AmountCents))
	if err != nil {
		return Asset{}, fmt.Errorf("update asset %d: %w", id, err)
	}
	return a, nil
}

func (s *Store) DeleteAsset(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM assets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete asset %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete asset %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete asset %d: %w", id, ErrAssetNotFound)
	}
	return nil
}

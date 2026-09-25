package finance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sakusi4/monolith/internal/money"
)

type AssetType string

const (
	AssetCash       AssetType = "cash"
	AssetDeposit    AssetType = "deposit"
	AssetStock      AssetType = "stock"
	AssetCrypto     AssetType = "crypto"
	AssetRealEstate AssetType = "real_estate"
	AssetLoan       AssetType = "loan"
	AssetOther      AssetType = "other"
)

type Asset struct {
	ID       int64
	Type     AssetType
	Name     string
	Currency money.Currency
}

var assetTypes = []AssetType{AssetCash, AssetDeposit, AssetStock, AssetCrypto, AssetRealEstate, AssetLoan, AssetOther}

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
	case AssetLoan:
		return "Loan"
	case AssetOther:
		return "Other"
	}
	return string(t)
}

func (s *Store) Assets(ctx context.Context) ([]Asset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, type, name, currency FROM assets ORDER BY lower(name), id`)
	if err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	defer rows.Close()
	assets := []Asset{}
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.Type, &a.Name, &a.Currency); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	return assets, nil
}

// assetNamed finds the asset whose name matches name, ignoring case and surrounding spaces
// like the unique index on asset names.
func assetNamed(assets []Asset, name string) (Asset, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	i := slices.IndexFunc(assets, func(a Asset) bool { return strings.ToLower(a.Name) == name })
	if i < 0 {
		return Asset{}, false
	}
	return assets[i], true
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

package finance

import (
	"errors"
	"slices"

	"github.com/jackc/pgx/v5/pgconn"
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

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

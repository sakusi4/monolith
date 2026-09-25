package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

const uniqueViolation = "23505"

var firstMonth = time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)

var ErrSnapshotNotFound = errors.New("snapshot not found")

type Snapshot struct {
	ID    int64
	Month time.Time
	Note  string
	Items []SnapshotItem
}

// SnapshotItem carries the exchange rate of the month closest to the snapshot month in RateMonth
// and PerUSD. PerUSD is nil for USD assets and for currencies without any stored rate.
type SnapshotItem struct {
	Asset     Asset
	Amount    int64
	RateMonth time.Time
	PerUSD    *big.Rat
}

func (it SnapshotItem) USD() (int64, bool) {
	if it.Asset.Currency == money.USD {
		return it.Amount, true
	}
	if it.PerUSD == nil {
		return 0, false
	}
	return money.ToUSD(it.Asset.Currency, it.Amount, it.PerUSD), true
}

// Totals is the value of a snapshot in USD cents. NetWorth sums every item and Loans the
// negative ones. The sums leave out the currencies in Missing, which have no exchange rate.
type Totals struct {
	NetWorth int64
	Loans    int64
	Missing  []money.Currency
}

func (s Snapshot) Totals() Totals {
	var t Totals
	for _, it := range s.Items {
		usd, ok := it.USD()
		switch {
		case !ok:
			if !slices.Contains(t.Missing, it.Asset.Currency) {
				t.Missing = append(t.Missing, it.Asset.Currency)
			}
		case usd < 0:
			t.NetWorth += usd
			t.Loans += usd
		default:
			t.NetWorth += usd
		}
	}
	return t
}

func currentMonth(now time.Time, loc *time.Location) time.Time {
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// monthsUntil lists the months from last back to firstMonth, newest first.
func monthsUntil(last time.Time) []time.Time {
	var months []time.Time
	for m := last; !m.Before(firstMonth); m = m.AddDate(0, -1, 0) {
		months = append(months, m)
	}
	return months
}

const snapshotQuery = `
	SELECT s.id, s.month, s.note, a.id, a.type, a.name, a.currency, i.amount, r.month, r.per_usd
	FROM snapshots s
	JOIN snapshot_items i ON i.snapshot_id = s.id
	JOIN assets a ON a.id = i.asset_id
	LEFT JOIN LATERAL (
		SELECT er.month, er.per_usd::text AS per_usd
		FROM exchange_rates er
		WHERE er.currency = a.currency
		ORDER BY abs(er.month - s.month), er.month
		LIMIT 1
	) r ON true`

// Snapshots returns every snapshot, newest month first.
func (s *Store) Snapshots(ctx context.Context) ([]Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, snapshotQuery+` ORDER BY s.month DESC, a.type, a.name, a.id`)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

func scanSnapshots(rows *sql.Rows) ([]Snapshot, error) {
	snapshots := []Snapshot{}
	for rows.Next() {
		var (
			id        int64
			month     time.Time
			note      string
			it        SnapshotItem
			rateMonth sql.Null[time.Time]
			perUSD    sql.Null[string]
		)
		err := rows.Scan(&id, &month, &note, &it.Asset.ID, &it.Asset.Type, &it.Asset.Name, &it.Asset.Currency, &it.Amount, &rateMonth, &perUSD)
		if err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		if perUSD.Valid {
			rate, ok := new(big.Rat).SetString(perUSD.V)
			if !ok {
				return nil, fmt.Errorf("parse rate %q", perUSD.V)
			}
			it.RateMonth, it.PerUSD = rateMonth.V, rate
		}
		if len(snapshots) == 0 || snapshots[len(snapshots)-1].ID != id {
			snapshots = append(snapshots, Snapshot{ID: id, Month: month, Note: note})
		}
		last := &snapshots[len(snapshots)-1]
		last.Items = append(last.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	return snapshots, nil
}

// SetNote replaces the note of month's snapshot. It returns ErrSnapshotNotFound when the month has no snapshot.
func (s *Store) SetNote(ctx context.Context, month time.Time, note string) error {
	query := `UPDATE snapshots SET note = $2, updated_at = now() WHERE month = $1`
	res, err := s.db.ExecContext(ctx, query, month, strings.TrimSpace(note))
	if err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	return requireRow(res, ErrSnapshotNotFound)
}

package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

const (
	ratesURL          = "https://open.er-api.com/v6/latest/USD"
	rateCheckInterval = time.Hour
)

// RunRateUpdates stores the exchange rates of the current UTC month when they are missing,
// checking at start and every hour until ctx is canceled.
func RunRateUpdates(ctx context.Context, store *Store, client *http.Client) {
	ticker := time.NewTicker(rateCheckInterval)
	defer ticker.Stop()
	for {
		if err := store.updateRates(ctx, client, time.Now()); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "update exchange rates", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) updateRates(ctx context.Context, client *http.Client, now time.Time) error {
	month := currentMonth(now, time.UTC)
	has, err := s.hasRates(ctx, month)
	if err != nil || has {
		return err
	}
	perUSD, err := fetchRates(ctx, client, ratesURL)
	if err != nil {
		return err
	}
	return s.saveRates(ctx, month, perUSD)
}

func fetchRates(ctx context.Context, client *http.Client, url string) (map[money.Currency]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build rates request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch rates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch rates: status %d", resp.StatusCode)
	}
	var body struct {
		Result string                 `json:"result"`
		Rates  map[string]json.Number `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode rates: %w", err)
	}
	if body.Result != "success" {
		return nil, fmt.Errorf("fetch rates: result %q", body.Result)
	}
	perUSD := make(map[money.Currency]string)
	for _, c := range money.Currencies {
		if c == money.USD {
			continue
		}
		n, ok := body.Rates[string(c)]
		if !ok {
			return nil, fmt.Errorf("fetch rates: %s is missing", c)
		}
		rate, ok := new(big.Rat).SetString(n.String())
		if !ok || rate.Sign() <= 0 {
			return nil, fmt.Errorf("fetch rates: %s rate %q is invalid", c, n)
		}
		perUSD[c] = n.String()
	}
	return perUSD, nil
}

func (s *Store) hasRates(ctx context.Context, month time.Time) (bool, error) {
	var has bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM exchange_rates WHERE month = $1)`, month).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("check rates: %w", err)
	}
	return has, nil
}

func (s *Store) saveRates(ctx context.Context, month time.Time, perUSD map[money.Currency]string) error {
	var currencies, values []string
	for c, rate := range perUSD {
		currencies = append(currencies, string(c))
		values = append(values, rate)
	}
	query := `
		INSERT INTO exchange_rates (month, currency, per_usd)
		SELECT $1, unnest($2::text[]), unnest($3::numeric[])
		ON CONFLICT DO NOTHING`
	if _, err := s.db.ExecContext(ctx, query, month, currencies, values); err != nil {
		return fmt.Errorf("insert rates: %w", err)
	}
	return nil
}

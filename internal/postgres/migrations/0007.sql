ALTER TABLE snapshots ADD COLUMN note text NOT NULL DEFAULT '';

CREATE TEMP TABLE monthly_rates ON COMMIT DROP AS
SELECT m.month, m.currency, r.per_usd
FROM (SELECT DISTINCT date_trunc('month', rate_date)::date AS month, currency FROM exchange_rates) m
CROSS JOIN LATERAL (
    SELECT d.per_usd
    FROM exchange_rates d
    WHERE d.currency = m.currency
    ORDER BY abs(d.rate_date - m.month), d.rate_date
    LIMIT 1
) r;

DELETE FROM exchange_rates;
ALTER TABLE exchange_rates RENAME COLUMN rate_date TO month;
ALTER TABLE exchange_rates ADD CONSTRAINT exchange_rates_month_check CHECK (extract(day FROM month) = 1);
INSERT INTO exchange_rates (month, currency, per_usd) SELECT month, currency, per_usd FROM monthly_rates;

ALTER TABLE assets DROP CONSTRAINT assets_type_check;
ALTER TABLE assets ADD CONSTRAINT assets_type_check
    CHECK (type IN ('cash', 'deposit', 'stock', 'crypto', 'real_estate', 'loan', 'other'));
ALTER TABLE assets DROP CONSTRAINT assets_currency_check;
ALTER TABLE assets ADD CONSTRAINT assets_currency_check
    CHECK (currency IN ('USD', 'KRW', 'AED', 'JPY'));

CREATE TABLE snapshots (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    month date NOT NULL UNIQUE CHECK (extract(day FROM month) = 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE snapshot_items (
    snapshot_id bigint NOT NULL REFERENCES snapshots (id) ON DELETE CASCADE,
    asset_id bigint NOT NULL REFERENCES assets (id),
    amount bigint NOT NULL,
    PRIMARY KEY (snapshot_id, asset_id)
);

INSERT INTO snapshots (month)
SELECT date_trunc('month', now() AT TIME ZONE 'UTC')::date
WHERE EXISTS (SELECT 1 FROM assets);

INSERT INTO snapshot_items (snapshot_id, asset_id, amount)
SELECT s.id, a.id, a.amount FROM assets a CROSS JOIN snapshots s;

ALTER TABLE assets DROP COLUMN amount;

CREATE TABLE exchange_rates (
    rate_date date NOT NULL,
    currency text NOT NULL CHECK (currency IN ('KRW', 'AED', 'JPY')),
    per_usd numeric NOT NULL CHECK (per_usd > 0),
    PRIMARY KEY (rate_date, currency)
);

ALTER TABLE assets RENAME COLUMN amount_cents TO amount;
ALTER TABLE assets RENAME CONSTRAINT assets_amount_cents_check TO assets_amount_check;
ALTER TABLE assets ADD COLUMN currency text NOT NULL DEFAULT 'USD' CHECK (currency IN ('USD', 'KRW', 'AED'));
ALTER TABLE assets ALTER COLUMN currency DROP DEFAULT;

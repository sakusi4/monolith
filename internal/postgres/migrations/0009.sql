ALTER TABLE snapshot_items
    ADD COLUMN id bigint GENERATED ALWAYS AS IDENTITY,
    ADD COLUMN name text,
    ADD COLUMN type text,
    ADD COLUMN currency text;

UPDATE snapshot_items i
SET name = a.name, type = a.type, currency = a.currency
FROM assets a
WHERE a.id = i.asset_id;

ALTER TABLE snapshot_items
    DROP CONSTRAINT snapshot_items_pkey,
    DROP COLUMN asset_id,
    ADD PRIMARY KEY (id),
    ALTER COLUMN name SET NOT NULL,
    ALTER COLUMN type SET NOT NULL,
    ALTER COLUMN currency SET NOT NULL,
    ADD CONSTRAINT snapshot_items_name_check CHECK (name <> ''),
    ADD CONSTRAINT snapshot_items_type_check
        CHECK (type IN ('cash', 'deposit', 'stock', 'crypto', 'real_estate', 'loan', 'other')),
    ADD CONSTRAINT snapshot_items_currency_check CHECK (currency IN ('USD', 'KRW', 'AED', 'JPY'));

CREATE INDEX snapshot_items_snapshot_id_idx ON snapshot_items (snapshot_id);

DROP TABLE assets;

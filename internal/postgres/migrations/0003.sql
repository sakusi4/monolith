CREATE TABLE assets (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type text NOT NULL CHECK (type IN ('cash', 'deposit', 'stock', 'crypto', 'real_estate', 'other')),
    name text NOT NULL CHECK (name <> ''),
    amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

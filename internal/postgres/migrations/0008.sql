CREATE TABLE expenses (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    spent_on date NOT NULL,
    name text NOT NULL CHECK (name <> ''),
    category text NOT NULL CHECK (category IN ('housing', 'food', 'transport', 'bills', 'shopping', 'travel', 'family', 'loan_payment', 'other')),
    currency text NOT NULL CHECK (currency IN ('USD', 'KRW', 'AED', 'JPY')),
    amount bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX expenses_spent_on_idx ON expenses (spent_on);

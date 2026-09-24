CREATE TABLE users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email text NOT NULL UNIQUE CHECK (email = lower(email)),
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

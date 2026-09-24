CREATE TABLE schema_version (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

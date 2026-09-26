CREATE TABLE folders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id bigint REFERENCES folders (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> '' AND strpos(name, '/') = 0 AND char_length(name) <= 255),
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX folders_name_idx ON folders (parent_id, name) NULLS NOT DISTINCT WHERE trashed_at IS NULL;
CREATE INDEX folders_parent_id_idx ON folders (parent_id);

CREATE TABLE blobs (
    sha256 bytea PRIMARY KEY CHECK (length(sha256) = 32),
    size bigint NOT NULL CHECK (size >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE files (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    folder_id bigint REFERENCES folders (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> '' AND strpos(name, '/') = 0 AND char_length(name) <= 255),
    sha256 bytea NOT NULL REFERENCES blobs (sha256),
    content_type text NOT NULL,
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX files_name_idx ON files (folder_id, name) NULLS NOT DISTINCT WHERE trashed_at IS NULL;
CREATE INDEX files_folder_id_idx ON files (folder_id);
CREATE INDEX files_sha256_idx ON files (sha256);

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE pages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id bigint REFERENCES pages (id) ON DELETE CASCADE,
    project_id bigint UNIQUE REFERENCES projects (id) ON DELETE SET NULL,
    task_id bigint UNIQUE REFERENCES tasks (id) ON DELETE SET NULL,
    title text NOT NULL CHECK (title <> ''),
    body text NOT NULL DEFAULT '',
    folder_id bigint REFERENCES folders (id) ON DELETE SET NULL,
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(project_id, task_id) <= 1),
    CHECK (parent_id IS NULL OR num_nonnulls(project_id, task_id) = 0)
);
CREATE UNIQUE INDEX pages_project_title_idx ON pages (title) WHERE project_id IS NOT NULL;
CREATE INDEX pages_parent_id_idx ON pages (parent_id);
CREATE INDEX pages_title_trgm_idx ON pages USING gin (title gin_trgm_ops);
CREATE INDEX pages_body_trgm_idx ON pages USING gin (body gin_trgm_ops);

CREATE TABLE page_links (
    source_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    target_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    PRIMARY KEY (source_id, target_id),
    CHECK (source_id <> target_id)
);
CREATE INDEX page_links_target_id_idx ON page_links (target_id);

INSERT INTO pages (project_id, title, body, folder_id, created_at, updated_at)
SELECT id, name, body, folder_id, created_at, updated_at FROM projects;
INSERT INTO pages (task_id, title, body, folder_id, created_at, updated_at)
SELECT id, title, body, folder_id, created_at, updated_at FROM tasks;

ALTER TABLE projects DROP COLUMN name, DROP COLUMN body, DROP COLUMN folder_id;
ALTER TABLE tasks DROP COLUMN title, DROP COLUMN body, DROP COLUMN folder_id;

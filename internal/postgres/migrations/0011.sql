CREATE TABLE projects (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL UNIQUE CHECK (name <> ''),
    status text NOT NULL CHECK (status IN ('planned', 'active', 'paused', 'done', 'canceled')),
    started_on date,
    finished_on date CHECK (finished_on >= started_on),
    body text NOT NULL DEFAULT '',
    folder_id bigint REFERENCES folders (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text NOT NULL CHECK (title <> ''),
    project_id bigint REFERENCES projects (id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('todo', 'in_progress', 'done', 'canceled')),
    due_on date,
    body text NOT NULL DEFAULT '',
    folder_id bigint REFERENCES folders (id) ON DELETE SET NULL,
    completed_at timestamptz CHECK ((status = 'done') = (completed_at IS NOT NULL)),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_project_id_idx ON tasks (project_id);

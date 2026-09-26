package drive

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const maxNameLength = 255

type Folder struct {
	ID        int64
	ParentID  int64
	Name      string
	UpdatedAt time.Time
}

// FolderPath is a folder with the names of the folders above it, as "Projects / tunnel".
type FolderPath struct {
	ID   int64
	Path string
}

// cleanName trims name and composes its Unicode (NFC), so that a name typed on one system matches
// the same name sent decomposed by another, such as macOS.
func cleanName(name string) (string, error) {
	name = norm.NFC.String(strings.TrimSpace(name))
	switch {
	case name == "":
		return "", fmt.Errorf("%w: empty", ErrInvalidName)
	case strings.Contains(name, "/"):
		return "", fmt.Errorf("%w: %q has a slash", ErrInvalidName, name)
	case utf8.RuneCountInString(name) > maxNameLength:
		return "", fmt.Errorf("%w: longer than %d characters", ErrInvalidName, maxNameLength)
	}
	return name, nil
}

// Path returns the folders from the top level down to id. It returns ErrNotFound when id does not
// exist or when it or a folder above it is in the trash.
func (s *Store) Path(ctx context.Context, id int64) ([]Folder, error) {
	query := `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, name, updated_at, trashed_at, 0 AS depth FROM folders WHERE id = $1
			UNION ALL
			SELECT f.id, f.parent_id, f.name, f.updated_at, f.trashed_at, c.depth + 1
			FROM folders f JOIN chain c ON f.id = c.parent_id
		)
		SELECT id, parent_id, name, updated_at, trashed_at IS NOT NULL FROM chain ORDER BY depth DESC`
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("query folder path: %w", err)
	}
	defer rows.Close()
	var path []Folder
	hidden := false
	for rows.Next() {
		var (
			f       Folder
			parent  sql.Null[int64]
			trashed bool
		)
		if err := rows.Scan(&f.ID, &parent, &f.Name, &f.UpdatedAt, &trashed); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		f.ParentID = parent.V
		hidden = hidden || trashed
		path = append(path, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query folder path: %w", err)
	}
	if len(path) == 0 || hidden {
		return nil, ErrNotFound
	}
	return path, nil
}

// Folders lists the folders in parent, or at the top level when parent is 0, by name.
func (s *Store) Folders(ctx context.Context, parent int64) ([]Folder, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if parent == 0 {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, parent_id, name, updated_at FROM folders
			WHERE parent_id IS NULL AND trashed_at IS NULL
			ORDER BY lower(name), id`)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, parent_id, name, updated_at FROM folders
			WHERE parent_id = $1 AND trashed_at IS NULL
			ORDER BY lower(name), id`, parent)
	}
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()
	var folders []Folder
	for rows.Next() {
		var (
			f      Folder
			parent sql.Null[int64]
		)
		if err := rows.Scan(&f.ID, &parent, &f.Name, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		f.ParentID = parent.V
		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	return folders, nil
}

// FolderPaths lists every folder outside the trash by path, leaving out exclude and the folders in it.
func (s *Store) FolderPaths(ctx context.Context, exclude int64) ([]FolderPath, error) {
	query := `
		WITH RECURSIVE tree AS (
			SELECT id, name AS path FROM folders
			WHERE parent_id IS NULL AND trashed_at IS NULL AND id <> $1
			UNION ALL
			SELECT f.id, t.path || ' / ' || f.name FROM folders f JOIN tree t ON f.parent_id = t.id
			WHERE f.trashed_at IS NULL AND f.id <> $1
		)
		SELECT id, path FROM tree ORDER BY lower(path), id`
	rows, err := s.db.QueryContext(ctx, query, exclude)
	if err != nil {
		return nil, fmt.Errorf("query folder paths: %w", err)
	}
	defer rows.Close()
	var paths []FolderPath
	for rows.Next() {
		var p FolderPath
		if err := rows.Scan(&p.ID, &p.Path); err != nil {
			return nil, fmt.Errorf("scan folder path: %w", err)
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query folder paths: %w", err)
	}
	return paths, nil
}

// CreateFolder makes a folder named name in parent, or at the top level when parent is 0.
// It returns ErrInvalidName or ErrNameTaken when the name is not allowed there.
func (s *Store) CreateFolder(ctx context.Context, parent int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO folders (parent_id, name) VALUES ($1, $2)`, nullID(parent), name)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("insert folder: %w", err)
	}
	return nil
}

// UpdateFolder renames id and moves it into parent, or to the top level when parent is 0. It
// returns ErrMoveIntoItself when parent is id or a folder in it, ErrInvalidName or ErrNameTaken
// when the name is not allowed there, and ErrNotFound when id is not a folder outside the trash.
func (s *Store) UpdateFolder(ctx context.Context, id, parent int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	query := `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE id = $1
			UNION ALL
			SELECT f.id FROM folders f JOIN sub ON f.parent_id = sub.id
		)
		SELECT EXISTS (SELECT 1 FROM sub WHERE id = $2)`
	var inside bool
	if err := tx.QueryRowContext(ctx, query, id, parent).Scan(&inside); err != nil {
		return fmt.Errorf("check folder move: %w", err)
	}
	if inside {
		return ErrMoveIntoItself
	}
	query = `UPDATE folders SET parent_id = $2, name = $3, updated_at = now() WHERE id = $1 AND trashed_at IS NULL`
	res, err := tx.ExecContext(ctx, query, id, nullID(parent), name)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("update folder: %w", err)
	}
	if err := requireRow(res); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

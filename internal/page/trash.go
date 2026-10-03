package page

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TrashedPage is a page in the trash. Location names the section and the pages it was under.
type TrashedPage struct {
	ID        int64
	Title     string
	Location  string
	TrashedAt time.Time
}

// Trash moves pages ids, and with them the pages under them, to the trash.
func (s *Store) Trash(ctx context.Context, ids []int64) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE pages SET trashed_at = now() WHERE id = ANY($1) AND trashed_at IS NULL`, ids); err != nil {
		return fmt.Errorf("trash pages: %w", err)
	}
	return nil
}

// TrashedPages lists the pages in the trash, the latest first.
func (s *Store) TrashedPages(ctx context.Context) ([]TrashedPage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, trashed_at FROM pages WHERE trashed_at IS NOT NULL ORDER BY trashed_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query trashed pages: %w", err)
	}
	defer rows.Close()
	var ids []int64
	trashedAt := make(map[int64]time.Time)
	for rows.Next() {
		var (
			id int64
			at time.Time
		)
		if err := rows.Scan(&id, &at); err != nil {
			return nil, fmt.Errorf("scan trashed page: %w", err)
		}
		ids = append(ids, id)
		trashedAt[id] = at
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query trashed pages: %w", err)
	}
	entries, err := s.describe(ctx, ids, true)
	if err != nil {
		return nil, err
	}
	pages := make([]TrashedPage, len(entries))
	for i, e := range entries {
		pages[i] = TrashedPage{ID: e.ID, Title: e.Title, Location: e.Path, TrashedAt: trashedAt[e.ID]}
	}
	return pages, nil
}

// Restore takes page id out of the trash. It returns ErrParentTrashed when the page it was under is
// in the trash, and ErrNotFound when id is not a page in the trash.
func (s *Store) Restore(ctx context.Context, id int64) error {
	var parent sql.Null[int64]
	err := s.db.QueryRowContext(ctx, `SELECT parent_id FROM pages WHERE id = $1 AND trashed_at IS NOT NULL`, id).Scan(&parent)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("query trashed page: %w", err)
	}
	if parent.Valid {
		ok, err := s.isVisibleOutside(ctx, parent.V, 0)
		if err != nil {
			return err
		}
		if !ok {
			return ErrParentTrashed
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE pages SET trashed_at = NULL WHERE id = $1`, id); err != nil {
		return fmt.Errorf("restore page: %w", err)
	}
	return nil
}

// Delete removes page id, which is in the trash, and the pages under it for good, and moves their
// drive folders to the trash. It returns ErrNotFound when id is not a page in the trash.
func (s *Store) Delete(ctx context.Context, id int64) error {
	return s.purge(ctx, []int64{id})
}

// EmptyTrash removes every page in the trash as Delete does.
func (s *Store) EmptyTrash(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM pages WHERE trashed_at IS NOT NULL AND project_id IS NULL AND task_id IS NULL`)
	if err != nil {
		return fmt.Errorf("query trashed pages: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan trashed page: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query trashed pages: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	return s.purge(ctx, ids)
}

// purge removes the trashed pages ids of their own and the pages under them for good, and then
// moves their drive folders to the trash. It returns ErrNotFound when none of ids is such a page.
func (s *Store) purge(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	folders, err := purgedFolders(ctx, tx, ids)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM pages WHERE id = ANY($1) AND trashed_at IS NOT NULL AND project_id IS NULL AND task_id IS NULL`, ids)
	if err != nil {
		return fmt.Errorf("delete pages: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return s.drive.TrashFolders(ctx, folders)
}

// purgedFolders lists in tx the drive folders of the trashed pages ids of their own and of the pages
// under them.
func purgedFolders(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	query := `
		WITH RECURSIVE sub AS (
			SELECT id, folder_id FROM pages
			WHERE id = ANY($1) AND trashed_at IS NOT NULL AND project_id IS NULL AND task_id IS NULL
			UNION ALL
			SELECT p.id, p.folder_id FROM pages p JOIN sub ON p.parent_id = sub.id
		)
		SELECT DISTINCT folder_id FROM sub WHERE folder_id IS NOT NULL`
	rows, err := tx.QueryContext(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("query purged folders: %w", err)
	}
	defer rows.Close()
	var folders []int64
	for rows.Next() {
		var folder int64
		if err := rows.Scan(&folder); err != nil {
			return nil, fmt.Errorf("scan purged folder: %w", err)
		}
		folders = append(folders, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query purged folders: %w", err)
	}
	return folders, nil
}

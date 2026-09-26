package drive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TrashItem is a folder or file in the trash. Location is the path of the folder it was in, empty
// for the top level.
type TrashItem struct {
	Folder      bool
	ID          int64
	Name        string
	ContentType string
	Location    string
	TrashedAt   time.Time
}

func (t TrashItem) Kind() Kind {
	if t.Folder {
		return KindFolder
	}
	return kindOf(t.ContentType)
}

// TrashFolder moves id and everything in it to the trash. It returns ErrNotFound when id is not a
// folder outside the trash.
func (s *Store) TrashFolder(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE folders SET trashed_at = now() WHERE id = $1 AND trashed_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("trash folder: %w", err)
	}
	return requireRow(res)
}

// TrashFile moves id to the trash. It returns ErrNotFound when id is not a file outside the trash.
func (s *Store) TrashFile(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE files SET trashed_at = now() WHERE id = $1 AND trashed_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("trash file: %w", err)
	}
	return requireRow(res)
}

// Trash lists the folders and files moved to the trash, the latest first.
func (s *Store) Trash(ctx context.Context) ([]TrashItem, error) {
	query := `
		WITH RECURSIVE tree AS (
			SELECT id, name AS path FROM folders WHERE parent_id IS NULL
			UNION ALL
			SELECT f.id, t.path || ' / ' || f.name FROM folders f JOIN tree t ON f.parent_id = t.id
		)
		SELECT true AS folder, fo.id, fo.name, '' AS content_type, coalesce(t.path, '') AS location, fo.trashed_at
		FROM folders fo LEFT JOIN tree t ON t.id = fo.parent_id
		WHERE fo.trashed_at IS NOT NULL
		UNION ALL
		SELECT false, fi.id, fi.name, fi.content_type, coalesce(t.path, ''), fi.trashed_at
		FROM files fi LEFT JOIN tree t ON t.id = fi.folder_id
		WHERE fi.trashed_at IS NOT NULL
		ORDER BY trashed_at DESC, name`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query trash: %w", err)
	}
	defer rows.Close()
	var items []TrashItem
	for rows.Next() {
		var it TrashItem
		if err := rows.Scan(&it.Folder, &it.ID, &it.Name, &it.ContentType, &it.Location, &it.TrashedAt); err != nil {
			return nil, fmt.Errorf("scan trash item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query trash: %w", err)
	}
	return items, nil
}

// RestoreFolder takes id out of the trash. It returns ErrParentTrashed when the folder it was in is
// hidden in the trash, ErrNameTaken when that folder has another folder of the same name, and
// ErrNotFound when id is not a folder in the trash.
func (s *Store) RestoreFolder(ctx context.Context, id int64) error {
	var parent sql.Null[int64]
	err := s.db.QueryRowContext(ctx, `SELECT parent_id FROM folders WHERE id = $1 AND trashed_at IS NOT NULL`, id).Scan(&parent)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("query trashed folder: %w", err)
	}
	return s.restore(ctx, `UPDATE folders SET trashed_at = NULL WHERE id = $1`, id, parent.V)
}

// RestoreFile takes id out of the trash, with the errors of RestoreFolder.
func (s *Store) RestoreFile(ctx context.Context, id int64) error {
	var folder sql.Null[int64]
	err := s.db.QueryRowContext(ctx, `SELECT folder_id FROM files WHERE id = $1 AND trashed_at IS NOT NULL`, id).Scan(&folder)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("query trashed file: %w", err)
	}
	return s.restore(ctx, `UPDATE files SET trashed_at = NULL WHERE id = $1`, id, folder.V)
}

func (s *Store) restore(ctx context.Context, query string, id, parent int64) error {
	if parent != 0 {
		_, err := s.Path(ctx, parent)
		if errors.Is(err, ErrNotFound) {
			return ErrParentTrashed
		}
		if err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, query, id)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	return nil
}

// DeleteFolder removes the trashed folder id and everything in it for good. It returns ErrNotFound
// when id is not a folder in the trash.
func (s *Store) DeleteFolder(ctx context.Context, id int64) error {
	return s.purge(ctx, []int64{id}, nil)
}

// DeleteFile removes the trashed file id for good. It returns ErrNotFound when id is not a file in the trash.
func (s *Store) DeleteFile(ctx context.Context, id int64) error {
	return s.purge(ctx, nil, []int64{id})
}

// EmptyTrash removes everything in the trash for good.
func (s *Store) EmptyTrash(ctx context.Context) error {
	items, err := s.Trash(ctx)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	var folders, files []int64
	for _, it := range items {
		if it.Folder {
			folders = append(folders, it.ID)
			continue
		}
		files = append(files, it.ID)
	}
	return s.purge(ctx, folders, files)
}

// purge removes the trashed folders and files with the given ids for good, with everything in the
// folders, and then the content that no file refers to anymore. It returns ErrNotFound when none of
// them is in the trash.
func (s *Store) purge(ctx context.Context, folders, files []int64) error {
	s.blobs.mu.Lock()
	defer s.blobs.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	sums, err := purgedSums(ctx, tx, folders, files)
	if err != nil {
		return err
	}
	deleted, err := deleteTrashed(ctx, tx, folders, files)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrNotFound
	}
	gone, err := deleteUnreferenced(ctx, tx, sums)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return s.removeBlobs(gone)
}

// purgedSums lists the content of the trashed files and of every file in the trashed folders.
func purgedSums(ctx context.Context, tx *sql.Tx, folders, files []int64) ([][]byte, error) {
	query := `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE id = ANY($1) AND trashed_at IS NOT NULL
			UNION ALL
			SELECT f.id FROM folders f JOIN sub ON f.parent_id = sub.id
		)
		SELECT DISTINCT sha256 FROM files
		WHERE folder_id IN (SELECT id FROM sub) OR (id = ANY($2) AND trashed_at IS NOT NULL)`
	rows, err := tx.QueryContext(ctx, query, folders, files)
	if err != nil {
		return nil, fmt.Errorf("query purged content: %w", err)
	}
	defer rows.Close()
	var sums [][]byte
	for rows.Next() {
		var sum []byte
		if err := rows.Scan(&sum); err != nil {
			return nil, fmt.Errorf("scan purged content: %w", err)
		}
		sums = append(sums, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query purged content: %w", err)
	}
	return sums, nil
}

func deleteTrashed(ctx context.Context, tx *sql.Tx, folders, files []int64) (int64, error) {
	res, err := tx.ExecContext(ctx, `DELETE FROM folders WHERE id = ANY($1) AND trashed_at IS NOT NULL`, folders)
	if err != nil {
		return 0, fmt.Errorf("delete folders: %w", err)
	}
	folderRows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	res, err = tx.ExecContext(ctx, `DELETE FROM files WHERE id = ANY($1) AND trashed_at IS NOT NULL`, files)
	if err != nil {
		return 0, fmt.Errorf("delete files: %w", err)
	}
	fileRows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return folderRows + fileRows, nil
}

// deleteUnreferenced removes the rows of the content in sums that no file refers to, and returns their sums.
func deleteUnreferenced(ctx context.Context, tx *sql.Tx, sums [][]byte) ([][]byte, error) {
	query := `
		DELETE FROM blobs b
		WHERE b.sha256 = ANY($1::bytea[]) AND NOT EXISTS (SELECT 1 FROM files f WHERE f.sha256 = b.sha256)
		RETURNING b.sha256`
	rows, err := tx.QueryContext(ctx, query, sums)
	if err != nil {
		return nil, fmt.Errorf("delete unreferenced blobs: %w", err)
	}
	defer rows.Close()
	var gone [][]byte
	for rows.Next() {
		var sum []byte
		if err := rows.Scan(&sum); err != nil {
			return nil, fmt.Errorf("scan unreferenced blob: %w", err)
		}
		gone = append(gone, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("delete unreferenced blobs: %w", err)
	}
	return gone, nil
}

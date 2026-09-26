// Package drive keeps files of every kind in a folder tree. The content of a file lives on disk,
// named by its SHA-256, and its name, folder, and type live in PostgreSQL.
package drive

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolation = "23505"

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidName    = errors.New("invalid name")
	ErrNameTaken      = errors.New("name taken")
	ErrMoveIntoItself = errors.New("folder moved into itself")
	ErrParentTrashed  = errors.New("parent folder in the trash")
)

type Store struct {
	db    *sql.DB
	blobs *blobStore
}

// NewStore keeps file contents under dir, creating the directories it needs. Only one process may use dir.
func NewStore(db *sql.DB, dir string) (*Store, error) {
	blobs, err := newBlobStore(dir)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, blobs: blobs}, nil
}

func nullID(id int64) sql.Null[int64] {
	return sql.Null[int64]{V: id, Valid: id != 0}
}

func requireRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

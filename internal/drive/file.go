package drive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"
)

const (
	maxTextPreview = 1 << 20
	sizeUnit       = 1024
)

type Kind string

const (
	KindFolder Kind = "folder"
	KindImage  Kind = "image"
	KindVideo  Kind = "video"
	KindAudio  Kind = "audio"
	KindPDF    Kind = "pdf"
	KindText   Kind = "text"
	KindOther  Kind = "other"
)

var sizeUnits = []string{"KB", "MB", "GB", "TB"}

var scriptableTypes = []string{"text/html", "text/xml", "application/xml"}

func (k Kind) Label() string {
	switch k {
	case KindFolder:
		return "Folder"
	case KindImage:
		return "Image"
	case KindVideo:
		return "Video"
	case KindAudio:
		return "Audio"
	case KindPDF:
		return "PDF"
	case KindText:
		return "Text"
	case KindOther:
		return "File"
	}
	return string(k)
}

type File struct {
	ID          int64
	FolderID    int64
	Name        string
	ContentType string
	Size        int64
	SHA256      []byte
	UpdatedAt   time.Time
}

// Upload is a file whose content is staged on disk, waiting for AddFiles or Discard.
type Upload struct {
	Name        string
	ContentType string
	blob        stagedBlob
}

func (f File) Kind() Kind {
	return kindOf(f.ContentType)
}

func kindOf(contentType string) Kind {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return KindOther
	}
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return KindImage
	case strings.HasPrefix(mediaType, "video/"):
		return KindVideo
	case strings.HasPrefix(mediaType, "audio/"):
		return KindAudio
	case mediaType == "application/pdf":
		return KindPDF
	case strings.HasPrefix(mediaType, "text/"), mediaType == "application/json":
		return KindText
	}
	return KindOther
}

// contentType names the type of a file from its name, or from its first bytes when the name does not tell.
func contentType(name string, head []byte) string {
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return http.DetectContentType(head)
}

// mustDownload reports whether content of this type could run script in the browser, so that it
// is only ever sent as an attachment. Browsers may treat any "+xml" type as XML.
func mustDownload(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err != nil || slices.Contains(scriptableTypes, mediaType) || strings.HasSuffix(mediaType, "+xml")
}

func formatSize(n int64) string {
	if n < sizeUnit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n) / sizeUnit
	unit := 0
	for value >= sizeUnit && unit < len(sizeUnits)-1 {
		value /= sizeUnit
		unit++
	}
	return fmt.Sprintf("%.1f %s", value, sizeUnits[unit])
}

// File returns the file id. It returns ErrNotFound when id is not a file, or when it or a folder
// above it is in the trash.
func (s *Store) File(ctx context.Context, id int64) (File, error) {
	var (
		f      File
		folder sql.Null[int64]
	)
	query := `
		SELECT f.id, f.folder_id, f.name, f.content_type, b.size, f.sha256, f.updated_at
		FROM files f JOIN blobs b ON b.sha256 = f.sha256
		WHERE f.id = $1 AND f.trashed_at IS NULL`
	err := s.db.QueryRowContext(ctx, query, id).Scan(&f.ID, &folder, &f.Name, &f.ContentType, &f.Size, &f.SHA256, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("query file: %w", err)
	}
	f.FolderID = folder.V
	if f.FolderID != 0 {
		if _, err := s.Path(ctx, f.FolderID); err != nil {
			return File{}, err
		}
	}
	return f, nil
}

// Files lists the files in folder, or at the top level when folder is 0, by name.
func (s *Store) Files(ctx context.Context, folder int64) ([]File, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if folder == 0 {
		rows, err = s.db.QueryContext(ctx, `
			SELECT f.id, f.folder_id, f.name, f.content_type, b.size, f.sha256, f.updated_at
			FROM files f JOIN blobs b ON b.sha256 = f.sha256
			WHERE f.folder_id IS NULL AND f.trashed_at IS NULL
			ORDER BY lower(f.name), f.id`)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT f.id, f.folder_id, f.name, f.content_type, b.size, f.sha256, f.updated_at
			FROM files f JOIN blobs b ON b.sha256 = f.sha256
			WHERE f.folder_id = $1 AND f.trashed_at IS NULL
			ORDER BY lower(f.name), f.id`, folder)
	}
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	defer rows.Close()
	var files []File
	for rows.Next() {
		var (
			f      File
			folder sql.Null[int64]
		)
		if err := rows.Scan(&f.ID, &folder, &f.Name, &f.ContentType, &f.Size, &f.SHA256, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		f.FolderID = folder.V
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	return files, nil
}

// Stage writes the content of a file named name to temporary storage.
func (s *Store) Stage(name string, r io.Reader) (Upload, error) {
	blob, err := s.blobs.stage(r)
	if err != nil {
		return Upload{}, err
	}
	return Upload{Name: name, ContentType: contentType(name, blob.head), blob: blob}, nil
}

// Discard removes the staged content of uploads that will not be added.
func (s *Store) Discard(uploads []Upload) error {
	var errs []error
	for _, u := range uploads {
		errs = append(errs, s.blobs.discard(u.blob))
	}
	return errors.Join(errs...)
}

// AddFiles adds all uploads to folder, or to the top level when folder is 0, or on error none of
// them. It returns ErrInvalidName or ErrNameTaken when a name is not allowed there. It uses up the
// staged content either way.
func (s *Store) AddFiles(ctx context.Context, folder int64, uploads []Upload) error {
	for i := range uploads {
		name, err := cleanName(uploads[i].Name)
		if err != nil {
			return errors.Join(err, s.Discard(uploads))
		}
		uploads[i].Name = name
	}
	s.blobs.mu.Lock()
	defer s.blobs.mu.Unlock()
	var created [][]byte
	for i, u := range uploads {
		isNew, err := s.blobs.keep(u.blob)
		if err != nil {
			return errors.Join(err, s.Discard(uploads[i:]), s.removeBlobs(created))
		}
		if isNew {
			created = append(created, u.blob.sum)
		}
	}
	if err := s.insertFiles(ctx, folder, uploads); err != nil {
		return errors.Join(err, s.removeBlobs(created))
	}
	return nil
}

func (s *Store) insertFiles(ctx context.Context, folder int64, uploads []Upload) error {
	sums := make([][]byte, len(uploads))
	sizes := make([]int64, len(uploads))
	names := make([]string, len(uploads))
	types := make([]string, len(uploads))
	for i, u := range uploads {
		sums[i], sizes[i], names[i], types[i] = u.blob.sum, u.blob.size, u.Name, u.ContentType
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	query := `
		INSERT INTO blobs (sha256, size)
		SELECT u.sha256, u.size FROM unnest($1::bytea[], $2::bigint[]) AS u(sha256, size)
		ON CONFLICT DO NOTHING`
	if _, err := tx.ExecContext(ctx, query, sums, sizes); err != nil {
		return fmt.Errorf("insert blobs: %w", err)
	}
	query = `
		INSERT INTO files (folder_id, name, sha256, content_type)
		SELECT $1::bigint, u.name, u.sha256, u.content_type
		FROM unnest($2::text[], $3::bytea[], $4::text[]) AS u(name, sha256, content_type)`
	_, err = tx.ExecContext(ctx, query, nullID(folder), names, sums, types)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("insert files: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Store) removeBlobs(sums [][]byte) error {
	var errs []error
	for _, sum := range sums {
		errs = append(errs, s.blobs.remove(sum))
	}
	return errors.Join(errs...)
}

// Open returns the content of f for reading. The caller closes it.
func (s *Store) Open(f File) (*os.File, error) {
	return s.blobs.open(f.SHA256)
}

// UpdateFile renames id and moves it into folder, or to the top level when folder is 0. It returns
// ErrInvalidName or ErrNameTaken when the name is not allowed there, and ErrNotFound when id is not
// a file outside the trash.
func (s *Store) UpdateFile(ctx context.Context, id, folder int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	query := `UPDATE files SET folder_id = $2, name = $3, updated_at = now() WHERE id = $1 AND trashed_at IS NULL`
	res, err := s.db.ExecContext(ctx, query, id, nullID(folder), name)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("update file: %w", err)
	}
	return requireRow(res)
}

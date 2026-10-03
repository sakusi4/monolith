// Package page keeps the pages that hold the writing of the app: a title, a markdown body, the
// pages under it, and a drive folder for its attachments. The page of a project or a task is its
// body, and every other page stands on its own. The package reads and changes the drive only through
// the drive's Store.
package page

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/unicode/norm"

	"github.com/sakusi4/monolith/internal/drive"
)

const (
	uniqueViolation = "23505"
	moveLock        = 1
	pagesURL        = "/page"
	projectsURL     = "/task/projects"
	tasksURL        = "/task/tasks"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidTitle  = errors.New("invalid title")
	ErrInvalidParent = errors.New("invalid parent")
	ErrTitleTaken    = errors.New("project title taken")
	ErrParentTrashed = errors.New("parent page in the trash")
	ErrStale         = errors.New("page saved since")
)

type Store struct {
	db    *sql.DB
	drive *drive.Store
}

func NewStore(db *sql.DB, driveStore *drive.Store) *Store {
	return &Store{db: db, drive: driveStore}
}

// Owner is the project or the task whose body a page is. Both are 0 for a page of its own.
type Owner struct {
	ProjectID int64
	TaskID    int64
}

// Page is a page outside the trash. ParentID is 0 at the top level, FolderID is 0 until the page
// has attachments, and UpdatedAt is when its title or body was last saved.
type Page struct {
	ID        int64
	ParentID  int64
	Owner     Owner
	Title     string
	Body      string
	FolderID  int64
	UpdatedAt time.Time
}

// Crumb is one link of the path to a page.
type Crumb struct {
	Title string
	URL   string
}

// Entry is a page in a list. Path names the section and the pages above it, as "Pages / 기술 정리",
// and URL is the screen of the project or task when the page is the body of one.
type Entry struct {
	ID    int64
	Title string
	Path  string
	URL   string
}

// CleanTitle trims title and composes its Unicode (NFC), as the drive does with names.
func CleanTitle(title string) (string, error) {
	title = norm.NFC.String(strings.TrimSpace(title))
	if title == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidTitle)
	}
	return title, nil
}

func PageURL(id int64) string {
	return pagesURL + "/pages/" + strconv.FormatInt(id, 10)
}

// entryURL is the screen of page id, whose owner is owner.
func entryURL(id int64, owner Owner) string {
	switch {
	case owner.ProjectID != 0:
		return projectsURL + "/" + strconv.FormatInt(owner.ProjectID, 10)
	case owner.TaskID != 0:
		return tasksURL + "/" + strconv.FormatInt(owner.TaskID, 10)
	}
	return PageURL(id)
}

// section is the list that a tree of pages belongs to, by the owner of its top page.
func section(owner Owner) Crumb {
	switch {
	case owner.ProjectID != 0:
		return Crumb{Title: "Projects", URL: projectsURL}
	case owner.TaskID != 0:
		return Crumb{Title: "Tasks", URL: tasksURL}
	}
	return Crumb{Title: "Pages", URL: pagesURL}
}

// joinPath appends path to the name of a section unless path is empty.
func joinPath(section, path string) string {
	if path == "" {
		return section
	}
	return section + " / " + path
}

// Page returns page id. It returns ErrNotFound when there is no such page or when it or a page
// above it is in the trash.
func (s *Store) Page(ctx context.Context, id int64) (Page, error) {
	query := `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, trashed_at FROM pages WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id, p.trashed_at FROM pages p JOIN chain c ON p.id = c.parent_id
		)
		SELECT id, parent_id, project_id, task_id, title, body, folder_id, updated_at FROM pages
		WHERE id = $1 AND NOT EXISTS (SELECT 1 FROM chain WHERE trashed_at IS NOT NULL)`
	var (
		p                             Page
		parent, project, task, folder sql.Null[int64]
	)
	err := s.db.QueryRowContext(ctx, query, id).Scan(&p.ID, &parent, &project, &task, &p.Title, &p.Body, &folder, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("query page: %w", err)
	}
	p.ParentID, p.FolderID = parent.V, folder.V
	p.Owner = Owner{ProjectID: project.V, TaskID: task.V}
	return p, nil
}

// Create adds a page of its own titled title under parent, or at the top level when parent is 0,
// and returns its id. It returns ErrInvalidTitle when the title is blank and ErrInvalidParent when
// parent is not a page outside the trash.
func (s *Store) Create(ctx context.Context, parent int64, title string) (int64, error) {
	title, err := CleanTitle(title)
	if err != nil {
		return 0, err
	}
	if parent != 0 {
		ok, err := s.isVisibleOutside(ctx, parent, 0)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, fmt.Errorf("%w: %d", ErrInvalidParent, parent)
		}
	}
	var id int64
	if err := s.db.QueryRowContext(ctx, `INSERT INTO pages (parent_id, title) VALUES ($1, $2) RETURNING id`, nullID(parent), title).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert page: %w", err)
	}
	return id, nil
}

// CreateOwned adds in tx the page of owner titled title, with its attachments in folder (0 for
// none), and returns its id. It returns ErrInvalidTitle when the title is blank and ErrTitleTaken
// when the page of another project has the title.
func (s *Store) CreateOwned(ctx context.Context, tx *sql.Tx, owner Owner, title string, folder int64) (int64, error) {
	title, err := CleanTitle(title)
	if err != nil {
		return 0, err
	}
	var id int64
	query := `INSERT INTO pages (project_id, task_id, title, folder_id) VALUES ($1, $2, $3, $4) RETURNING id`
	err = tx.QueryRowContext(ctx, query, nullID(owner.ProjectID), nullID(owner.TaskID), title, nullID(folder)).Scan(&id)
	if isPgError(err, uniqueViolation) {
		return 0, ErrTitleTaken
	}
	if err != nil {
		return 0, fmt.Errorf("insert page: %w", err)
	}
	return id, nil
}

// SaveContent stores title and body as page id if the page was last saved at version, and returns
// the time of this save, its next version. Line breaks are stored as "\n", since forms send them as
// "\r\n". The folder of a page of its own is renamed with the title. It returns ErrInvalidTitle, ErrStale when the page was saved since version, ErrTitleTaken
// when the page of another project has the title, and ErrNotFound when there is no such page outside
// the trash.
func (s *Store) SaveContent(ctx context.Context, id int64, title, body string, version time.Time) (time.Time, error) {
	title, err := CleanTitle(title)
	if err != nil {
		return time.Time{}, err
	}
	cur, err := s.Page(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	saved, err := s.writeContent(ctx, id, title, body, version)
	if err != nil {
		return time.Time{}, err
	}
	if cur.Owner == (Owner{}) && title != cur.Title {
		if err := s.renameFolder(ctx, cur, title); err != nil {
			return time.Time{}, err
		}
	}
	return saved, nil
}

// writeContent stores title and body as page id, if it was last saved at version, with the links of
// the body in one transaction.
func (s *Store) writeContent(ctx context.Context, id int64, title, body string, version time.Time) (time.Time, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	var saved time.Time
	query := `
		UPDATE pages SET title = $2, body = $3, updated_at = now()
		WHERE id = $1 AND updated_at = $4 AND trashed_at IS NULL
		RETURNING updated_at`
	err = tx.QueryRowContext(ctx, query, id, title, body, version).Scan(&saved)
	if errors.Is(err, sql.ErrNoRows) {
		return s.savedAs(ctx, id, title, body)
	}
	if isPgError(err, uniqueViolation) {
		return time.Time{}, ErrTitleTaken
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("update page: %w", err)
	}
	if err := writeLinks(ctx, tx, id, body); err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, fmt.Errorf("commit: %w", err)
	}
	return saved, nil
}

// savedAs returns when page id was last saved if it holds title and body already. It returns ErrStale
// when it holds something else, and ErrNotFound when it is gone or in the trash.
func (s *Store) savedAs(ctx context.Context, id int64, title, body string) (time.Time, error) {
	var (
		saved time.Time
		same  bool
	)
	query := `SELECT updated_at, title = $2 AND body = $3 FROM pages WHERE id = $1 AND trashed_at IS NULL`
	err := s.db.QueryRowContext(ctx, query, id, title, body).Scan(&saved, &same)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("query page: %w", err)
	}
	if !same {
		return time.Time{}, ErrStale
	}
	return saved, nil
}

// Move puts page id, a page of its own, under parent, or at the top level when parent is 0, without
// changing its version. It returns ErrInvalidParent when parent is id, a page under it, or not a
// page outside the trash, or when id belongs to a project or task, and ErrNotFound when there is no
// such page. Moves run one at a time, so two moves at once cannot put two pages under each other.
func (s *Store) Move(ctx context.Context, id, parent int64) error {
	p, err := s.Page(ctx, id)
	if err != nil {
		return err
	}
	if p.Owner != (Owner{}) {
		return fmt.Errorf("%w: the page of a project or task stays at the top", ErrInvalidParent)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, moveLock); err != nil {
		return fmt.Errorf("lock page moves: %w", err)
	}
	if parent != 0 {
		ok, err := s.isVisibleOutside(ctx, parent, id)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %d", ErrInvalidParent, parent)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pages SET parent_id = $2 WHERE id = $1`, id, nullID(parent)); err != nil {
		return fmt.Errorf("move page: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// isVisibleOutside reports whether page id exists and neither it nor a page above it is in the
// trash or is page root.
func (s *Store) isVisibleOutside(ctx context.Context, id, root int64) (bool, error) {
	query := `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, trashed_at FROM pages WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id, p.trashed_at FROM pages p JOIN chain c ON p.id = c.parent_id
		)
		SELECT coalesce(bool_and(trashed_at IS NULL AND id <> $2), false) FROM chain`
	var ok bool
	if err := s.db.QueryRowContext(ctx, query, id, root).Scan(&ok); err != nil {
		return false, fmt.Errorf("check page: %w", err)
	}
	return ok, nil
}

// Crumbs returns the path to p: its section and then the pages above it, top first.
func (s *Store) Crumbs(ctx context.Context, p Page) ([]Crumb, error) {
	query := `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, title, project_id, task_id, 0 AS depth FROM pages WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id, p.title, p.project_id, p.task_id, c.depth + 1
			FROM pages p JOIN chain c ON p.id = c.parent_id
		)
		SELECT id, title, project_id, task_id FROM chain ORDER BY depth DESC`
	rows, err := s.db.QueryContext(ctx, query, p.ParentID)
	if err != nil {
		return nil, fmt.Errorf("query page path: %w", err)
	}
	defer rows.Close()
	var crumbs []Crumb
	for rows.Next() {
		var (
			id            int64
			title         string
			project, task sql.Null[int64]
		)
		if err := rows.Scan(&id, &title, &project, &task); err != nil {
			return nil, fmt.Errorf("scan page path: %w", err)
		}
		owner := Owner{ProjectID: project.V, TaskID: task.V}
		if len(crumbs) == 0 {
			crumbs = append(crumbs, section(owner))
		}
		crumbs = append(crumbs, Crumb{Title: title, URL: entryURL(id, owner)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query page path: %w", err)
	}
	if len(crumbs) == 0 {
		crumbs = append(crumbs, section(Owner{}))
	}
	return crumbs, nil
}

// TopPages returns the pages of their own at the top level outside the trash, by title.
func (s *Store) TopPages(ctx context.Context) ([]Entry, error) {
	query := `
		SELECT id, title FROM pages
		WHERE parent_id IS NULL AND project_id IS NULL AND task_id IS NULL AND trashed_at IS NULL
		ORDER BY lower(title), id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query top pages: %w", err)
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Title); err != nil {
			return nil, fmt.Errorf("scan top page: %w", err)
		}
		e.URL = PageURL(e.ID)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query top pages: %w", err)
	}
	return entries, nil
}

// ParentChoices returns the pages outside the trash that page id can move under, which are all but
// id and the pages under it, by path.
func (s *Store) ParentChoices(ctx context.Context, id int64) ([]Entry, error) {
	query := `
		WITH RECURSIVE tree AS (
			SELECT id, title, project_id, task_id, ''::text AS path
			FROM pages WHERE parent_id IS NULL AND trashed_at IS NULL AND id <> $1
			UNION ALL
			SELECT p.id, p.title, tree.project_id, tree.task_id, concat_ws(' / ', nullif(tree.path, ''), tree.title)
			FROM pages p JOIN tree ON p.parent_id = tree.id
			WHERE p.trashed_at IS NULL AND p.id <> $1
		)
		SELECT id, title, path, project_id, task_id FROM tree`
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("query parent choices: %w", err)
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var (
			e             Entry
			path          string
			project, task sql.Null[int64]
		)
		if err := rows.Scan(&e.ID, &e.Title, &path, &project, &task); err != nil {
			return nil, fmt.Errorf("scan parent choice: %w", err)
		}
		e.Path = joinPath(section(Owner{ProjectID: project.V, TaskID: task.V}).Title, path)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query parent choices: %w", err)
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		return strings.Compare(strings.ToLower(a.Path+" / "+a.Title), strings.ToLower(b.Path+" / "+b.Title))
	})
	return entries, nil
}

// describe returns pages ids in that order with their paths, leaving out the ones in the trash or
// under a page in the trash unless hidden is set.
func (s *Store) describe(ctx context.Context, ids []int64, hidden bool) ([]Entry, error) {
	query := `
		WITH RECURSIVE chain AS (
			SELECT id AS page_id, id, parent_id, title, project_id, task_id, trashed_at, 0 AS depth
			FROM pages WHERE id = ANY($1)
			UNION ALL
			SELECT c.page_id, p.id, p.parent_id, p.title, p.project_id, p.task_id, p.trashed_at, c.depth + 1
			FROM chain c JOIN pages p ON p.id = c.parent_id
		)
		SELECT page_id,
			max(title) FILTER (WHERE depth = 0),
			coalesce(string_agg(title, ' / ' ORDER BY depth DESC) FILTER (WHERE depth > 0), ''),
			max(project_id) FILTER (WHERE parent_id IS NULL),
			max(task_id) FILTER (WHERE parent_id IS NULL)
		FROM chain
		GROUP BY page_id
		HAVING $2 OR bool_and(trashed_at IS NULL)`
	rows, err := s.db.QueryContext(ctx, query, ids, hidden)
	if err != nil {
		return nil, fmt.Errorf("query page paths: %w", err)
	}
	defer rows.Close()
	byID := make(map[int64]Entry, len(ids))
	for rows.Next() {
		var (
			e             Entry
			path          string
			project, task sql.Null[int64]
		)
		if err := rows.Scan(&e.ID, &e.Title, &path, &project, &task); err != nil {
			return nil, fmt.Errorf("scan page path: %w", err)
		}
		owner := Owner{ProjectID: project.V, TaskID: task.V}
		e.Path, e.URL = joinPath(section(owner).Title, path), PageURL(e.ID)
		if path == "" {
			e.URL = entryURL(e.ID, owner)
		}
		byID[e.ID] = e
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query page paths: %w", err)
	}
	var entries []Entry
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func nullID(id int64) sql.Null[int64] {
	return sql.Null[int64]{V: id, Valid: id != 0}
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

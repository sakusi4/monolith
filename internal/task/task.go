// Package task keeps projects and their tasks. The title, markdown body, and attachments of each
// live in its page, which the package changes only through the page Store. A project owns a folder
// in the drive, and so does a task once it has attachments; the package changes them only through
// the drive's Store.
package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/internal/page"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidTask    = errors.New("invalid task")
	ErrInvalidProject = errors.New("invalid project")
	ErrNameTaken      = errors.New("project name taken")
	ErrFolderTaken    = errors.New("project folder name taken")
	ErrNoFolder       = errors.New("project folder missing or in the trash")
)

type Store struct {
	db    *sql.DB
	drive *drive.Store
	pages *page.Store
}

func NewStore(db *sql.DB, driveStore *drive.Store, pageStore *page.Store) *Store {
	return &Store{db: db, drive: driveStore, pages: pageStore}
}

type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
	StatusCanceled   TaskStatus = "canceled"
)

var (
	taskStatuses = []TaskStatus{StatusTodo, StatusInProgress, StatusDone, StatusCanceled}
	openStatuses = []TaskStatus{StatusTodo, StatusInProgress}
)

func (s TaskStatus) Label() string {
	switch s {
	case StatusTodo:
		return "To do"
	case StatusInProgress:
		return "In progress"
	case StatusDone:
		return "Done"
	case StatusCanceled:
		return "Canceled"
	}
	return string(s)
}

func (s TaskStatus) IsOpen() bool {
	return slices.Contains(openStatuses, s)
}

// Task is a task with the name of its project, which is empty for a task in the inbox, and the title,
// body, and folder of its page. Due and CompletedAt are zero when unset, and FolderID is 0 until the
// task has attachments.
type Task struct {
	ID          int64
	PageID      int64
	Title       string
	ProjectID   int64
	ProjectName string
	Status      TaskStatus
	Due         time.Time
	Body        string
	FolderID    int64
	CompletedAt time.Time
	CreatedAt   time.Time
}

// TaskInput is a task as added or edited. ProjectID is 0 for the inbox, and Due is zero for no due date.
type TaskInput struct {
	Title     string
	ProjectID int64
	Status    TaskStatus
	Due       time.Time
	Body      string
}

// TaskFilter picks the tasks with one of Statuses, in project ProjectID or in every project when
// it is 0, and only in the inbox when Inbox is set.
type TaskFilter struct {
	Statuses  []TaskStatus
	ProjectID int64
	Inbox     bool
}

func (in TaskInput) Clean() (TaskInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	switch {
	case in.Title == "":
		return TaskInput{}, fmt.Errorf("%w: title is empty", ErrInvalidTask)
	case !slices.Contains(taskStatuses, in.Status):
		return TaskInput{}, fmt.Errorf("%w: unknown status %q", ErrInvalidTask, in.Status)
	}
	return in, nil
}

func nullID(id int64) sql.Null[int64] {
	return sql.Null[int64]{V: id, Valid: id != 0}
}

func nullDate(t time.Time) sql.Null[time.Time] {
	return sql.Null[time.Time]{V: t, Valid: !t.IsZero()}
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

// Tasks returns the tasks that f picks, oldest first.
func (s *Store) Tasks(ctx context.Context, f TaskFilter) ([]Task, error) {
	return s.tasks(ctx, f, 0)
}

// Task returns task id. It returns ErrNotFound when there is no such task.
func (s *Store) Task(ctx context.Context, id int64) (Task, error) {
	tasks, err := s.tasks(ctx, TaskFilter{Statuses: taskStatuses}, id)
	if err != nil {
		return Task{}, err
	}
	if len(tasks) == 0 {
		return Task{}, ErrNotFound
	}
	return tasks[0], nil
}

// tasks returns the tasks that f picks, oldest first, only task id when it is not 0.
func (s *Store) tasks(ctx context.Context, f TaskFilter, id int64) ([]Task, error) {
	statuses := make([]string, len(f.Statuses))
	for i, st := range f.Statuses {
		statuses[i] = string(st)
	}
	query := `
		SELECT t.id, pg.id, pg.title, t.project_id, coalesce(pp.title, ''), t.status, t.due_on, pg.body, pg.folder_id, t.completed_at, t.created_at
		FROM tasks t
		JOIN pages pg ON pg.task_id = t.id
		LEFT JOIN pages pp ON pp.project_id = t.project_id
		WHERE t.status = ANY($1) AND ($2 = 0 OR t.project_id = $2) AND (NOT $3 OR t.project_id IS NULL) AND ($4 = 0 OR t.id = $4)
		ORDER BY t.created_at, t.id`
	rows, err := s.db.QueryContext(ctx, query, statuses, f.ProjectID, f.Inbox, id)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var (
			t               Task
			project, folder sql.Null[int64]
			due, completed  sql.Null[time.Time]
		)
		if err := rows.Scan(&t.ID, &t.PageID, &t.Title, &project, &t.ProjectName, &t.Status, &due, &t.Body, &folder, &completed, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		t.ProjectID, t.Due, t.FolderID, t.CompletedAt = project.V, due.V, folder.V, completed.V
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	return tasks, nil
}

// AddTask adds in with an empty body. It returns ErrInvalidTask when in breaks a rule or names a
// project that does not exist.
func (s *Store) AddTask(ctx context.Context, in TaskInput) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	var id int64
	query := `
		INSERT INTO tasks (project_id, status, due_on, completed_at)
		VALUES ($1, $2, $3, CASE WHEN $2 = 'done' THEN now() END) RETURNING id`
	err = tx.QueryRowContext(ctx, query, nullID(in.ProjectID), in.Status, nullDate(in.Due)).Scan(&id)
	if isPgError(err, foreignKeyViolation) {
		return fmt.Errorf("%w: no project %d", ErrInvalidTask, in.ProjectID)
	}
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	if _, err := s.pages.CreateOwned(ctx, tx, page.Owner{TaskID: id}, in.Title, 0); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// UpdateTask saves in as task id and moves the task's folder to its project and renames it after
// the title, unless the folder is gone or in the trash. When the title or body changed or uploads
// came, it saves them to the task's page, which attaches the uploads that the body links to and moves
// the files it no longer links to the trash as page.Store.Save does. A task saved as done keeps the time it was first completed, and
// one saved as any other status loses it. It returns ErrInvalidTask when in breaks a rule or names a
// project that does not exist, ErrNoFolder when the task has a folder and the new project has none,
// the errors of AttachToTask when an upload cannot be attached, and ErrNotFound when there is no
// such task.
func (s *Store) UpdateTask(ctx context.Context, id int64, in TaskInput, uploads []drive.Upload) error {
	in, err := in.Clean()
	if err != nil {
		return errors.Join(err, s.drive.Discard(uploads))
	}
	cur, err := s.Task(ctx, id)
	if err != nil {
		return errors.Join(err, s.drive.Discard(uploads))
	}
	if in.ProjectID != cur.ProjectID {
		if err := s.moveTask(ctx, cur, in.ProjectID); err != nil {
			return errors.Join(err, s.drive.Discard(uploads))
		}
	}
	if in.Title != cur.Title || in.Body != cur.Body || len(uploads) > 0 {
		err := s.pages.Save(ctx, cur.PageID, page.Input{Title: in.Title, Body: in.Body}, uploads, func(used []drive.Upload) ([]drive.File, error) {
			return s.AttachToTask(ctx, id, used)
		})
		if err != nil {
			return err
		}
	}
	query := `
		UPDATE tasks SET status = $2, due_on = $3,
			completed_at = CASE WHEN $2 = 'done' THEN coalesce(completed_at, now()) END, updated_at = now()
		WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id, in.Status, nullDate(in.Due))
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	if err := requireRow(res); err != nil {
		return err
	}
	if in.Title != cur.Title && cur.FolderID != 0 {
		return s.renameTaskFolder(ctx, cur, in.Title)
	}
	return nil
}

// DeleteTask removes task id and moves its page to the page trash, leaving its drive folder. It
// returns ErrNotFound when there is no such task.
func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	t, err := s.Task(ctx, id)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if err := requireRow(res); err != nil {
		return err
	}
	return s.pages.Trash(ctx, []int64{t.PageID})
}

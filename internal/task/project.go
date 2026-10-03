package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/internal/page"
)

const projectsFolder = "Projects"

type ProjectStatus string

const (
	ProjectPlanned  ProjectStatus = "planned"
	ProjectActive   ProjectStatus = "active"
	ProjectPaused   ProjectStatus = "paused"
	ProjectDone     ProjectStatus = "done"
	ProjectCanceled ProjectStatus = "canceled"
)

var projectStatuses = []ProjectStatus{ProjectPlanned, ProjectActive, ProjectPaused, ProjectDone, ProjectCanceled}

func (s ProjectStatus) Label() string {
	switch s {
	case ProjectPlanned:
		return "Planned"
	case ProjectActive:
		return "Active"
	case ProjectPaused:
		return "Paused"
	case ProjectDone:
		return "Done"
	case ProjectCanceled:
		return "Canceled"
	}
	return string(s)
}

// Project is a project with the name, body, and folder of its page and the number of its tasks that
// are still open. The dates are zero when unset, and FolderID is 0 once its drive folder is deleted.
type Project struct {
	ID            int64
	PageID        int64
	PageUpdatedAt time.Time
	Name          string
	Status        ProjectStatus
	StartedOn     time.Time
	FinishedOn    time.Time
	Body          string
	FolderID      int64
	OpenTasks     int
}

// ProjectInput is a project as created or edited. The dates are zero when unset.
type ProjectInput struct {
	Name       string
	Status     ProjectStatus
	StartedOn  time.Time
	FinishedOn time.Time
}

// Clean applies the drive's folder name rule to the name, which also names the project's folder.
func (in ProjectInput) Clean() (ProjectInput, error) {
	name, err := drive.CleanName(in.Name)
	if err != nil {
		return ProjectInput{}, fmt.Errorf("%w: %w", ErrInvalidProject, err)
	}
	in.Name = name
	switch {
	case !slices.Contains(projectStatuses, in.Status):
		return ProjectInput{}, fmt.Errorf("%w: unknown status %q", ErrInvalidProject, in.Status)
	case !in.StartedOn.IsZero() && !in.FinishedOn.IsZero() && in.FinishedOn.Before(in.StartedOn):
		return ProjectInput{}, fmt.Errorf("%w: finishes before it starts", ErrInvalidProject)
	}
	return in, nil
}

// Projects returns the projects with one of statuses by name.
func (s *Store) Projects(ctx context.Context, statuses []ProjectStatus) ([]Project, error) {
	return s.projects(ctx, statuses, 0)
}

// Project returns project id. It returns ErrNotFound when there is no such project.
func (s *Store) Project(ctx context.Context, id int64) (Project, error) {
	projects, err := s.projects(ctx, projectStatuses, id)
	if err != nil {
		return Project{}, err
	}
	if len(projects) == 0 {
		return Project{}, ErrNotFound
	}
	return projects[0], nil
}

// projects returns the projects with one of statuses by name, only project id when it is not 0.
func (s *Store) projects(ctx context.Context, statuses []ProjectStatus, id int64) ([]Project, error) {
	names := make([]string, len(statuses))
	for i, st := range statuses {
		names[i] = string(st)
	}
	open := make([]string, len(openStatuses))
	for i, st := range openStatuses {
		open[i] = string(st)
	}
	query := `
		SELECT p.id, pg.id, pg.updated_at, pg.title, p.status, p.started_on, p.finished_on, pg.body, pg.folder_id,
			count(t.id) FILTER (WHERE t.status = ANY($3))
		FROM projects p
		JOIN pages pg ON pg.project_id = p.id
		LEFT JOIN tasks t ON t.project_id = p.id
		WHERE p.status = ANY($1) AND ($2 = 0 OR p.id = $2)
		GROUP BY p.id, pg.id
		ORDER BY lower(pg.title), p.id`
	rows, err := s.db.QueryContext(ctx, query, names, id, open)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var (
			p                 Project
			started, finished sql.Null[time.Time]
			folder            sql.Null[int64]
		)
		if err := rows.Scan(&p.ID, &p.PageID, &p.PageUpdatedAt, &p.Name, &p.Status, &started, &finished, &p.Body, &folder, &p.OpenTasks); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.StartedOn, p.FinishedOn, p.FolderID = started.V, finished.V, folder.V
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	return projects, nil
}

// CreateProject adds a project with an empty body and a new drive folder, Projects/<name>, and
// returns its id. It returns ErrInvalidProject, ErrNameTaken, or ErrFolderTaken when the input is
// not allowed.
func (s *Store) CreateProject(ctx context.Context, in ProjectInput) (int64, error) {
	in, err := in.Clean()
	if err != nil {
		return 0, err
	}
	taken, err := s.nameTaken(ctx, in.Name, 0)
	if err != nil {
		return 0, err
	}
	if taken {
		return 0, ErrNameTaken
	}
	parent, err := s.drive.EnsureFolder(ctx, 0, projectsFolder)
	if err != nil {
		return 0, err
	}
	folder, err := s.drive.CreateFolder(ctx, parent, in.Name)
	if errors.Is(err, drive.ErrNameTaken) {
		return 0, ErrFolderTaken
	}
	if err != nil {
		return 0, err
	}
	id, err := s.insertProject(ctx, in, folder)
	if errors.Is(err, page.ErrTitleTaken) {
		return 0, errors.Join(ErrNameTaken, s.drive.TrashFolder(ctx, folder))
	}
	if err != nil {
		return 0, errors.Join(err, s.drive.TrashFolder(ctx, folder))
	}
	return id, nil
}

// insertProject adds the row and the page of project in, with its attachments in folder, in one
// transaction.
func (s *Store) insertProject(ctx context.Context, in ProjectInput, folder int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	var id int64
	query := `INSERT INTO projects (status, started_on, finished_on) VALUES ($1, $2, $3) RETURNING id`
	if err := tx.QueryRowContext(ctx, query, in.Status, nullDate(in.StartedOn), nullDate(in.FinishedOn)).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert project: %w", err)
	}
	if _, err := s.pages.CreateOwned(ctx, tx, page.Owner{ProjectID: id}, in.Name, folder); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// UpdateProjectFields saves the status and dates of project id. It returns ErrInvalidProject when
// the status is unknown or the project finishes before it starts, and ErrNotFound when there is no
// such project.
func (s *Store) UpdateProjectFields(ctx context.Context, id int64, status ProjectStatus, started, finished time.Time) error {
	cur, err := s.Project(ctx, id)
	if err != nil {
		return err
	}
	if _, err := (ProjectInput{Name: cur.Name, Status: status, StartedOn: started, FinishedOn: finished}).Clean(); err != nil {
		return err
	}
	query := `UPDATE projects SET status = $2, started_on = $3, finished_on = $4, updated_at = now() WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id, status, nullDate(started), nullDate(finished))
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	return requireRow(res)
}

// SaveProjectContent saves name and body as the page of project id if it was last saved at version,
// and renames the project's drive folder with the name, unless the folder is gone or in the trash.
// It returns the next version, ErrInvalidProject when the name breaks the drive's name rule,
// ErrNameTaken or ErrFolderTaken when another project or folder has the name, and the errors of
// page.Store.SaveContent.
func (s *Store) SaveProjectContent(ctx context.Context, id int64, name, body string, version time.Time) (time.Time, error) {
	name, err := drive.CleanName(name)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %w", ErrInvalidProject, err)
	}
	cur, err := s.Project(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	taken, err := s.nameTaken(ctx, name, id)
	if err != nil {
		return time.Time{}, err
	}
	if taken {
		return time.Time{}, ErrNameTaken
	}
	if !cur.PageUpdatedAt.Equal(version) {
		return time.Time{}, page.ErrStale
	}
	if name != cur.Name && cur.FolderID != 0 {
		if err := s.renameFolder(ctx, cur.FolderID, name); err != nil {
			return time.Time{}, err
		}
	}
	saved, err := s.pages.SaveContent(ctx, cur.PageID, name, body, version)
	if errors.Is(err, page.ErrTitleTaken) {
		return time.Time{}, ErrNameTaken
	}
	return saved, err
}

// renameFolder renames the drive folder id to name, unless it is gone or in the trash.
func (s *Store) renameFolder(ctx context.Context, id int64, name string) error {
	path, err := s.drive.Path(ctx, id)
	if errors.Is(err, drive.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	err = s.drive.UpdateFolder(ctx, id, path[len(path)-1].ParentID, name)
	if errors.Is(err, drive.ErrNameTaken) {
		return ErrFolderTaken
	}
	return err
}

func (s *Store) nameTaken(ctx context.Context, name string, except int64) (bool, error) {
	var taken bool
	query := `SELECT EXISTS (SELECT 1 FROM pages WHERE project_id IS NOT NULL AND project_id <> $2 AND title = $1)`
	if err := s.db.QueryRowContext(ctx, query, name, except).Scan(&taken); err != nil {
		return false, fmt.Errorf("check project name: %w", err)
	}
	return taken, nil
}

// DeleteProject removes project id with its tasks and moves their pages to the page trash, leaving
// their drive folders. The project's folder takes the name of the folder of a page of its own, which
// frees its name for a new project. It returns ErrNotFound when there is no such project.
func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	p, err := s.Project(ctx, id)
	if err != nil {
		return err
	}
	pages, err := s.projectPages(ctx, id)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if err := requireRow(res); err != nil {
		return err
	}
	if err := s.pages.RenameFolder(ctx, p.PageID); err != nil {
		return err
	}
	return s.pages.Trash(ctx, pages)
}

// projectPages returns the page of project id and the pages of its tasks.
func (s *Store) projectPages(ctx context.Context, id int64) ([]int64, error) {
	query := `
		SELECT id FROM pages WHERE project_id = $1
		UNION ALL
		SELECT pg.id FROM pages pg JOIN tasks t ON t.id = pg.task_id WHERE t.project_id = $1`
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("query project pages: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var pageID int64
		if err := rows.Scan(&pageID); err != nil {
			return nil, fmt.Errorf("scan project page: %w", err)
		}
		ids = append(ids, pageID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query project pages: %w", err)
	}
	return ids, nil
}

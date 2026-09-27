package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/drive"
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

// Project is a project with the number of its tasks that are still open. The dates are zero when
// unset, and FolderID is 0 once its drive folder is deleted.
type Project struct {
	ID         int64
	Name       string
	Status     ProjectStatus
	StartedOn  time.Time
	FinishedOn time.Time
	Body       string
	FolderID   int64
	OpenTasks  int
}

// ProjectInput is a project as created or edited. The dates are zero when unset.
type ProjectInput struct {
	Name       string
	Status     ProjectStatus
	StartedOn  time.Time
	FinishedOn time.Time
	Body       string
}

// Clean applies the drive's folder name rule to the name, which also names the project's folder.
func (in ProjectInput) Clean() (ProjectInput, error) {
	name, err := drive.CleanName(in.Name)
	if err != nil {
		return ProjectInput{}, fmt.Errorf("%w: %w", ErrInvalidProject, err)
	}
	in.Name = name
	in.Body = strings.TrimSpace(in.Body)
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
		SELECT p.id, p.name, p.status, p.started_on, p.finished_on, p.body, p.folder_id,
			count(t.id) FILTER (WHERE t.status = ANY($3))
		FROM projects p LEFT JOIN tasks t ON t.project_id = p.id
		WHERE p.status = ANY($1) AND ($2 = 0 OR p.id = $2)
		GROUP BY p.id
		ORDER BY lower(p.name), p.id`
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
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &started, &finished, &p.Body, &folder, &p.OpenTasks); err != nil {
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

// CreateProject adds a project with a new drive folder, Projects/<name>, and returns its id. It
// returns ErrInvalidProject, ErrNameTaken, or ErrFolderTaken when the input is not allowed.
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
	var id int64
	query := `
		INSERT INTO projects (name, status, started_on, finished_on, body, folder_id)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	err = s.db.QueryRowContext(ctx, query, in.Name, in.Status, nullDate(in.StartedOn), nullDate(in.FinishedOn), in.Body, folder).Scan(&id)
	if isPgError(err, uniqueViolation) {
		return 0, errors.Join(ErrNameTaken, s.drive.TrashFolder(ctx, folder))
	}
	if err != nil {
		return 0, errors.Join(fmt.Errorf("insert project: %w", err), s.drive.TrashFolder(ctx, folder))
	}
	return id, nil
}

// UpdateProject saves in as project id and renames its drive folder with it, unless the folder is
// gone or in the trash. It attaches the uploads that the body links to and moves the files it no
// longer links to the trash as UpdateTask does. It
// returns the errors of CreateProject, the errors of AttachToProject when an upload cannot be
// attached, and ErrNotFound when there is no such project.
func (s *Store) UpdateProject(ctx context.Context, id int64, in ProjectInput, uploads []drive.Upload) error {
	in, err := in.Clean()
	if err != nil {
		return errors.Join(err, s.drive.Discard(uploads))
	}
	cur, err := s.Project(ctx, id)
	if err != nil {
		return errors.Join(err, s.drive.Discard(uploads))
	}
	taken, err := s.nameTaken(ctx, in.Name, id)
	if err != nil {
		return errors.Join(err, s.drive.Discard(uploads))
	}
	if taken {
		return errors.Join(ErrNameTaken, s.drive.Discard(uploads))
	}
	in.Body, err = s.attachLinked(in.Body, uploads, func(used []drive.Upload) ([]drive.File, error) {
		return s.AttachToProject(ctx, id, used)
	})
	if err != nil {
		return err
	}
	if in.Name != cur.Name && cur.FolderID != 0 {
		if err := s.renameFolder(ctx, cur.FolderID, in.Name); err != nil {
			return err
		}
	}
	query := `
		UPDATE projects SET name = $2, status = $3, started_on = $4, finished_on = $5, body = $6, updated_at = now()
		WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id, in.Name, in.Status, nullDate(in.StartedOn), nullDate(in.FinishedOn), in.Body)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	if err := requireRow(res); err != nil {
		return err
	}
	return s.trashDropped(ctx, cur.FolderID, cur.Body, in.Body)
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
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE name = $1 AND id <> $2)`, name, except).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("check project name: %w", err)
	}
	return taken, nil
}

// DeleteProject removes project id with its tasks and moves its folder to the trash. It returns
// ErrNotFound when there is no such project.
func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	p, err := s.Project(ctx, id)
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
	return s.trashFolder(ctx, p.FolderID)
}

package task

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/sakusi4/monolith/internal/drive"
)

const (
	tasksFolder         = "Tasks"
	inboxFolder         = "Inbox"
	maxFolderNameLength = 200
)

var fileLink = regexp.MustCompile(`\]\(/drive/files/(\d+)/content\)`)

// taskFolderName is the drive folder name of task id titled title, and the fallback to use when
// that name is taken.
func taskFolderName(title string, id int64) (name, fallback string) {
	name = strings.TrimSpace(strings.ReplaceAll(title, "/", "-"))
	if runes := []rune(name); len(runes) > maxFolderNameLength {
		name = strings.TrimSpace(string(runes[:maxFolderNameLength]))
	}
	if name == "" {
		name = "Task " + strconv.FormatInt(id, 10)
	}
	return name, fmt.Sprintf("%s (#%d)", name, id)
}

// usedUploads splits uploads into the ones that body links to by name, as the editor writes them,
// and the rest.
func usedUploads(body string, uploads []drive.Upload) (used, unused []drive.Upload) {
	for _, u := range uploads {
		if strings.Contains(body, "]("+u.Name+")") {
			used = append(used, u)
			continue
		}
		unused = append(unused, u)
	}
	return used, unused
}

// linkFiles points the links in body that name an upload at the file it became; ids maps the name of
// each upload to the id of its file.
func linkFiles(body string, ids map[string]int64) string {
	for name, id := range ids {
		body = strings.ReplaceAll(body, "]("+name+")", "]("+drive.FileURL(id)+"/content)")
	}
	return body
}

// droppedFiles returns the ids of the files that before links to and after no longer does.
func droppedFiles(before, after string) []int64 {
	var dropped []int64
	for _, m := range fileLink.FindAllStringSubmatch(before, -1) {
		if strings.Contains(after, m[0]) {
			continue
		}
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		dropped = append(dropped, id)
	}
	return dropped
}

// trashDropped moves to the trash the files in folder that before links to and after no longer does.
func (s *Store) trashDropped(ctx context.Context, folder int64, before, after string) error {
	dropped := droppedFiles(before, after)
	if folder == 0 || len(dropped) == 0 {
		return nil
	}
	return s.drive.TrashFiles(ctx, folder, dropped)
}

// attachLinked adds with attach the uploads that body links to, discards the rest, and returns body
// with those links pointing at the added files. It uses up uploads either way.
func (s *Store) attachLinked(body string, uploads []drive.Upload, attach func([]drive.Upload) ([]drive.File, error)) (string, error) {
	used, unused := usedUploads(body, uploads)
	if err := s.drive.Discard(unused); err != nil {
		return "", errors.Join(err, s.drive.Discard(used))
	}
	if len(used) == 0 {
		return body, nil
	}
	names := make([]string, len(used))
	for i, u := range used {
		names[i] = u.Name
	}
	files, err := attach(used)
	if err != nil {
		return "", err
	}
	ids := make(map[string]int64, len(files))
	for i, f := range files {
		ids[names[i]] = f.ID
	}
	return linkFiles(body, ids), nil
}

// AttachToTask adds uploads to the drive folder of task id, creating the folder on the first upload,
// and returns the files added. It returns ErrNoFolder when the task's project has no folder outside
// the trash, and the drive's ErrInvalidName or ErrNameTaken when a file name is not allowed. It uses
// up uploads either way.
func (s *Store) AttachToTask(ctx context.Context, id int64, uploads []drive.Upload) ([]drive.File, error) {
	t, err := s.Task(ctx, id)
	if err != nil {
		return nil, errors.Join(err, s.drive.Discard(uploads))
	}
	folder, err := s.taskFolder(ctx, t)
	if err != nil {
		return nil, errors.Join(err, s.drive.Discard(uploads))
	}
	return s.drive.AddFiles(ctx, folder, uploads)
}

// AttachToProject adds uploads to the drive folder of project id, with the errors of AttachToTask.
func (s *Store) AttachToProject(ctx context.Context, id int64, uploads []drive.Upload) ([]drive.File, error) {
	p, err := s.Project(ctx, id)
	if err != nil {
		return nil, errors.Join(err, s.drive.Discard(uploads))
	}
	live, err := s.liveFolder(ctx, p.FolderID)
	if err != nil {
		return nil, errors.Join(err, s.drive.Discard(uploads))
	}
	if !live {
		return nil, errors.Join(ErrNoFolder, s.drive.Discard(uploads))
	}
	return s.drive.AddFiles(ctx, p.FolderID, uploads)
}

// taskFolder returns the drive folder of t, creating it under its project's Tasks folder, or under
// Inbox for a task without a project, when t has none outside the trash.
func (s *Store) taskFolder(ctx context.Context, t Task) (int64, error) {
	live, err := s.liveFolder(ctx, t.FolderID)
	if err != nil {
		return 0, err
	}
	if live {
		return t.FolderID, nil
	}
	parent, err := s.taskFolderParent(ctx, t)
	if err != nil {
		return 0, err
	}
	name, fallback := taskFolderName(t.Title, t.ID)
	folder, err := s.drive.CreateFolder(ctx, parent, name)
	if errors.Is(err, drive.ErrNameTaken) {
		folder, err = s.drive.CreateFolder(ctx, parent, fallback)
	}
	if err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE tasks SET folder_id = $2 WHERE id = $1`, t.ID, folder); err != nil {
		return 0, fmt.Errorf("save task folder: %w", err)
	}
	return folder, nil
}

func (s *Store) taskFolderParent(ctx context.Context, t Task) (int64, error) {
	if t.ProjectID == 0 {
		return s.drive.EnsureFolder(ctx, 0, inboxFolder)
	}
	p, err := s.Project(ctx, t.ProjectID)
	if err != nil {
		return 0, err
	}
	live, err := s.liveFolder(ctx, p.FolderID)
	if err != nil {
		return 0, err
	}
	if !live {
		return 0, ErrNoFolder
	}
	return s.drive.EnsureFolder(ctx, p.FolderID, tasksFolder)
}

// moveTask puts t in project, or in the inbox when project is 0, and moves its folder under the new
// parent that taskFolder would pick. It returns ErrInvalidTask when there is no such project, and
// ErrNoFolder when t has a folder and the project has none outside the trash.
func (s *Store) moveTask(ctx context.Context, t Task, project int64) error {
	live, err := s.liveFolder(ctx, t.FolderID)
	if err != nil {
		return err
	}
	if live {
		parent, err := s.taskFolderParent(ctx, Task{ID: t.ID, ProjectID: project})
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: no project %d", ErrInvalidTask, project)
		}
		if err != nil {
			return err
		}
		name, fallback := taskFolderName(t.Title, t.ID)
		err = s.drive.UpdateFolder(ctx, t.FolderID, parent, name)
		if errors.Is(err, drive.ErrNameTaken) {
			err = s.drive.UpdateFolder(ctx, t.FolderID, parent, fallback)
		}
		if err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `UPDATE tasks SET project_id = $2 WHERE id = $1`, t.ID, nullID(project))
	if isPgError(err, foreignKeyViolation) {
		return fmt.Errorf("%w: no project %d", ErrInvalidTask, project)
	}
	if err != nil {
		return fmt.Errorf("move task: %w", err)
	}
	return nil
}

// renameTaskFolder renames the folder of t after title, unless the folder is gone or in the trash.
func (s *Store) renameTaskFolder(ctx context.Context, t Task, title string) error {
	path, err := s.drive.Path(ctx, t.FolderID)
	if errors.Is(err, drive.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	parent := path[len(path)-1].ParentID
	name, fallback := taskFolderName(title, t.ID)
	err = s.drive.UpdateFolder(ctx, t.FolderID, parent, name)
	if errors.Is(err, drive.ErrNameTaken) {
		err = s.drive.UpdateFolder(ctx, t.FolderID, parent, fallback)
	}
	return err
}

// liveFolder reports whether id is a drive folder outside the trash; 0 is none.
func (s *Store) liveFolder(ctx context.Context, id int64) (bool, error) {
	if id == 0 {
		return false, nil
	}
	_, err := s.drive.Path(ctx, id)
	if errors.Is(err, drive.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// trashFolder moves drive folder id to the trash, unless it is 0 or already gone or in the trash.
func (s *Store) trashFolder(ctx context.Context, id int64) error {
	if id == 0 {
		return nil
	}
	err := s.drive.TrashFolder(ctx, id)
	if errors.Is(err, drive.ErrNotFound) {
		return nil
	}
	return err
}

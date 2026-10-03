package page

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
	pagesFolder         = "Pages"
	maxFolderNameLength = 200
)

var fileLink = regexp.MustCompile(`\]\(/drive/files/(\d+)/content\)`)

// folderName is the name of the drive folder of page id titled title.
func folderName(title string, id int64) string {
	name := strings.TrimSpace(strings.ReplaceAll(title, "/", "-"))
	if runes := []rune(name); len(runes) > maxFolderNameLength {
		name = strings.TrimSpace(string(runes[:maxFolderNameLength]))
	}
	return fmt.Sprintf("%s (#%d)", name, id)
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

// Attach adds uploads to the drive folder of page id, a page of its own, creating the folder in
// Pages on the first upload, and returns the files added. It returns the drive's ErrInvalidName or
// ErrNameTaken when a file name is not allowed, and ErrNotFound when there is no such page of its
// own. It uses up uploads either way.
func (s *Store) Attach(ctx context.Context, id int64, uploads []drive.Upload) ([]drive.File, error) {
	p, err := s.Page(ctx, id)
	if err != nil {
		return nil, errors.Join(err, s.drive.Discard(uploads))
	}
	if p.Owner != (Owner{}) {
		return nil, errors.Join(ErrNotFound, s.drive.Discard(uploads))
	}
	folder, err := s.folder(ctx, p)
	if err != nil {
		return nil, errors.Join(err, s.drive.Discard(uploads))
	}
	return s.drive.AddFiles(ctx, folder, uploads)
}

// folder returns the drive folder of p, creating it in Pages when p has none outside the trash.
func (s *Store) folder(ctx context.Context, p Page) (int64, error) {
	live, err := s.isLiveFolder(ctx, p.FolderID)
	if err != nil {
		return 0, err
	}
	if live {
		return p.FolderID, nil
	}
	parent, err := s.drive.EnsureFolder(ctx, 0, pagesFolder)
	if err != nil {
		return 0, err
	}
	folder, err := s.drive.CreateFolder(ctx, parent, folderName(p.Title, p.ID))
	if err != nil {
		return 0, err
	}
	if err := s.SetFolder(ctx, p.ID, folder); err != nil {
		return 0, err
	}
	return folder, nil
}

// SetFolder makes folder the drive folder of page id.
func (s *Store) SetFolder(ctx context.Context, id, folder int64) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE pages SET folder_id = $2 WHERE id = $1`, id, nullID(folder)); err != nil {
		return fmt.Errorf("save page folder: %w", err)
	}
	return nil
}

// RenameFolder renames the drive folder of page id, a page of its own, after its title as Save does,
// unless the folder is gone or in the trash. It returns ErrNotFound when there is no such page.
func (s *Store) RenameFolder(ctx context.Context, id int64) error {
	p, err := s.Page(ctx, id)
	if err != nil {
		return err
	}
	return s.renameFolder(ctx, p, p.Title)
}

// renameFolder renames the drive folder of p after title, unless the folder is gone or in the trash.
func (s *Store) renameFolder(ctx context.Context, p Page, title string) error {
	if p.FolderID == 0 {
		return nil
	}
	path, err := s.drive.Path(ctx, p.FolderID)
	if errors.Is(err, drive.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.drive.UpdateFolder(ctx, p.FolderID, path[len(path)-1].ParentID, folderName(title, p.ID))
}

// isLiveFolder reports whether id is a drive folder outside the trash; 0 is none.
func (s *Store) isLiveFolder(ctx context.Context, id int64) (bool, error) {
	if id == 0 {
		return false, nil
	}
	_, err := s.drive.Path(ctx, id)
	if errors.Is(err, drive.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
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

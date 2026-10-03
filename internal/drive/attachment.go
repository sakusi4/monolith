package drive

import (
	"context"
	"errors"
)

// Attachments is the files at the top of a folder, as the screens that keep attachments in a folder
// list them. Missing is set when there is no folder, and Trashed when the folder is in the trash.
type Attachments struct {
	Missing bool
	Trashed bool
	URL     string
	Files   []Attachment
}

type Attachment struct {
	Name string
	Kind string
	Size string
	URL  string
}

// Attachments lists the files at the top of folder, where 0 is no folder.
func (s *Store) Attachments(ctx context.Context, folder int64) (Attachments, error) {
	if folder == 0 {
		return Attachments{Missing: true}, nil
	}
	_, err := s.Path(ctx, folder)
	if errors.Is(err, ErrNotFound) {
		return Attachments{Trashed: true}, nil
	}
	if err != nil {
		return Attachments{}, err
	}
	files, err := s.Files(ctx, folder)
	if err != nil {
		return Attachments{}, err
	}
	a := Attachments{URL: FolderURL(folder)}
	for _, f := range files {
		a.Files = append(a.Files, Attachment{Name: f.Name, Kind: f.Kind().Label(), Size: FormatSize(f.Size), URL: FileURL(f.ID)})
	}
	return a, nil
}

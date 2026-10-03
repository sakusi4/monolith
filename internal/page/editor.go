package page

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/web"
)

const (
	versionHeader = "Page-Version"
	staleProblem  = "This page changed elsewhere. Reload to keep editing."
)

// Editor is what the editor templates need to edit the body of a page: its title and body, the
// version they were read at, and where the editor saves and uploads. PageID is the parent of the
// subpages made from the editor.
type Editor struct {
	PageID     int64
	Title      string
	Body       string
	Version    string
	ContentURL string
	ImagesURL  string
}

// FormatVersion writes the time a page was last saved as the version the editor sends back.
func FormatVersion(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func ParseVersion(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse version %q: %w", s, err)
	}
	return t, nil
}

// RespondSaved answers an autosave: 422 with problem when it is not empty, 409 for ErrStale, 404
// for ErrNotFound, 500 for any other error, and otherwise 204 with the new version in the
// Page-Version header.
func RespondSaved(w http.ResponseWriter, r *http.Request, saved time.Time, err error, problem string) {
	switch {
	case problem != "":
		http.Error(w, problem, http.StatusUnprocessableEntity)
	case errors.Is(err, ErrStale):
		http.Error(w, staleProblem, http.StatusConflict)
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		w.Header().Set(versionHeader, FormatVersion(saved))
		w.WriteHeader(http.StatusNoContent)
	}
}

// RespondUploaded answers an upload from the editor: 422 with problem when it is not empty, 404 for
// ErrNotFound, 500 for any other error, and otherwise 201 with the first file's address in Location.
func RespondUploaded(w http.ResponseWriter, r *http.Request, files []drive.File, err error, problem string) {
	switch {
	case problem != "":
		http.Error(w, problem, http.StatusUnprocessableEntity)
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		web.ServerError(w, r, err)
	case len(files) == 0:
		http.Error(w, noFilesProblem, http.StatusUnprocessableEntity)
	default:
		w.Header().Set("Location", drive.FileURL(files[0].ID))
		w.WriteHeader(http.StatusCreated)
	}
}

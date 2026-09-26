package drive

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sakusi4/monolith/web"
)

const (
	transferTimeout = 2 * time.Hour
	dateLayout      = "Jan 2, 2006"
	nameRule        = "Use a name of up to 255 characters without a slash."
	folderNameTaken = "A folder with that name is already there."
	fileNameTaken   = "A file with that name is already there."
	movedIntoItself = "A folder cannot move into itself or a folder inside it."
)

type handler struct {
	store     *Store
	loc       *time.Location
	maxUpload int64
}

// NewHandler serves the drive pages. loc decides the dates shown, and maxUpload caps the bytes of
// one upload request.
func NewHandler(store *Store, loc *time.Location, maxUpload int64) http.Handler {
	h := &handler{store: store, loc: loc, maxUpload: maxUpload}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /drive", h.showTop)
	mux.HandleFunc("GET /drive/folders/{id}", h.showFolder)
	mux.HandleFunc("POST /drive/folders/new", h.createFolder)
	mux.HandleFunc("POST /drive/files/new", h.uploadFiles)
	mux.HandleFunc("GET /drive/files/{id}", h.showFile)
	mux.HandleFunc("GET /drive/files/{id}/content", h.serveContent)
	mux.HandleFunc("GET /drive/files/{id}/download", h.downloadFile)
	mux.HandleFunc("GET /drive/folders/{id}/edit", h.editFolder)
	mux.HandleFunc("POST /drive/folders/{id}/edit", h.updateFolder)
	mux.HandleFunc("GET /drive/files/{id}/edit", h.editFile)
	mux.HandleFunc("POST /drive/files/{id}/edit", h.updateFile)
	mux.HandleFunc("POST /drive/folders/{id}/delete", h.trashFolder)
	mux.HandleFunc("POST /drive/files/{id}/delete", h.trashFile)
	mux.HandleFunc("GET /drive/trash", h.showTrash)
	mux.HandleFunc("POST /drive/trash/folders/{id}/restore", h.restoreFolder)
	mux.HandleFunc("POST /drive/trash/files/{id}/restore", h.restoreFile)
	mux.HandleFunc("POST /drive/trash/folders/{id}/delete", h.deleteFolder)
	mux.HandleFunc("POST /drive/trash/files/{id}/delete", h.deleteFile)
	mux.HandleFunc("POST /drive/trash/empty", h.emptyTrash)
	return mux
}

type crumb struct {
	Name string
	URL  string
}

// queryFolder reads the folder of a request from its "folder" query value, where empty means the top
// level. It writes the error response and returns false when the value is invalid or the folder is hidden.
func (h *handler) queryFolder(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := parseFolderID(r.URL.Query().Get("folder"))
	if !ok {
		http.Error(w, "Invalid folder.", http.StatusBadRequest)
		return 0, false
	}
	return id, h.requireFolder(w, r, id)
}

// requireFolder writes the error response and returns false unless id is the top level or a
// folder outside the trash.
func (h *handler) requireFolder(w http.ResponseWriter, r *http.Request, id int64) bool {
	if id == 0 {
		return true
	}
	if _, err := h.store.Path(r.Context(), id); err != nil {
		respondError(w, r, err)
		return false
	}
	return true
}

func (h *handler) date(t time.Time) string {
	return t.In(h.loc).Format(dateLayout)
}

// respondError writes 404 for ErrNotFound and 500 for any other error.
func respondError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	web.ServerError(w, r, err)
}

// parseFolderID reads a folder id where empty means the top level.
func parseFolderID(s string) (int64, bool) {
	if s == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil && id > 0
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func folderURL(id int64) string {
	if id == 0 {
		return "/drive"
	}
	return "/drive/folders/" + strconv.FormatInt(id, 10)
}

func fileURL(id int64) string {
	return "/drive/files/" + strconv.FormatInt(id, 10)
}

func folderQuery(id int64) string {
	if id == 0 {
		return ""
	}
	return "?folder=" + strconv.FormatInt(id, 10)
}

// crumbs links the top level and then folders, top first.
func crumbs(folders []Folder) []crumb {
	out := []crumb{{Name: "Drive", URL: folderURL(0)}}
	for _, f := range folders {
		out = append(out, crumb{Name: f.Name, URL: folderURL(f.ID)})
	}
	return out
}

// editInput reads the name and destination folder of an edit. It writes the error response and
// returns false when the folder is invalid or hidden in the trash.
func (h *handler) editInput(w http.ResponseWriter, r *http.Request) (editForm, bool) {
	folder, ok := parseFolderID(r.PostFormValue("folder"))
	if !ok {
		http.Error(w, "Invalid folder.", http.StatusBadRequest)
		return editForm{}, false
	}
	if !h.requireFolder(w, r, folder) {
		return editForm{}, false
	}
	return editForm{Name: r.PostFormValue("name"), Folder: folder, Submitted: true}, true
}

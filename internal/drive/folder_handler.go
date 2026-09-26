package drive

import (
	"errors"
	"net/http"

	"github.com/sakusi4/monolith/web"
)

type folderPage struct {
	Title        string
	Parents      []crumb
	Folders      []folderRow
	Files        []fileRow
	Edit         editForm
	Targets      []FolderPath
	NewFolder    string
	Error        string
	NewFolderURL string
	UploadURL    string
}

type folderRow struct {
	Folder    Folder
	Modified  string
	Editing   bool
	URL       string
	EditURL   string
	DeleteURL string
}

type fileRow struct {
	File      File
	Size      string
	Modified  string
	Editing   bool
	URL       string
	EditURL   string
	DeleteURL string
}

// editForm holds the name and destination folder of the row being edited, as the user typed them.
type editForm struct {
	Name      string
	Folder    int64
	Submitted bool
	URL       string
	CancelURL string
}

// folderView is what a request adds to a folder page: a row being edited, rejected input, or an error.
type folderView struct {
	EditFolder int64
	EditFile   int64
	Edit       editForm
	NewFolder  string
	Error      string
}

func (h *handler) showTop(w http.ResponseWriter, r *http.Request) {
	h.renderFolder(w, r, http.StatusOK, 0, folderView{})
}

func (h *handler) showFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderFolder(w, r, http.StatusOK, id, folderView{})
}

// renderFolder shows folder id, or the top level when id is 0.
func (h *handler) renderFolder(w http.ResponseWriter, r *http.Request, status int, id int64, view folderView) {
	ctx := r.Context()
	var path []Folder
	if id != 0 {
		p, err := h.store.Path(ctx, id)
		if err != nil {
			respondError(w, r, err)
			return
		}
		path = p
	}
	folders, err := h.store.Folders(ctx, id)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	files, err := h.store.Files(ctx, id)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page, ok := h.newFolderPage(id, path, folders, files, view)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if view.EditFolder != 0 || view.EditFile != 0 {
		page.Targets, err = h.store.FolderPaths(ctx, view.EditFolder)
		if err != nil {
			web.ServerError(w, r, err)
			return
		}
	}
	web.Render(w, r, status, "drive_folder", page)
}

// newFolderPage returns false when view edits a folder or file that is not in the folder shown.
func (h *handler) newFolderPage(id int64, path []Folder, folders []Folder, files []File, view folderView) (folderPage, bool) {
	page := folderPage{
		Title:        "Drive",
		NewFolder:    view.NewFolder,
		Error:        view.Error,
		NewFolderURL: "/drive/folders/new" + folderQuery(id),
		UploadURL:    "/drive/files/new" + folderQuery(id),
	}
	if len(path) > 0 {
		page.Title = path[len(path)-1].Name
		page.Parents = crumbs(path[:len(path)-1])
	}
	edit := view.Edit
	found := view.EditFolder == 0 && view.EditFile == 0
	for _, f := range folders {
		row := folderRow{Folder: f, Modified: h.date(f.UpdatedAt), Editing: f.ID == view.EditFolder, URL: folderURL(f.ID)}
		row.EditURL, row.DeleteURL = row.URL+"/edit", row.URL+"/delete"
		if row.Editing {
			found = true
			if !edit.Submitted {
				edit = editForm{Name: f.Name, Folder: f.ParentID}
			}
			edit.URL, edit.CancelURL = row.EditURL, folderURL(id)
		}
		page.Folders = append(page.Folders, row)
	}
	for _, f := range files {
		row := fileRow{File: f, Size: formatSize(f.Size), Modified: h.date(f.UpdatedAt), Editing: f.ID == view.EditFile, URL: fileURL(f.ID)}
		row.EditURL, row.DeleteURL = row.URL+"/edit", row.URL+"/delete"
		if row.Editing {
			found = true
			if !edit.Submitted {
				edit = editForm{Name: f.Name, Folder: f.FolderID}
			}
			edit.URL, edit.CancelURL = row.EditURL, folderURL(id)
		}
		page.Files = append(page.Files, row)
	}
	page.Edit = edit
	return page, found
}

func (h *handler) createFolder(w http.ResponseWriter, r *http.Request) {
	parent, ok := h.queryFolder(w, r)
	if !ok {
		return
	}
	name := r.PostFormValue("name")
	err := h.store.CreateFolder(r.Context(), parent, name)
	var problem string
	switch {
	case errors.Is(err, ErrInvalidName):
		problem = nameRule
	case errors.Is(err, ErrNameTaken):
		problem = folderNameTaken
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, folderURL(parent), http.StatusSeeOther)
		return
	}
	h.renderFolder(w, r, http.StatusUnprocessableEntity, parent, folderView{NewFolder: name, Error: problem})
}

func (h *handler) editFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	path, err := h.store.Path(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	h.renderFolder(w, r, http.StatusOK, path[len(path)-1].ParentID, folderView{EditFolder: id})
}

func (h *handler) updateFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	path, err := h.store.Path(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	form, ok := h.editInput(w, r)
	if !ok {
		return
	}
	err = h.store.UpdateFolder(r.Context(), id, form.Folder, form.Name)
	var problem string
	switch {
	case errors.Is(err, ErrInvalidName):
		problem = nameRule
	case errors.Is(err, ErrNameTaken):
		problem = folderNameTaken
	case errors.Is(err, ErrMoveIntoItself):
		problem = movedIntoItself
	case err != nil:
		respondError(w, r, err)
		return
	default:
		http.Redirect(w, r, folderURL(form.Folder), http.StatusSeeOther)
		return
	}
	h.renderFolder(w, r, http.StatusUnprocessableEntity, path[len(path)-1].ParentID, folderView{EditFolder: id, Edit: form, Error: problem})
}

func (h *handler) trashFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	path, err := h.store.Path(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	if err := h.store.TrashFolder(r.Context(), id); err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, folderURL(path[len(path)-1].ParentID), http.StatusSeeOther)
}

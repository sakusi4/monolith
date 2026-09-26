package drive

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/sakusi4/monolith/web"
)

const trashURL = "/drive/trash"

type trashPage struct {
	Rows  []trashRow
	Error string
}

type trashRow struct {
	Item       TrashItem
	Location   string
	Deleted    string
	RestoreURL string
	DeleteURL  string
}

func (h *handler) showTrash(w http.ResponseWriter, r *http.Request) {
	h.renderTrash(w, r, http.StatusOK, "")
}

func (h *handler) renderTrash(w http.ResponseWriter, r *http.Request, status int, problem string) {
	items, err := h.store.Trash(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page := trashPage{Error: problem}
	for _, it := range items {
		kind := "files"
		if it.Folder {
			kind = "folders"
		}
		base := trashURL + "/" + kind + "/" + strconv.FormatInt(it.ID, 10)
		location := "Drive"
		if it.Location != "" {
			location += " / " + it.Location
		}
		page.Rows = append(page.Rows, trashRow{Item: it, Location: location, Deleted: h.date(it.TrashedAt), RestoreURL: base + "/restore", DeleteURL: base + "/delete"})
	}
	web.Render(w, r, status, "drive_trash", page)
}

func (h *handler) restoreFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.respondRestore(w, r, h.store.RestoreFolder(r.Context(), id))
}

func (h *handler) restoreFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.respondRestore(w, r, h.store.RestoreFile(r.Context(), id))
}

func (h *handler) respondRestore(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrParentTrashed):
		h.renderTrash(w, r, http.StatusUnprocessableEntity, "Restore the folder it was in first.")
	case errors.Is(err, ErrNameTaken):
		h.renderTrash(w, r, http.StatusUnprocessableEntity, "Its folder already has an item with that name.")
	case err != nil:
		respondError(w, r, err)
	default:
		http.Redirect(w, r, trashURL, http.StatusSeeOther)
	}
}

func (h *handler) deleteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	respondPurge(w, r, h.store.DeleteFolder(r.Context(), id))
}

func (h *handler) deleteFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	respondPurge(w, r, h.store.DeleteFile(r.Context(), id))
}

func (h *handler) emptyTrash(w http.ResponseWriter, r *http.Request) {
	respondPurge(w, r, h.store.EmptyTrash(r.Context()))
}

func respondPurge(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, trashURL, http.StatusSeeOther)
}

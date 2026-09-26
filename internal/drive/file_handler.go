package drive

import (
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/sakusi4/monolith/web"
)

var errBadUpload = errors.New("bad upload")

type filePage struct {
	File        File
	Parents     []crumb
	Size        string
	Modified    string
	ContentURL  string
	DownloadURL string
	EditURL     string
	DeleteURL   string
	Text        string
	TooLarge    bool
}

func (h *handler) uploadFiles(w http.ResponseWriter, r *http.Request) {
	folder, ok := h.queryFolder(w, r)
	if !ok {
		return
	}
	rc := http.NewResponseController(w)
	deadline := time.Now().Add(transferTimeout)
	if err := errors.Join(rc.SetReadDeadline(deadline), rc.SetWriteDeadline(deadline)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		web.ServerError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	uploads, err := h.stageUploads(r)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		http.Error(w, "The upload is too large.", http.StatusRequestEntityTooLarge)
		return
	case errors.Is(err, errBadUpload):
		http.Error(w, "The upload could not be read.", http.StatusBadRequest)
		return
	case err != nil:
		web.ServerError(w, r, err)
		return
	case len(uploads) == 0:
		h.renderFolder(w, r, http.StatusUnprocessableEntity, folder, folderView{Error: "Choose files to upload."})
		return
	}
	err = h.store.AddFiles(r.Context(), folder, uploads)
	var problem string
	switch {
	case errors.Is(err, ErrInvalidName):
		problem = nameRule
	case errors.Is(err, ErrNameTaken):
		problem = fileNameTaken
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, folderURL(folder), http.StatusSeeOther)
		return
	}
	h.renderFolder(w, r, http.StatusUnprocessableEntity, folder, folderView{Error: problem})
}

// stageUploads stages every file of the "files" field in the multipart body of r. On error it
// discards what it staged.
func (h *handler) stageUploads(r *http.Request) ([]Upload, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBadUpload, err)
	}
	var uploads []Upload
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return uploads, nil
		}
		if err != nil {
			return nil, errors.Join(fmt.Errorf("%w: %w", errBadUpload, err), h.store.Discard(uploads))
		}
		if part.FormName() != "files" || part.FileName() == "" {
			continue
		}
		u, err := h.store.Stage(part.FileName(), part)
		if err != nil {
			return nil, errors.Join(err, h.store.Discard(uploads))
		}
		uploads = append(uploads, u)
	}
}

func (h *handler) showFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	f, err := h.store.File(ctx, id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	var path []Folder
	if f.FolderID != 0 {
		path, err = h.store.Path(ctx, f.FolderID)
		if err != nil {
			respondError(w, r, err)
			return
		}
	}
	url := fileURL(f.ID)
	page := filePage{
		File:        f,
		Parents:     crumbs(path),
		Size:        formatSize(f.Size),
		Modified:    h.date(f.UpdatedAt),
		ContentURL:  url + "/content",
		DownloadURL: url + "/download",
		EditURL:     url + "/edit",
		DeleteURL:   url + "/delete",
		TooLarge:    f.Kind() == KindText && f.Size > maxTextPreview,
	}
	if f.Kind() == KindText && !page.TooLarge {
		page.Text, err = h.readText(f)
		if err != nil {
			web.ServerError(w, r, err)
			return
		}
	}
	web.Render(w, r, http.StatusOK, "drive_file", page)
}

func (h *handler) readText(f File) (string, error) {
	content, err := h.store.Open(f)
	if err != nil {
		return "", err
	}
	defer content.Close()
	b, err := io.ReadAll(content)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", f.Name, err)
	}
	return string(b), nil
}

func (h *handler) serveContent(w http.ResponseWriter, r *http.Request) {
	h.sendFile(w, r, false)
}

func (h *handler) downloadFile(w http.ResponseWriter, r *http.Request) {
	h.sendFile(w, r, true)
}

// sendFile writes the content of the file in the path, as an attachment when attachment is set or
// when its type could run script.
func (h *handler) sendFile(w http.ResponseWriter, r *http.Request, attachment bool) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := h.store.File(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	content, err := h.store.Open(f)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	defer content.Close()
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(transferTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		web.ServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", `"`+hex.EncodeToString(f.SHA256)+`"`)
	if attachment || mustDownload(f.ContentType) {
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": f.Name})
		w.Header().Set("Content-Disposition", cmp.Or(disposition, "attachment"))
	}
	http.ServeContent(w, r, f.Name, f.UpdatedAt, content)
}

func (h *handler) editFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := h.store.File(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	h.renderFolder(w, r, http.StatusOK, f.FolderID, folderView{EditFile: id})
}

func (h *handler) updateFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := h.store.File(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	form, ok := h.editInput(w, r)
	if !ok {
		return
	}
	err = h.store.UpdateFile(r.Context(), id, form.Folder, form.Name)
	var problem string
	switch {
	case errors.Is(err, ErrInvalidName):
		problem = nameRule
	case errors.Is(err, ErrNameTaken):
		problem = fileNameTaken
	case err != nil:
		respondError(w, r, err)
		return
	default:
		http.Redirect(w, r, folderURL(form.Folder), http.StatusSeeOther)
		return
	}
	h.renderFolder(w, r, http.StatusUnprocessableEntity, f.FolderID, folderView{EditFile: id, Edit: form, Error: problem})
}

func (h *handler) trashFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := h.store.File(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	if err := h.store.TrashFile(r.Context(), id); err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, folderURL(f.FolderID), http.StatusSeeOther)
}

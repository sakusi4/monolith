package page

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/web"
)

const (
	trashURL             = "/page/trash"
	dateLayout           = "Jan 2, 2006"
	maxLinks             = 10
	titleProblem         = "Enter a title."
	parentProblem        = "Pick a parent that is not this page or one of its subpages."
	parentTrashedProblem = "The parent page is in the trash."
	fileNameProblem      = "A file with that name is already attached."
	fileNameRule         = "Use file names of up to 255 characters without a slash."
	noFilesProblem       = "Choose files to attach."
)

type handler struct {
	store     *Store
	loc       *time.Location
	maxUpload int64
}

// NewHandler serves the page screens. loc decides the dates shown, and maxUpload caps the bytes of
// one upload request.
func NewHandler(store *Store, loc *time.Location, maxUpload int64) http.Handler {
	h := &handler{store: store, loc: loc, maxUpload: maxUpload}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /page", h.listPages)
	mux.HandleFunc("GET /page/links", h.listLinks)
	mux.HandleFunc("POST /page/pages/new", h.createPage)
	mux.HandleFunc("GET /page/pages/{id}", h.showPage)
	mux.HandleFunc("POST /page/pages/{id}/content", h.saveContent)
	mux.HandleFunc("POST /page/pages/{id}/images", h.uploadImages)
	mux.HandleFunc("POST /page/pages/{id}/files", h.attachFiles)
	mux.HandleFunc("POST /page/pages/{id}/move", h.movePage)
	mux.HandleFunc("POST /page/pages/{id}/delete", h.trashPage)
	mux.HandleFunc("GET /page/trash", h.showTrash)
	mux.HandleFunc("POST /page/trash/{id}/restore", h.restorePage)
	mux.HandleFunc("POST /page/trash/{id}/delete", h.deletePage)
	mux.HandleFunc("POST /page/trash/empty", h.emptyTrash)
	return mux
}

type listView struct {
	Filters   web.FilterBar
	Searching bool
	Pages     []Entry
	NewTitle  string
	Error     string
}

type detailView struct {
	Page        Page
	Crumbs      []Crumb
	Editor      Editor
	Links       Links
	Parents     []Entry
	Attachments drive.Attachments
	Error       string
	MoveURL     string
	FilesURL    string
	DeleteURL   string
}

type trashView struct {
	Rows  []trashRow
	Error string
}

type trashRow struct {
	Page       TrashedPage
	Deleted    string
	RestoreURL string
	DeleteURL  string
}

func (h *handler) listPages(w http.ResponseWriter, r *http.Request) {
	h.renderList(w, r, http.StatusOK, "", "")
}

// renderList shows the pages at the top, or the search results, of the kind the query asks for, with
// the new page form holding newTitle and problem.
func (h *handler) renderList(w http.ResponseWriter, r *http.Request, status int, newTitle, problem string) {
	q, err := parsePageQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid filter.", http.StatusBadRequest)
		return
	}
	var entries []Entry
	if q.Q != "" {
		entries, err = h.store.Search(r.Context(), q.Q)
	} else {
		entries, err = h.store.TopPages(r.Context())
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	view := listView{Filters: q.filters(), Searching: q.Q != "", Pages: q.pick(entries), NewTitle: newTitle, Error: problem}
	web.Render(w, r, status, "page_list", view)
}

// listLinks shows the pages that the editor's picker offers for the query value q: the matches, or
// the latest changed pages when q is empty.
func (h *handler) listLinks(w http.ResponseWriter, r *http.Request) {
	entries, err := h.store.Search(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.Render(w, r, http.StatusOK, "page_links", entries[:min(len(entries), maxLinks)])
}

func (h *handler) createPage(w http.ResponseWriter, r *http.Request) {
	title := r.PostFormValue("title")
	parent, ok := parseID(r.PostFormValue("parent"))
	if !ok {
		http.Error(w, "Invalid parent.", http.StatusBadRequest)
		return
	}
	id, err := h.store.Create(r.Context(), parent, title)
	switch {
	case errors.Is(err, ErrInvalidTitle):
		h.renderList(w, r, http.StatusUnprocessableEntity, title, titleProblem)
	case errors.Is(err, ErrInvalidParent):
		h.renderList(w, r, http.StatusUnprocessableEntity, title, parentTrashedProblem)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, PageURL(id), http.StatusSeeOther)
	}
}

func (h *handler) showPage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadPage(w, r)
	if !ok {
		return
	}
	if p.Owner != (Owner{}) {
		http.Redirect(w, r, entryURL(p.ID, p.Owner), http.StatusSeeOther)
		return
	}
	h.renderPage(w, r, http.StatusOK, p, "")
}

// renderPage shows p, a page of its own, with problem from a move or an attachment that failed.
func (h *handler) renderPage(w http.ResponseWriter, r *http.Request, status int, p Page, problem string) {
	ctx := r.Context()
	crumbs, err := h.store.Crumbs(ctx, p)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	links, err := h.store.Links(ctx, p.ID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	parents, err := h.store.ParentChoices(ctx, p.ID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	sec, err := h.store.drive.Attachments(ctx, p.FolderID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	u := PageURL(p.ID)
	view := detailView{
		Page:        p,
		Crumbs:      crumbs,
		Editor:      Editor{PageID: p.ID, Title: p.Title, Body: p.Body, Version: FormatVersion(p.UpdatedAt), ContentURL: u + "/content", ImagesURL: u + "/images"},
		Links:       links,
		Parents:     parents,
		Attachments: sec,
		Error:       problem,
		MoveURL:     u + "/move",
		FilesURL:    u + "/files",
		DeleteURL:   u + "/delete",
	}
	web.Render(w, r, status, "page_detail", view)
}

func (h *handler) saveContent(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadOwnPage(w, r)
	if !ok {
		return
	}
	version, err := ParseVersion(r.PostFormValue("version"))
	if err != nil {
		http.Error(w, "Invalid version.", http.StatusBadRequest)
		return
	}
	saved, err := h.store.SaveContent(r.Context(), p.ID, r.PostFormValue("title"), r.PostFormValue("body"), version)
	problem := ""
	if errors.Is(err, ErrInvalidTitle) {
		problem = titleProblem
	}
	RespondSaved(w, r, saved, err, problem)
}

func (h *handler) uploadImages(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadOwnPage(w, r)
	if !ok {
		return
	}
	_, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	files, err := h.store.Attach(r.Context(), p.ID, uploads)
	RespondUploaded(w, r, files, err, uploadProblem(err))
}

func (h *handler) attachFiles(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadOwnPage(w, r)
	if !ok {
		return
	}
	_, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	if len(uploads) == 0 {
		h.renderPage(w, r, http.StatusUnprocessableEntity, p, noFilesProblem)
		return
	}
	_, err := h.store.Attach(r.Context(), p.ID, uploads)
	if problem := uploadProblem(err); problem != "" {
		h.renderPage(w, r, http.StatusUnprocessableEntity, p, problem)
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, PageURL(p.ID), http.StatusSeeOther)
}

func (h *handler) movePage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadOwnPage(w, r)
	if !ok {
		return
	}
	parent, ok := parseID(r.PostFormValue("parent"))
	if !ok {
		http.Error(w, "Invalid parent.", http.StatusBadRequest)
		return
	}
	err := h.store.Move(r.Context(), p.ID, parent)
	switch {
	case errors.Is(err, ErrInvalidParent):
		h.renderPage(w, r, http.StatusUnprocessableEntity, p, parentProblem)
	case err != nil:
		respondError(w, r, err)
	default:
		http.Redirect(w, r, PageURL(p.ID), http.StatusSeeOther)
	}
}

func (h *handler) trashPage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadOwnPage(w, r)
	if !ok {
		return
	}
	crumbs, err := h.store.Crumbs(r.Context(), p)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if err := h.store.Trash(r.Context(), []int64{p.ID}); err != nil {
		web.ServerError(w, r, err)
		return
	}
	http.Redirect(w, r, crumbs[len(crumbs)-1].URL, http.StatusSeeOther)
}

func (h *handler) showTrash(w http.ResponseWriter, r *http.Request) {
	h.renderTrash(w, r, http.StatusOK, "")
}

func (h *handler) renderTrash(w http.ResponseWriter, r *http.Request, status int, problem string) {
	pages, err := h.store.TrashedPages(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	view := trashView{Error: problem}
	for _, p := range pages {
		base := trashURL + "/" + strconv.FormatInt(p.ID, 10)
		view.Rows = append(view.Rows, trashRow{Page: p, Deleted: p.TrashedAt.In(h.loc).Format(dateLayout), RestoreURL: base + "/restore", DeleteURL: base + "/delete"})
	}
	web.Render(w, r, status, "page_trash", view)
}

func (h *handler) restorePage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.store.Restore(r.Context(), id)
	switch {
	case errors.Is(err, ErrParentTrashed):
		h.renderTrash(w, r, http.StatusUnprocessableEntity, parentTrashedProblem)
	case err != nil:
		respondError(w, r, err)
	default:
		http.Redirect(w, r, trashURL, http.StatusSeeOther)
	}
}

func (h *handler) deletePage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	respondPurge(w, r, h.store.Delete(r.Context(), id))
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

// loadPage reads the page that the request path names. It writes the error response and returns
// false when there is no such page outside the trash.
func (h *handler) loadPage(w http.ResponseWriter, r *http.Request) (Page, bool) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return Page{}, false
	}
	p, err := h.store.Page(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return Page{}, false
	}
	return p, true
}

// loadOwnPage is loadPage for a page of its own. The page of a project or task changes on their screens.
func (h *handler) loadOwnPage(w http.ResponseWriter, r *http.Request) (Page, bool) {
	p, ok := h.loadPage(w, r)
	if !ok {
		return Page{}, false
	}
	if p.Owner != (Owner{}) {
		http.NotFound(w, r)
		return Page{}, false
	}
	return p, true
}

// receive reads a multipart form with its files. It writes the error response and returns false when
// the body is too large or malformed.
func (h *handler) receive(w http.ResponseWriter, r *http.Request) (url.Values, []drive.Upload, bool) {
	fields, uploads, err := drive.ReceiveUploads(w, r, h.store.drive, h.maxUpload)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		http.Error(w, "The upload is too large.", http.StatusRequestEntityTooLarge)
	case errors.Is(err, drive.ErrBadUpload):
		http.Error(w, "The upload could not be read.", http.StatusBadRequest)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		return fields, uploads, true
	}
	return nil, nil, false
}

// uploadProblem is the message for a file that err rejects, or empty when err is not about the file.
func uploadProblem(err error) string {
	switch {
	case errors.Is(err, drive.ErrNameTaken):
		return fileNameProblem
	case errors.Is(err, drive.ErrInvalidName):
		return fileNameRule
	}
	return ""
}

// respondError writes 404 for ErrNotFound and 500 for any other error.
func respondError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	web.ServerError(w, r, err)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

// parseID reads an id where empty means none.
func parseID(s string) (int64, bool) {
	if s == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil && id > 0
}

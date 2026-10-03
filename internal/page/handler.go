package page

import (
	"cmp"
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
	maxResults           = 100
	untitled             = "Untitled"
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
	mux.HandleFunc("GET /page/search", h.searchPages)
	mux.HandleFunc("GET /page/links", h.listLinks)
	mux.HandleFunc("POST /page/pages/new", h.createPage)
	mux.HandleFunc("GET /page/pages/{id}", h.showPage)
	mux.HandleFunc("POST /page/pages/{id}/content", h.saveContent)
	mux.HandleFunc("POST /page/pages/{id}/images", h.uploadImages)
	mux.HandleFunc("POST /page/pages/{id}/move", h.movePage)
	mux.HandleFunc("POST /page/pages/{id}/delete", h.trashPage)
	mux.HandleFunc("GET /page/trash", h.showTrash)
	mux.HandleFunc("POST /page/trash/{id}/restore", h.restorePage)
	mux.HandleFunc("POST /page/trash/{id}/delete", h.deletePage)
	mux.HandleFunc("POST /page/trash/empty", h.emptyTrash)
	return mux
}

type searchView struct {
	Query string
	Pages []Entry
}

type detailView struct {
	Page      Page
	Crumbs    []Crumb
	Editor    Editor
	Backlinks []Entry
	Parents   []Entry
	Error     string
	MoveURL   string
	DeleteURL string
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

// listPages shows the pages of their own at the top level, which the sidebar lists on wide screens.
func (h *handler) listPages(w http.ResponseWriter, r *http.Request) {
	entries, err := h.store.TopPages(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.Render(w, r, http.StatusOK, "page_list", entries)
}

// searchPages shows the search form and, when the query value q is set, the pages that match it.
func (h *handler) searchPages(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		web.Render(w, r, http.StatusOK, "page_search", searchView{})
		return
	}
	entries, err := h.store.Search(r.Context(), q)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.Render(w, r, http.StatusOK, "page_search", searchView{Query: q, Pages: entries[:min(len(entries), maxResults)]})
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

// createPage adds a page titled by the form value title, or Untitled without one, and opens it.
func (h *handler) createPage(w http.ResponseWriter, r *http.Request) {
	title := cmp.Or(r.PostFormValue("title"), untitled)
	parent, ok := parseID(r.PostFormValue("parent"))
	if !ok {
		http.Error(w, "Invalid parent.", http.StatusBadRequest)
		return
	}
	id, err := h.store.Create(r.Context(), parent, title)
	switch {
	case errors.Is(err, ErrInvalidTitle):
		http.Error(w, titleProblem, http.StatusUnprocessableEntity)
	case errors.Is(err, ErrInvalidParent):
		http.Error(w, parentTrashedProblem, http.StatusUnprocessableEntity)
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

// renderPage shows p, a page of its own, with problem from a move that failed.
func (h *handler) renderPage(w http.ResponseWriter, r *http.Request, status int, p Page, problem string) {
	ctx := r.Context()
	u := PageURL(p.ID)
	view := detailView{
		Page:      p,
		Editor:    Editor{PageID: p.ID, Title: p.Title, Body: p.Body, Version: FormatVersion(p.UpdatedAt), ContentURL: u + "/content", ImagesURL: u + "/images"},
		Error:     problem,
		MoveURL:   u + "/move",
		DeleteURL: u + "/delete",
	}
	var err error
	if view.Backlinks, err = h.store.Backlinks(ctx, p.ID); err != nil {
		web.ServerError(w, r, err)
		return
	}
	if view.Crumbs, err = h.store.Crumbs(ctx, p); err != nil {
		web.ServerError(w, r, err)
		return
	}
	if view.Parents, err = h.store.ParentChoices(ctx, p.ID); err != nil {
		web.ServerError(w, r, err)
		return
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

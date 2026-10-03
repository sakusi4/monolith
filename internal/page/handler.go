package page

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/web"
)

const (
	trashURL             = "/page/trash"
	dateLayout           = "Jan 2, 2006"
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
	mux.HandleFunc("POST /page/pages/new", h.createPage)
	mux.HandleFunc("GET /page/pages/{id}", h.showPage)
	mux.HandleFunc("GET /page/pages/{id}/edit", h.editPage)
	mux.HandleFunc("POST /page/pages/{id}/edit", h.updatePage)
	mux.HandleFunc("POST /page/pages/{id}/files", h.attachFiles)
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
	Links       Links
	Attachments drive.Attachments
	EditURL     string
	DeleteURL   string
}

type editView struct {
	Page        Page
	Crumbs      []Crumb
	Form        pageForm
	Parents     []Entry
	Attachments drive.Attachments
	Error       string
	URL         string
	FilesURL    string
	CancelURL   string
}

// pageForm holds the edit form's values as the user typed them.
type pageForm struct {
	Title     string
	ParentID  int64
	Body      string
	Submitted bool
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

func (h *handler) createPage(w http.ResponseWriter, r *http.Request) {
	title := r.PostFormValue("title")
	id, err := h.store.Create(r.Context(), title)
	switch {
	case errors.Is(err, ErrInvalidTitle):
		h.renderList(w, r, http.StatusUnprocessableEntity, title, titleProblem)
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
	sec, err := h.store.drive.Attachments(ctx, p.FolderID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	u := PageURL(p.ID)
	view := detailView{Page: p, Crumbs: crumbs, Links: links, Attachments: sec, EditURL: u + "/edit", DeleteURL: u + "/delete"}
	web.Render(w, r, http.StatusOK, "page_detail", view)
}

func (h *handler) editPage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadOwnPage(w, r)
	if !ok {
		return
	}
	h.renderEdit(w, r, http.StatusOK, p, pageForm{}, "")
}

// renderEdit shows the edit screen of p with form, or with the page's values when form is not submitted.
func (h *handler) renderEdit(w http.ResponseWriter, r *http.Request, status int, p Page, form pageForm, problem string) {
	ctx := r.Context()
	crumbs, err := h.store.Crumbs(ctx, p)
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
	if !form.Submitted {
		form = pageForm{Title: p.Title, ParentID: p.ParentID, Body: p.Body}
	}
	u := PageURL(p.ID)
	view := editView{
		Page:        p,
		Crumbs:      crumbs,
		Form:        form,
		Parents:     parents,
		Attachments: sec,
		Error:       problem,
		URL:         u + "/edit",
		FilesURL:    u + "/files",
		CancelURL:   u,
	}
	web.Render(w, r, status, "page_edit", view)
}

func (h *handler) updatePage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadOwnPage(w, r)
	if !ok {
		return
	}
	fields, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	form := pageForm{Title: fields.Get("title"), Body: fields.Get("body"), Submitted: true}
	parent, ok := parseID(fields.Get("parent"))
	if !ok {
		if err := h.store.drive.Discard(uploads); err != nil {
			web.ServerError(w, r, err)
			return
		}
		http.Error(w, "Invalid parent.", http.StatusBadRequest)
		return
	}
	form.ParentID = parent
	ctx := r.Context()
	err := h.store.Save(ctx, p.ID, Input{Title: form.Title, ParentID: parent, Body: form.Body}, uploads, func(used []drive.Upload) ([]drive.File, error) {
		return h.store.Attach(ctx, p.ID, used)
	})
	if problem := saveProblem(err); problem != "" {
		h.renderEdit(w, r, http.StatusUnprocessableEntity, p, form, problem)
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, PageURL(p.ID), http.StatusSeeOther)
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
		http.Error(w, noFilesProblem, http.StatusUnprocessableEntity)
		return
	}
	_, err := h.store.Attach(r.Context(), p.ID, uploads)
	if problem := saveProblem(err); problem != "" {
		h.renderEdit(w, r, http.StatusUnprocessableEntity, p, pageForm{}, problem)
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, PageURL(p.ID)+"/edit", http.StatusSeeOther)
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

// saveProblem is the message for an input that err rejects, or empty when err is not about the input.
func saveProblem(err error) string {
	switch {
	case errors.Is(err, ErrInvalidTitle):
		return titleProblem
	case errors.Is(err, ErrInvalidParent):
		return parentProblem
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

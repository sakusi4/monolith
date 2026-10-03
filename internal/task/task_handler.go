package task

import (
	"errors"
	"net/http"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/internal/page"
	"github.com/sakusi4/monolith/web"
)

type taskListPage struct {
	Filters web.FilterBar
	Table   taskTable
	Error   string
}

type taskPage struct {
	Task        Task
	Editor      page.Editor
	Projects    []Project
	Statuses    []TaskStatus
	ProjectURL  string
	Due         string
	Completed   string
	Links       page.Links
	Attachments drive.Attachments
	Error       string
	URL         string
	FieldsURL   string
	FilesURL    string
	DeleteURL   string
}

func (h *handler) listTasks(w http.ResponseWriter, r *http.Request) {
	q, err := parseTaskQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid filter.", http.StatusBadRequest)
		return
	}
	h.renderTasks(w, r, http.StatusOK, q, addForm{}, "")
}

func (h *handler) renderTasks(w http.ResponseWriter, r *http.Request, status int, q taskQuery, add addForm, problem string) {
	ctx := r.Context()
	projects, err := h.store.Projects(ctx, projectStatuses)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	tasks, err := h.store.Tasks(ctx, q.filter())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if !add.Submitted {
		add.ProjectID = q.ProjectID
	}
	table := h.newTaskTable(q.sort(tasks), projects, q.listURL(), add)
	table.ShowProject = true
	view := taskListPage{Filters: web.FilterBar{Action: tasksURL, Filters: q.filters(projects)}, Table: table, Error: problem}
	web.Render(w, r, status, "task_list", view)
}

func (h *handler) createTask(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.PostFormValue("next"))
	add := addForm{Title: r.PostFormValue("title"), Due: r.PostFormValue("due"), Submitted: true}
	project, ok := parseID(r.PostFormValue("project"))
	if !ok {
		http.Error(w, "Invalid project.", http.StatusBadRequest)
		return
	}
	add.ProjectID = project
	due, ok := parseDate(add.Due)
	if !ok {
		add.Error = dueProblem
		h.renderNext(w, r, http.StatusUnprocessableEntity, next, add, "")
		return
	}
	err := h.store.AddTask(r.Context(), TaskInput{Title: add.Title, ProjectID: project, Status: StatusTodo, Due: due})
	switch {
	case errors.Is(err, ErrInvalidTask):
		add.Error = taskProblem
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	h.renderNext(w, r, http.StatusUnprocessableEntity, next, add, "")
}

func (h *handler) showTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderTask(w, r, http.StatusOK, id, "")
}

// renderTask shows task id with problem from a change of its fields or an attachment that failed.
func (h *handler) renderTask(w http.ResponseWriter, r *http.Request, status int, id int64, problem string) {
	ctx := r.Context()
	t, err := h.store.Task(ctx, id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	projects, err := h.store.Projects(ctx, projectStatuses)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	sec, err := h.store.drive.Attachments(ctx, t.FolderID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	links, err := h.store.pages.Links(ctx, t.PageID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	u := taskURL(id)
	view := taskPage{
		Task:        t,
		Editor:      page.Editor{PageID: t.PageID, Title: t.Title, Body: t.Body, Version: page.FormatVersion(t.PageUpdatedAt), ContentURL: u + "/content", ImagesURL: u + "/images"},
		Projects:    projects,
		Statuses:    taskStatuses,
		Due:         dateInput(t.Due),
		Links:       links,
		Attachments: sec,
		Error:       problem,
		URL:         u,
		FieldsURL:   u + "/fields",
		FilesURL:    u + "/files",
		DeleteURL:   u + "/delete",
	}
	if t.ProjectID != 0 {
		view.ProjectURL = projectURL(t.ProjectID)
	}
	if !t.CompletedAt.IsZero() {
		view.Completed = t.CompletedAt.In(h.loc).Format(dateLayout)
	}
	web.Render(w, r, status, "task_detail", view)
}

func (h *handler) saveTaskContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	version, err := page.ParseVersion(r.PostFormValue("version"))
	if err != nil {
		http.Error(w, "Invalid version.", http.StatusBadRequest)
		return
	}
	saved, err := h.store.SaveTaskContent(r.Context(), id, r.PostFormValue("title"), r.PostFormValue("body"), version)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	problem := ""
	if errors.Is(err, ErrInvalidTask) {
		problem = titleProblem
	}
	page.RespondSaved(w, r, saved, err, problem)
}

func (h *handler) uploadTaskImages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	files, err := h.store.AttachToTask(r.Context(), id, uploads)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	page.RespondUploaded(w, r, files, err, attachProblem(err))
}

// updateTaskFields saves the project, status, and due date that a row of a task table sends,
// keeping the title and body.
func (h *handler) updateTaskFields(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	next := safeNext(r.PostFormValue("next"))
	project, okProject := parseID(r.PostFormValue("project"))
	due, okDue := parseDate(r.PostFormValue("due"))
	if !okProject || !okDue {
		http.Error(w, "Invalid task.", http.StatusBadRequest)
		return
	}
	err := h.store.UpdateTaskFields(r.Context(), id, project, TaskStatus(r.PostFormValue("status")), due)
	switch {
	case errors.Is(err, ErrInvalidTask):
		h.renderNext(w, r, http.StatusUnprocessableEntity, next, addForm{}, taskProblem)
	case attachProblem(err) != "":
		h.renderNext(w, r, http.StatusUnprocessableEntity, next, addForm{}, attachProblem(err))
	case err != nil:
		respondError(w, r, err)
	default:
		http.Redirect(w, r, next, http.StatusSeeOther)
	}
}

func (h *handler) attachTaskFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	if len(uploads) == 0 {
		h.renderTask(w, r, http.StatusUnprocessableEntity, id, noFilesProblem)
		return
	}
	_, err := h.store.AttachToTask(r.Context(), id, uploads)
	if problem := attachProblem(err); problem != "" {
		h.renderTask(w, r, http.StatusUnprocessableEntity, id, problem)
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, taskURL(id), http.StatusSeeOther)
}

func (h *handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := h.store.DeleteTask(r.Context(), id); err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, safeNext(r.PostFormValue("next")), http.StatusSeeOther)
}

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
	ProjectURL  string
	Due         string
	Completed   string
	Attachments drive.Attachments
	Links       page.Links
	EditURL     string
	DeleteURL   string
}

type taskEditPage struct {
	Task        Task
	Form        taskForm
	Projects    []Project
	Statuses    []TaskStatus
	Attachments drive.Attachments
	Error       string
	URL         string
	FilesURL    string
	CancelURL   string
}

// taskForm holds the edit form's values as the user typed them.
type taskForm struct {
	Title     string
	ProjectID int64
	Status    TaskStatus
	Due       string
	Body      string
	Submitted bool
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
	t, err := h.store.Task(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	sec, err := h.store.drive.Attachments(r.Context(), t.FolderID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	links, err := h.store.pages.Links(r.Context(), t.PageID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	view := taskPage{Task: t, Attachments: sec, Links: links, EditURL: taskURL(id) + "/edit", DeleteURL: taskURL(id) + "/delete"}
	if t.ProjectID != 0 {
		view.ProjectURL = projectURL(t.ProjectID)
	}
	if !t.Due.IsZero() {
		view.Due = t.Due.Format(dateLayout)
	}
	if !t.CompletedAt.IsZero() {
		view.Completed = t.CompletedAt.In(h.loc).Format(dateLayout)
	}
	web.Render(w, r, http.StatusOK, "task_detail", view)
}

func (h *handler) editTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderTaskEdit(w, r, http.StatusOK, id, taskForm{}, "")
}

// renderTaskEdit shows the edit page of task id with form, or with the task's values when form is
// not submitted.
func (h *handler) renderTaskEdit(w http.ResponseWriter, r *http.Request, status int, id int64, form taskForm, problem string) {
	t, err := h.store.Task(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	sec, err := h.store.drive.Attachments(r.Context(), t.FolderID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	projects, err := h.store.Projects(r.Context(), projectStatuses)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if !form.Submitted {
		form = taskForm{Title: t.Title, ProjectID: t.ProjectID, Status: t.Status, Due: dateInput(t.Due), Body: t.Body}
	}
	u := taskURL(id)
	view := taskEditPage{
		Task:        t,
		Form:        form,
		Projects:    projects,
		Statuses:    taskStatuses,
		Attachments: sec,
		Error:       problem,
		URL:         u + "/edit",
		FilesURL:    u + "/files",
		CancelURL:   u,
	}
	web.Render(w, r, status, "task_edit", view)
}

func (h *handler) updateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	fields, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	form := taskForm{
		Title:     fields.Get("title"),
		Status:    TaskStatus(fields.Get("status")),
		Due:       fields.Get("due"),
		Body:      fields.Get("body"),
		Submitted: true,
	}
	project, okProject := parseID(fields.Get("project"))
	due, okDue := parseDate(form.Due)
	if !okProject || !okDue {
		if err := h.store.drive.Discard(uploads); err != nil {
			web.ServerError(w, r, err)
			return
		}
		if !okProject {
			http.Error(w, "Invalid project.", http.StatusBadRequest)
			return
		}
		h.renderTaskEdit(w, r, http.StatusUnprocessableEntity, id, form, dueProblem)
		return
	}
	form.ProjectID = project
	err := h.store.UpdateTask(r.Context(), id, TaskInput{Title: form.Title, ProjectID: project, Status: form.Status, Due: due, Body: form.Body}, uploads)
	switch {
	case errors.Is(err, ErrInvalidTask):
		h.renderTaskEdit(w, r, http.StatusUnprocessableEntity, id, form, taskProblem)
	case attachProblem(err) != "":
		h.renderTaskEdit(w, r, http.StatusUnprocessableEntity, id, form, attachProblem(err))
	case err != nil:
		respondError(w, r, err)
	default:
		http.Redirect(w, r, taskURL(id), http.StatusSeeOther)
	}
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
	t, err := h.store.Task(r.Context(), id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	in := TaskInput{Title: t.Title, ProjectID: project, Status: TaskStatus(r.PostFormValue("status")), Due: due, Body: t.Body}
	err = h.store.UpdateTask(r.Context(), id, in, nil)
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
		http.Error(w, noFilesProblem, http.StatusUnprocessableEntity)
		return
	}
	_, err := h.store.AttachToTask(r.Context(), id, uploads)
	if problem := attachProblem(err); problem != "" {
		h.renderTaskEdit(w, r, http.StatusUnprocessableEntity, id, taskForm{}, problem)
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, taskURL(id)+"/edit", http.StatusSeeOther)
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

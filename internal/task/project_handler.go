package task

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/internal/page"
	"github.com/sakusi4/monolith/web"
)

type projectListPage struct {
	Filters web.FilterBar
	Rows    []projectRow
	NewName string
	Error   string
}

type projectRow struct {
	Project Project
	Period  string
	URL     string
}

type projectPage struct {
	Project     Project
	Period      string
	Editing     bool
	Edit        projectForm
	Statuses    []ProjectStatus
	Tasks       taskTable
	AllTasksURL string
	Attachments drive.Attachments
	Links       page.Links
	Error       string
	URL         string
	EditURL     string
	DeleteURL   string
	FilesURL    string
}

// projectForm holds a project's form values as the user typed them.
type projectForm struct {
	Name      string
	Status    ProjectStatus
	Started   string
	Finished  string
	Body      string
	Submitted bool
}

// projectView is what a request adds to a project page: its own form being edited, rejected input,
// or a quick add that failed.
type projectView struct {
	Editing bool
	Edit    projectForm
	Error   string
	Add     addForm
}

func (h *handler) listProjects(w http.ResponseWriter, r *http.Request) {
	h.renderProjects(w, r, http.StatusOK, r.URL.Query().Get("status"), "", "")
}

func (h *handler) renderProjects(w http.ResponseWriter, r *http.Request, status int, filter, newName, problem string) {
	statuses, err := parseProjectStatuses(filter)
	if err != nil {
		http.Error(w, "Invalid filter.", http.StatusBadRequest)
		return
	}
	projects, err := h.store.Projects(r.Context(), statuses)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	current := statuses[0]
	if filter == allValue {
		current = ""
	}
	options := append(web.Options(projectStatuses, ProjectStatus.Label, current), web.Option{Value: allValue, Label: "All", Selected: filter == allValue})
	view := projectListPage{
		Filters: web.FilterBar{Action: projectsURL, Filters: []web.Filter{{Name: "status", Label: "Status", Options: options}}},
		NewName: newName,
		Error:   problem,
	}
	for _, p := range projects {
		view.Rows = append(view.Rows, projectRow{Project: p, Period: period(p), URL: projectURL(p.ID)})
	}
	web.Render(w, r, status, "project_list", view)
}

func (h *handler) createProject(w http.ResponseWriter, r *http.Request) {
	name := r.PostFormValue("name")
	id, err := h.store.CreateProject(r.Context(), ProjectInput{Name: name, Status: ProjectActive})
	problem := projectErrorMessage(err, name)
	switch {
	case problem != "":
		h.renderProjects(w, r, http.StatusUnprocessableEntity, "", name, problem)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, projectURL(id), http.StatusSeeOther)
	}
}

func (h *handler) showProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderProject(w, r, http.StatusOK, id, projectView{})
}

func (h *handler) editProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderProject(w, r, http.StatusOK, id, projectView{Editing: true})
}

func (h *handler) renderProject(w http.ResponseWriter, r *http.Request, status int, id int64, view projectView) {
	ctx := r.Context()
	p, err := h.store.Project(ctx, id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	projects, err := h.store.Projects(ctx, projectStatuses)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	tasks, err := h.store.Tasks(ctx, TaskFilter{Statuses: openStatuses, ProjectID: id})
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	sec, err := h.store.drive.Attachments(ctx, p.FolderID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	links, err := h.store.pages.Links(ctx, p.PageID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	u := projectURL(id)
	table := h.newTaskTable(defaultTaskQuery.sort(tasks), projects, u, view.Add)
	table.ProjectID = id
	screen := projectPage{
		Project:     p,
		Period:      period(p),
		Editing:     view.Editing,
		Edit:        view.Edit,
		Statuses:    projectStatuses,
		Tasks:       table,
		AllTasksURL: tasksURL + "?" + taskQuery{Status: statusAll, ProjectID: id, Order: orderDue, Direction: web.Asc}.values().Encode(),
		Attachments: sec,
		Links:       links,
		Error:       view.Error,
		URL:         u,
		EditURL:     u + "/edit",
		DeleteURL:   u + "/delete",
		FilesURL:    u + "/files",
	}
	if view.Editing && !view.Edit.Submitted {
		screen.Edit = projectForm{Name: p.Name, Status: p.Status, Started: dateInput(p.StartedOn), Finished: dateInput(p.FinishedOn), Body: p.Body}
	}
	web.Render(w, r, status, "project_detail", screen)
}

func (h *handler) updateProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	fields, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	form := projectForm{
		Name:      fields.Get("name"),
		Status:    ProjectStatus(fields.Get("status")),
		Started:   fields.Get("started"),
		Finished:  fields.Get("finished"),
		Body:      fields.Get("body"),
		Submitted: true,
	}
	started, okStarted := parseDate(form.Started)
	finished, okFinished := parseDate(form.Finished)
	if !okStarted || !okFinished {
		if err := h.store.drive.Discard(uploads); err != nil {
			web.ServerError(w, r, err)
			return
		}
		h.renderProject(w, r, http.StatusUnprocessableEntity, id, projectView{Editing: true, Edit: form, Error: dateProblem})
		return
	}
	err := h.store.UpdateProject(r.Context(), id, ProjectInput{Name: form.Name, Status: form.Status, StartedOn: started, FinishedOn: finished, Body: form.Body}, uploads)
	problem := cmp.Or(projectErrorMessage(err, form.Name), attachProblem(err))
	switch {
	case problem != "":
		h.renderProject(w, r, http.StatusUnprocessableEntity, id, projectView{Editing: true, Edit: form, Error: problem})
	case err != nil:
		respondError(w, r, err)
	default:
		http.Redirect(w, r, projectURL(id), http.StatusSeeOther)
	}
}

func (h *handler) attachProjectFiles(w http.ResponseWriter, r *http.Request) {
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
	_, err := h.store.AttachToProject(r.Context(), id, uploads)
	if problem := attachProblem(err); problem != "" {
		h.renderProject(w, r, http.StatusUnprocessableEntity, id, projectView{Editing: true, Error: problem})
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, projectURL(id)+"/edit", http.StatusSeeOther)
}

func (h *handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := h.store.DeleteProject(r.Context(), id); err != nil {
		respondError(w, r, err)
		return
	}
	http.Redirect(w, r, projectsURL, http.StatusSeeOther)
}

// projectErrorMessage is the message for a project input that err rejects, or empty when err is
// not about the input.
func projectErrorMessage(err error, name string) string {
	switch {
	case errors.Is(err, ErrInvalidProject):
		return projectProblem
	case errors.Is(err, ErrNameTaken):
		return nameTakenProblem
	case errors.Is(err, ErrFolderTaken):
		return fmt.Sprintf("Drive already has a folder named %s in Projects.", name)
	}
	return ""
}

// period shows the dates of p, leaving out the ones that are unset.
func period(p Project) string {
	switch {
	case p.StartedOn.IsZero() && p.FinishedOn.IsZero():
		return ""
	case p.FinishedOn.IsZero():
		return p.StartedOn.Format(dateLayout) + " –"
	case p.StartedOn.IsZero():
		return "– " + p.FinishedOn.Format(dateLayout)
	}
	return p.StartedOn.Format(dateLayout) + " – " + p.FinishedOn.Format(dateLayout)
}

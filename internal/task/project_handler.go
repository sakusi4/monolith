package task

import (
	"errors"
	"fmt"
	"net/http"

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
	Editor      page.Editor
	Statuses    []ProjectStatus
	Started     string
	Finished    string
	Tasks       taskTable
	AllTasksURL string
	Backlinks   []page.Entry
	FieldsError string
	Error       string
	URL         string
	FieldsURL   string
	DeleteURL   string
}

// projectView is what a request adds to a project page: a change of its fields that failed, or a quick
// add or a change in the task table that failed.
type projectView struct {
	FieldsError string
	Error       string
	Add         addForm
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
	backlinks, err := h.store.pages.Backlinks(ctx, p.PageID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	u := projectURL(id)
	table := h.newTaskTable(defaultTaskQuery.sort(tasks), projects, u, view.Add)
	table.ProjectID = id
	screen := projectPage{
		Project:     p,
		Editor:      page.Editor{PageID: p.PageID, Title: p.Name, Body: p.Body, Version: page.FormatVersion(p.PageUpdatedAt), ContentURL: u + "/content", ImagesURL: u + "/images"},
		Statuses:    projectStatuses,
		Started:     dateInput(p.StartedOn),
		Finished:    dateInput(p.FinishedOn),
		Tasks:       table,
		AllTasksURL: tasksURL + "?" + taskQuery{Status: statusAll, ProjectID: id, Order: orderDue, Direction: web.Asc}.values().Encode(),
		Backlinks:   backlinks,
		FieldsError: view.FieldsError,
		Error:       view.Error,
		URL:         u,
		FieldsURL:   u + "/fields",
		DeleteURL:   u + "/delete",
	}
	web.Render(w, r, status, "project_detail", screen)
}

func (h *handler) updateProjectFields(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	started, okStarted := parseDate(r.PostFormValue("started"))
	finished, okFinished := parseDate(r.PostFormValue("finished"))
	if !okStarted || !okFinished {
		h.renderProject(w, r, http.StatusUnprocessableEntity, id, projectView{FieldsError: dateProblem})
		return
	}
	err := h.store.UpdateProjectFields(r.Context(), id, ProjectStatus(r.PostFormValue("status")), started, finished)
	switch {
	case errors.Is(err, ErrInvalidProject):
		h.renderProject(w, r, http.StatusUnprocessableEntity, id, projectView{FieldsError: fieldsProblem})
	case err != nil:
		respondError(w, r, err)
	default:
		http.Redirect(w, r, projectURL(id), http.StatusSeeOther)
	}
}

func (h *handler) saveProjectContent(w http.ResponseWriter, r *http.Request) {
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
	name := r.PostFormValue("title")
	saved, err := h.store.SaveProjectContent(r.Context(), id, name, r.PostFormValue("body"), version)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	problem := projectErrorMessage(err, name)
	if errors.Is(err, ErrInvalidProject) {
		problem = nameProblem
	}
	page.RespondSaved(w, r, saved, err, problem)
}

func (h *handler) uploadProjectImages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, uploads, ok := h.receive(w, r)
	if !ok {
		return
	}
	files, err := h.store.AttachToProject(r.Context(), id, uploads)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	page.RespondUploaded(w, r, files, err, attachProblem(err))
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

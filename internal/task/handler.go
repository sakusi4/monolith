package task

import (
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/drive"
	"github.com/sakusi4/monolith/web"
)

const (
	projectsURL      = "/task/projects"
	dateLayout       = "Jan 2, 2006"
	taskProblem      = "Enter a title, and pick the project and status from the lists."
	titleProblem     = "Enter a title."
	fieldsProblem    = "Pick a status from the list and dates in order."
	dueProblem       = "Enter the due date as YYYY-MM-DD."
	projectProblem   = "Use a name of up to 255 characters without a slash, a status from the list, and dates in order."
	nameProblem      = "Use a name of up to 255 characters without a slash."
	nameTakenProblem = "A project with that name already exists."
	dateProblem      = "Enter dates as YYYY-MM-DD."
	noFolderProblem  = "The project's folder is missing or in the trash."
	fileNameProblem  = "A file with that name is already attached."
	fileNameRule     = "Use file names of up to 255 characters without a slash."
)

type handler struct {
	store     *Store
	loc       *time.Location
	maxUpload int64
}

// NewHandler serves the task and project pages. loc decides which day is today, for overdue tasks,
// and maxUpload caps the bytes of one upload request.
func NewHandler(store *Store, loc *time.Location, maxUpload int64) http.Handler {
	h := &handler{store: store, loc: loc, maxUpload: maxUpload}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /task/tasks", h.listTasks)
	mux.HandleFunc("POST /task/tasks/new", h.createTask)
	mux.HandleFunc("GET /task/tasks/{id}", h.showTask)
	mux.HandleFunc("POST /task/tasks/{id}/fields", h.updateTaskFields)
	mux.HandleFunc("POST /task/tasks/{id}/content", h.saveTaskContent)
	mux.HandleFunc("POST /task/tasks/{id}/images", h.uploadTaskImages)
	mux.HandleFunc("POST /task/tasks/{id}/delete", h.deleteTask)
	mux.HandleFunc("GET /task/projects", h.listProjects)
	mux.HandleFunc("POST /task/projects/new", h.createProject)
	mux.HandleFunc("GET /task/projects/{id}", h.showProject)
	mux.HandleFunc("POST /task/projects/{id}/fields", h.updateProjectFields)
	mux.HandleFunc("POST /task/projects/{id}/content", h.saveProjectContent)
	mux.HandleFunc("POST /task/projects/{id}/images", h.uploadProjectImages)
	mux.HandleFunc("POST /task/projects/{id}/delete", h.deleteProject)
	return mux
}

// taskTable is a table of tasks, whose project, status, and due date change in place, with its add
// form, as the task list and the project page show it. ProjectID fixes the project of the add form
// and the rows on a project page; it is 0 on the task list.
type taskTable struct {
	Rows        []taskRow
	Add         addForm
	Projects    []Project
	Statuses    []TaskStatus
	Next        string
	ProjectID   int64
	ShowProject bool
}

type taskRow struct {
	Task      Task
	Due       string
	Overdue   bool
	URL       string
	FieldsURL string
	DeleteURL string
}

// addForm holds the quick add form's values as the user typed them, with the error to show next to them.
type addForm struct {
	Title     string
	ProjectID int64
	Due       string
	Submitted bool
	URL       string
	Error     string
}

func (h *handler) newTaskTable(tasks []Task, projects []Project, next string, add addForm) taskTable {
	table := taskTable{Add: add, Projects: projects, Statuses: taskStatuses, Next: next}
	table.Add.URL = tasksURL + "/new"
	day := today(time.Now(), h.loc)
	for _, t := range tasks {
		table.Rows = append(table.Rows, taskRow{
			Task:      t,
			Due:       dateInput(t.Due),
			Overdue:   !t.Due.IsZero() && t.Due.Before(day) && t.Status.IsOpen(),
			URL:       taskURL(t.ID),
			FieldsURL: taskURL(t.ID) + "/fields",
			DeleteURL: taskURL(t.ID) + "/delete",
		})
	}
	return table
}

// renderNext shows the page that next points to with add and problem, for a quick add or a change
// in the table that failed.
func (h *handler) renderNext(w http.ResponseWriter, r *http.Request, status int, next string, add addForm, problem string) {
	u, err := url.Parse(next)
	if err != nil {
		h.renderTasks(w, r, status, defaultTaskQuery, add, problem)
		return
	}
	if id, ok := projectIDFromPath(u.Path); ok {
		h.renderProject(w, r, status, id, projectView{Add: add, Error: problem})
		return
	}
	if id, ok := taskIDFromPath(u.Path); ok {
		h.renderTask(w, r, status, id, problem)
		return
	}
	q, err := parseTaskQuery(u.Query())
	if err != nil {
		q = defaultTaskQuery
	}
	h.renderTasks(w, r, status, q, add, problem)
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

// attachProblem is the message for an upload that err rejects, or empty when err is not about the upload.
func attachProblem(err error) string {
	switch {
	case errors.Is(err, ErrNoFolder):
		return noFolderProblem
	case errors.Is(err, drive.ErrNameTaken):
		return fileNameProblem
	case errors.Is(err, drive.ErrInvalidName):
		return fileNameRule
	}
	return ""
}

// safeNext returns next when it is under /task/ both as written and once cleaned, and the task list otherwise.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/task/") && strings.HasPrefix(path.Clean(next), "/task/") {
		return next
	}
	return tasksURL
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

// parseDate reads a date input where empty means no date.
func parseDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

func dateInput(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateOnly)
}

func taskURL(id int64) string {
	return tasksURL + "/" + strconv.FormatInt(id, 10)
}

func projectURL(id int64) string {
	return projectsURL + "/" + strconv.FormatInt(id, 10)
}

// projectIDFromPath reads the id of a project page path.
func projectIDFromPath(path string) (int64, bool) {
	return idFromPath(path, projectsURL+"/")
}

// taskIDFromPath reads the id of a task page path.
func taskIDFromPath(path string) (int64, bool) {
	return idFromPath(path, tasksURL+"/")
}

func idFromPath(path, prefix string) (int64, bool) {
	rest, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	return id, err == nil
}

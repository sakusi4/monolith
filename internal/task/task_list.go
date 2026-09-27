package task

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sakusi4/monolith/web"
)

const (
	tasksURL   = "/task/tasks"
	inboxValue = "inbox"
	allValue   = "all"
)

type taskOrder string

const (
	orderDue     taskOrder = "due"
	orderProject taskOrder = "project"
	orderStatus  taskOrder = "status"
	orderCreated taskOrder = "created"
)

var taskOrders = []taskOrder{orderDue, orderProject, orderStatus, orderCreated}

func (o taskOrder) Label() string {
	switch o {
	case orderDue:
		return "Due"
	case orderProject:
		return "Project"
	case orderStatus:
		return "Status"
	case orderCreated:
		return "Created"
	}
	return string(o)
}

// statusFilter is one task status, or "open" for the statuses still to do, or "all".
type statusFilter string

const (
	statusOpen statusFilter = "open"
	statusAll  statusFilter = allValue
)

var statusFilters = []statusFilter{statusOpen, statusFilter(StatusTodo), statusFilter(StatusInProgress), statusFilter(StatusDone), statusFilter(StatusCanceled), statusAll}

func (f statusFilter) Label() string {
	switch f {
	case statusOpen:
		return "Open"
	case statusAll:
		return "All"
	}
	return TaskStatus(f).Label()
}

func (f statusFilter) statuses() []TaskStatus {
	switch f {
	case statusOpen:
		return openStatuses
	case statusAll:
		return taskStatuses
	}
	return []TaskStatus{TaskStatus(f)}
}

// taskQuery is the task list state in the URL. ProjectID is 0 for every project, and Inbox
// narrows the list to tasks without one.
type taskQuery struct {
	Status    statusFilter
	ProjectID int64
	Inbox     bool
	Order     taskOrder
	Direction web.Direction
}

var defaultTaskQuery = taskQuery{Status: statusOpen, Order: orderDue, Direction: web.Asc}

func parseTaskQuery(v url.Values) (taskQuery, error) {
	var q taskQuery
	var statusErr, orderErr, directionErr, projectErr error
	q.Status, statusErr = web.ParseChoice("status", v.Get("status"), statusFilters, statusOpen)
	q.Order, orderErr = web.ParseChoice("order", v.Get("order"), taskOrders, orderDue)
	q.Direction, directionErr = web.ParseChoice("direction", v.Get("direction"), web.Directions, web.Asc)
	switch p := v.Get("project"); p {
	case "":
	case inboxValue:
		q.Inbox = true
	default:
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id < 1 {
			projectErr = fmt.Errorf("invalid project %q", p)
		}
		q.ProjectID = id
	}
	if err := errors.Join(statusErr, orderErr, directionErr, projectErr); err != nil {
		return taskQuery{}, err
	}
	return q, nil
}

func (q taskQuery) filter() TaskFilter {
	return TaskFilter{Statuses: q.Status.statuses(), ProjectID: q.ProjectID, Inbox: q.Inbox}
}

func (q taskQuery) filters(projects []Project) []web.Filter {
	projectOptions := []web.Option{{Label: "All", Selected: q.ProjectID == 0 && !q.Inbox}, {Value: inboxValue, Label: "Inbox", Selected: q.Inbox}}
	for _, p := range projects {
		projectOptions = append(projectOptions, web.Option{Value: strconv.FormatInt(p.ID, 10), Label: p.Name, Selected: p.ID == q.ProjectID})
	}
	return []web.Filter{
		{Name: "status", Label: "Status", Options: web.Options(statusFilters, statusFilter.Label, q.Status)},
		{Name: "project", Label: "Project", Options: projectOptions},
		{Name: "order", Label: "Sort by", Options: web.Options(taskOrders, taskOrder.Label, q.Order)},
		{Name: "direction", Label: "Direction", Options: web.Options(web.Directions, web.Direction.Label, q.Direction)},
	}
}

// sort orders tasks by q. Tasks without a due date come last in either direction when sorting by
// due date, and tasks that compare equal keep the order they were created in.
func (q taskQuery) sort(tasks []Task) []Task {
	tasks = slices.Clone(tasks)
	slices.SortStableFunc(tasks, func(a, b Task) int {
		if q.Order == orderDue && a.Due.IsZero() != b.Due.IsZero() {
			if a.Due.IsZero() {
				return 1
			}
			return -1
		}
		c := q.compare(a, b)
		if q.Direction == web.Desc {
			c = -c
		}
		return cmp.Or(c, a.CreatedAt.Compare(b.CreatedAt))
	})
	return tasks
}

func (q taskQuery) compare(a, b Task) int {
	switch q.Order {
	case orderDue:
		return a.Due.Compare(b.Due)
	case orderProject:
		return strings.Compare(strings.ToLower(a.ProjectName), strings.ToLower(b.ProjectName))
	case orderStatus:
		return cmp.Compare(slices.Index(taskStatuses, a.Status), slices.Index(taskStatuses, b.Status))
	case orderCreated:
		return a.CreatedAt.Compare(b.CreatedAt)
	}
	return 0
}

func (q taskQuery) listURL() string {
	return tasksURL + "?" + q.values().Encode()
}

func (q taskQuery) values() url.Values {
	v := url.Values{}
	v.Set("status", string(q.Status))
	v.Set("order", string(q.Order))
	v.Set("direction", string(q.Direction))
	switch {
	case q.Inbox:
		v.Set("project", inboxValue)
	case q.ProjectID != 0:
		v.Set("project", strconv.FormatInt(q.ProjectID, 10))
	}
	return v
}

// parseProjectStatuses reads the status filter of the project list: one status, "all", or Active when empty.
func parseProjectStatuses(value string) ([]ProjectStatus, error) {
	if value == allValue {
		return projectStatuses, nil
	}
	status, err := web.ParseChoice("status", value, projectStatuses, ProjectActive)
	if err != nil {
		return nil, err
	}
	return []ProjectStatus{status}, nil
}

// today is the date in loc at now, as midnight UTC like the dates read from the database.
func today(now time.Time, loc *time.Location) time.Time {
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

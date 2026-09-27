package task

import (
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/sakusi4/monolith/web"
)

func TestParseTaskQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    taskQuery
		wantErr bool
	}{
		{"every parameter", "status=done&project=7&order=project&direction=desc", taskQuery{Status: "done", ProjectID: 7, Order: orderProject, Direction: web.Desc}, false},
		{"the inbox", "project=inbox", taskQuery{Status: statusOpen, Inbox: true, Order: orderDue, Direction: web.Asc}, false},
		{"a project that is not an id", "project=abc", taskQuery{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseTaskQuery(values)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("parseTaskQuery(%q) = %+v, %v, want %+v, error %v", tt.query, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestStatusFilter_Statuses(t *testing.T) {
	tests := []struct {
		filter statusFilter
		want   []TaskStatus
	}{
		{statusOpen, []TaskStatus{StatusTodo, StatusInProgress}},
		{statusAll, []TaskStatus{StatusTodo, StatusInProgress, StatusDone, StatusCanceled}},
		{"done", []TaskStatus{StatusDone}},
	}
	for _, tt := range tests {
		t.Run(string(tt.filter), func(t *testing.T) {
			if got := tt.filter.statuses(); !slices.Equal(got, tt.want) {
				t.Errorf("%q.statuses() = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}

func TestTaskQuery_Sort(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, time.October, d, 0, 0, 0, 0, time.UTC) }
	created := func(h int) time.Time { return time.Date(2026, time.September, 1, h, 0, 0, 0, time.UTC) }
	tasks := []Task{
		{ID: 1, ProjectName: "tunnel", Status: StatusDone, Due: day(5), CreatedAt: created(1)},
		{ID: 2, ProjectName: "", Status: StatusTodo, CreatedAt: created(2)},
		{ID: 3, ProjectName: "Heriot", Status: StatusInProgress, Due: day(1), CreatedAt: created(3)},
		{ID: 4, ProjectName: "tunnel", Status: StatusTodo, Due: day(5), CreatedAt: created(4)},
	}
	tests := []struct {
		name string
		q    taskQuery
		want []int64
	}{
		{"nearest due first, same due by creation, no due last", taskQuery{Order: orderDue, Direction: web.Asc}, []int64{3, 1, 4, 2}},
		{"latest due first, no due still last", taskQuery{Order: orderDue, Direction: web.Desc}, []int64{1, 4, 3, 2}},
		{"inbox first, then projects by name", taskQuery{Order: orderProject, Direction: web.Asc}, []int64{2, 3, 1, 4}},
		{"status in workflow order", taskQuery{Order: orderStatus, Direction: web.Asc}, []int64{2, 4, 3, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int64
			for _, task := range tt.q.sort(tasks) {
				got = append(got, task.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("sort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToday(t *testing.T) {
	dubai, err := time.LoadLocation("Asia/Dubai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 30, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		loc  *time.Location
		want time.Time
	}{
		{"UTC is still September 30", time.UTC, time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)},
		{"Dubai is already October 1", dubai, time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := today(now, tt.loc); !got.Equal(tt.want) {
				t.Errorf("today() = %v, want %v", got, tt.want)
			}
		})
	}
}

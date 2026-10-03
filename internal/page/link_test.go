package page

import (
	"slices"
	"testing"
)

func TestLinksOf(t *testing.T) {
	body := "[a](/page/pages/1) see http://localhost:8080/task/projects/2 and /task/tasks/3/edit ![c](/drive/files/4/content)"
	got := linksOf(body)
	if !slices.Equal(got.Pages, []int64{1}) || !slices.Equal(got.Projects, []int64{2}) || !slices.Equal(got.Tasks, []int64{3}) {
		t.Errorf("linksOf() = %+v, want pages [1], projects [2], tasks [3]", got)
	}
}

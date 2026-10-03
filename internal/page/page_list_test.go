package page

import (
	"slices"
	"testing"
)

func TestPageQuery_pick(t *testing.T) {
	ids := func(entries []Entry) []int64 {
		var ids []int64
		for _, e := range entries {
			ids = append(ids, e.ID)
		}
		return ids
	}
	kinds := []Entry{{ID: 1, Kind: kindPage}, {ID: 2, Kind: kindProject}, {ID: 3, Kind: kindTask}}
	many := make([]Entry, maxResults+1)
	for i := range many {
		many[i] = Entry{ID: int64(i), Kind: kindPage}
	}
	tests := []struct {
		name    string
		q       pageQuery
		entries []Entry
		want    []int64
	}{
		{"all keeps every kind", pageQuery{Kind: kindAll}, kinds, []int64{1, 2, 3}},
		{"a kind keeps only its pages", pageQuery{Kind: kindProject}, kinds, []int64{2}},
		{"a search keeps the first 100", pageQuery{Q: "x", Kind: kindAll}, many, ids(many[:maxResults])},
		{"the list keeps them all", pageQuery{Kind: kindAll}, many, ids(many)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(tt.q.pick(tt.entries)); !slices.Equal(got, tt.want) {
				t.Errorf("pick() = %v, want %v", got, tt.want)
			}
		})
	}
}

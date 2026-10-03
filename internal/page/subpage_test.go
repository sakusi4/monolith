package page

import (
	"slices"
	"testing"
)

func TestSubpageLinks(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"a placeholder in text", "See [[SDN]] here", []string{"SDN"}},
		{"a code span keeps it as text", "Run `[[ -f x ]]` first", nil},
		{"a code block keeps it as text", "```\n[[ -f x ]]\n```", nil},
		{"a blank title stays text", "[[   ]]", nil},
		{"a bracket inside is no placeholder", "[[a]b]]", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range subpageLinks(tt.body) {
				got = append(got, l.Title)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("subpageLinks(%q) titles = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestReplaceSubpages(t *testing.T) {
	body := "[[SDN]], [[OpenFlow]] and [[SDN]] again"
	links := subpageLinks(body)
	if got := uniqueTitles(links); !slices.Equal(got, []string{"SDN", "OpenFlow"}) {
		t.Errorf("uniqueTitles() = %q, want [SDN OpenFlow]", got)
	}
	want := "[SDN](/page/pages/4), [OpenFlow](/page/pages/5) and [SDN](/page/pages/4) again"
	if got := replaceSubpages(body, links, map[string]int64{"SDN": 4, "OpenFlow": 5}); got != want {
		t.Errorf("replaceSubpages() = %q, want %q", got, want)
	}
}

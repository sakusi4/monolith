package web

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    string
		notWant string
	}{
		{"a task list gets checkboxes", "- [x] passport", `type="checkbox"`, ""},
		{"a table", "| a |\n|---|\n| 1 |", "<table>", ""},
		{"headings move one level down", "# Plan", "<h2", "<h1"},
		{"raw HTML is left out", "<script>alert(1)</script>", "", "<script>"},
		{"an unsafe link keeps no target", "[x](javascript:alert(1))", "", "javascript:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderMarkdown(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), tt.want) || (tt.notWant != "" && strings.Contains(string(got), tt.notWant)) {
				t.Errorf("renderMarkdown(%q) = %q, want it to contain %q and not %q", tt.source, got, tt.want, tt.notWant)
			}
		})
	}
}

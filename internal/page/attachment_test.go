package page

import (
	"strings"
	"testing"
)

func TestFolderName(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"the title as it is", "SDN", "SDN (#7)"},
		{"slashes become dashes", "A/B test", "A-B test (#7)"},
		{"a long title is cut", strings.Repeat("가", 250), strings.Repeat("가", 200) + " (#7)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := folderName(tt.title, 7); got != tt.want {
				t.Errorf("folderName(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

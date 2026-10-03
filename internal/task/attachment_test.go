package task

import (
	"strings"
	"testing"
)

func TestTaskFolderName(t *testing.T) {
	tests := []struct {
		name         string
		title        string
		wantName     string
		wantFallback string
	}{
		{"the title as it is", "Visa renewal", "Visa renewal", "Visa renewal (#7)"},
		{"slashes become dashes", "A/B test", "A-B test", "A-B test (#7)"},
		{"a long title is cut", strings.Repeat("가", 250), strings.Repeat("가", 200), strings.Repeat("가", 200) + " (#7)"},
		{"a blank result uses the id", "  ", "Task 7", "Task 7 (#7)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, fallback := taskFolderName(tt.title, 7)
			if name != tt.wantName || fallback != tt.wantFallback {
				t.Errorf("taskFolderName(%q) = %q, %q, want %q, %q", tt.title, name, fallback, tt.wantName, tt.wantFallback)
			}
		})
	}
}

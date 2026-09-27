package task

import "testing"

func TestSafeNext(t *testing.T) {
	tests := []struct {
		next string
		want string
	}{
		{"/task/projects/3", "/task/projects/3"},
		{"//evil.example/task/", "/task/tasks"},
		{"/task/../drive", "/task/tasks"},
		{"//evil.example/../task/tasks", "/task/tasks"},
		{"https://evil.example/task/", "/task/tasks"},
		{"", "/task/tasks"},
	}
	for _, tt := range tests {
		t.Run(tt.next, func(t *testing.T) {
			if got := safeNext(tt.next); got != tt.want {
				t.Errorf("safeNext(%q) = %q, want %q", tt.next, got, tt.want)
			}
		})
	}
}

package task

import (
	"errors"
	"testing"
	"time"
)

func TestProjectInput_Clean(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, time.September, d, 0, 0, 0, 0, time.UTC) }
	tests := []struct {
		name    string
		in      ProjectInput
		wantErr error
	}{
		{"a name the drive accepts", ProjectInput{Name: " tunnel ", Status: ProjectActive, StartedOn: day(1), FinishedOn: day(1)}, nil},
		{"a name the drive rejects", ProjectInput{Name: "a/b", Status: ProjectActive}, ErrInvalidProject},
		{"unknown status", ProjectInput{Name: "tunnel", Status: "someday"}, ErrInvalidProject},
		{"finishes before it starts", ProjectInput{Name: "tunnel", Status: ProjectActive, StartedOn: day(2), FinishedOn: day(1)}, ErrInvalidProject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.in.Clean(); !errors.Is(err, tt.wantErr) {
				t.Errorf("Clean() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

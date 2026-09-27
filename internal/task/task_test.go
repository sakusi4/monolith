package task

import (
	"errors"
	"testing"
)

func TestTaskInput_Clean(t *testing.T) {
	tests := []struct {
		name    string
		in      TaskInput
		want    TaskInput
		wantErr error
	}{
		{"title and body are trimmed", TaskInput{Title: " Visa ", Status: StatusTodo, Body: " renew \n"}, TaskInput{Title: "Visa", Status: StatusTodo, Body: "renew"}, nil},
		{"blank title", TaskInput{Title: "  ", Status: StatusTodo}, TaskInput{}, ErrInvalidTask},
		{"unknown status", TaskInput{Title: "Visa", Status: "later"}, TaskInput{}, ErrInvalidTask},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Clean()
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Errorf("Clean() = %+v, %v, want %+v, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

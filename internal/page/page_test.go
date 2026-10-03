package page

import (
	"errors"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		want    string
		wantErr error
	}{
		{"spaces are trimmed", "  SDN  ", "SDN", nil},
		{"decomposed hangul is composed", "가", "가", nil},
		{"a blank title", " \t ", "", ErrInvalidTitle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanTitle(tt.title)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Errorf("CleanTitle(%q) = %q, %v, want %q, %v", tt.title, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

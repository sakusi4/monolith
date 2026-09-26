package drive

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"spaces are trimmed", "  Projects ", "Projects", nil},
		{"decomposed Hangul from macOS is composed", "\u1100\u1175\u1112\u116c\u11a8\u110b\u1161\u11ab.md", "기획안.md", nil},
		{"blank", "   ", "", ErrInvalidName},
		{"slash", "a/b", "", ErrInvalidName},
		{"255 characters", strings.Repeat("가", 255), strings.Repeat("가", 255), nil},
		{"256 characters", strings.Repeat("가", 256), "", ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanName(tt.in)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Errorf("cleanName(%q) = %q, %v, want %q, %v", tt.in, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

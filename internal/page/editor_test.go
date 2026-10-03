package page

import (
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	saved := time.Date(2026, time.October, 3, 9, 30, 15, 123456000, time.UTC)
	got, err := ParseVersion(FormatVersion(saved))
	if err != nil || !got.Equal(saved) {
		t.Errorf("ParseVersion(FormatVersion(%v)) = %v, %v, want the same time", saved, got, err)
	}
	if _, err := ParseVersion("yesterday"); err == nil {
		t.Errorf("ParseVersion(%q) error = nil, want an error", "yesterday")
	}
}

package finance

import (
	"testing"
	"time"
)

func TestCurrentMonth(t *testing.T) {
	dubai, err := time.LoadLocation("Asia/Dubai")
	if err != nil {
		t.Fatal(err)
	}
	lastHourOfSeptemberUTC := time.Date(2026, time.September, 30, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		loc  *time.Location
		want time.Time
	}{
		{"UTC is still September", time.UTC, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)},
		{"Dubai is already October", dubai, time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := currentMonth(lastHourOfSeptemberUTC, tt.loc); !got.Equal(tt.want) {
				t.Errorf("currentMonth(%v, %s) = %v, want %v", lastHourOfSeptemberUTC, tt.loc, got, tt.want)
			}
		})
	}
}

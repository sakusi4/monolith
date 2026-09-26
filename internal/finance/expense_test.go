package finance

import (
	"errors"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func TestExpenseInput_Clean(t *testing.T) {
	day := time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		in      ExpenseInput
		want    ExpenseInput
		wantErr bool
	}{
		{"trims the name", ExpenseInput{Date: day, Name: "  Rent ", Category: CategoryHousing, Currency: money.AED, Amount: -5}, ExpenseInput{Date: day, Name: "Rent", Category: CategoryHousing, Currency: money.AED, Amount: -5}, false},
		{"no date", ExpenseInput{Name: "Rent", Category: CategoryHousing, Currency: money.AED}, ExpenseInput{}, true},
		{"empty name", ExpenseInput{Date: day, Name: "  ", Category: CategoryFood, Currency: money.USD}, ExpenseInput{}, true},
		{"unknown category", ExpenseInput{Date: day, Name: "Gym", Category: "sports", Currency: money.USD}, ExpenseInput{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Clean()
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("Clean() = %+v, %v, want %+v, error %v", got, err, tt.want, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidItem) {
				t.Errorf("Clean() error = %v, want ErrInvalidItem", err)
			}
		})
	}
}

func TestDefaultExpenseDate(t *testing.T) {
	dubai, err := time.LoadLocation("Asia/Dubai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 30, 21, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		loc   *time.Location
		month time.Time
		want  time.Time
	}{
		{"today in the current month", time.UTC, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)},
		{"today follows the time zone", dubai, time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)},
		{"another month starts on its first day", time.UTC, time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultExpenseDate(now, tt.loc, tt.month); !got.Equal(tt.want) {
				t.Errorf("defaultExpenseDate() = %s, want %s", got.Format(time.DateOnly), tt.want.Format(time.DateOnly))
			}
		})
	}
}

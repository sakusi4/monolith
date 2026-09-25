package finance

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func TestSnapshot_Totals(t *testing.T) {
	krwRate := big.NewRat(1000, 1)
	tests := []struct {
		name  string
		items []SnapshotItem
		want  Totals
	}{
		{
			name: "loans subtract from assets",
			items: []SnapshotItem{
				{Currency: money.USD, Amount: 500000},
				{Currency: money.KRW, Amount: -1000000, PerUSD: krwRate},
			},
			want: Totals{NetWorth: 400000, Loans: -100000},
		},
		{
			name: "currencies without a rate are left out and reported once",
			items: []SnapshotItem{
				{Currency: money.USD, Amount: 100},
				{Currency: money.AED, Amount: 100},
				{Currency: money.AED, Amount: 200},
			},
			want: Totals{NetWorth: 100, Missing: []money.Currency{money.AED}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Snapshot{Items: tt.items}).Totals(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Totals() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

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

func TestMonthsUntil(t *testing.T) {
	got := monthsUntil(time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC))
	want := []time.Time{
		time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("monthsUntil(Apr 2024) = %v, want %v", got, want)
	}
	if got := monthsUntil(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)); len(got) != 32 {
		t.Errorf("monthsUntil(Sep 2026) has %d months, want 32 from Feb 2024", len(got))
	}
}

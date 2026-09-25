package finance

import (
	"errors"
	"testing"

	"github.com/sakusi4/monolith/internal/money"
)

func TestItemInput_Clean(t *testing.T) {
	tests := []struct {
		name    string
		in      ItemInput
		want    ItemInput
		wantErr error
	}{
		{"name is trimmed and loans may be negative", ItemInput{Name: " Mortgage ", Type: AssetLoan, Currency: money.KRW, Amount: -5}, ItemInput{Name: "Mortgage", Type: AssetLoan, Currency: money.KRW, Amount: -5}, nil},
		{"blank name", ItemInput{Name: "  ", Type: AssetCash, Currency: money.USD}, ItemInput{}, ErrInvalidItem},
		{"unknown type", ItemInput{Name: "bar", Type: "gold", Currency: money.USD}, ItemInput{}, ErrInvalidItem},
		{"unknown currency", ItemInput{Name: "wallet", Type: AssetCash, Currency: "EUR"}, ItemInput{}, ErrInvalidItem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Clean()
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("Clean() = %+v, %v, want %+v, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

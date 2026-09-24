package finance

import (
	"errors"
	"testing"
)

func TestAssetInput_Clean(t *testing.T) {
	tests := []struct {
		name    string
		in      AssetInput
		want    AssetInput
		wantErr error
	}{
		{
			name: "valid input has its name trimmed",
			in:   AssetInput{Type: AssetStock, Name: "  VOO ", AmountCents: 1250},
			want: AssetInput{Type: AssetStock, Name: "VOO", AmountCents: 1250},
		},
		{
			name: "zero amount is allowed",
			in:   AssetInput{Type: AssetCash, Name: "wallet", AmountCents: 0},
			want: AssetInput{Type: AssetCash, Name: "wallet", AmountCents: 0},
		},
		{
			name:    "unknown type is invalid",
			in:      AssetInput{Type: "gold", Name: "bar", AmountCents: 1},
			wantErr: ErrInvalidAsset,
		},
		{
			name:    "blank name is invalid",
			in:      AssetInput{Type: AssetCash, Name: "   ", AmountCents: 1},
			wantErr: ErrInvalidAsset,
		},
		{
			name:    "negative amount is invalid",
			in:      AssetInput{Type: AssetCash, Name: "wallet", AmountCents: -1},
			wantErr: ErrInvalidAsset,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Clean()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Clean() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Clean() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

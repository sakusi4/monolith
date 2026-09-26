package finance

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sakusi4/monolith/internal/money"
)

func TestFetchRates(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    map[money.Currency]string
		wantErr bool
	}{
		{
			name: "success keeps the exact decimal text",
			body: `{"result":"success","time_last_update_unix":1790294400,"rates":{"USD":1,"KRW":1368.604976,"AED":3.6725,"JPY":149.5,"EUR":0.9}}`,
			want: map[money.Currency]string{money.KRW: "1368.604976", money.AED: "3.6725", money.JPY: "149.5"},
		},
		{"missing currency", `{"result":"success","time_last_update_unix":1790294400,"rates":{"KRW":1368.6,"AED":3.6725}}`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(srv.Close)

			got, err := fetchRates(t.Context(), srv.Client(), srv.URL)
			if tt.wantErr {
				if err == nil {
					t.Errorf("fetchRates() = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("fetchRates() error = %v", err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("fetchRates() = %v, want %v", got, tt.want)
			}
		})
	}
}

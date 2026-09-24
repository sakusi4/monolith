package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestFinance(t *testing.T) {
	s := newTestServer(t)
	session := s.login(t)
	listBody := func(t *testing.T) string {
		t.Helper()
		rec := s.get(t, "/finance/assets", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /finance/assets = %d, want 200", rec.Code)
		}
		return rec.Body.String()
	}

	t.Run("create shows the asset in the list", func(t *testing.T) {
		form := url.Values{"type": {"stock"}, "name": {" VOO "}, "amount": {"12,345.67"}}
		wantRedirect(t, s.post(t, "/finance/assets/new", form, session), "/finance/assets")
		body := listBody(t)
		if !strings.Contains(body, "VOO") || !strings.Contains(body, "$12,345.67") || !strings.Contains(body, "Stock") {
			t.Errorf("asset list does not show Stock VOO $12,345.67:\n%s", body)
		}
	})

	t.Run("invalid input shows the form again", func(t *testing.T) {
		tests := []url.Values{
			{"type": {"stock"}, "name": {"VOO"}, "amount": {"-5"}},
			{"type": {"gold"}, "name": {"bar"}, "amount": {"1"}},
			{"type": {"cash"}, "name": {"  "}, "amount": {"1"}},
		}
		for _, form := range tests {
			if rec := s.post(t, "/finance/assets/new", form, session); rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("POST /finance/assets/new %v = %d, want 422", form, rec.Code)
			}
		}
	})

	var id int64
	if err := s.db.QueryRowContext(t.Context(), `SELECT id FROM assets`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	path := "/finance/assets/" + strconv.FormatInt(id, 10)

	t.Run("edit replaces the fields", func(t *testing.T) {
		if rec := s.get(t, path+"/edit", session); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "12345.67") {
			t.Errorf("GET %s/edit = %d, want 200 with the current amount", path, rec.Code)
		}
		form := url.Values{"type": {"cash"}, "name": {"wallet"}, "amount": {"0.29"}}
		wantRedirect(t, s.post(t, path+"/edit", form, session), "/finance/assets")
		body := listBody(t)
		if !strings.Contains(body, "wallet") || !strings.Contains(body, "$0.29") {
			t.Errorf("asset list does not show wallet $0.29:\n%s", body)
		}
	})

	t.Run("delete removes the asset", func(t *testing.T) {
		wantRedirect(t, s.post(t, path+"/delete", nil, session), "/finance/assets")
		if body := listBody(t); strings.Contains(body, "wallet") {
			t.Errorf("asset list still shows wallet after delete")
		}
		if rec := s.post(t, path+"/delete", nil, session); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s/delete twice = %d, want 404", path, rec.Code)
		}
		if rec := s.get(t, path+"/edit", session); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s/edit after delete = %d, want 404", path, rec.Code)
		}
	})
}

package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestDashboard(t *testing.T) {
	s := newTestServer(t)
	session := s.login(t)

	t.Run("home redirects to the dashboard", func(t *testing.T) {
		wantRedirect(t, s.get(t, "/", session), "/dashboard")
	})

	t.Run("the dashboard is empty for now", func(t *testing.T) {
		rec := s.get(t, "/dashboard", session)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, "Nothing here yet.") || !strings.Contains(body, `href="/dashboard" aria-current="page"`) {
			t.Errorf("GET /dashboard = %d, want 200 with an empty page marked as current:\n%s", rec.Code, body)
		}
	})
}

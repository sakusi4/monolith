package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAuth(t *testing.T) {
	s := newTestServer(t)

	t.Run("pages redirect to login without a session", func(t *testing.T) {
		wantRedirect(t, s.get(t, "/finance/assets", nil), "/auth/login")
	})

	t.Run("wrong password shows the form again", func(t *testing.T) {
		rec := s.post(t, "/auth/login", url.Values{"email": {testEmail}, "password": {"wrong"}}, nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Incorrect email or password") {
			t.Errorf("POST /auth/login with wrong password = %d, want 401 with an error message", rec.Code)
		}
	})

	t.Run("login ignores email case and sets a secure session cookie", func(t *testing.T) {
		rec := s.post(t, "/auth/login", url.Values{"email": {"ME@Example.com"}, "password": {testPassword}}, nil)
		session := sessionCookie(t, rec)
		wantRedirect(t, rec, "/")
		if !session.HttpOnly || !session.Secure {
			t.Errorf("session cookie HttpOnly=%v Secure=%v, want both true", session.HttpOnly, session.Secure)
		}
		if rec := s.get(t, "/finance/assets", session); rec.Code != http.StatusOK {
			t.Errorf("GET /finance/assets with session = %d, want 200", rec.Code)
		}
	})

	t.Run("logout ends the session", func(t *testing.T) {
		session := s.login(t)
		wantRedirect(t, s.post(t, "/auth/logout", nil, session), "/auth/login")
		wantRedirect(t, s.get(t, "/finance/assets", session), "/auth/login")
	})

	t.Run("expired session redirects to login", func(t *testing.T) {
		session := s.login(t)
		if _, err := s.db.ExecContext(t.Context(), `UPDATE sessions SET expires_at = now() - interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		wantRedirect(t, s.get(t, "/finance/assets", session), "/auth/login")
	})
}

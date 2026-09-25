package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/auth"
	"github.com/sakusi4/monolith/internal/postgres/postgrestest"
)

const (
	testEmail    = "me@example.com"
	testPassword = "correct horse"
)

type testServer struct {
	handler http.Handler
	db      *sql.DB
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	db := postgrestest.New(t)
	if err := auth.NewStore(db).SetUser(t.Context(), testEmail, testPassword); err != nil {
		t.Fatal(err)
	}
	return &testServer{handler: routes(db, time.UTC), db: db}
}

func (s *testServer) get(t *testing.T, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil), cookie)
}

func (s *testServer) post(t *testing.T, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.do(t, req, cookie)
}

func (s *testServer) do(t *testing.T, req *http.Request, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s *testServer) login(t *testing.T) *http.Cookie {
	t.Helper()
	rec := s.post(t, "/auth/login", url.Values{"email": {testEmail}, "password": {testPassword}}, nil)
	return sessionCookie(t, rec)
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /auth/login = %d, want 303", rec.Code)
	}
	cookie, err := http.ParseSetCookie(rec.Header().Get("Set-Cookie"))
	if err != nil {
		t.Fatal(err)
	}
	return cookie
}

func wantRedirect(t *testing.T, rec *httptest.ResponseRecorder, location string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != location {
		t.Errorf("response = %d to %q, want 303 to %q", rec.Code, rec.Header().Get("Location"), location)
	}
}

func TestCrossSitePostIsRejected(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/login", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := s.do(t, req, nil); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site POST /auth/login = %d, want 403", rec.Code)
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestPages(t *testing.T) {
	s := newTestServer(t)
	session := s.login(t)
	body := func(t *testing.T, path string) string {
		t.Helper()
		rec := s.get(t, path, session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		return rec.Body.String()
	}
	idOf := func(t *testing.T, query, arg string) string {
		t.Helper()
		var id int64
		if err := s.db.QueryRowContext(t.Context(), query, arg).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return strconv.FormatInt(id, 10)
	}
	count := func(t *testing.T, query, arg string) int {
		t.Helper()
		var n int
		if err := s.db.QueryRowContext(t.Context(), query, arg).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	pageID := func(t *testing.T, title string) string {
		t.Helper()
		return idOf(t, `SELECT id FROM pages WHERE title = $1`, title)
	}
	send := func(t *testing.T, path string, fields url.Values, files ...[2]string) *httptest.ResponseRecorder {
		t.Helper()
		body, contentType := multipartBody(t, fields, files...)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, body)
		req.Header.Set("Content-Type", contentType)
		return s.do(t, req, session)
	}
	save := func(t *testing.T, id, title, parent, text string) *httptest.ResponseRecorder {
		t.Helper()
		return send(t, "/page/pages/"+id+"/edit", url.Values{"title": {title}, "parent": {parent}, "body": {text}})
	}
	create := func(t *testing.T, title string) string {
		t.Helper()
		rec := s.post(t, "/page/pages/new", url.Values{"title": {title}}, session)
		id := pageID(t, strings.TrimSpace(title))
		wantRedirect(t, rec, "/page/pages/"+id)
		return id
	}

	t.Run("the pages need a session", func(t *testing.T) {
		wantRedirect(t, s.get(t, "/page", nil), "/auth/login")
	})

	t.Run("a new page is listed and a blank title is rejected", func(t *testing.T) {
		create(t, " 기술 정리 ")
		if page := body(t, "/page"); !strings.Contains(page, ">기술 정리</a>") {
			t.Errorf("Pages does not list the new page:\n%s", page)
		}
		if rec := s.post(t, "/page/pages/new", url.Values{"title": {"  "}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("a blank title = %d, want 422", rec.Code)
		}
	})

	t.Run("[[Title]] makes one subpage and code keeps it as text", func(t *testing.T) {
		tech := pageID(t, "기술 정리")
		text := "See [[SDN]] and [[SDN]].\n\n```\n[[ -f x ]]\n```"
		wantRedirect(t, save(t, tech, "기술 정리", "", text), "/page/pages/"+tech)
		wantRedirect(t, save(t, tech, "기술 정리", "", text), "/page/pages/"+tech)
		if n := count(t, `SELECT count(*) FROM pages WHERE title = $1`, "SDN"); n != 1 {
			t.Errorf("pages titled SDN after saving [[SDN]] twice = %d, want 1", n)
		}
		page := body(t, "/page/pages/"+tech)
		if !strings.Contains(page, `<a href="/page/pages/`+pageID(t, "SDN")+`">SDN</a>`) || !strings.Contains(page, "[[ -f x ]]") {
			t.Errorf("page does not link its subpage or lost the code:\n%s", page)
		}
	})

	t.Run("a page lists the pages that link to it", func(t *testing.T) {
		sdn := pageID(t, "SDN")
		rec := s.post(t, "/task/projects/new", url.Values{"name": {"OS Dev"}}, session)
		osdev := idOf(t, `SELECT project_id FROM pages WHERE title = $1 AND project_id IS NOT NULL`, "OS Dev")
		wantRedirect(t, rec, "/task/projects/"+osdev)
		notes := create(t, "Notes")
		text := "[SDN](/page/pages/" + sdn + ") and http://localhost:8080/task/projects/" + osdev + " and [me](/page/pages/" + notes + ")"
		wantRedirect(t, save(t, notes, "Notes", "", text), "/page/pages/"+notes)
		link := `<a href="/page/pages/` + notes + `">Notes</a>`
		if page := body(t, "/page/pages/"+sdn); !strings.Contains(page, link) {
			t.Errorf("SDN does not list Notes as linking to it:\n%s", page)
		}
		if page := body(t, "/task/projects/"+osdev); !strings.Contains(page, link) {
			t.Errorf("the project does not list Notes as linking to it:\n%s", page)
		}
	})

	t.Run("the list shows the pages of projects and tasks and kind narrows it", func(t *testing.T) {
		wantRedirect(t, s.post(t, "/task/tasks/new", url.Values{"title": {"Renew visa"}, "next": {"/task/tasks"}}, session), "/task/tasks")
		if page := body(t, "/page"); !strings.Contains(page, ">OS Dev</a>") || !strings.Contains(page, ">Renew visa</a>") {
			t.Errorf("Pages does not list the project and task pages:\n%s", page)
		}
		if page := body(t, "/page?kind=project"); !strings.Contains(page, ">OS Dev</a>") || strings.Contains(page, ">Renew visa</a>") {
			t.Errorf("kind=project does not show only project pages:\n%s", page)
		}
		if rec := s.get(t, "/page?kind=someday", session); rec.Code != http.StatusBadRequest {
			t.Errorf("GET with an unknown kind = %d, want 400", rec.Code)
		}
	})

	t.Run("a page moves under another page but not under its own subpage", func(t *testing.T) {
		tech, notes := pageID(t, "기술 정리"), pageID(t, "Notes")
		wantRedirect(t, save(t, notes, "Notes", tech, ""), "/page/pages/"+notes)
		if page := body(t, "/page/pages/"+notes); !strings.Contains(page, `<a href="/page/pages/`+tech+`">기술 정리</a>`) {
			t.Errorf("the moved page's path does not go through its new parent:\n%s", page)
		}
		if page := body(t, "/page"); strings.Contains(page, ">Notes</a>") {
			t.Errorf("Pages still lists the moved page at the top:\n%s", page)
		}
		if rec := save(t, tech, "기술 정리", notes, ""); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("moving a page under its own subpage = %d, want 422", rec.Code)
		}
	})

	t.Run("a deleted page hides the pages under it until it is restored", func(t *testing.T) {
		tech, sdn, notes := pageID(t, "기술 정리"), pageID(t, "SDN"), pageID(t, "Notes")
		wantRedirect(t, s.post(t, "/page/pages/"+sdn+"/delete", nil, session), "/page/pages/"+tech)
		wantRedirect(t, s.post(t, "/page/pages/"+tech+"/delete", nil, session), "/page")
		if rec := s.get(t, "/page/pages/"+notes, session); rec.Code != http.StatusNotFound {
			t.Errorf("GET a page under a deleted page = %d, want 404", rec.Code)
		}
		if page := body(t, "/page?q=Notes"); strings.Contains(page, ">Notes</a>") {
			t.Errorf("search finds a page under a deleted page:\n%s", page)
		}
		if rec := s.post(t, "/page/trash/"+sdn+"/restore", nil, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("restoring a page whose parent is in the trash = %d, want 422", rec.Code)
		}
		wantRedirect(t, s.post(t, "/page/trash/"+tech+"/restore", nil, session), "/page/trash")
		wantRedirect(t, s.post(t, "/page/trash/"+sdn+"/restore", nil, session), "/page/trash")
		body(t, "/page/pages/"+notes)
		body(t, "/page/pages/"+sdn)
	})

	t.Run("deleting for good sends the attachments to the drive trash", func(t *testing.T) {
		scratch := create(t, "Scratch")
		wantRedirect(t, send(t, "/page/pages/"+scratch+"/files", nil, [2]string{"note.txt", "hi"}), "/page/pages/"+scratch+"/edit")
		folder := idOf(t, `SELECT folder_id FROM pages WHERE id = $1`, scratch)
		var path string
		err := s.db.QueryRowContext(t.Context(), `SELECT p.name || '/' || f.name FROM folders f JOIN folders p ON p.id = f.parent_id WHERE f.id = $1`, folder).Scan(&path)
		if want := "Pages/Scratch (#" + scratch + ")"; err != nil || path != want {
			t.Errorf("attachment folder = %q, %v, want %q", path, err, want)
		}
		wantRedirect(t, s.post(t, "/page/pages/"+scratch+"/delete", nil, session), "/page")
		wantRedirect(t, s.post(t, "/page/trash/"+scratch+"/delete", nil, session), "/page/trash")
		if n := count(t, `SELECT count(*) FROM folders WHERE id = $1 AND trashed_at IS NOT NULL`, folder); n != 1 {
			t.Errorf("the folder of a page deleted for good is not in the drive trash")
		}
		if n := count(t, `SELECT count(*) FROM pages WHERE id = $1`, scratch); n != 0 {
			t.Errorf("pages with the id of a page deleted for good = %d, want 0", n)
		}
	})

	t.Run("search finds a Korean part of a body", func(t *testing.T) {
		cook := create(t, "요리")
		wantRedirect(t, save(t, cook, "요리", "", "된장찌개 끓이는 법"), "/page/pages/"+cook)
		if page := body(t, "/page?q="+url.QueryEscape("찌개")); !strings.Contains(page, ">요리</a>") {
			t.Errorf("search for 찌개 does not find the page:\n%s", page)
		}
	})
}

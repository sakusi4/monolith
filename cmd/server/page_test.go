package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/page"
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
	version := func(t *testing.T, id string) string {
		t.Helper()
		var at time.Time
		if err := s.db.QueryRowContext(t.Context(), `SELECT updated_at FROM pages WHERE id = $1`, id).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return page.FormatVersion(at)
	}
	saveAt := func(t *testing.T, id, title, text, at string) *httptest.ResponseRecorder {
		t.Helper()
		return s.post(t, "/page/pages/"+id+"/content", url.Values{"title": {title}, "body": {text}, "version": {at}}, session)
	}
	save := func(t *testing.T, id, title, text string) *httptest.ResponseRecorder {
		t.Helper()
		return saveAt(t, id, title, text, version(t, id))
	}
	upload := func(t *testing.T, path string, files ...[2]string) *httptest.ResponseRecorder {
		t.Helper()
		body, contentType := multipartBody(t, nil, files...)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, body)
		req.Header.Set("Content-Type", contentType)
		return s.do(t, req, session)
	}
	create := func(t *testing.T, title, parent string) string {
		t.Helper()
		rec := s.post(t, "/page/pages/new", url.Values{"title": {title}, "parent": {parent}}, session)
		id := pageID(t, strings.TrimSpace(title))
		wantRedirect(t, rec, "/page/pages/"+id)
		return id
	}

	t.Run("the pages need a session", func(t *testing.T) {
		wantRedirect(t, s.get(t, "/page", nil), "/auth/login")
	})

	t.Run("a new page is listed at the top and a blank title is rejected", func(t *testing.T) {
		tech := create(t, " 기술 정리 ", "")
		link := `<a href="/page/pages/` + tech + `">기술 정리</a>`
		if page := body(t, "/page"); !strings.Contains(page, link) {
			t.Errorf("Pages does not list the new page:\n%s", page)
		}
		if page := body(t, "/task/tasks"); !strings.Contains(page, link) {
			t.Errorf("the sidebar of another screen does not list the new page:\n%s", page)
		}
		if rec := s.post(t, "/page/pages/new", url.Values{"title": {"  "}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("a blank title = %d, want 422", rec.Code)
		}
	})

	t.Run("a page autosaves and a stale tab changes nothing", func(t *testing.T) {
		tech := pageID(t, "기술 정리")
		old := version(t, tech)
		rec := save(t, tech, "기술 정리", "네트워크 정리")
		if rec.Code != http.StatusNoContent || rec.Header().Get("Page-Version") == old {
			t.Fatalf("autosave = %d with version %q, want 204 with a new version", rec.Code, rec.Header().Get("Page-Version"))
		}
		if rec := saveAt(t, tech, "기술 정리", "옛 탭의 내용", old); rec.Code != http.StatusConflict {
			t.Errorf("autosave with an old version = %d, want 409", rec.Code)
		}
		if page := body(t, "/page/pages/"+tech); !strings.Contains(page, "네트워크 정리") || strings.Contains(page, "옛 탭의 내용") {
			t.Errorf("page after a stale save does not keep the newer body:\n%s", page)
		}
		if rec := save(t, tech, "  ", "네트워크 정리"); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("autosave with a blank title = %d, want 422", rec.Code)
		}
		if rec := save(t, tech, "기술 정리", "네트워크\r\n정리"); rec.Code != http.StatusNoContent {
			t.Fatalf("autosave with CRLF = %d, want 204", rec.Code)
		}
		var stored string
		if err := s.db.QueryRowContext(t.Context(), `SELECT body FROM pages WHERE id = $1`, tech).Scan(&stored); err != nil || stored != "네트워크\n정리" {
			t.Errorf("body saved from a form = %q, %v, want its line breaks as \\n", stored, err)
		}
	})

	t.Run("a subpage made from the editor sits under its parent", func(t *testing.T) {
		tech := pageID(t, "기술 정리")
		sdn := create(t, "SDN", tech)
		if page := body(t, "/page/pages/"+sdn); !strings.Contains(page, `<a href="/page/pages/`+tech+`">기술 정리</a>`) {
			t.Errorf("the subpage's path does not go through its parent:\n%s", page)
		}
		if rec := s.post(t, "/page/pages/new", url.Values{"title": {"Lost"}, "parent": {"999999"}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("a subpage of a missing page = %d, want 422", rec.Code)
		}
	})

	t.Run("the picker finds pages and projects", func(t *testing.T) {
		rec := s.post(t, "/task/projects/new", url.Values{"name": {"OS Dev"}}, session)
		osdev := idOf(t, `SELECT project_id FROM pages WHERE title = $1 AND project_id IS NOT NULL`, "OS Dev")
		wantRedirect(t, rec, "/task/projects/"+osdev)
		if links := body(t, "/page/links?q=SD"); !strings.Contains(links, `href="/page/pages/`+pageID(t, "SDN")+`"`) {
			t.Errorf("the picker does not find SDN:\n%s", links)
		}
		if links := body(t, "/page/links?q=OS"); !strings.Contains(links, `href="/task/projects/`+osdev+`"`) {
			t.Errorf("the picker does not find the project page:\n%s", links)
		}
	})

	t.Run("a page lists the pages that link to it", func(t *testing.T) {
		sdn := pageID(t, "SDN")
		osdev := idOf(t, `SELECT project_id FROM pages WHERE title = $1 AND project_id IS NOT NULL`, "OS Dev")
		notes := create(t, "Notes", "")
		text := "[SDN](/page/pages/" + sdn + ") and http://localhost:8080/task/projects/" + osdev + " and [me](/page/pages/" + notes + ")"
		if rec := save(t, notes, "Notes", text); rec.Code != http.StatusNoContent {
			t.Fatalf("autosave with links = %d, want 204", rec.Code)
		}
		link := `<a href="/page/pages/` + notes + `">Notes</a>`
		if page := body(t, "/page/pages/"+sdn); !strings.Contains(page, link) {
			t.Errorf("SDN does not list Notes as linking to it:\n%s", page)
		}
		if page := body(t, "/task/projects/"+osdev); !strings.Contains(page, link) {
			t.Errorf("the project does not list Notes as linking to it:\n%s", page)
		}
	})

	t.Run("Pages lists only the pages of their own, and search finds the rest", func(t *testing.T) {
		wantRedirect(t, s.post(t, "/task/tasks/new", url.Values{"title": {"Renew visa"}, "next": {"/task/tasks"}}, session), "/task/tasks")
		if page := body(t, "/page"); strings.Contains(page, ">OS Dev</a>") || strings.Contains(page, ">Renew visa</a>") {
			t.Errorf("Pages lists the pages of projects or tasks:\n%s", page)
		}
		if page := body(t, "/page/search?q=OS"); !strings.Contains(page, ">OS Dev</a>") {
			t.Errorf("search does not find the project page:\n%s", page)
		}
	})

	t.Run("a new page without a title is Untitled", func(t *testing.T) {
		rec := s.post(t, "/page/pages/new", url.Values{}, session)
		wantRedirect(t, rec, "/page/pages/"+pageID(t, "Untitled"))
	})

	t.Run("a page moves under another page but not under its own subpage", func(t *testing.T) {
		tech, notes := pageID(t, "기술 정리"), pageID(t, "Notes")
		wantRedirect(t, s.post(t, "/page/pages/"+notes+"/move", url.Values{"parent": {tech}}, session), "/page/pages/"+notes)
		if page := body(t, "/page/pages/"+notes); !strings.Contains(page, `<a href="/page/pages/`+tech+`">기술 정리</a>`) {
			t.Errorf("the moved page's path does not go through its new parent:\n%s", page)
		}
		if rec := s.post(t, "/page/pages/"+tech+"/move", url.Values{"parent": {notes}}, session); rec.Code != http.StatusUnprocessableEntity {
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
		if page := body(t, "/page/search?q=Notes"); strings.Contains(page, ">Notes</a>") {
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

	t.Run("a page moved to the top is listed again", func(t *testing.T) {
		notes := pageID(t, "Notes")
		wantRedirect(t, s.post(t, "/page/pages/"+notes+"/move", url.Values{"parent": {""}}, session), "/page/pages/"+notes)
		if page := body(t, "/page"); !strings.Contains(page, ">Notes</a>") {
			t.Errorf("Pages does not list the page moved back to the top:\n%s", page)
		}
	})

	t.Run("pasted images go to the page's folder, which deleting for good trashes", func(t *testing.T) {
		scratch := create(t, "Scratch", "")
		rec := upload(t, "/page/pages/"+scratch+"/images", [2]string{"pasted-1.png", "png"})
		shot := idOf(t, `SELECT id FROM files WHERE name = $1`, "pasted-1.png")
		if rec.Code != http.StatusCreated || rec.Header().Get("Location") != "/drive/files/"+shot {
			t.Errorf("image upload = %d to %q, want 201 to /drive/files/%s", rec.Code, rec.Header().Get("Location"), shot)
		}
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
	})

	t.Run("search finds a Korean part of a body", func(t *testing.T) {
		cook := create(t, "요리", "")
		if rec := save(t, cook, "요리", "된장찌개 끓이는 법"); rec.Code != http.StatusNoContent {
			t.Fatalf("autosave = %d, want 204", rec.Code)
		}
		if page := body(t, "/page/search?q="+url.QueryEscape("찌개")); !strings.Contains(page, ">요리</a>") {
			t.Errorf("search for 찌개 does not find the page:\n%s", page)
		}
	})
}

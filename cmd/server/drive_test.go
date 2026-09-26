package main

import (
	"bytes"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDrive(t *testing.T) {
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
	idOf := func(t *testing.T, query, name string) string {
		t.Helper()
		var id int64
		if err := s.db.QueryRowContext(t.Context(), query, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return strconv.FormatInt(id, 10)
	}
	folderID := func(t *testing.T, name string) string {
		t.Helper()
		return idOf(t, `SELECT id FROM folders WHERE name = $1`, name)
	}
	fileID := func(t *testing.T, name string) string {
		t.Helper()
		return idOf(t, `SELECT id FROM files WHERE name = $1`, name)
	}
	multipartBody := func(t *testing.T, files ...[2]string) (*bytes.Buffer, string) {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for _, f := range files {
			part, err := mw.CreateFormFile("files", f[0])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte(f[1])); err != nil {
				t.Fatal(err)
			}
		}
		if err := mw.Close(); err != nil {
			t.Fatal(err)
		}
		return &buf, mw.FormDataContentType()
	}
	upload := func(t *testing.T, folder string, files ...[2]string) *httptest.ResponseRecorder {
		t.Helper()
		body, contentType := multipartBody(t, files...)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/drive/files/new?folder="+folder, body)
		req.Header.Set("Content-Type", contentType)
		return s.do(t, req, session)
	}
	blobCount := func(t *testing.T) int {
		t.Helper()
		n := 0
		err := filepath.WalkDir(filepath.Join(s.filesDir, "blobs"), func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				n++
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("the drive needs a session", func(t *testing.T) {
		wantRedirect(t, s.get(t, "/drive", nil), "/auth/login")
	})

	t.Run("nested folders show their path and files download byte for byte", func(t *testing.T) {
		wantRedirect(t, s.post(t, "/drive/folders/new", url.Values{"name": {"Projects"}}, session), "/drive")
		projects := folderID(t, "Projects")
		wantRedirect(t, s.post(t, "/drive/folders/new?folder="+projects, url.Values{"name": {" tunnel "}}, session), "/drive/folders/"+projects)
		tunnel := folderID(t, "tunnel")
		wantRedirect(t, upload(t, tunnel, [2]string{"plan.md", "hello world"}, [2]string{"기획안.md", "# plan"}, [2]string{"empty.txt", ""}), "/drive/folders/"+tunnel)
		page := body(t, "/drive/folders/"+tunnel)
		for _, want := range []string{`<a href="/drive/folders/` + projects + `">Projects</a>`, "plan.md", "empty.txt"} {
			if !strings.Contains(page, want) {
				t.Errorf("GET tunnel does not contain %q:\n%s", want, page)
			}
		}
		plan := fileID(t, "plan.md")
		rec := s.get(t, "/drive/files/"+plan+"/download", session)
		if rec.Body.String() != "hello world" || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
			t.Errorf("download = %q with %q, want the content as an attachment", rec.Body.String(), rec.Header().Get("Content-Disposition"))
		}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/drive/files/"+plan+"/content", nil)
		req.Header.Set("Range", "bytes=0-4")
		if rec := s.do(t, req, session); rec.Code != http.StatusPartialContent || rec.Body.String() != "hello" {
			t.Errorf("range request = %d %q, want 206 %q", rec.Code, rec.Body.String(), "hello")
		}
		korean := s.get(t, "/drive/files/"+fileID(t, "기획안.md")+"/download", session)
		if !strings.Contains(korean.Header().Get("Content-Disposition"), "filename*=utf-8''") {
			t.Errorf("Content-Disposition = %q, want an RFC 2231 file name", korean.Header().Get("Content-Disposition"))
		}
		if rec := s.get(t, "/drive/files/"+fileID(t, "empty.txt")+"/download", session); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
			t.Errorf("empty file download = %d with %d bytes, want 200 with none", rec.Code, rec.Body.Len())
		}
	})

	t.Run("the same content is stored once", func(t *testing.T) {
		before := blobCount(t)
		tunnel := folderID(t, "tunnel")
		wantRedirect(t, upload(t, tunnel, [2]string{"copy.md", "hello world"}), "/drive/folders/"+tunnel)
		if n := blobCount(t); n != before {
			t.Errorf("blobs on disk = %d, want %d", n, before)
		}
	})

	t.Run("names are unique in a folder and a failed upload saves nothing", func(t *testing.T) {
		before := blobCount(t)
		tunnel := folderID(t, "tunnel")
		if rec := upload(t, tunnel, [2]string{"new.txt", "new"}, [2]string{"plan.md", "again"}); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("upload over plan.md = %d, want 422", rec.Code)
		}
		var n int
		if err := s.db.QueryRowContext(t.Context(), `SELECT count(*) FROM files WHERE name = 'new.txt'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 || blobCount(t) != before {
			t.Errorf("after a failed upload: %d new.txt rows, %d blobs, want 0 and %d", n, blobCount(t), before)
		}
		projects := folderID(t, "Projects")
		if rec := s.post(t, "/drive/folders/new?folder="+projects, url.Values{"name": {"tunnel"}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("second tunnel folder = %d, want 422", rec.Code)
		}
	})

	t.Run("an upload outlives the server's write timeout", func(t *testing.T) {
		srv := httptest.NewUnstartedServer(s.handler)
		srv.Config.WriteTimeout = time.Nanosecond
		srv.Start()
		t.Cleanup(srv.Close)
		body, contentType := multipartBody(t, [2]string{"slow.txt", "slow"})
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/drive/files/new", body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", contentType)
		req.AddCookie(session)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("upload past the write timeout: %v, want a 303 response", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("upload past the write timeout = %d, want 303", resp.StatusCode)
		}
	})

	t.Run("files and folders rename and move, but a folder not into itself", func(t *testing.T) {
		projects, tunnel, plan := folderID(t, "Projects"), folderID(t, "tunnel"), fileID(t, "plan.md")
		if page := body(t, "/drive/files/"+plan+"/edit"); !strings.Contains(page, `value="plan.md"`) {
			t.Errorf("edit row does not hold the file name:\n%s", page)
		}
		wantRedirect(t, s.post(t, "/drive/files/"+plan+"/edit", url.Values{"name": {"spec.md"}, "folder": {projects}}, session), "/drive/folders/"+projects)
		if page := body(t, "/drive/folders/"+projects); !strings.Contains(page, "spec.md") {
			t.Errorf("Projects does not list the moved spec.md:\n%s", page)
		}
		if rec := s.post(t, "/drive/folders/"+projects+"/edit", url.Values{"name": {"Projects"}, "folder": {tunnel}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("moving Projects into tunnel = %d, want 422", rec.Code)
		}
	})

	t.Run("a trashed folder hides its files until restored", func(t *testing.T) {
		projects, tunnel, copyID := folderID(t, "Projects"), folderID(t, "tunnel"), fileID(t, "copy.md")
		wantRedirect(t, s.post(t, "/drive/folders/"+tunnel+"/delete", nil, session), "/drive/folders/"+projects)
		if rec := s.get(t, "/drive/files/"+copyID, session); rec.Code != http.StatusNotFound {
			t.Errorf("file in a trashed folder = %d, want 404", rec.Code)
		}
		if page := body(t, "/drive/trash"); !strings.Contains(page, "tunnel") {
			t.Errorf("trash does not list tunnel:\n%s", page)
		}
		wantRedirect(t, s.post(t, "/drive/trash/folders/"+tunnel+"/restore", nil, session), "/drive/trash")
		if rec := s.get(t, "/drive/files/"+copyID, session); rec.Code != http.StatusOK {
			t.Errorf("file after restoring its folder = %d, want 200", rec.Code)
		}
	})

	t.Run("nothing moves into or is restored under a folder in the trash", func(t *testing.T) {
		projects, tunnel := folderID(t, "Projects"), folderID(t, "tunnel")
		note := fileID(t, "empty.txt")
		wantRedirect(t, s.post(t, "/drive/files/"+note+"/delete", nil, session), "/drive/folders/"+tunnel)
		wantRedirect(t, s.post(t, "/drive/folders/"+tunnel+"/delete", nil, session), "/drive/folders/"+projects)
		if rec := s.post(t, "/drive/trash/files/"+note+"/restore", nil, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("restoring a file whose folder is in the trash = %d, want 422", rec.Code)
		}
		if rec := s.post(t, "/drive/folders/"+projects+"/edit", url.Values{"name": {"Projects"}, "folder": {tunnel}}, session); rec.Code != http.StatusNotFound {
			t.Errorf("moving into a folder in the trash = %d, want 404", rec.Code)
		}
		wantRedirect(t, s.post(t, "/drive/trash/folders/"+tunnel+"/restore", nil, session), "/drive/trash")
		wantRedirect(t, s.post(t, "/drive/trash/files/"+note+"/restore", nil, session), "/drive/trash")
	})

	t.Run("deleting forever keeps content that another file still uses", func(t *testing.T) {
		projects, tunnel := folderID(t, "Projects"), folderID(t, "tunnel")
		copyID, spec := fileID(t, "copy.md"), fileID(t, "spec.md")
		before := blobCount(t)
		wantRedirect(t, s.post(t, "/drive/files/"+copyID+"/delete", nil, session), "/drive/folders/"+tunnel)
		wantRedirect(t, s.post(t, "/drive/trash/files/"+copyID+"/delete", nil, session), "/drive/trash")
		if n := blobCount(t); n != before {
			t.Errorf("blobs after deleting copy.md = %d, want %d while spec.md shares its content", n, before)
		}
		wantRedirect(t, s.post(t, "/drive/files/"+spec+"/delete", nil, session), "/drive/folders/"+projects)
		wantRedirect(t, s.post(t, "/drive/trash/empty", nil, session), "/drive/trash")
		if n := blobCount(t); n != before-1 {
			t.Errorf("blobs after emptying the trash = %d, want %d", n, before-1)
		}
	})

	t.Run("HTML is served as a download", func(t *testing.T) {
		wantRedirect(t, upload(t, "", [2]string{"page.html", "<script>alert(1)</script>"}), "/drive")
		rec := s.get(t, "/drive/files/"+fileID(t, "page.html")+"/content", session)
		if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
			t.Errorf("Content-Disposition = %q, want attachment", rec.Header().Get("Content-Disposition"))
		}
	})
}

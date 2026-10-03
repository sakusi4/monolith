package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestTasks(t *testing.T) {
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
	taskID := func(t *testing.T, title string) string {
		t.Helper()
		return idOf(t, `SELECT task_id FROM pages WHERE title = $1 AND task_id IS NOT NULL`, title)
	}
	projectID := func(t *testing.T, name string) string {
		t.Helper()
		return idOf(t, `SELECT project_id FROM pages WHERE title = $1 AND project_id IS NOT NULL`, name)
	}
	addTask := func(t *testing.T, title, project, due string) {
		t.Helper()
		form := url.Values{"title": {title}, "project": {project}, "due": {due}, "next": {"/task/tasks"}}
		wantRedirect(t, s.post(t, "/task/tasks/new", form, session), "/task/tasks")
	}
	inOrder := func(t *testing.T, page string, titles ...string) {
		t.Helper()
		last := -1
		for _, title := range titles {
			i := strings.Index(page, ">"+title+"</a>")
			if i <= last {
				t.Errorf("%q is missing or out of order in %v:\n%s", title, titles, page)
				return
			}
			last = i
		}
	}

	send := func(t *testing.T, path string, fields url.Values, files ...[2]string) *httptest.ResponseRecorder {
		t.Helper()
		body, contentType := multipartBody(t, fields, files...)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, body)
		req.Header.Set("Content-Type", contentType)
		return s.do(t, req, session)
	}
	folderOf := func(t *testing.T, task string) (string, []string) {
		t.Helper()
		id := idOf(t, `SELECT folder_id FROM pages WHERE task_id = $1`, task)
		rows, err := s.db.QueryContext(t.Context(), `
			WITH RECURSIVE chain AS (
				SELECT id, parent_id, name, 0 AS depth FROM folders WHERE id = $1
				UNION ALL
				SELECT f.id, f.parent_id, f.name, c.depth + 1 FROM folders f JOIN chain c ON f.id = c.parent_id
			)
			SELECT name FROM chain ORDER BY depth DESC`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var path []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			path = append(path, name)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return id, path
	}
	trashed := func(t *testing.T, folder string) bool {
		t.Helper()
		var yes bool
		if err := s.db.QueryRowContext(t.Context(), `SELECT trashed_at IS NOT NULL FROM folders WHERE id = $1`, folder).Scan(&yes); err != nil {
			t.Fatal(err)
		}
		return yes
	}

	t.Run("the task pages need a session", func(t *testing.T) {
		wantRedirect(t, s.get(t, "/task/tasks", nil), "/auth/login")
	})

	if _, err := s.db.ExecContext(t.Context(), `
		WITH p AS (INSERT INTO projects (status) VALUES ('active') RETURNING id)
		INSERT INTO pages (project_id, title) SELECT id, 'tunnel' FROM p`); err != nil {
		t.Fatal(err)
	}

	t.Run("open tasks are listed by due date and the filters narrow them", func(t *testing.T) {
		tunnel := projectID(t, "tunnel")
		addTask(t, "Visa", "", "2026-10-10")
		addTask(t, "Architecture", tunnel, "2026-10-01")
		addTask(t, "Laundry", "", "")
		inOrder(t, body(t, "/task/tasks"), "Architecture", "Visa", "Laundry")
		inbox := body(t, "/task/tasks?project=inbox")
		if !strings.Contains(inbox, ">Visa</a>") || strings.Contains(inbox, ">Architecture</a>") {
			t.Errorf("inbox filter does not show only tasks without a project:\n%s", inbox)
		}
		if page := body(t, "/task/tasks?project="+tunnel); strings.Contains(page, ">Visa</a>") || !strings.Contains(page, ">Architecture</a>") {
			t.Errorf("project filter does not show only tunnel's tasks:\n%s", page)
		}
	})

	t.Run("a task saved as done leaves the open list and records when", func(t *testing.T) {
		visa := taskID(t, "Visa")
		done := url.Values{"title": {"Visa"}, "status": {"done"}, "due": {"2026-10-10"}, "body": {""}}
		wantRedirect(t, send(t, "/task/tasks/"+visa+"/edit", done), "/task/tasks/"+visa)
		if page := body(t, "/task/tasks"); strings.Contains(page, ">Visa</a>") {
			t.Errorf("open list still shows the done task:\n%s", page)
		}
		if page := body(t, "/task/tasks?status=done"); !strings.Contains(page, ">Visa</a>") {
			t.Errorf("done list does not show the done task:\n%s", page)
		}
		completed := func() bool {
			t.Helper()
			var done bool
			if err := s.db.QueryRowContext(t.Context(), `SELECT completed_at IS NOT NULL FROM tasks WHERE id = $1`, visa).Scan(&done); err != nil {
				t.Fatal(err)
			}
			return done
		}
		if !completed() {
			t.Errorf("completed_at is not set after done")
		}
		if _, err := s.db.ExecContext(t.Context(), `UPDATE tasks SET completed_at = '2020-01-01' WHERE id = $1`, visa); err != nil {
			t.Fatal(err)
		}
		again := url.Values{"title": {"Visa"}, "status": {"done"}, "due": {"2026-10-10"}, "body": {""}}
		wantRedirect(t, send(t, "/task/tasks/"+visa+"/edit", again), "/task/tasks/"+visa)
		var year int
		if err := s.db.QueryRowContext(t.Context(), `SELECT extract(year FROM completed_at) FROM tasks WHERE id = $1`, visa).Scan(&year); err != nil || year != 2020 {
			t.Errorf("completed_at year after saving done again = %d, %v, want the first completion kept (2020)", year, err)
		}
		reopen := url.Values{"title": {"Visa"}, "status": {"todo"}, "due": {"2026-10-10"}, "body": {""}}
		wantRedirect(t, send(t, "/task/tasks/"+visa+"/edit", reopen), "/task/tasks/"+visa)
		if completed() {
			t.Errorf("completed_at is still set after reopening")
		}
	})

	t.Run("a project gets a drive folder, and renaming it renames the folder", func(t *testing.T) {
		rec := s.post(t, "/task/projects/new", url.Values{"name": {"Heriot Watt"}}, session)
		heriot := projectID(t, "Heriot Watt")
		wantRedirect(t, rec, "/task/projects/"+heriot)
		var parent string
		if err := s.db.QueryRowContext(t.Context(), `SELECT p.name FROM folders f JOIN folders p ON p.id = f.parent_id WHERE f.name = 'Heriot Watt'`).Scan(&parent); err != nil || parent != "Projects" {
			t.Errorf("folder Heriot Watt is under %q, %v, want Projects", parent, err)
		}
		if page := body(t, "/task/projects/"+heriot); !strings.Contains(page, "Open in Drive") {
			t.Errorf("project page does not link its folder:\n%s", page)
		}
		if rec := s.post(t, "/task/projects/new", url.Values{"name": {"Heriot Watt"}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("second Heriot Watt project = %d, want 422", rec.Code)
		}
		edit := url.Values{"name": {"Heriot-Watt MSc"}, "status": {"active"}, "started": {"2026-09-01"}, "finished": {""}, "body": {""}}
		wantRedirect(t, send(t, "/task/projects/"+heriot+"/edit", edit), "/task/projects/"+heriot)
		var folder string
		if err := s.db.QueryRowContext(t.Context(), `SELECT f.name FROM folders f JOIN pages pg ON pg.folder_id = f.id WHERE pg.project_id = $1`, heriot).Scan(&folder); err != nil || folder != "Heriot-Watt MSc" {
			t.Errorf("project folder = %q, %v, want Heriot-Watt MSc", folder, err)
		}
	})

	t.Run("a folder name taken in Projects leaves no project", func(t *testing.T) {
		if rec := s.post(t, "/drive/folders/new?folder="+idOf(t, `SELECT id FROM folders WHERE name = $1`, "Projects"), url.Values{"name": {"Blog"}}, session); rec.Code != http.StatusSeeOther {
			t.Fatalf("create folder Blog = %d, want 303", rec.Code)
		}
		if rec := s.post(t, "/task/projects/new", url.Values{"name": {"Blog"}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("project over an existing folder = %d, want 422", rec.Code)
		}
		var n int
		if err := s.db.QueryRowContext(t.Context(), `SELECT count(*) FROM pages WHERE title = 'Blog' AND project_id IS NOT NULL`).Scan(&n); err != nil || n != 0 {
			t.Errorf("Blog projects = %d, %v, want 0", n, err)
		}
	})

	t.Run("a task's body renders as markdown", func(t *testing.T) {
		heriot := projectID(t, "Heriot-Watt MSc")
		addTask(t, "Enrol", heriot, "")
		enrol := taskID(t, "Enrol")
		edit := url.Values{"title": {"Enrol"}, "project": {heriot}, "status": {"todo"}, "due": {""}, "body": {"# Steps\n- [x] passport"}}
		wantRedirect(t, send(t, "/task/tasks/"+enrol+"/edit", edit), "/task/tasks/"+enrol)
		if page := body(t, "/task/tasks/"+enrol); !strings.Contains(page, "<h2>Steps</h2>") || !strings.Contains(page, `type="checkbox"`) {
			t.Errorf("task page does not render the body:\n%s", page)
		}
	})

	t.Run("attachments go to the task's own folder", func(t *testing.T) {
		enrol, laundry, architecture := taskID(t, "Enrol"), taskID(t, "Laundry"), taskID(t, "Architecture")
		wantRedirect(t, send(t, "/task/tasks/"+enrol+"/files", nil, [2]string{"offer.pdf", "%PDF"}), "/task/tasks/"+enrol+"/edit")
		if _, path := folderOf(t, enrol); !slices.Equal(path, []string{"Projects", "Heriot-Watt MSc", "Tasks", "Enrol"}) {
			t.Errorf("task folder = %v, want Projects/Heriot-Watt MSc/Tasks/Enrol", path)
		}
		if page := body(t, "/task/tasks/"+enrol); !strings.Contains(page, "offer.pdf") {
			t.Errorf("task page does not list the attachment:\n%s", page)
		}
		offer := idOf(t, `SELECT id FROM files WHERE name = $1`, "offer.pdf")
		save := url.Values{"title": {"Laundry"}, "status": {"todo"}, "due": {""}, "body": {"Before ![shot.png](shot.png) after [offer.pdf](/drive/files/" + offer + "/content)"}}
		wantRedirect(t, send(t, "/task/tasks/"+laundry+"/edit", save, [2]string{"shot.png", "png"}, [2]string{"removed.png", "png"}), "/task/tasks/"+laundry)
		shot := idOf(t, `SELECT id FROM files WHERE name = $1`, "shot.png")
		if page := body(t, "/task/tasks/"+laundry); !strings.Contains(page, `src="/drive/files/`+shot+`/content"`) {
			t.Errorf("task page does not show the saved image from its own file:\n%s", page)
		}
		var n int
		if err := s.db.QueryRowContext(t.Context(), `SELECT count(*) FROM files WHERE name = 'removed.png'`).Scan(&n); err != nil || n != 0 {
			t.Errorf("files named removed.png = %d, %v, want 0 for an upload the body does not link", n, err)
		}
		if _, path := folderOf(t, laundry); !slices.Equal(path, []string{"Inbox", "Laundry"}) {
			t.Errorf("inbox task folder = %v, want Inbox/Laundry", path)
		}
		if rec := send(t, "/task/tasks/"+architecture+"/files", nil, [2]string{"a.txt", "a"}); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("upload to a task whose project has no folder = %d, want 422", rec.Code)
		}
	})

	t.Run("a saved body moves its own files it no longer links to the trash", func(t *testing.T) {
		laundry := taskID(t, "Laundry")
		shot := idOf(t, `SELECT id FROM files WHERE name = $1`, "shot.png")
		offer := idOf(t, `SELECT id FROM files WHERE name = $1`, "offer.pdf")
		save := url.Values{"title": {"Laundry"}, "status": {"todo"}, "due": {""}, "body": {"Now ![other.png](other.png)"}}
		wantRedirect(t, send(t, "/task/tasks/"+laundry+"/edit", save, [2]string{"other.png", "png"}), "/task/tasks/"+laundry)
		fileTrashed := func(id string) bool {
			t.Helper()
			var yes bool
			if err := s.db.QueryRowContext(t.Context(), `SELECT trashed_at IS NOT NULL FROM files WHERE id = $1`, id).Scan(&yes); err != nil {
				t.Fatal(err)
			}
			return yes
		}
		if !fileTrashed(shot) {
			t.Errorf("shot.png, dropped from the body, is not in the trash")
		}
		if fileTrashed(offer) {
			t.Errorf("offer.pdf, another task's file dropped from the body, is in the trash")
		}
	})

	t.Run("changing a task's project moves its folder along", func(t *testing.T) {
		heriot := projectID(t, "Heriot-Watt MSc")
		tunnel := projectID(t, "tunnel")
		move := func(t *testing.T, title, project string) *httptest.ResponseRecorder {
			t.Helper()
			id := taskID(t, title)
			var text string
			if err := s.db.QueryRowContext(t.Context(), `SELECT body FROM pages WHERE task_id = $1`, id).Scan(&text); err != nil {
				t.Fatal(err)
			}
			edit := url.Values{"title": {title}, "project": {project}, "status": {"todo"}, "due": {""}, "body": {text}}
			return send(t, "/task/tasks/"+id+"/edit", edit)
		}
		wantRedirect(t, move(t, "Laundry", heriot), "/task/tasks/"+taskID(t, "Laundry"))
		if _, path := folderOf(t, taskID(t, "Laundry")); !slices.Equal(path, []string{"Projects", "Heriot-Watt MSc", "Tasks", "Laundry"}) {
			t.Errorf("folder of a task moved from the inbox = %v, want Projects/Heriot-Watt MSc/Tasks/Laundry", path)
		}
		wantRedirect(t, move(t, "Visa", tunnel), "/task/tasks/"+taskID(t, "Visa"))
		if project := idOf(t, `SELECT t.project_id FROM tasks t JOIN pages pg ON pg.task_id = t.id WHERE pg.title = $1`, "Visa"); project != tunnel {
			t.Errorf("project of a task without a folder moved to tunnel = %s, want %s", project, tunnel)
		}
		if rec := move(t, "Enrol", tunnel); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("moving a task with a folder to a project without one = %d, want 422", rec.Code)
		}
		if project := idOf(t, `SELECT t.project_id FROM tasks t JOIN pages pg ON pg.task_id = t.id WHERE pg.title = $1`, "Enrol"); project != heriot {
			t.Errorf("project after a refused move = %s, want %s", project, heriot)
		}
	})

	t.Run("status, project, and due change right from the list", func(t *testing.T) {
		visa := taskID(t, "Visa")
		text := "if [[ -f x ]]; then echo; fi"
		if _, err := s.db.ExecContext(t.Context(), `UPDATE pages SET body = $2 WHERE task_id = $1`, visa, text); err != nil {
			t.Fatal(err)
		}
		fields := url.Values{"project": {""}, "status": {"in_progress"}, "due": {"2026-10-20"}, "next": {"/task/tasks?status=all"}}
		wantRedirect(t, s.post(t, "/task/tasks/"+visa+"/fields", fields, session), "/task/tasks?status=all")
		var saved string
		if err := s.db.QueryRowContext(t.Context(), `SELECT body FROM pages WHERE task_id = $1`, visa).Scan(&saved); err != nil || saved != text {
			t.Errorf("body after a change from the list = %q, %v, want it untouched: %q", saved, err, text)
		}
		var got string
		if err := s.db.QueryRowContext(t.Context(), `SELECT coalesce(project_id::text, 'inbox') || ' ' || status || ' ' || due_on::text FROM tasks WHERE id = $1`, visa).Scan(&got); err != nil || got != "inbox in_progress 2026-10-20" {
			t.Errorf("Visa after a change from the list = %q, %v, want inbox in_progress 2026-10-20", got, err)
		}
		tunnel := projectID(t, "tunnel")
		move := url.Values{"project": {tunnel}, "status": {"todo"}, "due": {""}, "next": {"/task/tasks"}}
		if rec := s.post(t, "/task/tasks/"+taskID(t, "Enrol")+"/fields", move, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("moving a task with a folder to a project without one from the list = %d, want 422", rec.Code)
		}
	})

	t.Run("[[Title]] in a project body makes a subpage under the project", func(t *testing.T) {
		heriot := projectID(t, "Heriot-Watt MSc")
		edit := url.Values{"name": {"Heriot-Watt MSc"}, "status": {"active"}, "started": {"2026-09-01"}, "finished": {""}, "body": {"Read [[Reading list]] first."}}
		wantRedirect(t, send(t, "/task/projects/"+heriot+"/edit", edit), "/task/projects/"+heriot)
		list := idOf(t, `SELECT id FROM pages WHERE title = $1`, "Reading list")
		if page := body(t, "/task/projects/"+heriot); !strings.Contains(page, `href="/page/pages/`+list+`"`) {
			t.Errorf("project page does not link its subpage:\n%s", page)
		}
		if page := body(t, "/page/pages/"+list); !strings.Contains(page, `<a href="/task/projects/`+heriot+`">Heriot-Watt MSc</a>`) {
			t.Errorf("the subpage's path does not lead to the project:\n%s", page)
		}
	})

	t.Run("deleting moves the pages to the page trash and keeps the folders", func(t *testing.T) {
		laundry := taskID(t, "Laundry")
		laundryFolder, _ := folderOf(t, laundry)
		laundryPage := idOf(t, `SELECT id FROM pages WHERE task_id = $1`, laundry)
		wantRedirect(t, s.post(t, "/task/tasks/"+laundry+"/delete", url.Values{"next": {"/task/tasks"}}, session), "/task/tasks")
		if trashed(t, laundryFolder) {
			t.Errorf("a deleted task's folder is in the drive trash")
		}
		heriot := projectID(t, "Heriot-Watt MSc")
		heriotPage := idOf(t, `SELECT id FROM pages WHERE project_id = $1`, heriot)
		enrolPage := idOf(t, `SELECT id FROM pages WHERE task_id = $1`, taskID(t, "Enrol"))
		wantRedirect(t, s.post(t, "/task/projects/"+heriot+"/delete", nil, session), "/task/projects")
		if rec := s.post(t, "/task/projects/new", url.Values{"name": {"Heriot-Watt MSc"}}, session); rec.Code != http.StatusSeeOther {
			t.Errorf("a new project named after a deleted one = %d, want 303", rec.Code)
		}
		var n int
		query := `SELECT count(*) FROM pages WHERE id IN ($1, $2, $3) AND trashed_at IS NOT NULL AND project_id IS NULL AND task_id IS NULL`
		if err := s.db.QueryRowContext(t.Context(), query, laundryPage, heriotPage, enrolPage).Scan(&n); err != nil || n != 3 {
			t.Errorf("pages of the deleted task, project, and its task in the trash = %d, %v, want 3", n, err)
		}
	})

	t.Run("a deleted task's page comes back as a page of its own", func(t *testing.T) {
		laundry := idOf(t, `SELECT id FROM pages WHERE title = $1`, "Laundry")
		wantRedirect(t, s.post(t, "/page/trash/"+laundry+"/restore", nil, session), "/page/trash")
		if page := body(t, "/page"); !strings.Contains(page, ">Laundry</a>") {
			t.Errorf("Pages does not list the restored task page:\n%s", page)
		}
	})

	t.Run("bad input is rejected", func(t *testing.T) {
		if rec := s.get(t, "/task/tasks?status=later", session); rec.Code != http.StatusBadRequest {
			t.Errorf("GET with an unknown status = %d, want 400", rec.Code)
		}
		if rec := s.post(t, "/task/tasks/new", url.Values{"title": {" "}, "next": {"/task/tasks"}}, session); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("POST a blank title = %d, want 422", rec.Code)
		}
	})
}

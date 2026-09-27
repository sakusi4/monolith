# 프로젝트와 할 일

이 문서는 프로젝트·할 일 기능의 설계다. 구현을 바꾸면 이 문서도 같이 고친다.

## 목적

노션의 Projects와 Todo HQ를 대체한다. 프로젝트는 관련된 모든 자료를 모으는 곳이고, 할 일은 한 목록에 모두 모아 프로젝트와 연결해 한 화면에서 본다. 할 일과 프로젝트는 각자 상세 내용(마크다운 본문, 본문 속 이미지, 첨부 파일)을 가진다. 프로젝트는 다른 기능이 가리키는 허브다: 할 일과 드라이브 폴더가(나중에 메모도) 외래 키로 프로젝트를 가리키고, 프로젝트 화면이 그것들을 모아 보인다. 사이드바에 **Work** 섹션을 두고 **Tasks**(`/task/tasks`)와 **Projects**(`/task/projects`)를 둔다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 연결 방식 | 외래 키로만 연결한다(허브 방식). 무엇이든 무엇에 연결하는 범용 연결 테이블과 노션식 페이지·속성 모델은 두지 않는다. 새 연결은 필요할 때 테이블이나 컬럼으로 더한다 |
| 프로젝트 종류 | 끝이 있는 것(2026 Vacation)과 끝이 없는 것(English C1)을 구분하지 않는다. 끝이 없는 것은 Active로 두고 종료일을 비운다. 영역(Area)과 목표(Goal) 층은 두지 않는다 |
| 프로젝트 상태 | Planned, Active, Paused, Done, Canceled. 노션의 Not started → Planned, pending → Paused, In progress → Active |
| 프로젝트 이름 | 드라이브 폴더 이름 규칙과 같다(`drive.CleanName`: 앞뒤 공백 제거, NFC, 비지 않음, `/` 없음, 255자 이하). 프로젝트끼리 겹치지 않는다 |
| 프로젝트 폴더 | 프로젝트를 만들면 드라이브에 `Projects/<이름>` 폴더를 만들어 연결한다. 이름을 바꾸면 폴더 이름도 바꾼다 |
| 할 일 | 제목, 프로젝트, 상태, 마감일(없어도 된다), 본문. 우선순위는 두지 않는다(노션에서 20건 중 18건이 기본값 Mid였다) |
| 할 일의 프로젝트 | 수정 화면에서 바꿀 수 있다. 할 일에 폴더가 있으면 새 프로젝트의 `Tasks`(Inbox면 `Inbox`) 아래로 옮기고, 이름이 겹치면 `제목 (#id)`다. 폴더와 본문 링크는 id로 이어져 있어 옮겨도 첨부가 깨지지 않는다. 폴더가 있는데 새 프로젝트에 폴더가 없거나 휴지통에 있으면 422(`ErrNoFolder`)이고 아무것도 바뀌지 않는다. 할 일 폴더가 휴지통에 있으면 프로젝트만 바꾼다 |
| 할 일 상태 | To do, In progress, Done, Canceled. "Open"은 To do와 In progress를 뜻하는 필터 값이다. 상태는 할 일 수정 화면에서 바꾼다(목록에 Done 버튼은 없다) |
| 완료 시각 | 상태를 Done으로 저장하면 `completed_at`을 기록하고, 다른 상태로 바꾸면 지운다. 다시 Done으로 저장해도 처음 시각이 남는다 |
| 본문 | 마크다운(GFM: 제목, 목록, 체크박스, 링크, 코드, 표, 취소선, 자동 링크). 화면에서는 템플릿 함수 `markdown`(`web`, `github.com/yuin/goldmark`)으로 그린다. 원시 HTML은 빼고 그리며, `javascript:` 같은 위험한 링크는 버린다. 페이지의 `h1`이 하나이도록 제목은 한 단계씩 내려 그린다(`#` → `h2`). 입력 중 미리보기는 두지 않는다 |
| 첨부 | 할 일과 프로젝트의 파일은 드라이브의 자기 폴더에 둔다. 첨부 목록은 그 폴더의 최상위 파일이다. 본문의 이미지도 그 폴더의 파일이고, 본문에는 `![이름](/drive/files/{id}/content)`로 들어간다 |
| 할 일 폴더 | 첫 파일을 올릴 때 만든다. 프로젝트의 할 일은 `<프로젝트 폴더>/Tasks/<폴더 이름>`, Inbox 할 일은 `Inbox/<폴더 이름>`. 폴더 이름은 제목에서 `/`를 `-`로 바꾸고 200자로 자른 것이고(비면 `Task <id>`), 그 이름이 이미 있으면 `<폴더 이름> (#<id>)`이다. 제목을 바꾸면 같은 규칙으로 폴더 이름도 바꾼다 |
| 이미지 붙여넣기 | 본문 입력칸에 이미지를 붙여넣거나 끌어다 놓으면 `web/static/markdown_editor.js`가 파일을 편집 폼의 숨은 파일 입력칸에 담고, 커서 자리에 `![이름](이름)` 자리표시를 넣는다. 업로드는 Save 때 한 번이다: 편집 폼은 `multipart/form-data`이고, 서버는 본문이 `](이름)`으로 가리키는 파일만 첨부로 올려 그 링크를 `/drive/files/{id}/content`로 바꾸고 나머지는 버린다. 그래서 붙였다 지운 이미지나 Cancel한 편집은 아무것도 남기지 않는다. 저장할 때 원래 본문이 가리키던 파일 중 새 본문이 가리키지 않고 그 항목의 폴더에 있는 것은 휴지통으로 간다(`drive.TrashFiles`). Attach files로 올린 첨부는 본문이 가리키지 않았으므로 그대로다. 붙여넣은 것에 글자도 있으면(오피스 문서의 표처럼 글자와 그 그림이 같이 온 것) 글자를 붙여넣는다. 붙여넣은 파일 이름은 `pasted-<밀리초>-<n>`, 끌어다 놓은 파일은 원래 이름에서 공백과 괄호를 `-`로 바꾼 것이다. 검증에 실패한(422) 저장은 담아 둔 파일을 잃으므로 다시 붙여넣어야 한다 |
| 할 일 삭제 | 할 일을 지우고 그 폴더를 휴지통으로 옮긴다 |
| 프로젝트 삭제 | 프로젝트와 그 할 일을 지우고(`ON DELETE CASCADE`) 프로젝트 폴더(할 일 폴더를 포함한다)를 휴지통으로 옮긴다. 파일은 휴지통에서 되살릴 수 있다 |

## 데이터 모델: `0011.sql`

```sql
CREATE TABLE projects (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL UNIQUE CHECK (name <> ''),
    status text NOT NULL CHECK (status IN ('planned', 'active', 'paused', 'done', 'canceled')),
    started_on date,
    finished_on date CHECK (finished_on >= started_on),
    body text NOT NULL DEFAULT '',
    folder_id bigint REFERENCES folders (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text NOT NULL CHECK (title <> ''),
    project_id bigint REFERENCES projects (id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('todo', 'in_progress', 'done', 'canceled')),
    due_on date,
    body text NOT NULL DEFAULT '',
    folder_id bigint REFERENCES folders (id) ON DELETE SET NULL,
    completed_at timestamptz CHECK ((status = 'done') = (completed_at IS NOT NULL)),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_project_id_idx ON tasks (project_id);
```

- `completed_at`은 상태를 저장하는 같은 SQL 문장이 정한다: `CASE WHEN $status = 'done' THEN coalesce(completed_at, now()) END`.
- 할 일을 고치는 SQL은 `project_id`를 바꾸지 않는다.
- `folder_id`의 외래 키는 드라이브의 테이블을 가리키지만, task 패키지는 `folders`와 `files`를 쿼리하지 않는다. 폴더와 파일을 읽고 바꾸는 일은 드라이브의 exported 메서드로 한다(CLAUDE.md 3.1.9).

## 드라이브 연동

드라이브에 다음을 더한다. 드라이브는 여전히 프로젝트와 할 일을 모른다.

- `CleanName(name string) (string, error)`: 드라이브의 이름 규칙을 export한 것이다.
- `CreateFolder`가 새 폴더의 id를 돌려준다.
- `EnsureFolder(ctx, parent int64, name string) (int64, error)`: 휴지통 밖에 그 이름의 폴더가 있으면 그 id를, 없으면 만들어 id를 돌려준다.
- 주소 함수 `FolderURL`, `FileURL`을 export한다.
- `ReceiveUploads(w, r, store, maxUpload) (url.Values, []Upload, error)`: 업로드 요청의 기한을 늘리고 크기를 제한해 나머지 필드(필드마다 1 MiB까지)를 돌려주고 "files" 파트를 임시 저장한다. 드라이브의 업로드 화면도 이것을 쓴다. 형식이 틀린 본문은 `ErrBadUpload`, 크기 초과는 `*http.MaxBytesError`다.
- `AddFiles`가 추가한 파일(`[]File`)을 업로드 순서대로 돌려준다. 본문의 자리표시 링크를 바꿀 때 쓴다.
- 크기 표시 `FormatSize`를 export한다.

- `TrashFiles(ctx, folder int64, ids []int64) error`: 그 폴더에 있는 파일만 휴지통으로 옮긴다.

할 일과 프로젝트는 이 밖에 `Path`, `Files`, `UpdateFolder`, `TrashFolder`, `Discard`를 쓴다.

프로젝트 만들기:

1. 입력을 정리한다(`ProjectInput.Clean`, 이름은 `drive.CleanName`).
2. 같은 이름의 프로젝트가 있으면 `ErrNameTaken`.
3. `EnsureFolder(0, "Projects")`로 최상위 `Projects` 폴더를 얻는다.
4. `CreateFolder(Projects, 이름)`. 폴더 이름이 이미 있으면 `ErrFolderTaken`(422: "Drive already has a folder named … in Projects.").
5. 프로젝트 행을 넣는다. 실패하면 방금 만든 폴더를 휴지통으로 옮기고 에러를 돌려준다.

프로젝트 이름 바꾸기: 같은 이름의 다른 프로젝트가 있으면 `ErrNameTaken` → 폴더가 연결돼 있고 휴지통 밖이면 폴더 이름을 바꾼다(폴더 쪽 이름 중복은 `ErrFolderTaken`) → 프로젝트 행을 저장한다. 폴더가 없거나 휴지통에 있으면 프로젝트만 바꾼다.

할 일에 파일 올리기:

1. 할 일에 폴더가 있고 휴지통 밖이면 그 폴더를 쓴다.
2. 없으면 부모를 정한다: 프로젝트의 할 일은 `EnsureFolder(프로젝트 폴더, "Tasks")`, Inbox 할 일은 `EnsureFolder(0, "Inbox")`. 프로젝트 폴더가 없거나 휴지통에 있으면 `ErrNoFolder`(422: "The project's folder is missing or in the trash.").
3. `CreateFolder(부모, 폴더 이름)`, 이름이 있으면 `CreateFolder(부모, 폴더 이름 (#id))`. 할 일 행에 `folder_id`를 저장한다.
4. `drive.AddFiles(폴더, 파일들)`. 이름이 겹치면 422.

프로젝트에 파일 올리기는 프로젝트 폴더에 바로 `AddFiles`한다. 폴더가 없거나 휴지통에 있으면 `ErrNoFolder`.

두 저장소는 트랜잭션 하나로 묶이지 않는다. 사용자가 한 명이라 동시에 같은 이름을 쓰는 경쟁은 다루지 않는다.

## 화면

모든 라우트는 로그인이 필요하다(`auth.Require`). `{id}`가 없는 항목이면 404다.

| 라우트 | 동작 |
|---|---|
| `GET /task/tasks` | 할 일 목록. 쿼리 `status`, `project`, `order`, `direction` |
| `POST /task/tasks/new` | 빠른 추가: `title`, `project`, `due` |
| `GET /task/tasks/{id}` | 할 일 화면 |
| `GET /task/tasks/{id}/edit` | 할 일 수정 화면 |
| `POST /task/tasks/{id}/edit` | 제목, 프로젝트, 상태, 마감일, 본문 저장 |
| `POST /task/tasks/{id}/fields` | 목록의 한 행에서 프로젝트, 상태, 마감일 저장. 제목과 본문은 그대로다. `next`로 303 |
| `POST /task/tasks/{id}/files` | 파일 첨부(여러 개). 수정 화면으로 303 |
| `POST /task/tasks/{id}/delete` | 삭제 |
| `GET /task/projects` | 프로젝트 목록. 쿼리 `status` |
| `POST /task/projects/new` | 프로젝트 만들기: `name` |
| `GET /task/projects/{id}` | 프로젝트 화면 |
| `GET /task/projects/{id}/edit` | 프로젝트 화면의 머리와 본문을 입력칸으로 바꾼다 |
| `POST /task/projects/{id}/edit` | 이름, 상태, 시작일, 종료일, 본문 저장 |
| `POST /task/projects/{id}/files` | 파일 첨부. 수정 화면으로 303 |
| `POST /task/projects/{id}/delete` | 삭제 |

- 빠른 추가와 할 일 삭제는 폼 값 `next`로 돌아갈 주소를 받는다. 쓴 그대로도, `path.Clean`으로 정리한 뒤에도 `/task/`로 시작해야 받는다(`//evil.example/../task/…`, `/task/../drive`는 거절). 아니면 할 일 목록으로 간다. 성공하면 그 주소로 303을 보낸다.
- 할 일 수정이 성공하면 할 일 화면으로, 프로젝트의 POST가 성공하면 프로젝트 화면(삭제는 프로젝트 목록)으로 303을 보낸다.
- 이미지 올리기는 이미지 형식이면 `![이름](주소)`, 아니면 `[이름](주소)`를 파일마다 한 줄로 돌려준다.

### 할 일 목록 (`task_list.html`)

1. 제목 "Tasks".
2. 필터 막대(`web.FilterBar`, `taskQuery`): Status(Open 기본, To do, In progress, Done, Canceled, All), Project(All 기본, Inbox, 각 프로젝트), Sort by(Due 기본, Project, Status, Created), Direction(Ascending 기본). 목록 밖의 값이면 400이다.
3. 빠른 추가 폼: Title, Project(필터의 프로젝트가 기본값), Due. 상태는 To do로 시작한다.
4. 표: 제목(할 일 화면으로 가는 링크), Project(선택칸, Inbox와 프로젝트 목록), Status(선택칸), Due(날짜칸, 지난 마감일은 `.danger` 색), Delete. 선택칸은 바꾸는 즉시, 날짜칸은 칸을 벗어날 때 저장되고 목록이 다시 그려진다. 폴더가 있는 할 일을 폴더 없는 프로젝트로 옮기면 422로 표 위에 문구를 보인다. 제목과 본문은 할 일 화면에서 고친다.
   - 정렬: Due는 마감일 순이고 마감일이 없는 행은 방향과 무관하게 맨 뒤다. Project는 프로젝트 이름 순이고 Inbox가 먼저다. Status는 To do, In progress, Done, Canceled 순서다. Created는 만든 시각 순이다. 같은 값이면 만든 시각이 먼저인 행이 앞이다.
5. 비어 있으면 "No tasks match the filters."

### 할 일 화면 (`task_detail.html`)

1. 경로 링크(Tasks, 프로젝트가 있으면 프로젝트), 제목, Edit·Delete(확인 창).
2. 제목 아래 한 줄에 Project(링크 또는 Inbox), Status, Due(있을 때), Completed(Done일 때).
3. 본문(마크다운). 비어 있으면 "No details."
4. Attachments: 할 일 폴더의 최상위 파일(이름은 드라이브의 파일 화면으로 가는 링크, 종류, 크기)과 "Open in Drive". 폴더가 없으면 "No attachments."

### 할 일 수정 화면 (`task_edit.html`)

Title, Project(Inbox와 프로젝트 목록), Status, Due, Body(`<textarea>`, 이미지 붙여넣기), Save·Cancel. 그 아래 Attach files 폼(여러 파일)과 지금의 첨부 목록.

### 프로젝트 목록 (`project_list.html`)

제목 "Projects", 필터(Status: Active 기본, 각 상태, All), 새 프로젝트 폼(Name), 표(Name, Status, Period, Open tasks). Period는 `Sep 6, 2026 – ` 처럼 비어 있는 쪽을 비운다. 이름은 프로젝트 화면으로 가는 링크다.

### 프로젝트 화면 (`project_detail.html`)

1. 경로 링크(Projects), 제목(이름), Edit·Delete(확인 창: 할 일도 지워지고 폴더는 휴지통으로 간다).
2. 제목 아래 한 줄에 상태와 기간(있을 때), 그 아래 본문(마크다운). Edit를 누르면 이름, 상태, 시작일, 종료일, 본문(이미지 붙여넣기) 입력칸과 Attach files 폼으로 바뀐다.
3. Open tasks: 이 프로젝트의 Open 할 일을 할 일 목록과 같은 행으로 보인다(정렬은 Due). Project 칸은 없고 Status와 Due를 그 자리에서 고친다. 빠른 추가 폼의 프로젝트는 이 프로젝트로 고정된다. "All tasks of this project"는 `/task/tasks?project={id}&status=all`로 간다.
4. Attachments: 프로젝트 폴더의 최상위 파일과 "Open in Drive". 폴더가 없으면 "No attachments.", 휴지통에 있으면 "The folder is in the trash."

htmx는 지출 화면과 같은 방식으로 목록 화면들과 프로젝트 화면에서 불러온다. 필터 폼, 표, 빠른 추가 폼, 프로젝트 머리 수정에 `hx-boost`를 건다. 다른 화면(할 일 화면, 드라이브)으로 가는 링크는 boost하지 않는다. 파일 첨부 폼은 boost하지 않는다.

422가 되는 경우:

- 할 일: 제목이 비었음, 상태가 목록 밖, 프로젝트가 없음, 마감일 형식이 틀림, 첨부 이름이 폴더 안에서 겹침, 프로젝트 폴더가 없거나 휴지통에 있음.
- 프로젝트: 이름 규칙 위반, 상태가 목록 밖, 종료일이 시작일보다 앞섬, 날짜 형식이 틀림, 이름 중복, 드라이브의 `Projects`에 같은 이름의 폴더가 있음, 첨부 이름이 겹침, 폴더가 없거나 휴지통에 있음.

## 코드 배치

| 파일 | 내용 |
|---|---|
| `internal/postgres/migrations/0011.sql` | `projects`, `tasks` |
| `internal/task/task.go` | 패키지 문서, `Store`(`*sql.DB`, `*drive.Store`), `Task`, `TaskStatus`와 표시 이름, `TaskInput`과 `Clean`, 할 일 Store 메서드 |
| `internal/task/project.go` | `Project`, `ProjectStatus`와 표시 이름, `ProjectInput`과 `Clean`, 프로젝트 Store 메서드(드라이브 연동 포함) |
| `internal/task/attachment.go` | 할 일 폴더 이름 규칙(`taskFolderName`), 폴더 찾기·만들기, 첨부 올리기와 목록, 본문이 가리키는 업로드 고르기와 링크 바꾸기(`usedUploads`, `linkFiles`) |
| `internal/task/task_list.go` | `taskQuery`: 쿼리 해석, 필터 막대, 정렬, 목록 주소 |
| `internal/task/handler.go` | `NewHandler`, 라우트, 공통 도우미(`next` 해석, 업로드 읽기 등) |
| `internal/task/task_handler.go`, `project_handler.go` | 할 일, 프로젝트 핸들러 |
| `web/markdown.go` | 템플릿 함수 `markdown` |
| `web/static/markdown_editor.js` | 본문 입력칸의 이미지 붙여넣기·끌어다 놓기 |
| `web/templates/task_list.html`, `task_detail.html`, `task_edit.html`, `project_list.html`, `project_detail.html`, `_task_rows.html`, `_attachments.html` | 화면. 할 일 행과 첨부 목록은 여러 화면이 같이 쓴다 |
| `internal/drive/folder.go` | `CleanName` export, `CreateFolder`가 id 반환, `EnsureFolder` |
| `cmd/server/routes.go`, `web/templates/layout.html` | `/task/` 마운트, Work 섹션 |

- CLAUDE.md 3절의 패키지 목록에서 `internal/task/`의 설명을 "프로젝트, 할 일"로 고친다.

## 테스트

CLAUDE.md 9절을 따른다.

**단위 테스트** (DB 없음)

- `TaskInput.Clean`: 제목 앞뒤 공백 제거, 빈 제목, 목록 밖 상태
- `ProjectInput.Clean`: 드라이브가 거절하는 이름, 목록 밖 상태, 종료일이 시작일보다 앞섬
- `parseTaskQuery`, `statusFilter.statuses`, `taskQuery.sort`, `today`, `safeNext`
- `taskFolderName`: 그대로 쓰는 제목, `/`가 든 제목, 200자를 넘는 제목, 공백뿐인 제목
- `usedUploads`: 본문이 `](이름)`으로 가리키는 업로드와 나머지를 나눈다
- `linkFiles`: 자리표시 링크를 파일 주소로 바꾼다
- `droppedFiles`: 원래 본문에는 있고 새 본문에는 없는 파일 링크
- `web`의 `markdown`: 체크박스와 표를 그린다, 제목을 한 단계 내린다, 원시 HTML은 빠진다, `javascript:` 링크는 버린다

**HTTP 통합 테스트** (`cmd/server/task_test.go`의 `TestTasks`, 실제 DB)

- 로그인 없이는 로그인 화면으로 간다.
- 프로젝트를 만들면 드라이브에 `Projects/<이름>` 폴더가 생긴다. 같은 이름은 422다. 이름을 바꾸면 폴더 이름도 바뀐다. `Projects`에 같은 이름의 폴더가 있으면 422이고 프로젝트가 생기지 않는다.
- 빠른 추가한 할 일이 Open 목록에 마감일 순으로 보이고, Inbox와 프로젝트 필터가 행을 좁힌다.
- Done으로 저장하면 Open 목록에서 빠지고 `completed_at`이 기록되며, 다시 Done으로 저장해도 처음 시각이 남고, To do로 되돌리면 지워진다.
- 할 일의 본문이 할 일 화면에 마크다운으로 그려진다.
- 할 일의 프로젝트를 바꾸면 폴더가 새 프로젝트의 `Tasks` 아래로 옮겨진다. 폴더가 없는 할 일은 폴더 없는 프로젝트로도 옮길 수 있고, 폴더가 있는 할 일을 폴더 없는 프로젝트로 옮기면 422이고 그대로다.
- 할 일에 파일을 올리면 `Projects/<프로젝트>/Tasks/<제목>`에 저장되고 첨부 목록에 보인다. Inbox 할 일은 `Inbox/<제목>`이다. 본문과 함께 저장한 파일 중 본문이 가리키는 것만 첨부가 되고 그 이미지가 할 일 화면에 그려지며, 가리키지 않는 것은 남지 않는다. 다시 저장해 본문에서 뺀 파일은 휴지통으로 가되, 다른 할 일의 파일은 건드리지 않는다.
- 할 일을 지우면 그 폴더가 휴지통으로 간다. 프로젝트를 지우면 그 할 일이 사라지고 프로젝트 폴더가 휴지통으로 간다.
- 목록에서 프로젝트, 상태, 마감일을 바꾸면 저장되고 `next`로 간다. 폴더가 있는 할 일을 폴더 없는 프로젝트로 옮기면 422다.
- 잘못된 필터는 400이다.

이미지 붙여넣기와 끌어다 놓기(`markdown_editor.js`)는 자동 테스트가 없고, headless 브라우저로 직접 확인한다.

## 범위 밖

- 반복 할 일, 하위 할 일, 우선순위
- 할 일의 프로젝트 바꾸기
- 입력 중 미리보기
- 한 파일을 여러 할 일에 연결하기, 드라이브의 기존 파일을 첨부로 고르기
- 목표(Goal)와 영역(Area) 층
- 할 일 목록 페이지 나누기(Done이 쌓이면 지출 목록과 같은 방식으로 더한다)
- 노션 이전(D)
- 메모(C)

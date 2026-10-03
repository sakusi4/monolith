# 페이지

이 문서는 페이지 기능의 설계다. 구현을 바꾸면 이 문서도 같이 고친다.

## 목적

노션의 페이지를 대체한다. 노션에서 메모는 Areas(기술 정리, 영어, 요리 …) 아래와 프로젝트 아래에 마크다운 문서가 트리로 쌓인 것이고, Areas의 속성은 하나도 쓰이지 않는다. 그래서 앱의 모든 글은 **페이지** 하나로 다룬다. 페이지는 제목, 본문, 하위 페이지, 첨부만 가진다. 프로젝트와 할 일은 페이지를 가리키는 메타데이터다: 상태, 날짜, 소속 프로젝트처럼 미리 정해진 속성만 자기 테이블에 두고, 이름과 본문과 첨부는 그 페이지가 가진다. 그래서 프로젝트 본문이든 할 일 본문이든 독립 페이지든 하위 페이지를 만들고 서로 링크하는 방법이 같다. 사이드바에 **Pages**(`/page`)를 둔다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 모델 | 페이지에 사용자 정의 속성은 두지 않는다. 속성이 필요한 것은 타입이 있는 기능(프로젝트, 할 일)으로 만들고 그 행이 페이지를 가리킨다. 노션의 데이터베이스·뷰는 두지 않는다 |
| 트리 | 페이지는 부모 페이지 하나를 가진다. 부모가 없고 주인도 없는 페이지가 최상위 페이지(노션의 Area)다. 주인(프로젝트나 할 일)이 있는 페이지는 항상 뿌리이고, 그 하위 페이지는 주인 없는 보통 페이지다 |
| 제목 | 앞뒤 공백을 지우고 NFC로 합친 뒤(`golang.org/x/text/unicode/norm`) 비어 있지 않다(`CleanTitle`). 같은 부모 아래에서 겹쳐도 된다. 프로젝트 이름은 지금처럼 `drive.CleanName`을 따르고 프로젝트끼리 겹치지 않는다. 할 일 제목은 지금처럼 `TaskInput.Clean`을 따른다 |
| 하위 페이지 만들기 | 본문에 `[[제목]]`을 쓰고 저장하면 그 제목의 하위 페이지를 가리키는 링크가 된다. 휴지통 밖의 같은 제목 하위 페이지가 있으면 그것을(여럿이면 먼저 만든 것을), 없으면 새로 만든다. JS 없이 저장 때 서버가 처리하므로 Cancel한 편집은 아무것도 남기지 않는다. 하위 페이지를 따로 만드는 폼은 두지 않는다 |
| 링크와 백링크 | 저장할 때 본문이 가리키는 페이지를 `page_links`에 기록하고, 모든 페이지·프로젝트·할 일 화면에 그 페이지를 가리키는 페이지(Linked from)를 보인다 |
| 편집기 | 지금의 마크다운 입력칸(`markdown_editor.js`의 이미지 붙여넣기 포함)을 그대로 쓴다. 노션식 편집기는 별도 spike로 본다. 본문이 마크다운으로 저장되는 한 편집기는 나중에 바꿔 끼울 수 있다 |
| 첨부 폴더 | 주인 없는 페이지의 폴더는 첫 업로드 때 `Pages/<폴더 이름> (#id)`로 만든다. 트리를 따라가지 않고 한 곳에 평평하게 두므로 페이지를 옮겨도 폴더는 그대로다. 프로젝트·할 일 페이지의 폴더는 지금처럼 task가 정한다(`Projects/<이름>`, `Projects/<이름>/Tasks/…`, `Inbox/…`) |
| 본문에서 빠진 파일 | 지금의 할 일·프로젝트와 같다: 저장할 때 원래 본문이 가리키던 파일 중 새 본문이 가리키지 않고 그 페이지 폴더에 있는 것은 드라이브 휴지통으로 간다 |
| 삭제 | 페이지 휴지통. 지우면 하위 페이지와 함께 숨는다. 휴지통에 넣는 것은 드라이브를 건드리지 않는다. 영구 삭제할 때 그 페이지들의 첨부 폴더가 드라이브 휴지통으로 간다 |
| 검색 | 제목과 본문의 부분 일치(`ILIKE`). 한국어 단어를 나누지 못하는 전문 검색(`tsvector`) 대신 `pg_trgm` 인덱스를 쓴다 |
| 목록 화면 | Pages 화면은 최상위의 모든 페이지(주인 없는 페이지, 프로젝트 페이지, 할 일 페이지)를 보이고 Show 필터로 종류를 좁힌다. 하위 페이지는 각 페이지 화면에서 따라 들어간다 |

## 데이터 모델: `0012.sql`

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE pages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id bigint REFERENCES pages (id) ON DELETE CASCADE,
    project_id bigint UNIQUE REFERENCES projects (id) ON DELETE SET NULL,
    task_id bigint UNIQUE REFERENCES tasks (id) ON DELETE SET NULL,
    title text NOT NULL CHECK (title <> ''),
    body text NOT NULL DEFAULT '',
    folder_id bigint REFERENCES folders (id) ON DELETE SET NULL,
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(project_id, task_id) <= 1),
    CHECK (parent_id IS NULL OR num_nonnulls(project_id, task_id) = 0)
);
CREATE UNIQUE INDEX pages_project_title_idx ON pages (title) WHERE project_id IS NOT NULL;
CREATE INDEX pages_parent_id_idx ON pages (parent_id);
CREATE INDEX pages_title_trgm_idx ON pages USING gin (title gin_trgm_ops);
CREATE INDEX pages_body_trgm_idx ON pages USING gin (body gin_trgm_ops);

CREATE TABLE page_links (
    source_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    target_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    PRIMARY KEY (source_id, target_id),
    CHECK (source_id <> target_id)
);
CREATE INDEX page_links_target_id_idx ON page_links (target_id);

INSERT INTO pages (project_id, title, body, folder_id, created_at, updated_at)
SELECT id, name, body, folder_id, created_at, updated_at FROM projects;
INSERT INTO pages (task_id, title, body, folder_id, created_at, updated_at)
SELECT id, title, body, folder_id, created_at, updated_at FROM tasks;

ALTER TABLE projects DROP COLUMN name, DROP COLUMN body, DROP COLUMN folder_id;
ALTER TABLE tasks DROP COLUMN title, DROP COLUMN body, DROP COLUMN folder_id;
```

- 외래 키는 페이지가 주인을 가리킨다. 그래서 page 패키지는 task를 import하지 않고도 주인 있는 페이지를 Pages 목록에서 빼고, 경로의 첫 칸을 프로젝트·할 일 화면(`/task/projects/{id}`, `/task/tasks/{id}`)으로 잇는다. page 패키지가 아는 다른 기능의 주소는 프로젝트·할 일의 목록과 화면 주소뿐이다.
- 프로젝트 이름이 겹치지 않는다는 규칙은 `projects.name UNIQUE`에서 `pages_project_title_idx`로 옮긴다. `CreateOwned`와 `Save`는 이 인덱스에 걸리면 `ErrTitleTaken`을 돌려주고, task는 그것을 `ErrNameTaken`으로 바꾼다.
- 프로젝트와 할 일에 페이지가 항상 하나 있다는 규칙은 반대 방향이라 DB가 강제할 수 없다. 프로젝트·할 일을 만들 때 task가 트랜잭션을 열고 자기 행과 페이지(`page.Store.CreateOwned`, `*sql.Tx`를 받는다)를 함께 넣는다.
- 주인 행이 지워지면 `ON DELETE SET NULL`로 페이지가 남아 주인 없는 최상위 페이지가 된다. task는 주인 행을 지운 뒤 그 페이지를 휴지통에 넣는다. 그래서 휴지통에는 주인 있는 페이지가 없다. 두 단계 사이에서 실패하면 그 페이지가 Pages 목록에 최상위 페이지로 남는다.
- 페이지가 보이는 것은 그 페이지와 모든 조상에 `trashed_at`이 없을 때다. 조상과 하위 트리는 `WITH RECURSIVE`로 한 번에 읽는다.
- `pg_trgm`은 PostgreSQL에 들어 있는 확장이고 PostgreSQL 13부터 데이터베이스 소유자가 만들 수 있다. 3글자보다 짧은 검색어는 인덱스를 쓰지 못하고 전체를 훑는다.
- 옮겨 온 기존 본문의 링크는 그 페이지를 다음에 저장할 때 `page_links`에 기록된다.

## 동작

### 저장

제목, 부모, 본문을 저장하는 길은 하나다(`page.Store.Save`). 페이지 수정 화면도, 프로젝트·할 일 수정 화면도 이것을 쓴다. 주인 있는 페이지의 부모는 늘 비어 있다.

1. 제목(`CleanTitle`)과 본문을 정리하고 부모를 검사한다(아래 이동 규칙). 실패하면 업로드를 버리고 에러를 돌려준다.
2. 업로드 중 본문이 `](이름)`으로 가리키는 것만 첨부로 올려 그 링크를 `/drive/files/{id}/content`로 바꾸고 나머지는 버린다(지금의 `usedUploads`, `linkFiles`, `attachLinked`를 옮긴 것). 폴더는 호출한 쪽이 정한다: 주인 없는 페이지는 page가, 프로젝트·할 일 페이지는 task가 정한다.
3. 트랜잭션 하나로 `[[ ]]`를 하위 페이지로 바꾸고, 페이지 행을 저장하고, `page_links`를 다시 쓴다.
4. 원래 본문이 가리키던 파일 중 새 본문이 가리키지 않는 것을 휴지통으로 보낸다(`droppedFiles`, `trashDropped`를 옮긴 것).

두 저장소(드라이브와 페이지)는 트랜잭션 하나로 묶이지 않는다. 지금의 할 일 저장과 같다.

### `[[제목]]`

- 본문에서 `[[`와 `]]` 사이에 `[`, `]`, 줄바꿈이 없는 글자가 있는 곳을 찾는다. 인라인 코드와 코드 블록 안은 건너뛴다(goldmark로 코드 범위를 찾는다). bash의 `[[ -f x ]]`가 하위 페이지가 되지 않게 하기 위해서다.
- 안의 글자를 `CleanTitle`로 정리한다. 비면 글자로 둔다.
- 이 페이지의 휴지통 밖 하위 페이지 중 그 제목들을 한 쿼리로(`title = ANY($1)`) 찾고, 없는 제목만 한 쿼리로 넣는다. 반복문 안에서 쿼리하지 않는다.
- 같은 제목이 여러 번 나오면 하위 페이지는 하나이고 모두 같은 링크가 된다.
- 각 자리를 `[제목](/page/pages/{id})`로 바꾼다. 나중에 하위 페이지의 제목을 바꿔도 링크는 맞는 곳을 가리키고, 보이는 글자만 저장 당시 제목으로 남는다.
- 본문에서 링크를 지워도 하위 페이지는 남는다.

### 링크

- 저장할 때 본문의 `/page/pages/{id}`, `/task/projects/{id}`, `/task/tasks/{id}` 주소를 뽑는다. 앞에 호스트가 붙은 주소(`http://localhost:8080/task/projects/3`)와 뒤에 경로가 더 붙은 주소(`/task/tasks/5/edit`)도 같다. `/drive/files/…` 같은 다른 주소는 뺀다.
- 프로젝트·할 일 주소는 `project_id`, `task_id`로 그 페이지를 찾는다. 자기 자신과 없는 페이지는 뺀다. 그 페이지의 `page_links` 행을 지우고 한 쿼리로 다시 넣는다.
- Linked from: 이 페이지를 가리키는 보이는 페이지를 제목순으로, 각 페이지의 경로와 함께 보인다.

### 이동

- 페이지 수정 화면의 Parent 선택칸에서 고른다. 선택지는 "Top level"과 보이는 모든 페이지이고(경로로 표시, 프로젝트·할 일 페이지 포함), 자신과 자신의 하위 페이지는 빠진다.
- 부모가 없거나, 보이지 않거나, 자신이나 자신의 하위 페이지면 `ErrInvalidParent`(422)다.
- 주인 있는 페이지는 옮길 수 없다(그 화면에는 Parent 선택칸이 없다).
- 첨부 폴더는 평평하므로 옮겨도 드라이브는 바뀌지 않는다.

### 첨부 폴더 (주인 없는 페이지)

- 첫 업로드 때 `EnsureFolder(0, "Pages")` 아래에 `<폴더 이름> (#id)`를 만들어 `folder_id`에 저장한다. 폴더 이름은 제목에서 `/`를 `-`로 바꾸고 앞뒤 공백을 지워 200자로 자른 것이다. `(#id)`가 항상 붙으므로 이름이 겹치지 않는다.
- 폴더가 없거나 휴지통에 있으면 새로 만든다.
- 제목을 바꾸면 폴더 이름도 바꾼다(폴더가 휴지통 밖일 때).

### 휴지통

| 동작 | 결과 |
|---|---|
| 페이지 지우기 | 그 페이지에 `trashed_at`을 기록한다. 하위 페이지는 함께 숨는다. 숨은 페이지의 화면은 404이고, 검색, Linked from, Parent 선택지에서 빠진다 |
| 할 일 지우기 | 할 일 행을 지우고(페이지는 주인 없는 최상위 페이지가 된다) 그 페이지를 휴지통에 넣는다 |
| 프로젝트 지우기 | 프로젝트 페이지와 그 할 일들의 페이지 id를 읽고, 프로젝트를 지운 뒤(할 일은 `ON DELETE CASCADE`) 프로젝트 폴더의 이름을 주인 없는 페이지의 규칙(`<이름> (#페이지 id)`)으로 바꾸고, 그 페이지들을 한 쿼리로 휴지통에 넣는다. 폴더는 `Projects`에 그대로 있고, 이름이 비므로 같은 이름의 프로젝트를 다시 만들 수 있다 |
| 휴지통 목록 | `trashed_at`이 있는 페이지. 제목, 원래 경로, 지운 날짜 |
| 되살리기 | `trashed_at`을 지운다. 부모가 보이지 않으면 `ErrParentTrashed`(422: "The parent page is in the trash.")다. 주인을 잃은 페이지는 최상위 페이지로 돌아온다 |
| 영구 삭제, 비우기 | 그 페이지들과 하위 트리를 지우고(하위 페이지, 링크는 `ON DELETE CASCADE`), 지운 페이지들의 첨부 폴더를 드라이브 휴지통으로 보낸다. 행을 먼저 지운다 |

- 페이지를 휴지통에 넣는 것만으로는 드라이브를 건드리지 않는다. 그래서 되살린 페이지의 이미지가 깨지지 않는다. 할 일을 지우면 폴더가 바로 드라이브 휴지통으로 가던 지금의 동작이 이렇게 바뀐다.
- 지운 프로젝트의 할 일 페이지 하나만 되살리면 그 파일은 `Projects/<이름>/Tasks/…`에 그대로 있다. 그 뒤 프로젝트 페이지를 영구 삭제하면 프로젝트 폴더와 함께 그 파일도 드라이브 휴지통으로 간다. 파일은 드라이브 휴지통에서 되살릴 수 있다.

### 검색

- `GET /page?q=…`. 보이는 페이지 중 제목이나 본문에 검색어가 들어 있는 것이다. 주인 있는 페이지도 포함한다. 검색어의 `\`, `%`, `_`는 글자로 찾는다.
- 제목에 들어 있는 페이지가 먼저, 그 안에서는 최근에 고친 것이 먼저다. Show 필터(`kind`)로 종류를 좁힌 뒤 100개까지 보인다. 종류는 그 페이지가 속한 트리의 맨 위 페이지로 정한다(프로젝트 페이지 아래의 하위 페이지는 Projects).
- 결과는 제목(링크)과 경로다. 주인 있는 페이지의 링크는 프로젝트·할 일 화면이다.

## 드라이브 연동

드라이브에 다음을 더한다. 드라이브는 여전히 페이지를 모른다.

- `TrashFolders(ctx, ids []int64) error`: 휴지통 밖에 있는 그 폴더들을 한 쿼리로 휴지통에 넣는다. 없거나 이미 휴지통에 있는 id는 건너뛴다.

페이지는 이 밖에 `EnsureFolder`, `CreateFolder`, `UpdateFolder`, `Path`, `Files`, `AddFiles`, `TrashFiles`, `Discard`를 쓴다.

## task 연동

page가 task에 내놓는 것: `CleanTitle`, `Owner`, `CreateOwned`(트랜잭션 안에서 페이지를 만든다), `ErrTitleTaken`, 페이지 읽기, `Save`, 폴더 지정, 여러 페이지 휴지통 넣기, 하위 페이지와 Linked from 목록, `PageURL`.

task에서 바뀌는 것:

- `task.NewStore(db, driveStore, pageStore)`.
- `Project`, `Task`의 이름·제목, 본문, 폴더는 pages를 JOIN해서 읽는다. 두 타입에 `PageID`가 생긴다. task가 다른 기능의 테이블을 읽는 것은 pages뿐이고(목록 JOIN, 프로젝트 이름 중복 확인, 지울 페이지 id), CLAUDE.md 3.1.9에 예외로 적는다. pages에 쓰는 일은 page의 메서드로만 한다.
- 프로젝트 만들기: 이름 정리와 중복 확인 → `Projects/<이름>` 폴더 만들기 → 트랜잭션에서 `projects` 행과 `CreateOwned`(폴더 포함) → 실패하면 폴더를 휴지통으로. 할 일 빠른 추가: 트랜잭션에서 `tasks` 행과 `CreateOwned`.
- 프로젝트·할 일 저장: 상태, 날짜, 소속 프로젝트는 자기 테이블에, 제목과 본문은 `Save`로 쓴다(부모는 비운다). 업로드할 폴더는 지금의 규칙(`taskFolder`, 프로젝트 폴더)으로 task가 정해 `Save`에 넘긴다.
- 프로젝트 이름 변경 때 폴더 이름 바꾸기, 할 일의 프로젝트를 바꿀 때 폴더 옮기기, 할 일 제목 변경 때 폴더 이름 바꾸기는 task에 남는다. 폴더 id만 page를 거쳐 읽고 쓴다.
- 지우기는 위 휴지통 표와 같다. 드라이브 폴더는 휴지통에 넣지 않는다(프로젝트 폴더는 이름만 바꾼다).
- 목록에서 상태, 소속 프로젝트, 마감일만 바꿀 때(제목과 본문이 그대로이고 업로드가 없을 때)는 `Save`를 부르지 않는다. 본문 속 `[[ ]]`가 사용자가 고치지 않은 저장에서 하위 페이지로 바뀌지 않게 하기 위해서다.
- `usedUploads`, `linkFiles`, `droppedFiles`, `trashDropped`, `attachLinked`와 그 테스트는 page로 옮긴다.

## 화면

모든 라우트는 로그인이 필요하다(`auth.Require`). `{id}`가 없거나 보이지 않는 페이지면 404다. 사이드바는 Dashboard, **Pages**, Drive, Work(Tasks, Projects), Finance(Assets, Expenses) 순서다.

| 라우트 | 동작 |
|---|---|
| `GET /page` | Pages 목록. 쿼리 `q`(있으면 검색 결과), `kind`(`all` 기본, `page`, `project`, `task`; 목록 밖이면 400) |
| `POST /page/pages/new` | 최상위 페이지 만들기: `title`. 그 페이지 화면으로 303 |
| `GET /page/pages/{id}` | 페이지 화면. 주인 있는 페이지면 프로젝트·할 일 화면으로 303 |
| `GET /page/pages/{id}/edit` | 페이지 수정 화면 |
| `POST /page/pages/{id}/edit` | 제목, 부모, 본문 저장(`multipart/form-data`, 이미지 붙여넣기). 페이지 화면으로 303 |
| `POST /page/pages/{id}/files` | 파일 첨부(여러 개). 수정 화면으로 303 |
| `POST /page/pages/{id}/delete` | 휴지통에 넣는다. 부모 페이지 화면(부모가 주인 있는 페이지면 그 주인의 화면, 부모가 없으면 `/page`)으로 303 |
| `GET /page/trash` | 휴지통 |
| `POST /page/trash/{id}/restore` | 되살리기. 휴지통으로 303 |
| `POST /page/trash/{id}/delete` | 영구 삭제. 휴지통으로 303 |
| `POST /page/trash/empty` | 휴지통 비우기. 휴지통으로 303 |

- 주인 있는 페이지의 `edit`, `files`, `delete`는 404다. 그 페이지는 프로젝트·할 일 화면에서 고치고 지운다.
- 이미지 붙여넣기와 첨부의 업로드는 `drive.ReceiveUploads`로 받는다.

### Pages 목록 (`page_list.html`)

1. 제목 "Pages".
2. 필터 막대(`web.FilterBar`, `pageQuery`): Search(`q`), Show(`kind`: All 기본, Pages, Projects, Tasks). FilterBar에 검색 입력칸(`web.Search`)을 더했고, 이름을 비워 두면 다른 목록의 필터 막대는 그대로다.
3. 새 페이지 폼: Title.
4. 최상위의 모든 페이지 목록(제목순). 각 행은 제목(링크, 프로젝트·할 일 페이지는 그 화면)과 종류(Pages, Projects, Tasks)다. 비어 있으면 "No pages yet."
5. `q`가 있으면 4 대신 검색 결과(제목, 경로). 없으면 "No pages match."
6. Trash 링크.

### 페이지 화면 (`page_detail.html`)

1. 경로 링크. 주인 없는 뿌리 아래면 `Pages / 기술 정리 / SDN`, 프로젝트 아래면 `Projects / <이름> / …`, 할 일 아래면 `Tasks / <제목> / …`. 프로젝트와 할 일 칸은 그 화면으로 간다.
2. 제목, Edit·Delete(확인 창: "Move this page and its subpages to the trash?").
3. 본문(마크다운, `markdown` 템플릿 함수). 비어 있으면 "No content."
4. Pages와 Linked from(`_page_links.html`).
5. Attachments(`_attachments.html`): 페이지 폴더의 최상위 파일과 "Open in Drive". 폴더가 없으면 "No attachments."

### 페이지 수정 화면 (`page_edit.html`)

Title, Parent(위 이동 규칙의 선택지), Body(`<textarea>`, 이미지 붙여넣기, `[[ ]]`), Save·Cancel. 그 아래 Attach files 폼과 지금의 첨부 목록.

### 휴지통 (`page_trash.html`)

제목 "Page trash", 표(Title, Location, Deleted, Restore·Delete forever 버튼), Empty trash(확인 창). 비어 있으면 "The trash is empty." 되살리기가 거절되면 422로 표 위에 문구를 보인다.

### 프로젝트·할 일 화면

- 프로젝트 화면과 할 일 화면의 본문 아래에 Pages와 Linked from(`_page_links.html`)을 보인다. 나머지는 지금과 같다.
- 프로젝트·할 일 수정 화면의 Body에서도 `[[ ]]`를 쓸 수 있다. 그 하위 페이지는 프로젝트·할 일 페이지의 하위 페이지다.

htmx는 페이지 화면들에 쓰지 않는다. 같은 화면을 다시 그리는 목록 편집이 없어서 전체 페이지 이동으로 충분하다.

422가 되는 경우: 제목이 비었음, Parent가 없거나 보이지 않거나 자신·하위 페이지임, 첨부 이름이 폴더 안에서 겹침, 되살릴 페이지의 부모가 휴지통에 있음.

## 코드 배치

| 파일 | 내용 |
|---|---|
| `internal/postgres/migrations/0012.sql` | `pages`, `page_links`, 기존 본문 옮기기 |
| `internal/page/page.go` | 패키지 문서, `Store`(`*sql.DB`, `*drive.Store`), `Page`, `Owner`, `CleanTitle`, 만들기·읽기·경로·이동·저장 |
| `internal/page/subpage.go` | `[[ ]]` 찾기와 바꾸기, 하위 페이지 찾기·만들기 |
| `internal/page/link.go` | 링크 뽑기, `page_links` 쓰기, 하위 페이지·Linked from 목록 |
| `internal/page/attachment.go` | 폴더 이름 규칙, 주인 없는 페이지 폴더 만들기, 업로드 고르기와 링크 바꾸기, 빠진 파일 휴지통(task에서 옮긴 것) |
| `internal/page/trash.go` | 휴지통 넣기, 목록, 되살리기, 영구 삭제 |
| `internal/page/search.go` | 검색 |
| `internal/page/page_list.go` | `pageQuery`: 쿼리 해석, 종류로 거르기와 검색 결과 100개 제한, 필터 막대 |
| `web/list.go`, `web/templates/_filters.html` | FilterBar의 검색 입력칸(`Search`) |
| `internal/page/handler.go` | `NewHandler`, 라우트, 핸들러 |
| `internal/drive/trash.go` | `TrashFolders` |
| `internal/task/*` | pages JOIN, `CreateOwned`·`Save` 사용, 옮긴 코드 삭제 |
| `web/templates/page_list.html`, `page_detail.html`, `page_edit.html`, `page_trash.html`, `_page_links.html` | 화면 |
| `web/templates/project_detail.html`, `task_detail.html`, `layout.html` | Pages·Linked from, 사이드바 |
| `cmd/server/routes.go` | `page.NewStore`, `/page`·`/page/` 마운트, `task.NewStore`에 page 넘기기 |

- CLAUDE.md 3절 패키지 목록의 `internal/note/  메모, 태그, 링크`를 `internal/page/  페이지: 본문, 하위 페이지, 링크, 휴지통. 프로젝트와 할 일의 본문도 페이지다`로 고치고, 3.1.9에 task의 pages JOIN 예외를 적는다.
- 할 일 설계 문서(`2026-09-26-tasks-design.md`)의 본문·첨부·삭제 부분을 이 문서를 가리키게 고친다.

## 테스트

CLAUDE.md 9절을 따른다.

**단위 테스트** (DB 없음, `internal/page`)

- `CleanTitle`: 앞뒤 공백 제거, 분해된 한글을 합침, 빈 제목
- `[[ ]]` 찾기: 일반 글자의 자리, 인라인 코드와 코드 블록 안은 건너뜀, 비었거나 `[`·`]`·줄바꿈이 든 것은 글자로 둠, 같은 제목은 하나
- `[[ ]]` 바꾸기: 제목별 id로 링크를 만든다
- 링크 뽑기: `/page/pages/{id}`, `/task/projects/{id}`, `/task/tasks/{id}`, 호스트가 붙은 주소, 다른 주소는 무시
- 폴더 이름: 그대로 쓰는 제목, `/`가 든 제목, 200자를 넘는 제목
- 검색어의 `\`, `%`, `_`
- `pageQuery.pick`: All은 모든 종류, 한 종류는 그 종류만, 검색은 100개까지, 목록은 자르지 않음
- task에서 옮긴 `usedUploads`, `linkFiles`, `droppedFiles`

**HTTP 통합 테스트** (`cmd/server/page_test.go`의 `TestPages`, 실제 DB)

- 로그인 없이는 로그인 화면으로 간다.
- 만든 최상위 페이지가 `/page`에 보인다. 빈 제목은 422다.
- `/page`에 프로젝트 페이지와 할 일 페이지가 보이고, `kind=project`면 할 일 페이지가 빠진다. 목록 밖의 `kind`는 400이다.
- 본문의 `[[X]]`를 저장하면 하위 페이지 X가 생겨 Pages에 보이고 본문의 링크가 그리로 간다. 같은 본문을 다시 저장해도 X는 하나다. 코드 블록 안의 `[[X]]`는 그대로다.
- A의 본문이 B를 가리키면 B 화면의 Linked from에 A가 보인다. A가 프로젝트 주소를 가리키면 프로젝트 화면에 A가 보인다.
- 페이지를 다른 페이지 아래로 옮기면 새 경로가 보인다. 자기 하위 페이지 아래로 옮기면 422다.
- 페이지를 지우면 그 페이지와 하위 페이지가 404이고 검색에서 빠진다. 되살리면 돌아온다. 부모가 휴지통에 있는 페이지를 되살리면 422다. 영구 삭제하면 첨부 폴더가 드라이브 휴지통으로 간다.
- 본문의 한국어 부분 문자열로 검색된다.
- 주인 없는 페이지의 첨부는 `Pages/<제목> (#id)`에 저장된다.

**`TestTasks` 수정** (`cmd/server/task_test.go`)

- 이름 변경과 폴더, 마크다운 본문, 첨부, 본문에서 빠진 파일 휴지통, 할 일의 프로젝트를 바꿀 때 폴더 이동은 지금 테스트가 그대로 통과해야 한다.
- "deleting moves the folders to the trash"를 바꾼다: 할 일을 지우면 그 페이지가 페이지 휴지통으로 가고 폴더는 남으며, 되살리면 최상위 페이지로 보인다. 프로젝트를 지우면 프로젝트 페이지와 할 일 페이지가 휴지통으로 가고, 같은 이름의 프로젝트를 다시 만들 수 있다.
- 목록에서 상태를 바꿔도 본문은 그대로다(`[[ ]]`가 하위 페이지로 바뀌지 않는다).
- 프로젝트 본문의 `[[X]]`가 `Projects / <이름> / X` 경로의 하위 페이지가 된다.

## 범위 밖

- 노션식 편집기(WYSIWYG, `/` 명령, 블록 드래그): 다음 spike에서 라이브러리(Milkdown, Toast UI Editor)를 붙여 한글 입력과 마크다운 왕복을 확인한다
- 이전 버전
- 페이지 속성, 데이터베이스, 뷰
- 태그, 빠른 메모
- 하위 페이지 순서 정하기(제목순이다)
- 펼친 트리, 사이드바 트리
- `[[ ]]`로 다른 곳의 페이지 연결(주소를 붙여 링크한다), 제목을 바꿀 때 링크 글자 갱신
- Parent 선택지가 길어질 때 검색으로 고르기
- 노션 이전(D)

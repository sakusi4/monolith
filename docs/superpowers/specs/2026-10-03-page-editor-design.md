# 노션식 페이지 편집

이 문서는 페이지 편집기와 자동 저장의 설계다. 구현을 바꾸면 이 문서도 같이 고친다. 페이지 자체(트리, 링크, 휴지통, 검색)는 `2026-10-02-pages-design.md`가 정하고, 이 문서와 다른 것은 이 문서를 따른다.

## 목적

노션처럼 쓴다. 페이지 화면 자체가 편집기이고 저장 버튼이 없다. `/`로 블록을 넣고, 블록 손잡이로 옮기고, `[[`로 다른 페이지를 찾아 연결하거나 하위 페이지를 바로 만든다. 붙여넣은 이미지는 바로 올라간다. 페이지, 프로젝트, 할 일의 본문이 모두 같은 편집기다. 본문은 계속 마크다운으로 저장한다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 편집기 | Milkdown Crepe 7.22.2. spike에서 Toast UI(노션답지 않음)와 비교해 골랐고, 한글 입력은 사용자가 직접 확인했다 |
| 빌드 | `web/editor/`에서 Node와 esbuild로 JS 하나와 CSS 하나로 빌드하고 결과(`web/static/editor-7.22.2.js`, `.css`)를 커밋한다. Node는 이 빌드에만 쓴다. Go 빌드, 테스트, 실행에는 필요 없다 |
| 데이터 보존 설정 | 기본 설정으로는 데이터를 잃는다. 제목 없는 이미지(`![a](url)`)는 이미지 속성 검증(`validate: "string"`)에 걸려 버려지므로 remark 플러그인이 `title`을 `''`로 채운다. ImageBlock 기능은 alt에 크기 비율을 써넣으므로 쓰지 않는다. GFM의 단일 `~` 취소선(`4~5 … 7~8`)은 끈다(`singleTilde: false`) |
| 기능 | `CrepeBuilder`로 블록 편집(`/` 메뉴, 손잡이), 선택 툴바, 링크 툴팁, 목록, 표, 코드 블록, 커서, 자리표시 글만 넣는다. 수식, AI, TopBar, ImageBlock은 넣지 않는다 |
| 자동 저장 | 마지막 변경 후 1초 뒤 저장한다. 저장 중에 또 바뀌면 끝난 뒤 한 번 더 보낸다. 저장 요청은 크기가 맞으면 늘 `keepalive`로 보내 떠나도 끝까지 간다. 탭을 숨기거나 떠날 때 마지막 요청을 보낸다. 떠나기 전에 브라우저가 묻는 경우: 저장이 오가는 중에 새로 바뀐 글이 있을 때, 글이 `keepalive` 한도보다 클 때, 마지막 저장이 실패했거나 멈췄을 때(5xx, 네트워크, 로그아웃, 404, 409), 파일을 올리는 중일 때. 결과를 모르는 저장(네트워크 오류, 5xx)은 다음에 같은 내용을 같은 버전으로 다시 보낸 뒤 새 글을 보낸다. 화면 HTML은 `Cache-Control: no-store`로 보내 뒤로 가기가 옛 버전을 보이지 않는다 |
| 충돌 | 저장 요청은 읽은 버전(`pages.updated_at`)을 같이 보낸다. 그새 다른 곳에서 저장됐으면 409이고, 편집기는 자동 저장을 멈추고 새로고침하라고 보인다. 버전이 다르더라도 페이지가 보낸 제목과 본문을 이미 갖고 있으면(이미 들어간 저장을 다시 보낸 경우) 204와 지금 버전으로 답한다. 휴지통에 있으면 404다 |
| 제목 | 화면 맨 위 `<h1>` 안의 입력칸. 본문과 같은 요청으로 자동 저장한다. 비어 있는 동안은 마지막으로 저장된 제목으로 본문만 저장하고 상태에 "Saved. Enter a title."를 보인다 |
| 하위 페이지 | `[[`나 `/page`로 만들면 그 자리에서 서버에 만들고 링크를 넣는다. 만드는 동안 이어 쓴 글은 그대로 둔다. 고르기 창은 지금 편집하는 페이지를 내놓지 않는다. 서버의 `[[제목]]` 변환은 없앤다 |
| 링크 글자 | 링크를 넣을 때의 제목이다. 페이지 이름을 바꿔도 링크 글자는 그대로다 |
| 지운 이미지 | 본문에서 지워도 폴더에 남긴다. 되돌리기(Cmd+Z)를 해도 깨지지 않게 하기 위해서다. 치우려면 드라이브에서 지운다 |
| 본문 제목 | 페이지 제목이 `<h1>`이므로 본문의 `#`는 `<h2>`로, 한 단계씩 낮춰 그린다(`######`도 `<h6>`). 마크다운은 그대로이고, 편집기 안에서 복사해 붙여도 단계가 바뀌지 않도록 `data-level`을 함께 쓴다 |
| 수정 화면 | 페이지 수정 화면, 할 일 수정 화면, 프로젝트 Edit 모드를 없앤다. 본문 편집은 JS가 있어야 한다 |
| 주소 | 편집기는 저장과 업로드 주소를 화면의 `data-` 속성에서 읽는다. 페이지는 `/page/pages/{id}/…`, 할 일은 `/task/tasks/{id}/…`, 프로젝트는 `/task/projects/{id}/…`로 보내고, 각 핸들러가 지금의 규칙(프로젝트 이름 중복, 폴더 이름, 할 일 폴더 위치)을 쓴다 |
| 응답 | JSON은 쓰지 않는다. 자동 저장은 204(새 버전은 `Page-Version` 헤더), 이미지 업로드는 201과 `Location`, 오류는 상태 코드와 짧은 글자다. 고르기 창은 `GET /page/links`의 HTML에서 목록을 읽는다 |

## 화면

### 공통 편집 영역 (`_editor.html`)

- `<h1>` 안의 제목 입력칸(`data-editor-title`)과 저장 상태(`<p role="status">`).
- `data-editor` 요소: `data-content-url`, `data-images-url`, `data-page-id`(새 하위 페이지의 부모), `data-version`. 안에 본문을 담은 숨은 `<textarea>`와 편집기가 붙을 자리.
- 데이터는 page 패키지의 `Editor` 값이다. 페이지, 할 일, 프로젝트 핸들러가 채운다.
- 상태 글: 저장 중 "Saving…", 저장 뒤 "Saved", 422는 서버가 준 글.
  - 5xx와 네트워크 실패는 "Couldn't save. Retrying…"이고 5초마다 다시 보낸다.
  - 세션이 끝나 로그인 화면으로 보내지면 "Signed out. Sign in in another tab to keep saving."이고 5초마다 다시 보낸다. 다른 탭에서 로그인하면 이어서 저장된다.
  - 멈추는 경우: 409는 "This page changed elsewhere. Reload to keep editing.", 404는 "This page was deleted elsewhere. Copy any text you need before leaving.", 그 밖의 4xx는 "Couldn't save. Reload to keep editing."
  - 업로드가 세션이 끝나 실패하면 "Signed out. Sign in in another tab, then add <파일> again."이고, 4xx는 서버가 준 글이다.

### 페이지 화면 (`page_detail.html`)

경로, 제목 입력칸, Delete, 편집기, Pages·Linked from, Move(Parent 선택칸과 Move 버튼, 실패하면 422로 같은 화면에 문구), Attachments와 Attach files 폼. 수정 화면은 없다.

### 할 일 화면 (`task_detail.html`)

경로, 제목 입력칸, Delete, 칸 묶음(Project, Status, Due, Completed), 편집기, Pages·Linked from, Attachments와 Attach files 폼. 칸 묶음은 바꾸는 즉시 `POST /task/tasks/{id}/fields`(`next`는 이 화면)로 저장한다. htmx로 보내 응답에서 칸 아래의 상태 영역(Completed, 오류 문구)과 경로만 바꿔 끼우므로 편집기와 입력 중인 칸, 포커스는 그대로다. JS가 없으면 `<noscript>` Save 버튼으로 보낸다.

### 프로젝트 화면 (`project_detail.html`)

경로, 제목 입력칸, Delete, 칸 묶음(Status, Started, Finished), 편집기, Pages·Linked from, Open tasks, Attachments와 Attach files 폼. 칸 묶음은 `POST /task/projects/{id}/fields`로 할 일과 같은 방식으로 저장한다(상태 영역만 바꿔 끼운다). 거절되면 422로 입력한 상태와 날짜를 그대로 다시 보인다.

## 편집기 동작

- **`[[` 고르기 창**: 커서 앞이 `[[글자`(같은 줄, `[`·`]` 없음)면 커서 아래에 창이 뜬다. `/page/links?q=글자`를 150ms 뒤에 불러 제목과 경로를 보인다. 첫 항목은 "Create subpage ‘글자’"(글자가 있을 때). 화살표, Enter, Esc, 마우스로 고른다. 기존 페이지를 고르면 `[[글자`를 그 페이지 제목의 링크로 바꾼다. 새로 만들기는 `POST /page/pages/new`(`parent`, `title`)를 보내고, 따라간 주소의 페이지로 링크를 넣는다. Esc를 누르면 창을 닫고, 커서가 그 자리를 벗어날 때까지 다시 열지 않는다.
- **`/page`**: `/` 메뉴의 Pages → Page는 그 줄의 `/…`를 지우고 `[[`를 넣어 고르기 창을 연다.
- **이미지와 파일**: 붙여넣거나 끌어다 놓으면 업로드 자리표시를 보이며 파일마다 `…/images`로 올린다. 201이면 이미지는 `![이름](주소/content)`, 그 밖의 파일은 `[이름](주소)` 링크로 넣는다. 실패하면 자리표시를 지우고 상태에 서버의 글을 보인다. 붙여넣은 이미지의 이름은 `pasted-<밀리초>.<확장자>`다.
- **링크 누르기**: `/page/pages/`, `/task/` 주소의 링크는 한 번 누르면 남은 저장을 보낸 뒤 그리로 간다. 그 밖의 링크는 Cmd(Ctrl)+클릭으로 새 탭에서 연다.
- 빈 문서의 자리표시 글은 "Type / for commands"다.
- 색과 글꼴은 앱의 CSS 변수(`--bg`, `--text`, `--accent`, `--font` 등)를 따른다. 다크 모드는 그 변수가 바뀌는 것으로 따라간다.

## 서버

### 라우트

| 라우트 | 동작 |
|---|---|
| `POST /page/pages/new` | `title`, `parent`(비면 최상위). 303으로 새 페이지. 부모가 없거나 휴지통에 있으면 422 |
| `POST /page/pages/{id}/content` | `title`, `body`, `version`. 204와 `Page-Version`. 버전이 다르면 409, 빈 제목은 422, 버전 형식이 틀리면 400 |
| `POST /page/pages/{id}/images` | 파일(`files`). 201과 `Location: /drive/files/{fid}`. 이름이 겹치는 등 파일 문제는 422 |
| `POST /page/pages/{id}/move` | `parent`. 303으로 페이지 화면. 자신이나 하위 페이지, 휴지통의 페이지면 422 |
| `POST /page/pages/{id}/files` | Attach files 폼. 303으로 페이지 화면 |
| `GET /page/links` | 쿼리 `q`. 검색 결과(비면 최근에 고친 페이지) 10개를 `<ul id="links">`의 `<li><a href="주소" data-title="제목">제목</a> <small>경로</small></li>`로 그린 페이지 |
| `POST /task/tasks/{id}/content`, `/images` | 할 일 페이지의 같은 동작. 제목 규칙은 `TaskInput.Clean`과 같다 |
| `POST /task/projects/{id}/content`, `/images`, `/fields` | 프로젝트의 같은 동작. 이름 규칙은 `drive.CleanName`, 이름 중복은 422. `/fields`는 `status`, `started`, `finished`로 303 |

없어지는 라우트: `GET/POST /page/pages/{id}/edit`, `GET/POST /task/tasks/{id}/edit`, `GET/POST /task/projects/{id}/edit`.

### page 패키지

- `Page`에 `UpdatedAt`이 생긴다. 버전은 이 값을 `FormatVersion`(UTC, RFC3339Nano)으로 쓰고 `ParseVersion`으로 읽는다.
- `SaveContent(ctx, id, title, body, version) (time.Time, error)`: 한 트랜잭션에서 `UPDATE pages SET title, body, updated_at = now() WHERE id = $1 AND updated_at = $version AND trashed_at IS NULL RETURNING updated_at`, 그리고 `page_links`를 다시 쓴다. 바뀐 행이 없으면 페이지가 있으면 `ErrStale`, 없으면 `ErrNotFound`. 프로젝트 이름 중복은 `ErrTitleTaken`. 주인 없는 페이지는 제목이 바뀌면 폴더 이름을 바꾼다.
- `Create(ctx, parent, title)`: 부모가 0이 아니면 보이는 페이지여야 한다(`ErrInvalidParent`).
- `Move(ctx, id, parent)`: 주인 있는 페이지는 옮길 수 없다. 부모 검사는 지금의 이동 규칙과 같다. `updated_at`은 바꾸지 않아 편집 중인 편집기와 부딪히지 않는다.
- `Save`, `[[ ]]` 변환(`subpage.go`), 저장 시점 업로드와 본문에서 빠진 파일 휴지통 처리(`attachLinked`, `usedUploads`, `linkFiles`, `droppedFiles`, `trashDropped`), 수정 화면을 지운다.

### task 패키지

- `Task`, `Project`에 페이지 버전(`PageUpdatedAt`)이 생긴다.
- `SaveTaskContent(ctx, id, title, body, version)`: 제목 규칙 → `pages.SaveContent` → 제목이 바뀌었고 폴더가 있으면 폴더 이름 변경.
- `SaveProjectContent(ctx, id, name, body, version)`: 이름 규칙 → 이름 중복 → 버전 확인(다르면 `page.ErrStale`) → 이름이 바뀌었고 폴더가 있으면 폴더 이름 변경(`ErrFolderTaken`) → `pages.SaveContent`.
- `UpdateTaskFields(ctx, id, project, status, due)`: 지금 목록의 `/fields`가 하던 일(프로젝트를 바꾸면 폴더 이동). `UpdateTask`와 `TaskInput.Body`는 지운다.
- `UpdateProjectFields(ctx, id, status, started, finished)`. `UpdateProject`와 `ProjectInput.Body`는 지운다.
- 첨부 폼이 성공하면 상세 화면으로 303.

## CLAUDE.md 개정

- 3절: `web/`에 편집기 빌드(`web/editor/`)를 적는다.
- 3.3.5: 편집기가 읽는 `GET /page/links`는 HTML이다. JSON API 금지는 그대로다.
- 3.3.7: 예외로 자동 저장(`…/content`)은 204(충돌 409), 편집기의 업로드(`…/images`)는 201과 `Location`으로 답한다.
- 3.6.6: 본문 편집(페이지, 프로젝트, 할 일)은 Milkdown Crepe 편집기로 하고 자동 저장한다. 이 편집은 JS가 필요하다. 편집기는 `web/editor/`에서 Node와 esbuild로 빌드하고 결과를 커밋한다. Node는 이 빌드에만 쓴다.
- 8.8: 편집기의 npm 의존성(`@milkdown/crepe`, `@milkdown/kit`, `unist-util-visit`, `esbuild`)은 버전을 고정해 쓴다.
- `make editor`가 편집기를 빌드한다. air는 `node_modules`를 보지 않는다.

## 테스트

**단위 테스트**: `ParseVersion`(정상, 잘못된 값). 지운 함수의 테스트는 함께 지운다.

**HTTP 통합 테스트**

- 페이지 자동 저장: 204와 `Page-Version`, 화면에 본문이 보인다. 옛 버전은 409이고 내용이 그대로다. 빈 제목은 422.
- 자동 저장한 본문의 링크가 그 페이지의 Linked from에 보인다.
- `POST /page/pages/new`(`parent`, `title`)는 303이고 새 페이지의 경로가 부모를 지난다.
- `…/images`는 201과 `Location`, 파일은 페이지는 `Pages/<제목> (#id)`, 할 일은 `Projects/<p>/Tasks/<t>`에 있다.
- `/page/links?q=`에 페이지와 프로젝트 페이지가 나온다.
- Move: 다른 페이지 아래로 옮기면 303이고 경로가 바뀐다. 자기 하위 페이지 아래는 422.
- 할 일 제목 자동 저장은 폴더 이름을 바꾼다. 프로젝트 이름 자동 저장은 폴더 이름을 바꾸고, 다른 프로젝트 이름이면 422.
- 프로젝트 `/fields`로 상태가 바뀐다. 할 일 `/fields`는 지금 테스트 그대로다.

**편집기 확인** (Go 테스트 밖, 구현 때와 편집기 버전을 올릴 때마다)

- headless Chrome에서 빌드한 편집기로 실제 본문들의 마크다운 왕복을 하고, goldmark로 그린 HTML이 원본과 같은지 본다.
- 실제 화면에서 자동 저장, `[[` 고르기 창, `/page`, 링크 이동을 Chrome DevTools Protocol로 입력해 확인한다.
- 한글 입력은 사람이 직접 확인한다.

## 범위 밖

- 실시간 공동 편집, 여러 탭 병합(409와 새로고침 안내만)
- 이전 버전, 링크 글자 자동 갱신, 지운 이미지 자동 정리
- 이미지 크기 조절, 수식, AI 기능, 페이지 아이콘·표지, 링크 칩, 댓글, 오프라인 편집
- 사이드바 페이지 트리, 끌어서 페이지 옮기기
- 편집기 파일 크기 최적화(쓰지 않는 기능을 빼는 것까지만)

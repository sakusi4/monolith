# 드라이브 (파일 저장소)

이 문서는 드라이브 기능의 설계다. 구현을 바꾸면 이 문서도 같이 고친다.

## 목적

이 앱은 장기적으로 노션을 포함한 생산성 도구를 모두 대체한다. 그 바닥으로 모든 종류의 파일을 폴더 트리에 보관하는 드라이브를 만든다. 이후의 프로젝트·할 일(B), 메모(C), 노션 이전(D), 미디어(E)가 이 저장소를 쓴다. 이번 범위는 드라이브 자체와 그 화면(사이드바 **Drive**, `/drive`)이다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 정리 방식 | 폴더 트리. 폴더와 파일은 폴더 하나에 속하고, 최상위는 `NULL`이다. 한 파일을 여러 곳에 연결하는 구조와 태그는 두지 않는다 |
| 저장 위치 | 메타데이터는 PostgreSQL, 파일 내용은 `FILES_DIR` 디스크에 SHA-256 이름으로 둔다. 실행 환경(NAS, 클라우드)이 정해지지 않았으므로 내용을 다루는 코드를 한 타입(`blob.go`)에 모아 나중에 S3로 바꿀 수 있게 한다 |
| 중복 | 같은 내용은 디스크에 한 번만 둔다. 파일 행 여러 개가 같은 내용(`blobs`)을 가리킬 수 있다. 이름 변경과 이동은 디스크를 건드리지 않는다 |
| 이름 | 앞뒤 공백을 지우고 유니코드를 NFC로 합친 뒤(macOS가 보내는 분해된 한글을 같은 이름으로 보기 위해, `golang.org/x/text/unicode/norm`) 비어 있지 않고, `/`가 없고, 255자 이하다 |
| 이름 중복 | 휴지통 밖에서 같은 폴더 안의 파일끼리, 폴더끼리 이름이 겹치지 않는다. 파일과 폴더는 같은 이름이어도 된다. 겹치면 422다 |
| 삭제 | 휴지통으로 옮긴다. 폴더를 지우면 그 안의 모든 것이 함께 숨는다. 디스크에서는 영구 삭제와 휴지통 비우기에서만 지운다 |
| 크기 | 업로드 요청 하나는 `DRIVE_MAX_UPLOAD_GB`(기본 10 GiB) 이하다. 업로드와 내용 전송 요청만 서버의 읽기·쓰기 제한 시간을 늘린다 |
| 형식 | 확장자로 `mime.TypeByExtension`, 실패하면 앞 512바이트로 `http.DetectContentType`. 둘 다 없으면 `application/octet-stream` |
| 미리보기 | 이미지는 `<img>`, 영상·음성은 브라우저 내장 플레이어, PDF는 브라우저 뷰어, 텍스트(`text/*`, `application/json`)는 1 MiB 이하만 `<pre>`로 보인다. 나머지는 다운로드만 |
| 보안 | 브라우저에서 스크립트를 실행할 수 있는 형식(`text/html`, `text/xml`, `application/xml`, 그리고 `image/svg+xml`처럼 `+xml`로 끝나는 모든 형식)과 해석할 수 없는 형식은 내용 주소에서도 항상 `attachment`로 준다. 모든 내용 응답에 `X-Content-Type-Options: nosniff`를 단다 |
| 백업 | DB 덤프와 `FILES_DIR` 복사. 디스크의 파일 내용은 쓴 뒤 바뀌지 않으므로 `rsync`가 새 파일만 옮긴다 |

## 데이터 모델: `0010.sql`

```sql
CREATE TABLE folders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id bigint REFERENCES folders (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> '' AND strpos(name, '/') = 0 AND char_length(name) <= 255),
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX folders_name_idx ON folders (parent_id, name) NULLS NOT DISTINCT WHERE trashed_at IS NULL;
CREATE INDEX folders_parent_id_idx ON folders (parent_id);

CREATE TABLE blobs (
    sha256 bytea PRIMARY KEY CHECK (length(sha256) = 32),
    size bigint NOT NULL CHECK (size >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE files (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    folder_id bigint REFERENCES folders (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> '' AND strpos(name, '/') = 0 AND char_length(name) <= 255),
    sha256 bytea NOT NULL REFERENCES blobs (sha256),
    content_type text NOT NULL,
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX files_name_idx ON files (folder_id, name) NULLS NOT DISTINCT WHERE trashed_at IS NULL;
CREATE INDEX files_folder_id_idx ON files (folder_id);
CREATE INDEX files_sha256_idx ON files (sha256);
```

- `trashed_at`은 사용자가 지운 항목에만 기록한다. 폴더나 그 조상 중 하나에 `trashed_at`이 있으면 그 폴더 아래는 보이지 않는다. 숨은 폴더와 파일의 화면은 404다.
- 조상과 하위 트리는 `WITH RECURSIVE`로 한 번에 읽는다(반복문 안에서 쿼리하지 않는다).
- 폴더 목록은 최상위면 `IS NULL`, 아니면 `= $1`로 읽어 이름 인덱스를 쓴다(`IS NOT DISTINCT FROM`은 인덱스를 쓰지 못한다). `parent_id`, `folder_id`의 일반 인덱스는 영구 삭제의 연쇄 삭제가 휴지통 안의 행까지 찾을 때 쓴다.
- 영구 삭제는 `ON DELETE CASCADE`로 하위 트리를 함께 지운다. `blobs`는 연쇄 삭제하지 않고 아래의 정리 단계에서 지운다.

## 디스크 구조

```
FILES_DIR/blobs/ab/cd/abcdef…   SHA-256 hex 64자. 앞 2자, 다음 2자로 디렉터리를 나눈다
FILES_DIR/tmp/                  업로드 중인 임시 파일. blobs와 같은 파일 시스템이라 rename이 원자적이다
```

`run`이 시작할 때 두 디렉터리를 만들고, 앞선 실행이 끝내지 못한 업로드가 남은 `tmp`를 비운다. 그래서 `FILES_DIR`는 프로세스 하나만 쓴다. `FILES_DIR`의 기본값은 `files_data`이고 `.gitignore`와 air의 제외 목록에 넣는다.

## 업로드

1. `POST /drive/files/new?folder={id}`의 multipart 본문을 `r.MultipartReader`로 파일 하나씩 읽는다. 요청 본문 전체는 `http.MaxBytesReader`로 제한한다.
2. 파일마다 `tmp`에 쓰면서 SHA-256과 크기를 함께 계산하고, 다 쓰면 `fsync`한다. 전원이 끊겨도 반쯤 쓴 내용이 완전한 내용으로 남아 이후의 같은 내용이 그것을 재사용하는 일이 없게 하기 위해서다. 형식은 위의 규칙으로 정한다.
3. 뮤텍스를 잡고, 같은 해시의 내용이 디스크에 있으면 임시 파일을 지우고, 없으면 `blobs/…`로 rename한다. 이번 요청이 새로 만든 내용 파일을 기억한다.
4. 요청의 모든 파일을 트랜잭션 하나로 기록한다: `blobs`에 `ON CONFLICT DO NOTHING`으로 넣고 `files`에 넣는다. 하나라도 실패하면(이름 중복 등) 아무것도 기록하지 않고, 이번 요청이 새로 만든 내용 파일을 지운 뒤 422로 폴더 화면을 다시 그린다.
5. 성공하면 그 폴더 화면으로 303.

## 영구 삭제와 정리

1. 뮤텍스를 잡는다. 트랜잭션 안에서 지울 항목의 하위 트리에 있는 모든 파일의 해시를 모은다.
2. 폴더나 파일 행을 지운다(하위는 연쇄 삭제).
3. 모은 해시 중 어떤 `files` 행도 가리키지 않는 `blobs` 행을 지우고 그 해시를 돌려받는다.
4. 커밋한 뒤 돌려받은 해시의 디스크 파일을 지운다.

- 업로드의 3~4단계와 이 과정은 같은 뮤텍스(프로세스 하나)로 직렬화한다. 그래서 지우는 내용과 같은 내용이 동시에 올라와도 방금 올라온 파일을 지우지 않는다.
- 커밋과 디스크 삭제 사이에 프로세스가 죽으면 참조 없는 파일이 디스크에 남는다. 동작에는 영향이 없고 공간만 차지한다.

## 내용 전송

- `GET /drive/files/{id}/content`는 `http.ServeContent`로 준다. Range 요청(영상 탐색)과 조건부 요청을 지원하고, ETag는 SHA-256 hex다.
- `GET /drive/files/{id}/download`는 같은 내용을 `Content-Disposition: attachment`와 파일 이름으로 준다.
- 스크립트를 실행할 수 있는 형식은 `content`에서도 `attachment`다.

## 화면

모든 라우트는 로그인이 필요하다(`auth.Require`). 사이드바의 최상위에 Dashboard 다음으로 Drive 링크를 둔다. `{id}`는 폴더나 파일의 id이고, 없거나 휴지통 안에 숨은 항목이면 404다.

| 라우트 | 동작 |
|---|---|
| `GET /drive` | 최상위 폴더 |
| `GET /drive/folders/{id}` | 폴더 |
| `POST /drive/folders/new?folder={id}` | 그 폴더(없으면 최상위)에 새 폴더. 폼 값 `name` |
| `POST /drive/files/new?folder={id}` | 그 폴더(없으면 최상위)에 업로드. 여러 파일 |
| `GET /drive/folders/{id}/edit`, `GET /drive/files/{id}/edit` | 부모 폴더 화면에서 그 행만 이름·위치 입력칸으로 바꾼다 |
| `POST /drive/folders/{id}/edit`, `POST /drive/files/{id}/edit` | 이름과 위치 저장. 폼 값 `name`, `folder`(비면 최상위) |
| `POST /drive/folders/{id}/delete`, `POST /drive/files/{id}/delete` | 휴지통으로 |
| `GET /drive/files/{id}` | 파일 화면 |
| `GET /drive/files/{id}/content`, `GET /drive/files/{id}/download` | 내용 |
| `GET /drive/trash` | 휴지통 |
| `POST /drive/trash/folders/{id}/restore`, `POST /drive/trash/files/{id}/restore` | 복원 |
| `POST /drive/trash/folders/{id}/delete`, `POST /drive/trash/files/{id}/delete` | 영구 삭제 |
| `POST /drive/trash/empty` | 휴지통 비우기 |

POST가 성공하면 그 항목이 있는(또는 있던) 폴더 화면으로 303을 보낸다. 휴지통의 동작은 휴지통으로 돌아간다.

### 폴더 화면 (`drive_folder.html`)

1. 제목은 폴더 이름(최상위는 "Drive"), 그 위에 경로 링크(Drive / Projects / tunnel).
2. 새 폴더 폼(이름)과 업로드 폼(`<input type="file" multiple>`).
3. 표: Name, Kind, Size, Modified, Edit·Delete. 폴더가 먼저, 그 안에서 이름순(대소문자 무시). Kind는 Folder, Image, Video, Audio, PDF, Text, File 중 하나다. Size는 파일만 1024 단위로 `12.3 MB`처럼 보이고, Modified는 `TIMEZONE` 기준 날짜다.
   - 폴더 이름은 그 폴더로, 파일 이름은 파일 화면으로 가는 링크다.
   - 수정하면 그 행이 이름 입력칸과 위치 선택으로 바뀐다. 위치는 휴지통 밖의 모든 폴더를 경로(`Projects / tunnel`)로 보이고, 폴더를 옮길 때는 자기 자신과 하위 폴더를 빼고 보인다. 서버도 같은 규칙을 검사한다.
4. 비어 있으면 "This folder is empty."

### 파일 화면 (`drive_file.html`)

경로 링크, 제목(파일 이름), 미리보기, 정보(Kind, 형식, Size, Modified), Download 링크, Rename·Move(부모 폴더 화면의 수정 행으로), Delete. 1 MiB보다 큰 텍스트는 "Too large to preview."와 다운로드만 보인다.

### 휴지통 (`drive_trash.html`)

지운 항목(`trashed_at`이 있는 폴더와 파일)을 지운 시각의 최신순으로 보인다. 열은 Name, Kind, Location(원래 경로), Deleted, Restore·Delete forever. 위에 "Empty trash"(확인 창). 비어 있으면 "Trash is empty."

422가 되는 경우:

- 새 폴더, 업로드, 이름·위치 변경: 이름 규칙 위반, 같은 폴더의 이름 중복, 자기 자신이나 하위 폴더로 이동.
- 복원: 부모 폴더가 휴지통에 있음, 원래 폴더에 같은 이름이 생김.

htmx는 지출 화면과 같은 방식으로 폴더 화면과 휴지통에서만 불러온다. 폼과 표에 `hx-boost`를 건다. 파일 화면과 폴더 사이의 이동은 전체 페이지다.

## 설정

| 환경변수 | 기본값 | 뜻 |
|---|---|---|
| `FILES_DIR` | `files_data` | 파일 내용을 두는 디렉터리 |
| `DRIVE_MAX_UPLOAD_GB` | `10` | 업로드 요청 하나의 최대 크기(GiB) |

## 코드 배치

| 파일 | 내용 |
|---|---|
| `internal/postgres/migrations/0010.sql` | `folders`, `blobs`, `files` |
| `internal/drive/drive.go` | 패키지 문서, `Store`(`*sql.DB`와 내용 저장소) |
| `internal/drive/blob.go` | 내용 저장소: 해시→경로, 임시 쓰기, rename, 열기, 지우기, 뮤텍스. S3로 바꿀 때 이 파일만 바뀐다 |
| `internal/drive/folder.go` | `Folder`, 이름 규칙(`cleanName`), 폴더 만들기·이름·이동(순환 검사), 경로(조상 목록) |
| `internal/drive/file.go` | `File`, 형식 정하기, 미리보기 종류(`Kind`), 강제 다운로드 여부, 업로드·이름·이동 |
| `internal/drive/trash.go` | 휴지통 목록, 휴지통으로, 복원, 영구 삭제, 비우기, 내용 정리 |
| `internal/drive/handler.go` | `NewHandler`와 라우트, 공통 도우미(폴더 확인, 에러를 응답으로, 주소, 경로 링크) |
| `internal/drive/folder_handler.go` | 폴더 화면, 새 폴더, 폴더 이름·이동, 폴더를 휴지통으로 |
| `internal/drive/file_handler.go` | 업로드, 파일 화면, 내용·다운로드, 파일 이름·이동, 파일을 휴지통으로 |
| `internal/drive/trash_handler.go` | 휴지통 화면, 복원, 영구 삭제, 비우기 |
| `web/templates/drive_folder.html`, `drive_file.html`, `drive_trash.html` | 화면 |
| `cmd/server/config.go`, `main.go`, `routes.go` | 설정, 디렉터리 만들기, `/drive/` 마운트 |
| `web/templates/layout.html` | 사이드바 링크 |

- CLAUDE.md 3절의 패키지 목록에 `internal/drive/`를 넣는다.
- 프로젝트(B)는 나중에 드라이브 폴더 하나를 가리킨다. 드라이브는 프로젝트를 모른다.

## 테스트

CLAUDE.md 9절을 따른다. 한 동작은 한 계층에서만 확인한다.

**단위 테스트** (DB 없음)

- `cleanName`: 앞뒤 공백 제거, 분해된 한글의 NFC 합성, 빈 이름, `/` 포함, 255자와 256자
- 형식 정하기: 표준 확장자(`.png`), 확장자가 없을 때 내용 판별, 둘 다 실패하면 `application/octet-stream`
- `Kind`: 이미지, 영상, 음성, PDF, 텍스트, 그 밖
- 강제 다운로드: 목록의 형식(`text/html`), `+xml` 형식(`application/rss+xml`), 해석할 수 없는 형식은 attachment, `image/png`는 아님
- 내용 저장소를 열 때 `tmp`에 남은 업로드를 지운다
- 해시→경로: `ab/cd/abcd…`
- 크기 표시: 바이트, KB 경계, GB

**HTTP 통합 테스트** (`cmd/server/drive_test.go`의 `TestDrive`, 실제 DB와 `t.TempDir()`)

- 중첩 폴더를 만들면 경로 링크가 보이고, 업로드한 파일을 내려받으면 같은 바이트다. Range 요청은 206과 그 구간이다.
- 같은 내용을 두 이름으로 올리면 디스크의 내용 파일은 하나다.
- 같은 폴더의 같은 이름은 422이고, 여러 파일 업로드 중 하나가 실패하면 아무것도 저장되지 않는다.
- 이름 변경과 이동이 되고, 폴더를 자기 하위로 옮기면 422다.
- 폴더를 휴지통으로 옮기면 그 안의 파일 화면이 404이고, 복원하면 다시 보인다.
- 휴지통 안 폴더의 파일은 복원하면 422이고, 휴지통 안 폴더로 옮기면 404다.
- 영구 삭제는 다른 파일이 같은 내용을 가리키면 디스크 파일을 남기고, 마지막 참조였으면 지운다.
- 서버의 쓰기 제한 시간이 지나도 업로드는 303으로 끝난다(`httptest` 서버에 `WriteTimeout`을 아주 짧게 둔다).
- HTML 파일의 `content`는 `attachment`다.

브라우저의 미리보기(영상 재생, PDF 뷰어)는 자동 테스트가 없고, 구현 때 headless 브라우저로 직접 확인한다.

## 범위 밖

- 검색, 정렬 선택, 페이지 나누기(미디어 E와 함께)
- 폴더째 업로드, 드래그 앤 드롭, 덮어쓰기와 버전 보관
- 공유 링크, 여러 곳에 한 파일 연결, 태그
- 마크다운 렌더링(메모 C와 함께)
- 참조 없는 디스크 파일을 찾아 지우는 정기 작업
- S3 등 다른 저장소(실행 환경이 정해지면)

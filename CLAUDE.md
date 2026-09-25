# monolith 코딩 규약 (Go)

이 문서는 Go 코드와 `web/`의 HTML 템플릿에 적용한다.

## 0. 우선순위와 기준

1. 표준이 이 문서보다 우선한다. 판단 순서는 `gofmt` → [Effective Go](https://go.dev/doc/effective_go) → [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) → [Google Go Style Guide](https://google.github.io/styleguide/go/) → 이 문서다.
2. 이 문서의 규칙이 표준과 충돌하면 표준을 따르고, 충돌을 알린 뒤 이 문서를 고친다.
3. 백엔드 구조의 기준 구현은 [Miniflux](https://github.com/miniflux/v2)다. 표준 `net/http` 라우팅, `database/sql`과 손으로 쓴 SQL, PostgreSQL, 실제 DB 통합 테스트, 단일 바이너리, 서버에서 `html/template`로 그리는 화면. 이 문서가 정하지 않은 백엔드 문제는 Miniflux가 푸는 방식을 따른다.
4. Miniflux가 표준이나 이 문서와 어긋나는 곳은 따르지 않는다: DB 호출에 `context`를 넘기지 않는 것, 조회 실패를 `nil, nil`로 반환하는 것, 전역 설정(`config.Opts`), `%v`로 에러를 감싸는 것, 5xx 응답에 내부 에러 문구를 싣는 것.

## 1. 주석

1. 주석은 최상위 선언 위의 doc comment로만 쓴다. 함수 본문 안에는 주석을 쓰지 않는다.
2. doc comment는 이름과 시그니처만으로 알 수 없는 계약이 있을 때만 단다. exported라는 이유로 달지 않는다. Go Code Review Comments는 모든 exported 이름에 doc comment를 요구하지만, 이 저장소는 남이 import하는 라이브러리가 아니므로 따르지 않는다. 이름을 다시 말하는 주석(`NewStore returns a Store`)은 쓰지 않는다.
3. doc comment는 영어 완전한 문장이다. 식별자 이름으로 시작하고 마침표로 끝난다. 형식은 [Go Doc Comments](https://go.dev/doc/comment)를 따른다.
4. doc comment는 한 문장이 기본이다. 호출자가 알아야 할 계약(반환하는 에러, 동시 호출 안전 여부)이 있을 때만 문장을 더한다. 구현 과정, 배경, 이유는 적지 않는다.
5. 설명을 달고 싶어지는 것은 코드가 단순하지 않다는 신호다. 주석으로 메우지 않고 8.3, 8.4를 따른다.
6. 주석 처리된 코드, `TODO`, `FIXME`를 남기지 않는다.
7. `//go:build`, `//go:embed`, `//go:generate`는 컴파일러 지시어이며 주석 규칙의 대상이 아니다.

## 2. 타입

1. `any`(`interface{}`)를 선언하지 않는다(파라미터, 필드, 반환 타입). 표준 라이브러리 시그니처를 호출하거나 구현할 때만 예외다(`rows.Scan`, `template.Execute`, `sql.Scanner`, 이를 감싸는 `web.Render`).
2. 패키지 경계를 넘는 데이터는 struct 또는 타입이 있는 slice/map이다. `map[string]any`, `[]any`를 넘기지 않는다.
3. 도메인 값을 문자열로 들고 다니지 않는다. 경계(HTTP 요청, DB 행, 설정)에서 한 번 파싱해 타입으로 바꾼다. 시각은 `time.Time`, 기간은 `time.Duration`, URL은 `*url.URL`, 통화는 `type Currency string`.
4. 고정된 값의 집합은 named type + `const` 블록으로 정의한다(`type TaskStatus string`). bare `string`/`int`로 받지 않는다. 이 타입에 대한 `switch`는 모든 값을 다룬다.
5. 데이터 struct는 exported 필드를 가진 plain struct다. getter/setter를 만들지 않는다. zero value가 곧바로 쓸 수 있는 상태가 되도록 설계하고, 그럴 수 없거나 검증이 필요한 타입에만 생성자(`NewX`)를 만든다.
6. 포인터는 변경하거나 복사하면 안 되는 값에 쓴다. "값 없음"은 zero value 또는 `(T, bool)`로 표현한다. DB의 NULL 컬럼은 `sql.Null[T]`로 읽는다.
7. 한 타입의 메서드 receiver는 포인터와 값 중 하나로 통일한다.
8. 돈, 수량, 환율에 `float32`/`float64`를 쓰지 않는다. 금액은 통화 최소 단위의 정수(`int64`, DB `bigint`. KRW는 원, USD는 센트)와 `Currency`의 쌍이다. 소수 자릿수가 정해지지 않은 수량과 환율은 DB `numeric`, Go `*big.Rat`이다. 통화가 USD 하나로 정해진 값은 통화를 생략하고 이름에 단위를 적는다(`AmountCents`, `amount_cents`).
9. 곱셈과 나눗셈이 들어가는 금액 계산(평가액, 환산)은 `math/big`으로 하고, 금액 정수로 되돌리는 반올림은 한 함수에서만 한다.
10. 시각은 DB `timestamptz`에 UTC로 저장한다. 날짜만 의미가 있는 값(마감일, 거래일)은 DB `date`다. "오늘", "자정", "월말"처럼 날짜 경계가 있는 계산은 설정으로 받은 `*time.Location`으로 한다. `time.Local`에 의존하지 않는다.

## 3. 패키지

```
cmd/server/          실행 파일. 조립만 한다.
internal/finance/    자산 스냅샷, 지출
internal/task/       목표, 프로젝트, 태스크
internal/note/       메모, 태그, 링크
internal/dashboard/  다른 기능 패키지를 읽어 대시보드를 만든다. 쓰기 없음.
internal/auth/       로그인, 세션, 인증 미들웨어
internal/postgres/   연결과 마이그레이션. 도메인 쿼리는 두지 않는다.
internal/money/      금액 파싱과 표시. 순수 함수만.
web/                 HTML 템플릿, 정적 파일(CSS), 렌더링. 모든 기능이 같이 쓴다.
```

`go.mod`의 `go` 지시어는 1.25 이상이다(`http.CrossOriginProtection`, `testing/synctest`).

### 3.1 패키지 설계

1. 패키지는 제공하는 기능으로 나눈다. 패키지명은 안에 무엇이 들었는지가 아니라 무엇을 해주는지를 말한다(`finance`, `note`). `controllers`, `handlers`, `services`, `repository`, `models`, `domain`, `types`, `utils`, `common`, `helpers` 패키지를 만들지 않는다.
2. 기능 패키지 하나가 자기 도메인의 타입, SQL, HTTP 핸들러를 모두 가진다. 계층(핸들러 → 서비스 → 리포지토리)으로 패키지를 나누지 않는다.
3. 패키지는 나누지 않지만 패키지 안에서 역할은 나눈다. 계산과 검증 규칙은 DB와 HTTP를 모르는 순수 함수나 메서드다(`Transaction.Validate()`). SQL과 트랜잭션은 `Store` 메서드이고 규칙을 호출만 한다. HTTP는 `handler`다. 규칙은 DB 없이 단위 테스트한다.
4. 모든 코드는 `internal/` 아래에 둔다(`web/` 제외). `pkg/`를 만들지 않는다.
5. 기본은 unexported다. 다른 패키지가 실제로 쓰는 것만 export한다.
6. 의존성은 생성자 함수의 인자로 받는다. 패키지 수준 가변 상태, `init()`, 싱글턴, DI 컨테이너(wire, fx, dig)를 쓰지 않는다. 에러 값이나 컴파일된 정규식 같은 불변 값은 패키지 수준에 둘 수 있다. `slog`의 기본 로거만 예외다(6.1).
7. 인터페이스는 구현하는 쪽이 아니라 사용하는 쪽 패키지에 정의한다. 함수는 인터페이스를 받고 구체 타입을 반환한다.
8. 인터페이스는 구현체가 둘 이상이거나, 테스트에서 외부 경계(외부 HTTP API)를 대체해야 할 때만 만든다. DB는 인터페이스로 가리지 않는다(9.9). 시간은 인터페이스로 추상화하지 않는다(9.8). 메서드는 최소한으로 둔다.
9. 기능 패키지는 자기 테이블만 쿼리한다. 다른 기능의 데이터는 그 패키지의 exported 메서드로 읽는다. 여러 기능의 테이블을 JOIN해야 한다면 먼저 물어본다.
10. `main` 밖의 패키지는 `fmt.Print*`, `log.*`로 출력하지 않는다. 로그는 `slog`로만 쓴다(6.1).

### 3.2 `main`

1. `main` 패키지는 조립만 한다: 설정을 읽고, DB를 열어 마이그레이션을 적용하고, 핸들러를 조립해 서버와 주기 작업을 시작하고, 반환된 에러를 종료 코드로 바꾼다. 분기와 로직은 `internal` 패키지에 둔다.
2. `main()`은 `run(ctx) error`를 호출하고, 에러를 stderr에 쓴 뒤 `os.Exit(1)`하는 일만 한다. `ctx`는 `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`로 만들고, 취소되면 `srv.Shutdown`으로 끝낸다.
3. 설정은 환경변수로만 받는다(`DATABASE_URL`, `LISTEN_ADDR`, `TIMEZONE` 등). `run`에서 한 번 읽어 타입이 있는 struct로 만들어 아래로 넘긴다. 다른 패키지에서 `os.Getenv`, `flag`를 호출하지 않는다. 테스트 헬퍼 `postgrestest`가 `TEST_DATABASE_URL`을 읽는 것만 예외다.
4. `os.Exit`, `log.Fatal`은 여기서만 호출한다.
5. 라우트 마운트(기능 패키지의 핸들러를 경로 prefix에 붙이고 미들웨어를 씌우는 일)는 `cmd/server/routes.go` 한 파일에서 한다. 앱의 전체 URL이 이 파일에서 보인다.

### 3.3 HTTP

1. 라우팅은 표준 `http.ServeMux`의 메서드+경로 패턴으로 한다: `"GET /finance/assets/{id}/edit"`. 라우터 라이브러리(chi, gin, echo, gorilla/mux)를 쓰지 않는다. 경로 인자는 `r.PathValue`로 읽는다.
2. 핸들러는 `http.Handler` 또는 `http.HandlerFunc`, 미들웨어는 `func(http.Handler) http.Handler`다. 프레임워크식 context 타입을 만들지 않는다.
3. 기능 패키지는 `NewHandler(...) http.Handler` 하나를 export하고, 그 함수 안에서 자기 라우트를 모두 등록한다. 핸들러는 의존성을 필드로 가진 unexported `handler` 타입의 메서드다(Miniflux `api.NewHandler`).
4. 경로는 `/{패키지명}/`으로 시작한다. 패키지명이 곧 마운트 prefix다: `/finance/`, `/task/`, `/auth/`.
5. 화면은 서버에서 HTML로 그린다(3.6). JSON API를 만들지 않는다. 앱 밖의 클라이언트가 필요해지면 먼저 묻는다.
6. 폼은 HTML `<form>`의 GET과 POST만 쓴다. 화면과 그 제출은 같은 URL이다: `GET /finance/assets/new`가 폼을 보여 주고 `POST /finance/assets/new`가 저장한다. 수정은 `/{id}/edit`, 삭제는 `POST /{id}/delete`다.
7. POST가 성공하면 `303 See Other`로 GET 화면(목록 등)에 보낸다(Post/Redirect/Get). 성공한 POST 응답에 HTML을 바로 그리지 않는다.
8. 폼 값은 `r.PostFormValue`로 읽고, 경계에서 한 번 파싱해 도메인 타입으로 바꾼다(2.3). 금액은 `money.Parse`로 바꾼다.
9. 입력 검증은 입력 struct의 규칙 메서드(`AssetInput.Clean()`)가 하고, `Store`가 쓰기 전에 호출한다. 그래서 어느 경로로 들어와도 규칙을 거친다. 검증에 실패하면 422로 입력값과 영어 에러 문구를 담아 폼을 다시 그린다.
10. 없는 리소스는 `http.NotFound`로 404다. 500은 `web.ServerError`로만 준다. 원인은 로그에만 남기고 화면에는 고정 문구를 보인다.
11. `http.Server`는 `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`을 모두 설정한다.

### 3.4 DB (PostgreSQL)

1. `database/sql`과 `github.com/jackc/pgx/v5/stdlib` 드라이버를 쓴다. SQL은 손으로 쓴다. ORM(GORM 등), 쿼리 빌더, SQL 코드 생성기(sqlc 등)를 쓰지 않는다.
2. SQL은 기능 패키지 안, `*sql.DB`를 필드로 가진 `Store` 타입의 메서드에 raw string으로 둔다. 키워드는 대문자이고 컬럼을 명시한다. `SELECT *`를 쓰지 않는다.
3. 값은 `$1` 플레이스홀더로만 넘긴다. 입력값을 SQL 문자열에 이어 붙이지 않는다. 정렬 컬럼처럼 플레이스홀더를 쓸 수 없는 식별자는 코드 안의 고정 목록에서 고른다.
4. 모든 호출은 `ctx`를 받는 메서드(`QueryContext`, `QueryRowContext`, `ExecContext`, `BeginTx`)로 한다.
5. `rows`는 받은 직후 `defer rows.Close()`하고, 반복이 끝나면 `rows.Err()`를 확인한다.
6. `sql.ErrNoRows`는 `Store`에서 그 패키지의 에러(`ErrNotFound`)로 바꾸거나, "없음"이 정상인 조회면 `(T, bool)`로 바꿔 반환한다. `nil, nil`로 바꾸지 않는다.
7. 함께 성공해야 하는 쓰기는 트랜잭션 하나로 묶는다. `BeginTx` 직후 `defer tx.Rollback()`을 걸고 마지막에 `Commit`한다.
8. 반복문 안에서 쿼리하지 않는다. JOIN이나 `= ANY($1)`로 한 번에 읽는다.
9. DB가 지킬 수 있는 불변식은 제약조건(NOT NULL, FOREIGN KEY, UNIQUE, CHECK)으로 건다. 애플리케이션의 검증은 사용자에게 보일 에러를 만들기 위한 것이며 제약조건을 대신하지 않는다.
10. 테이블명은 복수형 snake_case, 컬럼은 snake_case다. PK는 `id bigint GENERATED ALWAYS AS IDENTITY`, 시각 컬럼은 `timestamptz`다.
11. 스키마는 마이그레이션으로만 바꾼다. 마이그레이션은 `internal/postgres/migrations/`의 4자리 번호만으로 된 SQL 파일(`0001.sql`)이고 `//go:embed`로 바이너리에 넣는다. 서버 시작 시 적용되지 않은 파일을 번호 순서대로, 파일마다 트랜잭션 하나로 적용하고 `schema_version` 테이블에 기록한다. 외부 마이그레이션 도구(golang-migrate, goose)를 쓰지 않는다.
12. `main`에 병합된 마이그레이션 파일은 수정하지 않는다. 바꿔야 하면 새 파일을 추가한다. down 마이그레이션을 만들지 않는다.

### 3.5 자산 스냅샷

1. 자산의 금액은 사용자가 달마다 입력하는 스냅샷 값이다. 거래나 분개를 기록해 잔액을 계산하지 않는다.
2. 부채는 음수로 저장한다. 순자산과 부채 합계는 그 달 스냅샷 항목의 USD 환산값으로 계산하고, 계산 결과를 저장하는 캐시 컬럼을 두지 않는다.
3. 지출 관리는 스냅샷과 별개의 메뉴다. 지출 기록은 스냅샷 금액을 바꾸지 않는다.
4. 자세한 설계는 `docs/superpowers/specs/2026-09-25-asset-snapshots-design.md`에 있고, 구현을 바꾸면 같이 고친다.

### 3.6 화면

1. 화면은 `html/template`로 그린다. 템플릿은 `web/templates/`에 두고 `//go:embed`로 바이너리에 넣는다. 핸들러는 `web.Render(w, r, status, 페이지, 데이터)`로만 그린다.
2. 모든 페이지는 `layout.html`(왼쪽 사이드바 + 본문, 좁은 화면에서는 사이드바가 하단 탭 바가 된다)을 쓰고 `title`, `content`만 정의한다. 사이드바가 없는 페이지(로그인)는 `body` 블록 전체를 다시 정의한다. 빈 `{{define}}`은 기본 블록을 덮어쓰지 못하므로 블록을 비우는 방식으로 숨기지 않는다.
3. 목록의 필터와 정렬은 URL 쿼리로 받는 GET 폼이다. 정렬은 `order`와 `direction`(Miniflux와 같다), 필터는 필드 이름을 파라미터로 쓴다. 값은 `web.ParseChoice`로 고정 목록에서 파싱하고, 비어 있으면 기본값, 목록 밖이면 400이다. 필터 막대는 `web.FilterBar`로 만들어 `{{template "filters" ...}}`로 그린다. 필터와 정렬 규칙은 기능 패키지의 `{목록}Query` 타입이 갖고 단위 테스트한다.
4. 템플릿에는 표시만 둔다. 계산과 분기는 Go에서 끝내고 결과를 넘긴다. 금액은 템플릿 함수 `money`로, 종류 이름은 타입의 메서드(`.Type.Label`)로 표시한다.
5. 스타일은 직접 쓴 `web/static/app.css` 하나다. CSS 프레임워크를 쓰지 않는다. 글자 크기, 간격, 컨트롤 높이, 색은 `:root`의 변수에서만 정하고 규칙에서는 변수를 쓴다. 라이트와 다크는 `prefers-color-scheme`으로 변수만 바꾼다. 요소는 시맨틱 태그로 고르고, 클래스는 태그로 구분할 수 없는 곳(`.primary`, `.button`, `.danger`, `.num`, `.actions`)에만 쓴다. 인라인 `style`을 쓰지 않는다.
6. JavaScript 프레임워크와 Node 빌드를 쓰지 않는다. 삭제 확인 같은 한 줄은 인라인 속성(`onsubmit="return confirm(...)"`)으로 쓴다. 페이지 일부만 바꾸는 화면(목록에서 바로 추가·수정)은 htmx(`web/static/htmx-2.0.11.min.js`)를 그 화면의 `head` 블록에서 불러 쓴다. `layout.html`의 `<main>`에는 바꿔 끼우는 방식(`hx-target`, `hx-select`, `hx-swap`)만 있고, `hx-boost`는 같은 화면을 다시 그리는 요소(필터 폼, 표, 추가 폼)에만 건다. 다른 화면으로 가는 링크는 boost하지 않는다. 그러면 다른 화면은 항상 전체 페이지로 열려 그 화면의 스크립트가 순서대로 실행된다. 서버는 항상 전체 페이지를 그리고(성공은 303, 검증 실패는 422) JS 없이도 같은 URL로 동작한다. 행 수정은 `GET /{id}/edit`이 그 행만 입력칸으로 바꾼 목록을 그린다. 차트는 Chart.js를 쓰고 차트가 있는 화면에만 붙인다. 파일은 버전을 이름에 넣어 `web/static/`에 둔다(`chart-4.5.1.umd.min.js`). 데이터는 Go가 `<script type="application/json">`에 넣고, 화면별 스크립트(`web/static/dashboard.js`)가 읽어 그린다.
7. 화면 문구는 영어로 템플릿과 핸들러에 바로 쓴다. 한국어를 쓰지 않는다. i18n을 쓰지 않는다.
8. 클릭할 수 있는 것은 `<a>`나 `<button>`이다. 입력마다 `<label>`이 있다. 목록의 행마다 반복되는 버튼에는 대상 이름을 담은 `aria-label`을 단다. 페이지마다 `h1`은 하나다.

## 4. 에러 처리

1. 실패는 마지막 반환값 `error`로 표현한다. `panic`은 프로그래머 실수로만 도달할 수 있는 상태에만 쓴다. `recover`로 흐름을 제어하지 않는다.
2. 실패를 `nil`, `false`, `-1`, 빈 문자열로 표현하지 않는다. `nil, nil`을 반환하지 않는다. "없음"이 정상 결과인 조회만 `(T, bool)`을 쓴다.
3. 반환된 `error`는 전부 확인한다. `_`로 버리지 않는다. `defer rows.Close()`, `defer tx.Rollback()`처럼 표준 관용구로 인정되는 경우만 예외다.
4. 에러는 처리하거나 반환하거나 둘 중 하나만 한다. 로그를 남기고 다시 반환하지 않는다.
5. 위로 반환할 때는 하려던 일을 맥락으로 붙인다: `fmt.Errorf("insert item: %w", err)`. `failed to`, `unable to`, `error` 같은 단어는 붙이지 않는다.
6. 에러 문자열은 소문자로 시작하고 마침표로 끝내지 않는다.
7. 호출자가 분기해야 하는 에러만 식별 가능하게 만든다. 값이면 `var ErrNotFound = errors.New(...)`, 데이터를 실어야 하면 `type ValidationError struct`. 에러는 그것을 반환하는 패키지에 정의한다. 분기하는 호출자가 없으면 `fmt.Errorf`로 충분하다. 미리 만들어두지 않는다.
8. 에러 비교는 `errors.Is`, `errors.As`로만 한다. `==` 비교와 문자열 매칭 금지.
9. 에러를 응답으로 바꾸는 일은 기능 패키지의 handler에서만 한다(`respondSave`). 검증 에러는 422 폼, 없음은 404, 나머지는 `web.ServerError`의 500이고, 500일 때만 원인을 로그에 남긴다. `Store`와 도메인 코드는 HTTP 상태를 모른다.
10. 에러 흐름을 먼저 처리하고 반환한다. 정상 흐름은 들여쓰기 없이 아래로 이어진다. `return` 뒤에 `else`를 쓰지 않는다.

## 5. 동시성과 자원

1. 블로킹되거나 I/O를 하는 함수는 첫 인자로 `ctx context.Context`를 받는다. `context`를 struct 필드에 저장하지 않는다. 핸들러는 `r.Context()`를 넘긴다.
2. goroutine을 시작하는 코드가 그 종료를 책임진다. 종료 경로(ctx 취소, 채널 close)가 없는 goroutine을 만들지 않고, 시작한 쪽이 종료를 기다린다. 핸들러 안에서 응답보다 오래 사는 goroutine을 띄우지 않는다.
3. 주기 작업(환율 수집)은 `run`이 시작하는 goroutine에서 `time.Timer`/`time.Ticker` 루프로 돌리고 ctx 취소로 끝낸다. cron 라이브러리를 쓰지 않는다.
4. 여러 goroutine이 공유하는 상태는 `sync.Mutex` 또는 채널로 보호한다. mutex는 보호하는 필드 바로 위에 둔다.
5. 자원은 획득 직후 `defer`로 해제를 예약한다. `resp.Body`는 반드시 닫는다.
6. 타임아웃 없는 네트워크 호출을 하지 않는다. 외부 API는 `Timeout`이 설정된 `*http.Client`를 생성자로 받아 호출한다. `http.DefaultClient`를 쓰지 않는다.

## 6. 로그와 보안

### 6.1 로그

1. 로그는 `log/slog`로만 쓴다. `run`에서 `slog.SetDefault`로 한 번 설정하고, 다른 패키지는 `slog.InfoContext(ctx, ...)`처럼 ctx를 받는 패키지 함수를 쓴다. 로거를 인자로 넘기지 않는다.
2. 메시지는 소문자 영어 고정 문자열이다. 변하는 값은 snake_case 키의 속성으로 붙인다: `slog.Int64("account_id", id)`. 메시지 안에 값을 포맷하지 않는다.
3. 로그는 에러를 처리하는 경계(4.9의 500 변환, 주기 작업 루프, `run`)에서만 남긴다.

### 6.2 보안

1. 비밀번호는 `golang.org/x/crypto/bcrypt`로 해시해 저장한다.
2. 인증은 서버 세션이다. `crypto/rand.Text()`로 만든 토큰을 쿠키로 주고 DB에는 토큰의 SHA-256 해시만 저장한다. 쿠키는 `HttpOnly`, `SameSite=Lax`이고, localhost로 온 요청이 아니면 `Secure`다. Safari가 `http://localhost`의 `Secure` 쿠키를 버리기 때문이다.
3. 상태를 바꾸는 요청은 `http.CrossOriginProtection`으로 막는다. CSRF 토큰을 직접 구현하지 않는다.
4. `/auth/`와 `/static/` 외의 모든 기능 라우트는 `auth.Require`를 거쳐 마운트한다. 세션이 없으면 `/auth/login`으로 303을 보낸다.
5. 비밀번호, 세션 토큰, `DATABASE_URL`을 로그와 에러 문구에 넣지 않는다.
6. 사용자는 한 명이다. 계정은 `run`이 시작할 때 `ADMIN_EMAIL`, `ADMIN_PASSWORD`로 만들거나 비밀번호를 바꾼다. 회원가입 화면을 만들지 않는다.

## 7. 파일 위치와 네이밍

| 종류 | 규칙 | 예 |
|---|---|---|
| 패키지 | 짧은 소문자 한 단어, 단수형. `_`와 대문자 금지 | `finance`, `note` |
| 파일 | 소문자 snake_case, 주제 단위 | `exchange_rate.go` |
| 테스트 파일 | 대상 파일명 + `_test.go`, 같은 디렉터리 | `exchange_rate_test.go` |
| exported | `MixedCaps` | `CreateAccount` |
| unexported | `mixedCaps` | `assetForm` |
| 약어 | 대소문자를 통일 | `accountID`, `ServeHTTP`, `baseURL` |
| 인터페이스 | 메서드가 하나면 메서드명 + `er` | `RateFetcher` |
| 에러 값 | `Err` 접두 | `ErrNotFound` |
| 에러 타입 | `Error` 접미 | `ValidationError` |
| 생성자 | `New` 또는 `NewX` | `finance.NewStore` |
| 핸들러 메서드 | 동사 + 리소스, 접미사 없음 | `func (h *handler) createAsset` |
| 템플릿 | `{주제}_{화면}.html`. 여러 페이지가 쓰는 조각은 `_` 접두 | `asset_list.html`, `_filters.html` |
| receiver | 타입명 앞 1~2글자, 타입 안에서 통일 | `func (s *Store)` |
| 테이블, 컬럼 | snake_case, 테이블은 복수형 | `snapshot_items.snapshot_id` |
| 마이그레이션 | 4자리 번호만. 1씩 증가, 설명을 붙이지 않는다 | `0003.sql` |

1. 이름에 패키지명을 반복하지 않는다. 호출부에서 `패키지.이름`으로 읽힌다. `finance.Asset`(O), `finance.FinanceAsset`(X).
2. 역할 접미사 `Service`, `Controller`, `Repository`, `Manager`, `Data`, `DTO`, `Impl`, `Helper`와 인터페이스 접두 `I`를 붙이지 않는다.
3. getter와 조회 메서드에 `Get`을 붙이지 않는다: `URL()`, `store.Account(ctx, id)`, `store.Accounts(ctx)`. 쓰기는 동사로 시작한다: `CreateAccount`, `DeleteTask`. 불리언 반환은 `Is`, `Has`, `Can`으로 시작한다.
4. 이름 길이는 스코프에 비례한다. 몇 줄짜리 스코프에서는 `i`, `r`, `w`, `err`, `ctx`, `tx`를 쓰고, 패키지 수준 이름은 서술적으로 짓는다. receiver에 `this`, `self`를 쓰지 않는다.
5. 파일은 타입 하나가 아니라 주제 하나를 담고, 파일 목록이 곧 패키지의 목차가 되게 한다.
   - `{패키지}.go`: 패키지 doc comment와 `Store` 타입.
   - `{도메인 주제}.go`(`account.go`, `session.go`): 그 주제의 타입, 에러, 규칙 함수, `Store` 메서드.
   - `handler.go`: `NewHandler`와 라우트 목록, 폼 데이터 struct, 핸들러 메서드, 에러 변환. 커지면 `{주제}_handler.go`로 나눈다.
   - `middleware.go`: 다른 패키지가 쓰는 미들웨어.
6. 파일명 접미사 `_test`, `_{os}`, `_{arch}`는 빌드 조건이다. 그 의도가 아니면 파일명을 `_linux.go`, `_windows.go`처럼 끝내지 않는다.
7. import는 표준 라이브러리, 외부, 내부 세 그룹으로 나눈다. dot import와 불필요한 alias를 쓰지 않는다.

## 8. 작성 원칙

1. 가장 평범한 방법으로 쓴다. 처음 보는 Go 개발자가 제일 먼저 떠올릴 방법이 기준이다. 같은 일을 하는 더 단순한 방법이 있으면 그것이 정답이다. 영리한 코드를 쓰지 않는다: `reflect`, 순차 실행으로 충분한 곳의 goroutine과 channel, 타입 기교.
2. 도구는 단순한 것부터 고른다: 언어 기본 기능(struct, slice, map, 반복문) → 표준 라이브러리 → DB 기능(제약조건, 트랜잭션) → 이미 있는 의존성.
3. 코드는 설명 없이 읽혀야 한다. 처음 보는 사람이 코드만 읽고 무엇을 왜 하는지 납득할 수 없으면 코드를 바꾼다: 이름을 고치고, 함수를 쪼개고, 접근을 단순하게 바꾼다.
4. 그래도 단순해지지 않으면 구현을 멈추고 이유와 함께 물어본다.
5. 요청받지 않은 기능, 옵션, 엔드포인트, 쿼리 파라미터를 추가하지 않는다.
6. 요청받지 않은 추상화를 만들지 않는다. 구현체가 하나뿐인 인터페이스, 한 곳에서만 쓰는 헬퍼, 한 곳에서만 쓰는 제네릭, functional options, 빌더, 범용 CRUD 베이스를 만들지 않는다.
7. struct embedding으로 상속(기반 클래스, 추상 클래스)을 흉내 내지 않는다. 재사용은 필드로 가진 값에 위임한다.
8. 새 의존성은 먼저 물어본다. 허락 없이 `go get` 하지 않는다. 이 문서가 이름으로 정한 의존성(`pgx`, `x/crypto`)과 개발 도구 air(`make dev`, `go run`으로 버전 고정)만 예외다.
9. 새 파일을 만들기 전에 같은 패키지의 기존 파일을 먼저 읽고 그 구조와 스타일을 따른다.
10. 기존 코드를 수정할 때 요청 범위 밖의 코드를 건드리지 않는다. 리팩토링은 별도 요청이 있을 때만 한다.
11. 매직 넘버, 매직 스트링을 쓰지 않는다. named `const`로 뺀다. 시간은 `30 * time.Second`처럼 `time.Duration`으로 쓴다.

## 9. 테스트

1. 테스트는 두 종류만 쓴다. 기준 구현 Miniflux와 같은 방식이다.
   - **로직 단위 테스트**: 직접 짠 분기, 계산, 파서, 검증 규칙(3.1.3의 규칙 함수). DB와 HTTP 없이, 대상과 같은 패키지에서 테스트한다.
   - **HTTP 통합 테스트**: `cmd/server/{기능}_test.go`에서 `routes()` 전체에 실제 DB로 `httptest` 요청(폼 POST 포함)을 보내 기능의 사용자 흐름을 따라간다. 상태 코드, 리다이렉트 위치, HTML 본문에 기대한 문구가 있는지 확인한다.
2. `Store` 메서드, 마이그레이션, 설정 읽기, 라우트 배선은 따로 테스트하지 않는다. SQL과 템플릿은 HTTP 통합 테스트가 지나가며 검증한다. 배선과 설정이 틀리면 서버가 시작하지 않거나 첫 요청에서 드러난다.
3. 틀려도 겉으로 드러나지 않는 동작(세션 만료, 권한, 금액 계산, 환율 환산)은 둘 중 한 테스트에 반드시 있어야 한다. 같은 동작을 두 테스트에서 중복으로 확인하지 않는다.
4. 테스트는 대상과 같은 디렉터리의 `_test.go` 파일에 둔다. 별도 `tests/` 디렉터리를 만들지 않는다.
5. 테스트 함수명은 `Test{Func}`, `Test{Type}_{Method}`, `Test{기능}`(`TestFinance`)이다. 조건과 기대결과는 서브테스트 이름에 영어로 적는다: `t.Run("expired session redirects to login", ...)`.
6. 같은 함수의 케이스가 둘 이상이면 table-driven으로 쓴다.
7. 표준 `testing`만 쓴다. assert 라이브러리를 쓰지 않는다. 실패 메시지는 `t.Errorf("Balance(%d) = %d, want %d", id, got, want)` 형식이다.
8. `time.Sleep`으로 동기화하지 않는다. 채널이나 `context`로 기다린다. 날짜 경계 계산처럼 현재 시각을 쓰는 로직은 `now time.Time`을 인자로 받아 테스트하고, 타이머와 주기 작업은 `testing/synctest`로 테스트한다. 가짜 시계 인터페이스를 만들지 않는다.
9. DB는 실제 PostgreSQL을 쓴다. DB를 mock하거나 인터페이스로 가리지 않는다. sqlmock을 쓰지 않는다.
10. 테스트 헬퍼는 첫 줄에서 `t.Helper()`를 호출한다. 정리는 `t.Cleanup`, 임시 파일은 `t.TempDir()`, ctx는 `t.Context()`, 픽스처 파일은 `testdata/`를 쓴다.
11. 외부 HTTP(환율 API)는 `httptest.NewServer`로 대체한다. 실제 외부 호출 금지.
12. HTTP 통합 테스트는 `postgrestest.New(t)`로 테스트마다 마이그레이션이 적용된 새 DB를 만들어 쓴다. 테스트끼리 데이터를 공유하지 않는다.
13. `TEST_DATABASE_URL`이 비어 있으면 DB 테스트는 `t.Skip`한다. 10절의 검사는 이 값을 설정하고 실행하므로 거기서는 건너뛰는 테스트가 없어야 한다.

## 10. 완료 기준

아래를 모두 통과하기 전에는 작업을 완료했다고 말하지 않는다. `go test`는 `docker compose up -d --wait db`로 DB를 띄우고 `TEST_DATABASE_URL`을 설정한 상태에서 실행한다. `make check`가 DB를 띄우고 이 검사를 모두 실행한다.

```bash
test -z "$(gofmt -l .)"
golangci-lint run
go test -race ./...
go mod tidy -diff
```

1. 실패한 항목이 있으면 고친 뒤 다시 실행한다.
2. 통과시키기 위해 테스트를 삭제하거나 `t.Skip`하지 않는다. `//nolint`를 추가하거나 `.golangci.yml`의 규칙을 끄지 않는다.
3. `.golangci.yml`은 기본 린터(`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`)에 `revive`(기본 규칙에서 `exported`, `package-comments`만 끔, 1.2), `exhaustive`(2.4), `errorlint`(4.8), `bodyclose`(5.5), `noctx`(3.4.4, 5.1), `sqlclosecheck`와 `rowserrcheck`(3.4.5), `sloglint`(6.1)를 더해 켠다.

## 11. Git

GitHub Flow를 따른다. 커밋 형식은 Miniflux와 같은 Conventional Commits다.

### 11.1 브랜치

1. `main`은 항상 10절의 완료 기준을 통과하는 상태다. `main`에 직접 커밋하지 않는다.
2. 작업은 `main`에서 딴 짧게 사는 브랜치에서 한다. `develop`, `release` 같은 장기 브랜치를 두지 않는다.
3. 브랜치 이름은 `{type}/{kebab-case-주제}`다. type은 11.2의 type이다. 예: `feat/finance-expenses`, `fix/snapshot-timezone`.
4. 브랜치 하나는 PR 하나, 주제 하나다. 병합되면 브랜치를 지운다.

### 11.2 커밋 메시지

1. [Conventional Commits](https://www.conventionalcommits.org/)를 따른다: `{type}({scope}): {description}`. scope는 선택이며 패키지명을 쓴다. 예: `feat(finance): reject negative asset amounts`.
2. type은 `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore` 중 하나다. 호환성을 깨는 변경은 type 뒤에 `!`를 붙인다.
3. description은 영어 명령형 현재 시제다. 소문자로 시작하고 마침표로 끝내지 않는다. 제목 줄은 72자 이내다.
4. 본문은 선택이다. 제목과 한 줄 띄우고, 무엇을 했는지가 아니라 왜 했는지를 적는다. 72자에서 줄을 바꾼다. 코드에 주석을 달 수 없으므로(1절) 이유를 남길 곳은 여기다.
5. 커밋 메시지와 PR 본문에 AI 도구 표기를 넣지 않는다. `Co-Authored-By: Claude ...`, `Generated with Claude Code` 등 어떤 형태도 금지.
6. 커밋 작성자는 이 저장소의 로컬 git 설정(개인 계정)을 그대로 쓴다. `--author`로 바꾸거나 전역 설정을 건드리지 않는다.

### 11.3 병합

1. `main`으로의 병합은 PR로만 한다. 10절의 검사를 통과해야 병합한다.
2. Squash and merge만 쓴다. `main`의 히스토리는 PR당 커밋 하나인 직선이다. merge commit을 만들지 않는다.
3. PR 제목이 `main`에 남는 커밋 메시지다. 11.2의 형식을 따른다. `(#번호)`는 GitHub가 붙인다.
4. 브랜치 안의 작업 커밋은 병합 때 하나로 합쳐지므로 작게 쪼개도 된다. 메시지 형식은 지킨다. 병합 전에 손으로 squash하지 않는다.
5. `main`은 force-push하지 않는다. 자기 작업 브랜치는 `--force-with-lease`로만 force-push한다.

### 11.4 릴리스

1. 버전은 [SemVer](https://semver.org/)이고 태그는 `v`로 시작한다: `v0.1.0`.
2. 1.0 전에는 `v0.x.y`다. 호환성을 깨면 minor를, 그 외에는 patch를 올린다.
3. 릴리스는 `main`에 annotated 태그를 push해서 만든다. CHANGELOG 파일을 손으로 쓰지 않는다.

### 11.5 역할

1. Claude는 요청받았을 때만, 작업 브랜치에, 로컬 커밋만 만든다.
2. push, PR, 병합, 태그, 히스토리 재작성(rebase, amend, force-push)은 사용자가 한다. Claude는 필요한 명령을 안내한다.

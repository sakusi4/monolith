# monolith

필요한 것: Go, Docker, [golangci-lint](https://golangci-lint.run)(`make check`에만 필요)

## 실행

```bash
make dev   # 파일을 저장하면 자동으로 다시 빌드하고 재시작한다 (air)
make run   # 한 번만 실행
```

http://localhost:8080 을 열고 `admin@localhost` / `admin`으로 로그인한다. `make dev`로 띄웠으면 `.go`, `.html`, `.css`, `.js`, `.sql`을 저장한 뒤 브라우저만 새로고침하면 된다.

### 단일 바이너리

```bash
make build   # 템플릿과 CSS까지 들어간 bin/server
make db
DATABASE_URL='postgres://postgres:postgres@localhost:5432/monolith?sslmode=disable' \
ADMIN_EMAIL=me@example.com ADMIN_PASSWORD='...' ./bin/server
```

서버는 시작할 때 마이그레이션을 적용한다.

| 환경변수 | 기본값 | |
|---|---|---|
| `DATABASE_URL` | 없음(필수) | Postgres 접속 URL |
| `LISTEN_ADDR` | `:8080` | 서버 주소 |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD` | 없음 | 로그인 계정. 시작할 때 이 계정을 만들거나 비밀번호를 바꾼다. 둘 다 주거나 둘 다 뺀다 |
| `TIMEZONE` | `UTC` | "이번 달"을 정하는 시간대(IANA 이름, 예: `Asia/Dubai`) |

DB 데이터는 프로젝트의 `db_data/` 폴더에 파일로 남는다(git에는 올라가지 않는다). DB 끄기: `docker compose down`. 데이터까지 지우려면 DB를 끈 뒤 `db_data/`를 지운다.

## 테스트

```bash
make test    # DB를 띄우고 전체 테스트
make check   # 커밋 전 검사: 포맷, 린트, 테스트, go.mod 정리 여부
```

테스트는 두 종류다. 로직 단위 테스트(DB 없음)와 `cmd/server/*_test.go`의 HTTP 통합 테스트(실제 요청 + 실제 DB)다. 통합 테스트는 `TEST_DATABASE_URL`의 Postgres에 테스트마다 새 DB를 만들고 끝나면 지운다. 이 값이 없으면 통합 테스트는 건너뛴다(SKIP).

```bash
make db
export TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable'

go test ./...                            # 전체
go test -run TestFinance ./cmd/server/   # 하나만
```

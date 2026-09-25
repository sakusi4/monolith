# 월별 자산 스냅샷과 USD 환산

이 문서는 구현된 동작을 설명한다. 코드를 바꾸면 이 문서도 같이 고친다.

## 목적

매월 자산과 부채의 금액을 기록하고, 각 달의 순자산을 USD로 비교한다. 자산은 USD, KRW, AED, JPY로 섞여 있다. 환율은 달마다 하나를 저장하고, 스냅샷은 그 달의 환율로 환산한다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 자산의 정체성 | 자산은 이름, 종류, 통화를 가진다. 금액은 월별 스냅샷 항목에만 있다. 이름은 대소문자를 무시하고 유일하다 |
| 자산 생성과 삭제 | 자산 화면은 따로 없다. 어떤 달에 새 이름으로 항목을 추가하면 자산이 생기고, 어느 달에도 쓰이지 않게 되면 지워진다 |
| 자산 수정 | 이름과 종류는 어느 달의 행에서 고쳐도 모든 달에 적용된다. 통화는 만든 뒤 바꿀 수 없다 |
| 스냅샷 단위 | 한 달에 하나다. `month`는 그 달 1일이다. 첫 항목을 추가할 때 생기고, 마지막 항목을 지우면 메모와 함께 지워진다 |
| 기록 가능한 달 | 2024년 2월(`firstMonth`)부터 `TIMEZONE` 기준 이번 달까지. 범위 밖은 404다 |
| 부채 | 종류 `loan` 하나만 둔다. 금액은 부호를 그대로 저장한다: 부채는 음수다. 부호는 종류와 무관하다 |
| 순자산 | 스냅샷 항목의 USD 환산값을 모두 더한 값이다. 부채는 음수 그대로 더한다. Loans는 음수 항목의 합이다. 총자산(양수 항목의 합)은 두지 않는다 |
| 메모 | 스냅샷마다 한 줄 메모(`note`)를 둔다. 앞뒤 공백을 지워 저장한다 |
| 환율 저장 | 달마다 통화별로 하나만 저장한다. `month`는 그 달 1일이다 |
| 환산 환율 | 스냅샷 `month`와 가장 가까운 달의 환율을 쓴다. 거리가 같으면 이전 달을 쓴다 |
| 환율 출처 | `GET https://open.er-api.com/v6/latest/USD`. 키가 없고 최신 환율만 준다. 출처 표기가 필요하다 |
| 통화 | USD, KRW, AED, JPY. USD 환율은 저장하지 않고 1로 본다 |

## 데이터 모델

```sql
CREATE TABLE assets (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type text NOT NULL CHECK (type IN ('cash', 'deposit', 'stock', 'crypto', 'real_estate', 'loan', 'other')),
    name text NOT NULL CHECK (name <> ''),
    currency text NOT NULL CHECK (currency IN ('USD', 'KRW', 'AED', 'JPY')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX assets_name_key ON assets (lower(name));

CREATE TABLE snapshots (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    month date NOT NULL UNIQUE CHECK (extract(day FROM month) = 1),
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE snapshot_items (
    snapshot_id bigint NOT NULL REFERENCES snapshots (id) ON DELETE CASCADE,
    asset_id bigint NOT NULL REFERENCES assets (id),
    amount bigint NOT NULL,
    PRIMARY KEY (snapshot_id, asset_id)
);

CREATE TABLE exchange_rates (
    month date NOT NULL CHECK (extract(day FROM month) = 1),
    currency text NOT NULL CHECK (currency IN ('KRW', 'AED', 'JPY')),
    per_usd numeric NOT NULL CHECK (per_usd > 0),
    PRIMARY KEY (month, currency)
);
```

마이그레이션 `0003`~`0007`이 이 모양을 만든다. 위 SQL은 결과를 보여 줄 뿐 실행하는 파일이 아니다.

- `amount`는 통화의 최소 단위 정수다(USD·AED는 센트, KRW·JPY는 1단위).
- `snapshot_items`에 없는 자산은 그달에 보유하지 않은 것이다. `amount = 0`은 "보유했지만 0"이다.
- `per_usd`는 1 USD가 해당 통화로 얼마인지다(KRW 1374.61).
- `0007`은 그전까지 쌓인 일별 환율을 달마다 1일에 가장 가까운 값 하나로 줄였다. 그 값은 일별 환율일 때 스냅샷에 적용되던 값과 같다.

## 환율 수집

- `run`이 goroutine에서 `finance.RunRateUpdates(ctx, store, client)`를 실행하고, 종료할 때 끝나기를 기다린다. `*http.Client`의 `Timeout`은 10초다.
- 시작할 때와 이후 1시간마다(`time.Ticker`) UTC 기준 이번 달의 환율이 있는지 확인한다. 없을 때만 API를 호출해 이번 달 1일 값으로 저장한다(`ON CONFLICT DO NOTHING`).
- API는 최신 환율만 준다. 그래서 한 달의 환율은 그 달에 서버가 처음 확인한 시점의 환율이다. 1일에 서버가 켜져 있었다면 1일 값이다.
- 응답의 `result`가 `"success"`가 아니거나, KRW, AED, JPY 중 하나라도 없거나, 값이 0 이하이면 에러다.
- 숫자는 `json.Number`로 읽어 `big.Rat`으로 검증하고, 문자열 그대로 `numeric`에 저장한다. float를 거치지 않는다.
- 실패하면 `slog.ErrorContext`로 남기고 다음 주기에 다시 시도한다. 서버 시작을 막지 않는다.
- 과거 달의 환율을 채우는 기능은 없다. 서버가 한 달 내내 꺼져 있었다면 그 달의 스냅샷은 가장 가까운 달의 환율을 빌려 쓴다.

## 환산

- 스냅샷은 쿼리 한 번으로 모든 달의 항목을 읽는다. 항목마다 가장 가까운 달의 환율을 `LATERAL JOIN`으로 붙인다(`Store.Snapshots`).
- `numeric`은 문자열로 스캔해 `big.Rat`으로 바꾼다.
- `money.ToUSD`가 최소 단위 금액을 USD 센트로 바꾼다. `math/big`으로 계산하고, 0.5는 0에서 먼 쪽으로 반올림한다. 금액 정수로 돌아오는 반올림은 이 함수에서만 한다.
- USD가 아닌 통화인데 저장된 환율이 하나도 없으면 그 항목의 USD 값은 "없음"이다. 그 달의 요약은 순자산 대신 빠진 통화를 표시한다.

## 화면

모든 라우트는 로그인이 필요하다(`auth.Require`). `{month}`는 `2006-01` 형식이다.

| 라우트 | 동작 |
|---|---|
| `GET /finance/snapshots` | 한 달의 스냅샷. `month`가 없으면 이번 달 |
| `POST /finance/snapshots/{month}/items/new` | 항목 추가 |
| `GET /finance/snapshots/{month}/items/{id}/edit` | 그 행만 입력칸으로 바꾼 목록 |
| `POST /finance/snapshots/{month}/items/{id}/edit` | 항목 수정 |
| `POST /finance/snapshots/{month}/items/{id}/delete` | 그 달에서 항목 제거 |
| `POST /finance/snapshots/{month}/copy` | 빈 달에 직전 스냅샷의 항목을 복사 |
| `POST /finance/snapshots/{month}/note` | 메모 저장. 스냅샷이 없는 달이면 404 |
| `GET /dashboard` | 최신 스냅샷의 요약과 월별 차트 |

POST가 성공하면 같은 필터를 유지한 목록으로 303을 보낸다. 입력이 잘못되면 422로 목록을 다시 그리고 입력값과 에러 문구를 보여 준다.

### 스냅샷 화면 (`snapshot_list.html`)

위에서 아래로:

1. 제목과 월 선택. 월을 바꾸면 바로 이동한다.
2. 요약(스냅샷이 있는 달만): Net worth, Loans 카드. Net worth 카드에는 직전 스냅샷 대비 증감과 비율을 표시한다. 환율이 없는 통화가 있으면 카드 대신 빠진 통화를 알린다.
3. 적용 환율(스냅샷이 있는 달만): `Rates: 1 USD = KRW 1,374.61 · AED 3.6725`. 그 달 항목에 쓰인 통화만, `money.Currencies` 순서로 보여 준다. 다른 달의 환율을 빌렸으면 `(Aug 2026)`처럼 그 달을 붙인다. 값은 소수 넷째 자리까지다(`money.FormatRate`).
4. 메모(스냅샷이 있는 달만): 한 줄 입력칸과 Save.
5. 필터: Type, Currency(비면 전체), Sort by(USD·Name·Type, 기본 USD), Direction(기본 Descending). 목록 밖의 값이면 400이다. USD 값이 없는 행은 정렬과 무관하게 맨 뒤다. 필터는 표만 좁히고 요약은 바꾸지 않는다.
6. 표: Name, Type, Amount(원래 통화), USD, Edit·Delete. 합계 행의 Amount는 행이 모두 같은 통화일 때만, USD는 모든 행에 USD 값이 있을 때만 표시한다. 삭제는 확인 창을 띄운다.
7. 빈 달이고 이전 스냅샷이 있으면 `Copy Aug 2026 (5 assets)` 버튼.
8. 추가 폼: Name(그 달에 없는 기존 자산 이름을 제안), Type, Currency, Amount. 기존 자산 이름을 입력하면 Type과 Currency가 그 자산의 값으로 바뀌고 잠긴다(`snapshot_list.js`). 서버도 이름으로 기존 자산을 찾아 그 종류와 통화를 쓰고, 금액을 그 통화로 해석한다. 그래서 잠긴 칸이 전송되지 않거나 JS가 없어도 결과가 같다. 422로 다시 그릴 때도 칸을 잠근 채로 그린다.
9. 출처 링크 "Rates by Exchange Rate API".

422가 되는 경우:

- 추가: 이름이 비었거나 종류·통화가 목록 밖, 금액을 해석할 수 없음(통화의 소수 자릿수 초과 포함), 이미 그 달에 있는 자산.
- 수정: 이름이 비었거나 종류가 목록 밖, 금액을 해석할 수 없음, 다른 자산이 이미 쓰는 이름.
- 복사: 이미 항목이 있는 달, 이전 스냅샷이 없는 달.

htmx와 `snapshot_list.js`는 이 화면에서만 불러온다. 필터 폼, 표, 복사·메모·추가 폼에 `hx-boost`를 걸어 `main`만 바꿔 끼운다. 서버는 항상 전체 페이지를 그리므로 JS 없이도 같은 URL로 동작한다.

### 대시보드 (`dashboard.html`, `dashboard.js`)

- 최신 스냅샷의 Net worth(기준 월 표시), Loans 카드.
- 선 차트 하나에 Net worth와 Loans를 월별로 그린다. 색은 `--series-1`, `--series-2`다. Loans는 차트에서만 양수(부채 잔액)로 그리고, 카드와 표에서는 음수다.
- 환율이 없는 통화가 있는 달은 차트에서 뺀다. 기록이 없는 달도 빠지므로 가로축은 실제 시간 간격을 반영하지 않는다.
- 데이터는 Go가 `<script type="application/json">`에 USD 센트로 넣고 `dashboard.js`가 읽는다.

## 코드 배치

| 파일 | 내용 |
|---|---|
| `internal/money/money.go` | `Currency`, `Parse`, `Format`, `Input`, `ToUSD`, `FormatRate` |
| `internal/finance/asset.go` | `Asset`, `AssetType`과 표시 이름, 이름으로 찾기(`assetNamed`), `Store.Assets` |
| `internal/finance/snapshot.go` | `Snapshot`, `SnapshotItem`, `Totals`, `currentMonth`, `monthsUntil`, `Store.Snapshots`, `Store.SetNote` |
| `internal/finance/snapshot_item.go` | `ItemInput`/`ItemUpdate`와 `Clean`, `Store.AddItem`/`UpdateItem`/`DeleteItem`/`CopyPreviousSnapshot` |
| `internal/finance/snapshot_list.go` | 목록 필터와 정렬(`assetQuery`), 행과 합계 |
| `internal/finance/summary.go` | 요약과 전월 대비(`summarize`), 적용 환율(`appliedRates`) |
| `internal/finance/exchange_rate.go` | `RunRateUpdates`, `fetchRates`, 환율 저장 |
| `internal/finance/handler.go` | `NewHandler`와 라우트 |
| `internal/finance/snapshot_handler.go` | 스냅샷 화면의 핸들러와 페이지 데이터 |
| `internal/dashboard/handler.go` | 대시보드 페이지와 차트 데이터 |
| `web/static/snapshot_list.js` | 추가 폼에서 기존 자산의 종류와 통화를 채우고 잠근다 |
| `web/static/dashboard.js` | 대시보드 차트 |

## 설정

`TIMEZONE`(IANA 이름, 기본값 `UTC`)이 "이번 달"을 정한다. `run`에서 한 번 읽어 `finance.NewHandler(store, loc)`로 넘긴다. 환율의 달은 이 값과 무관하게 UTC다.

## 테스트

**단위 테스트** (DB 없음)

- `money`: `Parse`(음수, 소수 자릿수), `Format`, `Input` 왕복, `ToUSD`(반올림 경계, 음수), `FormatRate`
- `fetchRates`: `httptest.NewServer`로 정상 응답, `result` 실패, 통화 누락, 0 이하 값, 깨진 JSON, 5xx
- `ItemInput.Clean`, `ItemUpdate.Clean`
- `Snapshot.Totals`, `currentMonth`, `monthsUntil`
- `parseAssetQuery`, `assetQuery.apply`, `totalOf`, `listURL`
- `summarize`, `percent`, `appliedRates`
- 대시보드 `newPage`

**HTTP 통합 테스트** (`cmd/server`, 실제 DB)

- `TestFinance`: 가장 가까운 달의 환율로 환산되고 적용 환율이 표시된다. 잘못된 입력은 422다. 메모를 저장하고, 스냅샷 없는 달의 메모는 404다. 기존 이름은 그 자산의 종류와 통화를 쓰고, 422로 다시 그릴 때 두 칸을 잠근다. 빈 달 복사, 행 수정, 삭제(빈 달과 쓰이지 않는 자산까지 정리), 필터, 범위 밖 월, htmx 요청을 확인한다.
- `TestDashboard`: 빈 상태, 최신 요약, 차트 JSON.

## 범위 밖

- 과거 달의 환율 채우기
- 자산 보관(archive)
- 차트 가로축에 기록 없는 달 표시
- 환율 수집 루프 자체의 테스트. DB I/O가 섞여 `synctest`로 다루기 어렵고, 로직은 "이번 달 환율이 없으면 가져온다" 한 줄이다.

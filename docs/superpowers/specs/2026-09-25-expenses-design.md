# 월별 지출

이 문서는 지출 기능의 설계다. 구현을 바꾸면 이 문서도 같이 고친다.

## 목적

어디에 얼마를 쓰는지 항목별로 본다. 달마다 항목(월세, 넷플릭스, 비행기표 등)의 그 달 합계를 적고, 한 달 단위로 합계와 직전 기록 달 대비 증감을 USD로 본다. 분류, 예산, 건별 기록, 여러 달 보기는 하지 않는다.

지출은 자산 스냅샷과 별개다. 지출을 기록해도 스냅샷 금액은 바뀌지 않는다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 기록 단위 | 달마다 항목별 합계. 건별 기록은 하지 않는다 |
| 항목의 정체성 | 항목은 이름과 통화를 가진다. 이름은 대소문자를 무시하고 유일하다. 분류는 없다 |
| 항목 생성과 삭제 | 어떤 달에 새 이름으로 추가하면 생기고, 어느 달에도 쓰이지 않게 되면 지워진다 |
| 항목 수정 | 이름은 어느 달에서 고쳐도 모든 달에 적용된다. 통화는 만든 뒤 바꿀 수 없다 |
| 금액 | 통화의 최소 단위 정수. 부호 제한이 없다. 지출은 양수, 환불처럼 합계를 줄이는 값은 음수로 적는다 |
| 기록 가능한 달 | 스냅샷과 같다: 2024년 2월부터 `TIMEZONE` 기준 이번 달까지. 범위 밖은 404 |
| 환산 | 스냅샷과 같은 월 환율(`exchange_rates`)에서 가장 가까운 달의 값을 쓴다 |
| 달 테이블 | 두지 않는다. 그 달에 행이 없으면 빈 달이다 |

## 데이터 모델: `0008.sql`

```sql
CREATE TABLE expense_items (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (name <> ''),
    currency text NOT NULL CHECK (currency IN ('USD', 'KRW', 'AED', 'JPY')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX expense_items_name_key ON expense_items (lower(name));

CREATE TABLE expenses (
    month date NOT NULL CHECK (extract(day FROM month) = 1),
    item_id bigint NOT NULL REFERENCES expense_items (id),
    amount bigint NOT NULL,
    PRIMARY KEY (month, item_id)
);
```

- 쓰기는 스냅샷 항목과 같은 방식으로 트랜잭션 하나에서 한다.
  - 추가: 이름으로 항목을 찾거나 만든 뒤 그 달의 행을 넣는다.
  - 삭제: 그 달의 행을 지우고, 어느 달에도 쓰이지 않는 항목도 지운다.
- 다른 항목이 쓰는 이름으로 바꾸면 유일 인덱스가 막는다. 화면에는 422로 알린다.

## 화면

모든 라우트는 로그인이 필요하다. `{month}`는 `2006-01` 형식이다. 사이드바의 Finance 아래에 Snapshots 다음으로 Expenses 링크를 둔다.

| 라우트 | 동작 |
|---|---|
| `GET /finance/expenses` | 한 달의 지출. `month`가 없으면 이번 달 |
| `POST /finance/expenses/{month}/items/new` | 항목 추가 |
| `GET /finance/expenses/{month}/items/{id}/edit` | 그 행만 입력칸으로 바꾼 목록 |
| `POST /finance/expenses/{month}/items/{id}/edit` | 이름과 금액 수정 |
| `POST /finance/expenses/{month}/items/{id}/delete` | 그 달에서 항목 제거 |
| `POST /finance/expenses/{month}/copy` | 빈 달에 직전 기록 달의 항목과 금액을 복사 |

POST가 성공하면 그 달의 목록(`/finance/expenses?month=…`)으로 303을 보낸다. 그 달에 없는 항목을 수정하거나 삭제하면 404다.

### 지출 화면 (`expense_list.html`)

위에서 아래로:

1. 제목 "Expenses"와 월 선택. 월을 바꾸면 바로 이동한다.
2. 요약 카드(기록이 있는 달만): Total(USD)과 직전 기록 달 대비 증감과 비율. 환율이 없는 통화가 있으면 카드 대신 빠진 통화를 알린다.
3. 적용 환율(기록이 있는 달만): 스냅샷 화면과 같은 한 줄.
4. 표: Name, Amount(원래 통화), USD, Edit·Delete, 합계 행. USD가 큰 순서로 고정하고, USD 값이 없는 행은 맨 뒤에 둔다. 필터와 정렬 막대는 두지 않는다. 합계 행의 Amount는 행이 모두 같은 통화일 때만, USD는 모든 행에 USD 값이 있을 때만 표시한다.
5. 빈 달이고 이전 기록이 있으면 `Copy Aug 2026 (8 items)` 버튼.
6. 추가 폼: Name(그 달에 없는 기존 항목을 제안), Currency, Amount. 기존 항목 이름을 입력하면 Currency가 그 항목의 값으로 바뀌고 잠긴다. 서버도 이름으로 기존 항목을 찾아 그 통화를 쓰고 금액을 그 통화로 해석한다. 422로 다시 그릴 때도 잠근 채로 그린다.
7. 환율 출처 링크 "Rates by Exchange Rate API".

422가 되는 경우:

- 추가: 이름이 비었거나 통화가 목록 밖, 금액을 해석할 수 없음, 이미 그 달에 있는 항목.
- 수정: 이름이 비었음, 금액을 해석할 수 없음, 다른 항목이 이미 쓰는 이름.
- 복사: 이미 기록이 있는 달, 이전 기록이 없는 달.

htmx는 스냅샷 화면과 같은 방식으로 이 화면에서만 불러온다. 표와 복사·추가 폼에 `hx-boost`를 건다. 서버는 항상 전체 페이지를 그린다.

## 코드 배치

| 파일 | 내용 |
|---|---|
| `internal/postgres/migrations/0008.sql` | 위의 두 테이블 |
| `internal/finance/expense.go` | `ExpenseItem`, `Expense`와 `USD()`, `ExpenseInput`/`ExpenseUpdate`와 `Clean`, 에러, 이름으로 항목 찾기, `Store`의 조회와 쓰기(`Expenses`, `ExpenseItems`, `AddExpense`, `UpdateExpense`, `DeleteExpense`, `CopyPreviousExpenses`) |
| `internal/finance/expense_list.go` | 한 달의 합계와 직전 기록 달 대비 증감, 표의 행과 정렬, 합계 행 |
| `internal/finance/expense_handler.go` | 지출 화면의 핸들러와 페이지 데이터 |
| `internal/finance/handler.go` | 지출 라우트 추가 |
| `internal/finance/summary.go` | `appliedRates`가 스냅샷과 지출을 모두 받도록 입력을 (통화, 환율의 달, 환율) 목록으로 바꾼다 |
| `web/templates/expense_list.html` | 지출 화면 |
| `web/templates/layout.html` | 사이드바 링크 |
| `web/static/item_form.js` | `snapshot_list.js`의 이름을 바꿔 두 화면의 추가 폼이 같이 쓴다 |

- 재사용하는 것: `money`(해석, 표시, 환산, 환율 표시), `currentMonth`, `monthsUntil`, `monthFilter`, `percent`, `requireRow`, `isPgError`.
- 스냅샷 핸들러와 구조가 비슷해도 공통 추상화를 만들지 않는다(CLAUDE.md 8.6).

## 테스트

**단위 테스트** (DB 없음)

- `ExpenseInput.Clean`, `ExpenseUpdate.Clean`: 빈 이름, 목록 밖 통화, 앞뒤 공백
- 한 달 합계: 여러 통화, 음수 금액, 환율 없는 통화
- 직전 기록 달 대비 증감: 증가, 감소, 이전 합계 0
- 표 정렬: USD 내림차순, USD 값이 없는 행은 맨 뒤
- `appliedRates`: 기존 테스트를 새 입력에 맞춘다

**HTTP 통합 테스트** (`cmd/server/expense_test.go`의 `TestExpenses`, 실제 DB)

- USD, KRW, AED 항목을 추가하면 원래 통화와 USD 값, 합계가 보이고 가장 가까운 달의 환율이 적용된다.
- 음수 금액(환불)은 합계를 줄인다.
- 잘못된 입력은 422다. 기존 이름에 잘못된 금액을 넣으면 통화가 잠긴 폼이 다시 그려진다.
- 기존 이름은 그 항목의 통화를 쓴다.
- 행 수정은 이름과 금액을 바꾸고, 다른 항목의 이름이면 422, 그 달에 없는 항목이면 404다.
- 삭제는 그 달의 행을 지우고, 쓰이지 않게 된 항목도 지운다.
- 빈 달은 직전 기록 달을 한 번 복사하고, 두 번째 복사와 이전 기록이 없는 달은 422다.
- 범위 밖 달은 404다.
- 사이드바에 Expenses 링크가 있다.

## 범위 밖

- 분류, 예산, 건별 기록
- 여러 달 보기(항목×월 표), 대시보드 표시
- 월별 메모
- 필터와 정렬 선택

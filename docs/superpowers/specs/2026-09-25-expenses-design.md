# 월별 지출

이 문서는 지출 기능의 설계다. 구현을 바꾸면 이 문서도 같이 고친다.

## 목적

어디에 얼마를 쓰는지 항목별로 본다. 월세, 넷플릭스, 비행기표 같은 지출을 날짜와 함께 적고, 한 달 단위로 합계를 USD로 본다. 각 행에는 고정 목록의 분류(category)를 하나 붙인다. 여러 달의 흐름은 월 합계 막대 차트 하나로 본다. 분류별 합계, 예산, 다른 달과의 비교는 하지 않는다.

지출은 자산 스냅샷과 별개다. 지출을 기록해도 스냅샷 금액은 바뀌지 않는다.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 날짜 | 행마다 날짜(`spent_on`, DB `date`)가 있다. 월 화면은 그 달에 속한 날짜의 행을 모은다. 한 건씩 적어도 되고, 한 달 치를 합쳐 한 행으로 적어도 된다 |
| 행 | 지출의 각 행이 날짜, 이름, 분류, 통화, 금액을 직접 가진다. 여러 달에 걸친 "항목" 개체는 없다. 이름은 이름일 뿐 식별자가 아니다 |
| 분류 | 고정 목록 하나(스냅샷 행의 `type`과 같은 방식): Housing(월세, DEWA, 인터넷), Food(장보기, 외식), Transport(주유, Salik, 택시), Bills(휴대폰, 구독, 보험료), Shopping, Travel(항공권, 숙소), Family & gifts(가족 송금, 선물), Loan payments(할부 상환), Other. DB 값은 `housing`, `food`, `transport`, `bills`, `shopping`, `travel`, `family`, `loan_payment`, `other`다. 빌려준 돈은 돌려받을 돈이라 지출이 아니다 |
| 이름 중복 | 같은 달 안에서도 이름이 겹쳐도 된다(예: KRW 식비와 AED 식비). 행은 자기 `id`로 수정·삭제한다 |
| 행 수정 | 날짜(그 달 안에서), 이름, 분류, 통화, 금액을 모두 바꿀 수 있고, 그 행만 바뀐다 |
| 이름 추천 | 추가 폼은 과거에 쓴 이름을 추천한다. 고르면 그 이름이 가장 최근에 쓴 분류와 통화를 폼에 복사할 뿐, 행끼리 연결하지 않는다 |
| 금액 | 통화의 최소 단위 정수. 부호 제한이 없다. 지출은 양수, 환불처럼 합계를 줄이는 값은 음수로 적는다 |
| 기록 가능한 달 | 스냅샷과 같다: 2024년 2월부터 `TIMEZONE` 기준 이번 달까지. 범위 밖은 404 |
| 환산 | 스냅샷과 같은 월 환율(`exchange_rates`)에서 가장 가까운 달의 값을 쓴다 |
| 날짜 기본값 | 추가 폼은 `TIMEZONE` 기준 오늘로 시작한다. 이번 달이 아닌 달의 화면에서는 오늘이 그 달에 없으므로 그 달 1일로 시작한다 |
| 조회 범위 | Records 패널은 선택한 달의 행과, 추천을 위한 이름 목록만 읽는다. Trend 패널의 차트만 모든 달의 행을 읽는다. 다른 달과 비교하지 않는다 |
| 달 테이블 | 두지 않는다. 그 달의 날짜로 된 행이 없으면 빈 달이다 |

## 데이터 모델: `0008.sql`

```sql
CREATE TABLE expenses (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    spent_on date NOT NULL,
    name text NOT NULL CHECK (name <> ''),
    category text NOT NULL CHECK (category IN ('housing', 'food', 'transport', 'bills', 'shopping', 'travel', 'family', 'loan_payment', 'other')),
    currency text NOT NULL CHECK (currency IN ('USD', 'KRW', 'AED', 'JPY')),
    amount bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX expenses_spent_on_idx ON expenses (spent_on);
```

- 한 달의 행은 `spent_on >= 그 달 1일 AND spent_on < 다음 달 1일`로 읽는다. 환율은 그 행 날짜의 달에서 가장 가까운 달의 값을 쓴다.
- 추가, 수정, 삭제는 각각 SQL 한 문장이다. 수정과 삭제는 `id`와 위의 날짜 범위로 그 달의 행만 건드린다.

## 화면

모든 라우트는 로그인이 필요하다. `{month}`는 `2006-01` 형식이다. 사이드바의 Finance 아래에 Assets 다음으로 Expenses 링크를 둔다.

| 라우트 | 동작 |
|---|---|
| `GET /finance/expenses` | 한 달의 지출. `month`가 없으면 이번 달. `category`, `order`, `direction`, `page`로 표를 거르고 정렬하고 나눈다 |
| `POST /finance/expenses/{month}/items/new` | 행 추가 |
| `GET /finance/expenses/{month}/items/{id}/edit` | 그 행만 입력칸으로 바꾼 목록. `{id}`는 행 id |
| `POST /finance/expenses/{month}/items/{id}/edit` | 날짜, 이름, 분류, 통화, 금액 수정 |
| `POST /finance/expenses/{month}/items/{id}/delete` | 행 삭제 |

행 링크와 폼의 주소는 지금의 필터·정렬·페이지를 쿼리로 달고 다니고, POST가 성공하면 같은 쿼리를 유지한 그 달의 목록(`/finance/expenses?direction=desc&month=…&order=date`)으로 303을 보낸다. 그 달의 행이 아닌 id를 수정하거나 삭제하면 404다.

### 지출 화면 (`expense_list.html`)

Assets 화면과 같은 구조다. 제목 "Expenses" 아래에 역할이 다른 두 패널(`<article>`)이 있다. 위의 Trend는 여러 달의 추이이고 선택한 달과 무관하다. 아래의 Records는 선택한 달의 기록이다.

위에서 아래로:

1. 제목 "Expenses". 헤더에는 제목만 있다.
2. Trend 패널(기록이 한 달이라도 있으면): "Spending by month". 달마다 그 달 합계(USD)를 막대 하나로 그린다(`newSpendingChart`, `spending_chart.js`).
   - 색은 `--series-1` 하나다. 계열이 하나라 범례는 없고, 툴팁은 `Total: USD 3,522.95`다.
   - 합계는 필터가 없을 때의 Records Total 카드와 같은 계산(`summarizeExpenses`)이다. 환율이 없는 통화가 있는 달과 기록이 없는 달은 빠진다.
   - 데이터는 Go가 `<script type="application/json">`에 USD 센트로 넣고 `spending_chart.js`가 읽는다. htmx가 `main`을 바꿔 끼우면(`htmx:load`) 새 캔버스에 다시 그린다.
3. Records 패널. 헤더에 "Records"와 월 선택이 있다. 월 선택은 필터 막대와 같은 폼(`form="filters"`)이라 월을 바꾸면 지금의 필터를 유지한 채 1페이지로 바로 이동한다. 아래 4~9가 이 패널 안에 있다.
4. 요약(기록이 있는 달만): Total(USD) 카드와 적용 환율(스냅샷 화면과 같은 한 줄). 둘 다 필터에 맞는 행(모든 페이지)으로 계산한다. 환율이 없는 통화가 있으면 카드 대신 빠진 통화를 알린다.
5. 필터 막대(`web.FilterBar`, `expenseQuery`): Category(비면 전체), Sort by(Date·USD, 기본 Date), Direction(기본 Descending). 목록 밖의 값이면 400이다. 필터는 표와 요약을 좁히고 Trend는 바꾸지 않는다. 필터를 바꾸면 1페이지로 간다.
6. 표: Date(`Sep 25`), Category, Name, Amount(원래 통화), USD, Edit·Delete. 합계 행은 없다(요약 카드와 중복). 좁은 화면에서는 날짜가 행의 제목이 되고 분류와 이름이 그 아래에 보인다.
   - Date 정렬: 날짜를 고른 방향으로 두고, 같은 날짜 안에서는 USD가 큰 순서, USD 값이 없는 행은 그 날짜의 맨 뒤다.
   - USD 정렬: USD를 고른 방향으로 두고, 같은 값이면 최신 날짜가 먼저다. USD 값이 없는 행은 방향과 무관하게 맨 뒤다.
   - 수정하면 그 행이 날짜, 분류, 이름, 금액, 통화 입력칸으로 바뀐다. 보이는 페이지에 없는 행 id를 수정하면 404다.
7. 페이지: 거르고 정렬한 행을 20개씩(`expensesPerPage`) 나눈다. 표 아래에 `21–40 of 110`과 Previous·Next 링크(지금의 필터 유지)가 있다. `page`가 숫자가 아니거나 1보다 작으면 400이고, 마지막 페이지보다 크면 마지막 페이지를 보인다(마지막 페이지의 마지막 행을 지워도 404가 되지 않게).
   - 그 달에 기록이 있는데 필터에 맞는 행이 없으면 "No expenses match the filters.", 기록이 없는 달이면 "No expenses in … yet."이다. 지출은 달마다 달라서 다른 달을 복사하는 기능은 없다(스냅샷과 다른 점).
8. 추가 폼: Date, Category, Name, Currency, Amount(표와 같은 순서).
   - Date는 `<input type="date">`이고 그 달의 1일~말일만 고를 수 있다(`min`, `max`). 기본값은 위의 날짜 기본값이다.
   - Name은 과거에 쓴 이름을 추천한다. 이름마다 한 번, 가장 최근의 분류·통화와 함께이고, 이번 달에 이미 있는 이름은 빼고 추천한다.
   - 추천한 이름을 입력하면 Category와 Currency가 그 값으로 채워지지만 잠기지 않는다(`item_form.js`).
   - 서버는 이름으로 아무것도 찾지 않고 폼에 온 값 그대로 행을 만든다.
9. 패널 밖, 환율 출처 링크 "Rates by Exchange Rate API".

422가 되는 경우:

- 추가와 수정: 이름이 비었거나 분류·통화가 목록 밖, 금액을 그 통화로 해석할 수 없음, 날짜가 없거나 형식이 틀리거나 그 달 밖("Pick a date in Sep 2026.").

htmx는 스냅샷 화면과 같은 방식으로 불러오고, Chart.js와 `spending_chart.js`는 이 화면에서만 불러온다. 필터 폼, 표, 페이지 링크, 추가 폼에 `hx-boost`를 건다. 서버는 항상 전체 페이지를 그린다.

## 코드 배치

| 파일 | 내용 |
|---|---|
| `internal/postgres/migrations/0008.sql` | `expenses` 테이블 |
| `internal/finance/expense.go` | `ExpenseCategory`와 표시 이름, `Expense`와 `USD()`, `ExpenseInput`과 `Clean`, 이름 추천(`expenseSuggestions`), `Store`의 조회와 쓰기(`Expenses(ctx, from, to)`, `AddExpense`, `UpdateExpense`, `DeleteExpense`) |
| `internal/finance/expense_list.go` | 한 달의 합계(`summarizeExpenses`), 표의 행(`expenseRows`), 목록 상태(`expenseQuery`: 쿼리 해석, 필터 막대, 분류로 거르기(`matching`), 정렬(`sort`), 페이지 나누기, 목록·행 주소), 날짜 기본값(`defaultExpenseDate`)과 달 범위(`inMonth`) |
| `internal/finance/spending_chart.go` | Trend 차트 데이터(`newSpendingChart`) |
| `internal/finance/expense_handler.go` | 지출 화면의 핸들러와 페이지 데이터, 폼 값을 `ExpenseInput`으로 바꾸는 `expenseInput` |
| `internal/finance/handler.go` | 지출 라우트 |
| `web/templates/expense_list.html` | 지출 화면 |
| `web/templates/layout.html` | 사이드바 링크 |
| `web/static/spending_chart.js` | Trend 막대 차트 |
| `web/static/item_form.js` | 추가 폼에서 추천한 이름의 분류와 통화를 채운다. 스냅샷 화면과 같이 쓴다 |

- 재사용하는 것: `money`(해석, 표시, 환산, 환율 표시), `currentMonth`, `monthsUntil`, `monthFilter`, `usdValue`, `itemForm`, `listView`, `appliedRates`, `requireRow`, 그리고 스냅샷과 같은 에러 값(`ErrInvalidItem`, `ErrItemNotFound`).
- 스냅샷 핸들러와 구조가 비슷해도 공통 추상화를 만들지 않는다(CLAUDE.md 8.6).

## 테스트

CLAUDE.md 9절(특히 9.14~9.18)을 따른다. 한 동작은 한 계층에서만 확인한다.

**단위 테스트** (DB 없음)

- `ExpenseInput.Clean`: 날짜 없음, 빈 이름, 목록 밖 분류, 앞뒤 공백
- `defaultExpenseDate`: 이번 달이면 오늘, 시간대에 따라 바뀌는 오늘, 다른 달이면 1일
- `parseExpenseQuery`: 모든 파라미터, 1보다 작은 페이지
- `expenseQuery.sort`: 날짜 내림차순(같은 날짜는 USD 내림차순, USD 없는 행은 뒤), 날짜 오름차순, USD 오름차순(같은 값은 최신 먼저, USD 없는 행은 맨 뒤)
- `expenseQuery.paginate`: 가운데 페이지의 앞뒤 링크, 마지막을 넘은 페이지는 마지막 페이지
- Trend 차트: 달별 합계를 오래된 달부터, 환율이 빠진 달 제외

**HTTP 통합 테스트** (`cmd/server/expense_test.go`의 `TestExpenses`, 실제 DB)

- USD, KRW, AED 행을 추가하면 분류, 원래 통화와 USD 값, 합계가 보이고 가장 가까운 달의 환율이 적용된다. 음수 금액(환불)은 합계를 줄인다.
- 같은 이름을 다른 통화로 두 번 적을 수 있고, 과거 이름이 분류·통화와 함께 추천되되 이번 달에 있는 이름은 빠진다.
- Trend 차트는 어느 달을 보든 모든 달의 합계를 오래된 달부터 담는다.
- 잘못된 입력은 handler의 실패 분기마다 하나씩(빈 이름, 해석할 수 없는 금액, 그 달 밖의 날짜) 422다.
- 행 수정은 날짜, 이름, 분류, 통화, 금액을 바꾸고(다른 달 날짜는 422), 다른 달의 행 id는 GET·POST 모두 404다.
- 삭제는 그 행만 지우고, 두 번째 삭제는 404다.
- 25행인 달은 1페이지에 20행과 2페이지 링크, 2페이지에 나머지 5행을 보인다. 분류 필터는 행과 Total을 함께 좁힌다. 필터와 페이지를 단 삭제는 같은 쿼리의 목록으로 돌아간다.
- 범위 밖 달은 404, 잘못된 월 형식은 400이다.

## 범위 밖

- 분류별 합계, 예산
- 분류별 추이(분류마다 색을 나눈 누적 막대), 여러 달 보기(항목×월 표), 다른 달과의 비교
- 다른 달 복사. 지출은 달마다 달라서 복사할 이유가 없다
- 월별 메모
- 이름 검색, 페이지 크기 선택

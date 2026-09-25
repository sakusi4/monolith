# 월별 지출 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 달마다 항목별 지출 합계를 적고, 한 달 단위로 합계와 직전 기록 달 대비 증감을 USD로 보는 `/finance/expenses` 화면을 만든다.

**Architecture:** `internal/finance` 안에 스냅샷과 같은 구조로 별도 테이블(`expense_items`, `expenses`), `Store` 메서드, 순수 계산(`expense_list.go`), 핸들러와 템플릿을 둔다. 스냅샷과 겹치는 작은 로직(USD 환산, 이름 비교, 증감 비율, 적용 환율, 합계 행 타입, 에러, 추가 폼 스크립트)은 먼저 공용으로 바꾼 뒤 두 화면이 같이 쓴다.

**Tech Stack:** Go 1.25+, `net/http`, `database/sql` + pgx, PostgreSQL, `html/template`, htmx 2.0.11

**Spec:** `docs/superpowers/specs/2026-09-25-expenses-design.md`

## Global Constraints

- CLAUDE.md가 모든 코드에 적용된다. 특히: 함수 본문 안에 주석 금지, doc comment는 계약이 있을 때만(영어 완전한 문장), `any` 금지, SQL은 손으로(키워드 대문자, 컬럼 명시, `$n`), 모든 DB 호출은 ctx 메서드, 에러는 `fmt.Errorf("...: %w", err)`.
- 화면 문구는 영어. 인라인 `style` 금지. CSS 값은 `:root` 변수만.
- 새 의존성 없음.
- 마이그레이션은 `internal/postgres/migrations/0008.sql`(4자리 번호만).
- 커밋하지 않는다. 사용자가 직접 커밋한다(사용자 지시가 이 계획의 커밋 단계를 대신한다).
- 완료 기준: `test -z "$(gofmt -l .)"`, `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run`, `TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable go test -race -count=1 ./...`, `go mod tidy -diff`가 모두 통과.
- 통합 테스트 명령의 환경변수: `TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"` (아래에서 `$TDB`로 줄여 쓴다). DB는 `docker compose up -d --wait db`로 띄운다.

## Review Focus

1. **환율이 하나도 없는 통화(JPY)의 항목** — 그 행의 USD는 `—`, 요약 카드 대신 빠진 통화 알림, 환율 줄에서는 제외. → Task 3의 `TestSummarizeExpenses`, `TestExpenseRows`.
2. **대소문자·공백만 다른 이름(`" rent "`)** — 같은 항목으로 취급해 그 통화를 쓴다. → Task 4 통합 테스트.
3. **기록 사이에 빈 달이 있을 때** — 전월 대비와 복사는 바로 전 달력 달이 아니라 직전 "기록" 달 기준. → Task 3 `TestMonthExpenses`.
4. **이전 달 합계가 0이거나 음수일 때의 비율** — 0이면 비율 없음, 음수 기준은 절댓값으로 나눈다. → Task 3 `TestSummarizeExpenses`.
5. **자기 이름의 대소문자만 바꿔 저장("Streaming" → "STREAMING")** — 422가 아니라 저장된다. → Task 4 통합 테스트.

---

### Task 1: 스냅샷 코드에서 공용 조각 떼어 내기 (동작 변화 없음)

**Files:**
- Modify: `internal/finance/snapshot_item.go` (에러 문구)
- Modify: `internal/finance/asset.go` (`sameName`)
- Modify: `internal/finance/snapshot.go` (`usdValue`)
- Modify: `internal/finance/summary.go` (`changePercent`, `itemRate`, `appliedRates`, `snapshotRates`)
- Modify: `internal/finance/summary_test.go` (`TestAppliedRates`, `TestChangePercent`)
- Modify: `internal/finance/snapshot_list.go`, `internal/finance/snapshot_list_test.go`, `internal/finance/snapshot_handler.go` (`assetTotal` → `rowTotal`, `itemForm.AssetID` 제거)
- Rename: `web/static/snapshot_list.js` → `web/static/item_form.js`
- Modify: `web/templates/snapshot_list.html` (스크립트 경로)

**Interfaces:**
- Produces:
  - `func sameName(name, stored string) bool`
  - `func usdValue(c money.Currency, amount int64, perUSD *big.Rat) (int64, bool)`
  - `func changePercent(change, base int64) string`
  - `type itemRate struct { Currency money.Currency; Month time.Time; PerUSD *big.Rat }`
  - `func appliedRates(month time.Time, rates []itemRate) []appliedRate`
  - `func snapshotRates(s Snapshot) []itemRate`
  - `type rowTotal struct { Amount int64; Currency money.Currency; HasAmount bool; USD int64; HasUSD bool }`
  - `itemForm` 필드: `Name, Type, Currency, Amount, Existing, Error, Submitted, URL, CancelURL`
  - 에러 값 이름은 그대로(`ErrInvalidItem`, `ErrItemNotFound`, `ErrItemExists`, `ErrNameTaken`, `ErrNothingToCopy`, `ErrMonthNotEmpty`), 문구만 일반화

- [ ] **Step 1: 테스트를 새 시그니처로 바꾼다**

`internal/finance/summary_test.go`의 `TestAppliedRates` 전체를 다음으로 바꾸고, 파일 끝에 `TestChangePercent`를 추가한다.

```go
func TestAppliedRates(t *testing.T) {
	september := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	rate := func(s string) *big.Rat {
		r, _ := new(big.Rat).SetString(s)
		return r
	}
	rates := []itemRate{
		{Currency: money.AED, Month: august, PerUSD: rate("3.6725")},
		{Currency: money.USD},
		{Currency: money.KRW, Month: september, PerUSD: rate("1374.61")},
		{Currency: money.KRW, Month: september, PerUSD: rate("1374.61")},
		{Currency: money.JPY},
	}
	want := []appliedRate{
		{Currency: money.KRW, PerUSD: "1,374.61", Month: september},
		{Currency: money.AED, PerUSD: "3.6725", Month: august, OtherMonth: true},
	}
	if got := appliedRates(september, rates); !reflect.DeepEqual(got, want) {
		t.Errorf("appliedRates() = %+v, want %+v without USD and the currency missing a rate", got, want)
	}
}

func TestChangePercent(t *testing.T) {
	tests := []struct {
		change, base int64
		want         string
	}{
		{100, 1000, "+10.0%"},
		{-100, 1000, "-10.0%"},
		{100, -1000, "+10.0%"},
		{0, 1000, "0.0%"},
		{100, 0, ""},
	}
	for _, tt := range tests {
		if got := changePercent(tt.change, tt.base); got != tt.want {
			t.Errorf("changePercent(%d, %d) = %q, want %q", tt.change, tt.base, got, tt.want)
		}
	}
}
```

`internal/finance/snapshot_list_test.go`의 `TestTotalOf`에서 `assetTotal`을 모두 `rowTotal`로 바꾼다(`want rowTotal`, 기대값 네 개).

- [ ] **Step 2: 컴파일이 실패하는지 확인**

Run: `go vet ./internal/finance/`
Expected: FAIL — `undefined: itemRate`, `undefined: changePercent`, `undefined: rowTotal`

- [ ] **Step 3: 구현**

`internal/finance/snapshot_item.go`의 에러 선언 중 네 개의 문구를 일반화한다(이름과 `errors.Is` 동작은 그대로):

```go
var ErrItemExists = errors.New("item is already in the month")

var ErrNameTaken = errors.New("name is taken")

var ErrNothingToCopy = errors.New("no earlier month to copy")

var ErrMonthNotEmpty = errors.New("month is not empty")
```

`internal/finance/asset.go`의 `assetNamed`를 다음으로 바꾼다:

```go
// sameName reports whether name, as the user typed it, is the stored name, ignoring case and
// surrounding spaces like the unique indexes on names.
func sameName(name, stored string) bool {
	return strings.ToLower(strings.TrimSpace(name)) == strings.ToLower(stored)
}

func assetNamed(assets []Asset, name string) (Asset, bool) {
	i := slices.IndexFunc(assets, func(a Asset) bool { return sameName(name, a.Name) })
	if i < 0 {
		return Asset{}, false
	}
	return assets[i], true
}
```

`internal/finance/snapshot.go`의 `SnapshotItem.USD`를 다음으로 바꾼다:

```go
func (it SnapshotItem) USD() (int64, bool) {
	return usdValue(it.Asset.Currency, it.Amount, it.PerUSD)
}

// usdValue converts amount in c to USD cents. It reports false when c is not USD and has no rate.
func usdValue(c money.Currency, amount int64, perUSD *big.Rat) (int64, bool) {
	if c == money.USD {
		return amount, true
	}
	if perUSD == nil {
		return 0, false
	}
	return money.ToUSD(c, amount, perUSD), true
}
```

`internal/finance/summary.go`:
- `summarize`의 비율 세 줄

```go
	s.ChangePercent = percent(s.Change, max(prevTotal, -prevTotal))
	if s.Change > 0 && s.ChangePercent != "" {
		s.ChangePercent = "+" + s.ChangePercent
	}
```

을 `s.ChangePercent = changePercent(s.Change, prevTotal)` 한 줄로 바꾼다.
- `percent` 아래에 추가:

```go
// changePercent formats change as a signed percentage of the size of base, as in "+18.9%".
// It returns an empty string when base is zero.
func changePercent(change, base int64) string {
	p := percent(change, max(base, -base))
	if change > 0 && p != "" {
		return "+" + p
	}
	return p
}
```

- `appliedRate`의 doc comment와 `appliedRates`를 다음으로 바꾼다:

```go
// appliedRate is the exchange rate that values a month's items in one currency.
// OtherMonth reports that the rate comes from Month because the month has none.
type appliedRate struct {
	Currency   money.Currency
	PerUSD     string
	Month      time.Time
	OtherMonth bool
}

// itemRate is the exchange rate that values one item: the rate of Month, the month with a rate
// closest to the item's month. PerUSD is nil when the currency has no rate.
type itemRate struct {
	Currency money.Currency
	Month    time.Time
	PerUSD   *big.Rat
}

// appliedRates lists rates once per currency in the order of money.Currencies, leaving out
// the currencies without a rate, such as USD. OtherMonth compares each rate's month with month.
func appliedRates(month time.Time, rates []itemRate) []appliedRate {
	var applied []appliedRate
	for _, c := range money.Currencies {
		i := slices.IndexFunc(rates, func(r itemRate) bool { return r.Currency == c && r.PerUSD != nil })
		if i < 0 {
			continue
		}
		r := rates[i]
		applied = append(applied, appliedRate{
			Currency:   c,
			PerUSD:     money.FormatRate(r.PerUSD),
			Month:      r.Month,
			OtherMonth: !r.Month.Equal(month),
		})
	}
	return applied
}

func snapshotRates(s Snapshot) []itemRate {
	rates := make([]itemRate, len(s.Items))
	for i, it := range s.Items {
		rates[i] = itemRate{Currency: it.Asset.Currency, Month: it.RateMonth, PerUSD: it.PerUSD}
	}
	return rates
}
```

`internal/finance/snapshot_handler.go`:
- `page.Rates = appliedRates(cur)` → `page.Rates = appliedRates(cur.Month, snapshotRates(cur))`
- `snapshotListPage.Total`의 타입 `assetTotal` → `rowTotal`
- `itemForm`에서 `AssetID int64` 필드를 지우고, `editableRows`의 `edit.AssetID, edit.Currency = row.Asset.ID, row.Asset.Currency`를 `edit.Currency = row.Asset.Currency`로 바꾼다. `itemForm`의 doc comment 둘째 문장을 다음으로 바꾼다: `// Existing marks an add form whose name is an existing asset or expense item, which fixes Type and Currency.`

`internal/finance/snapshot_list.go`: `assetTotal`을 `rowTotal`로 바꾸고(타입 선언, doc comment 첫 단어, `totalOf`의 반환 타입과 본문 두 곳), doc comment를 `// rowTotal sums a table's rows. Amount is set only when they share one currency,`로 시작하게 한다.

JS 파일 이름을 바꾸고 변수 이름을 일반화한다:

```bash
git mv web/static/snapshot_list.js web/static/item_form.js
```

`web/static/item_form.js` 전체:

```js
document.addEventListener("input", (event) => {
  const input = event.target;
  if (input.form?.id !== "add-item" || input.name !== "name") {
    return;
  }
  const name = input.value.trim().toLowerCase();
  const item = [...input.list.options].find((option) => option.value.toLowerCase() === name);
  for (const select of input.form.querySelectorAll("select")) {
    select.disabled = item !== undefined;
    if (item) {
      select.value = item.dataset[select.name];
    }
  }
});
```

`web/templates/snapshot_list.html`의 `<script src="/static/snapshot_list.js" defer></script>`를 `<script src="/static/item_form.js" defer></script>`로 바꾼다.

- [ ] **Step 4: 전부 통과하는지 확인**

Run: `gofmt -l internal; go vet ./... && $TDB go test -count=1 ./...`
Expected: 출력 없는 gofmt, 모든 패키지 `ok` (기존 통합 테스트가 동작 불변을 보장한다)

---

### Task 2: 테이블과 지출 도메인·Store

**Files:**
- Create: `internal/postgres/migrations/0008.sql`
- Create: `internal/finance/expense.go`
- Test: `internal/finance/expense_test.go`

**Interfaces:**
- Consumes: `sameName`, `usdValue`, `requireRow`, `isPgError`, `uniqueViolation`, 에러 값(Task 1)
- Produces:
  - `type ExpenseItem struct { ID int64; Name string; Currency money.Currency }`
  - `type Expense struct { Month time.Time; Item ExpenseItem; Amount int64; RateMonth time.Time; PerUSD *big.Rat }`, `func (e Expense) USD() (int64, bool)`
  - `type ExpenseInput struct { Name string; Currency money.Currency; Amount int64 }`, `Clean() (ExpenseInput, error)`
  - `type ExpenseUpdate struct { Name string; Amount int64 }`, `Clean() (ExpenseUpdate, error)`
  - `func expenseItemNamed(items []ExpenseItem, name string) (ExpenseItem, bool)`
  - `func (s *Store) Expenses(ctx) ([]Expense, error)` — 모든 달, 최신 달 먼저, 달 안에서는 이름순
  - `func (s *Store) ExpenseItems(ctx) ([]ExpenseItem, error)`
  - `func (s *Store) AddExpense(ctx, month time.Time, in ExpenseInput) error` — `ErrInvalidItem`, `ErrItemExists`
  - `func (s *Store) UpdateExpense(ctx, month time.Time, itemID int64, in ExpenseUpdate) error` — `ErrInvalidItem`, `ErrItemNotFound`, `ErrNameTaken`
  - `func (s *Store) DeleteExpense(ctx, month time.Time, itemID int64) error` — `ErrItemNotFound`
  - `func (s *Store) CopyPreviousExpenses(ctx, month time.Time) error` — `ErrMonthNotEmpty`, `ErrNothingToCopy`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/finance/expense_test.go`:

```go
package finance

import (
	"errors"
	"testing"

	"github.com/sakusi4/monolith/internal/money"
)

func TestExpenseInput_Clean(t *testing.T) {
	tests := []struct {
		name    string
		in      ExpenseInput
		want    ExpenseInput
		wantErr bool
	}{
		{"trims the name", ExpenseInput{Name: "  Rent ", Currency: money.AED, Amount: -5}, ExpenseInput{Name: "Rent", Currency: money.AED, Amount: -5}, false},
		{"empty name", ExpenseInput{Name: "  ", Currency: money.USD}, ExpenseInput{}, true},
		{"unknown currency", ExpenseInput{Name: "Rent", Currency: "EUR"}, ExpenseInput{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Clean()
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("Clean() = %+v, %v, want %+v, error %v", got, err, tt.want, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidItem) {
				t.Errorf("Clean() error = %v, want ErrInvalidItem", err)
			}
		})
	}
}

func TestExpenseUpdate_Clean(t *testing.T) {
	tests := []struct {
		name    string
		in      ExpenseUpdate
		want    ExpenseUpdate
		wantErr bool
	}{
		{"trims the name", ExpenseUpdate{Name: " Rent ", Amount: 1}, ExpenseUpdate{Name: "Rent", Amount: 1}, false},
		{"empty name", ExpenseUpdate{Name: " "}, ExpenseUpdate{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Clean()
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("Clean() = %+v, %v, want %+v, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/finance/ -run 'TestExpense'`
Expected: FAIL — `undefined: ExpenseInput`

- [ ] **Step 3: 마이그레이션 작성**

`internal/postgres/migrations/0008.sql`:

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

- [ ] **Step 4: 도메인과 Store 구현**

`internal/finance/expense.go`:

```go
package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

type ExpenseItem struct {
	ID       int64
	Name     string
	Currency money.Currency
}

// Expense is an item's total for Month. RateMonth and PerUSD hold the exchange rate of the month
// closest to Month; PerUSD is nil for USD items and for currencies without any stored rate.
type Expense struct {
	Month     time.Time
	Item      ExpenseItem
	Amount    int64
	RateMonth time.Time
	PerUSD    *big.Rat
}

// ExpenseInput adds an item's total to a month. Currency applies only when Name is a new item.
type ExpenseInput struct {
	Name     string
	Currency money.Currency
	Amount   int64
}

// ExpenseUpdate changes an item's total for a month. Name belongs to the item, so it changes
// it in every month.
type ExpenseUpdate struct {
	Name   string
	Amount int64
}

func (e Expense) USD() (int64, bool) {
	return usdValue(e.Item.Currency, e.Amount, e.PerUSD)
}

func (in ExpenseInput) Clean() (ExpenseInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		return ExpenseInput{}, fmt.Errorf("%w: name is empty", ErrInvalidItem)
	case !in.Currency.IsValid():
		return ExpenseInput{}, fmt.Errorf("%w: unknown currency %q", ErrInvalidItem, in.Currency)
	}
	return in, nil
}

func (in ExpenseUpdate) Clean() (ExpenseUpdate, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return ExpenseUpdate{}, fmt.Errorf("%w: name is empty", ErrInvalidItem)
	}
	return in, nil
}

func expenseItemNamed(items []ExpenseItem, name string) (ExpenseItem, bool) {
	i := slices.IndexFunc(items, func(it ExpenseItem) bool { return sameName(name, it.Name) })
	if i < 0 {
		return ExpenseItem{}, false
	}
	return items[i], true
}

// Expenses returns every month's expenses, newest month first and by item name within a month.
func (s *Store) Expenses(ctx context.Context) ([]Expense, error) {
	query := `
		SELECT e.month, i.id, i.name, i.currency, e.amount, r.month, r.per_usd
		FROM expenses e
		JOIN expense_items i ON i.id = e.item_id
		LEFT JOIN LATERAL (
			SELECT er.month, er.per_usd::text AS per_usd
			FROM exchange_rates er
			WHERE er.currency = i.currency
			ORDER BY abs(er.month - e.month), er.month
			LIMIT 1
		) r ON true
		ORDER BY e.month DESC, lower(i.name), i.id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query expenses: %w", err)
	}
	defer rows.Close()
	expenses := []Expense{}
	for rows.Next() {
		var (
			e         Expense
			rateMonth sql.Null[time.Time]
			perUSD    sql.Null[string]
		)
		if err := rows.Scan(&e.Month, &e.Item.ID, &e.Item.Name, &e.Item.Currency, &e.Amount, &rateMonth, &perUSD); err != nil {
			return nil, fmt.Errorf("scan expense: %w", err)
		}
		if perUSD.Valid {
			rate, ok := new(big.Rat).SetString(perUSD.V)
			if !ok {
				return nil, fmt.Errorf("parse rate %q", perUSD.V)
			}
			e.RateMonth, e.PerUSD = rateMonth.V, rate
		}
		expenses = append(expenses, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query expenses: %w", err)
	}
	return expenses, nil
}

func (s *Store) ExpenseItems(ctx context.Context) ([]ExpenseItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, currency FROM expense_items ORDER BY lower(name), id`)
	if err != nil {
		return nil, fmt.Errorf("query expense items: %w", err)
	}
	defer rows.Close()
	items := []ExpenseItem{}
	for rows.Next() {
		var it ExpenseItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Currency); err != nil {
			return nil, fmt.Errorf("scan expense item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query expense items: %w", err)
	}
	return items, nil
}

// AddExpense records in for month, creating the item when it does not exist.
// It returns ErrItemExists when the item is already in the month.
func (s *Store) AddExpense(ctx context.Context, month time.Time, in ExpenseInput) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	var itemID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM expense_items WHERE lower(name) = lower($1)`, in.Name).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		query := `INSERT INTO expense_items (name, currency) VALUES ($1, $2) RETURNING id`
		err = tx.QueryRowContext(ctx, query, in.Name, in.Currency).Scan(&itemID)
	}
	if err != nil {
		return fmt.Errorf("find or insert expense item: %w", err)
	}
	query := `INSERT INTO expenses (month, item_id, amount) VALUES ($1, $2, $3)`
	_, err = tx.ExecContext(ctx, query, month, itemID, in.Amount)
	if isPgError(err, uniqueViolation) {
		return ErrItemExists
	}
	if err != nil {
		return fmt.Errorf("insert expense: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// UpdateExpense returns ErrItemNotFound when the item is not in month and ErrNameTaken when
// another item has the new name.
func (s *Store) UpdateExpense(ctx context.Context, month time.Time, itemID int64, in ExpenseUpdate) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	query := `UPDATE expenses SET amount = $3 WHERE month = $1 AND item_id = $2`
	res, err := tx.ExecContext(ctx, query, month, itemID, in.Amount)
	if err != nil {
		return fmt.Errorf("update expense: %w", err)
	}
	if err := requireRow(res, ErrItemNotFound); err != nil {
		return err
	}
	query = `UPDATE expense_items SET name = $2, updated_at = now() WHERE id = $1`
	_, err = tx.ExecContext(ctx, query, itemID, in.Name)
	if isPgError(err, uniqueViolation) {
		return ErrNameTaken
	}
	if err != nil {
		return fmt.Errorf("update expense item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// DeleteExpense removes the item from month, and the item itself when no month has it any more.
// It returns ErrItemNotFound when the item is not in month.
func (s *Store) DeleteExpense(ctx context.Context, month time.Time, itemID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM expenses WHERE month = $1 AND item_id = $2`, month, itemID)
	if err != nil {
		return fmt.Errorf("delete expense: %w", err)
	}
	if err := requireRow(res, ErrItemNotFound); err != nil {
		return err
	}
	query := `
		DELETE FROM expense_items i WHERE i.id = $1
		AND NOT EXISTS (SELECT 1 FROM expenses e WHERE e.item_id = i.id)`
	if _, err := tx.ExecContext(ctx, query, itemID); err != nil {
		return fmt.Errorf("delete unused expense item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// CopyPreviousExpenses fills an empty month with the expenses of the latest earlier month
// that has any. It returns ErrMonthNotEmpty or ErrNothingToCopy when it cannot.
func (s *Store) CopyPreviousExpenses(ctx context.Context, month time.Time) error {
	var has bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM expenses WHERE month = $1)`, month).Scan(&has)
	if err != nil {
		return fmt.Errorf("check expenses: %w", err)
	}
	if has {
		return ErrMonthNotEmpty
	}
	query := `
		INSERT INTO expenses (month, item_id, amount)
		SELECT $1, e.item_id, e.amount
		FROM expenses e
		WHERE e.month = (SELECT max(month) FROM expenses WHERE month < $1)`
	res, err := s.db.ExecContext(ctx, query, month)
	if isPgError(err, uniqueViolation) {
		return ErrMonthNotEmpty
	}
	if err != nil {
		return fmt.Errorf("copy expenses: %w", err)
	}
	return requireRow(res, ErrNothingToCopy)
}
```

- [ ] **Step 5: 통과 확인**

Run: `go vet ./internal/finance/ && go test ./internal/finance/ -run 'TestExpense'`
Expected: `ok`

---

### Task 3: 한 달 화면의 계산 (순수 함수)

**Files:**
- Create: `internal/finance/expense_list.go`
- Test: `internal/finance/expense_list_test.go`

**Interfaces:**
- Consumes: `Expense`, `usdValue`(간접), `changePercent`, `itemRate`, `rowTotal`(Task 1–2)
- Produces:
  - `func monthExpenses(expenses []Expense, month time.Time) (cur, prev []Expense)`
  - `type expenseSummary struct { Total int64; Missing []money.Currency; HasChange bool; Change int64; ChangePercent string; PreviousMonth time.Time }`
  - `func summarizeExpenses(cur, prev []Expense) expenseSummary`
  - `type expenseRow struct { Expense Expense; USD int64; HasUSD bool }`
  - `func expenseRows(expenses []Expense) []expenseRow`
  - `func expenseTotalOf(rows []expenseRow) rowTotal`
  - `func expenseRates(expenses []Expense) []itemRate`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/finance/expense_list_test.go`:

```go
package finance

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

func expenseMonth(m time.Month) time.Time {
	return time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)
}

func usdExpense(month time.Time, id, amount int64) Expense {
	return Expense{Month: month, Item: ExpenseItem{ID: id, Currency: money.USD}, Amount: amount}
}

func TestMonthExpenses(t *testing.T) {
	expenses := []Expense{
		usdExpense(expenseMonth(time.September), 1, 100),
		usdExpense(expenseMonth(time.June), 1, 50),
		usdExpense(expenseMonth(time.June), 2, 60),
		usdExpense(expenseMonth(time.May), 1, 70),
	}
	tests := []struct {
		name     string
		month    time.Time
		wantCur  int
		wantPrev int
	}{
		{"a gap compares with the latest recorded month", expenseMonth(time.September), 1, 2},
		{"an empty month still finds the previous one", expenseMonth(time.August), 0, 2},
		{"the first month has no previous", expenseMonth(time.May), 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cur, prev := monthExpenses(expenses, tt.month)
			if len(cur) != tt.wantCur || len(prev) != tt.wantPrev {
				t.Errorf("monthExpenses(%s) = %d, %d expenses, want %d, %d", tt.month.Format("2006-01"), len(cur), len(prev), tt.wantCur, tt.wantPrev)
			}
		})
	}
}

func TestSummarizeExpenses(t *testing.T) {
	june := expenseMonth(time.June)
	krw := Expense{Month: expenseMonth(time.July), Item: ExpenseItem{ID: 3, Currency: money.KRW}, Amount: 200000, PerUSD: big.NewRat(1000, 1)}
	jpy := Expense{Month: expenseMonth(time.July), Item: ExpenseItem{ID: 4, Currency: money.JPY}, Amount: 100}
	tests := []struct {
		name string
		cur  []Expense
		prev []Expense
		want expenseSummary
	}{
		{
			name: "a refund lowers the total and the change compares with the previous month",
			cur:  []Expense{usdExpense(expenseMonth(time.July), 1, 30000), usdExpense(expenseMonth(time.July), 2, -5000), krw},
			prev: []Expense{usdExpense(june, 1, 40000)},
			want: expenseSummary{Total: 45000, HasChange: true, Change: 5000, ChangePercent: "+12.5%", PreviousMonth: june},
		},
		{
			name: "no previous month has no change",
			cur:  []Expense{usdExpense(expenseMonth(time.July), 1, 100)},
			want: expenseSummary{Total: 100},
		},
		{
			name: "a zero previous total has no percentage",
			cur:  []Expense{usdExpense(expenseMonth(time.July), 1, 100)},
			prev: []Expense{usdExpense(june, 1, 0)},
			want: expenseSummary{Total: 100, HasChange: true, Change: 100, PreviousMonth: june},
		},
		{
			name: "a negative previous total divides by its size",
			cur:  []Expense{usdExpense(expenseMonth(time.July), 1, 100)},
			prev: []Expense{usdExpense(june, 1, -100)},
			want: expenseSummary{Total: 100, HasChange: true, Change: 200, ChangePercent: "+200.0%", PreviousMonth: june},
		},
		{
			name: "a currency without a rate leaves only the total and the currency",
			cur:  []Expense{usdExpense(expenseMonth(time.July), 1, 100), jpy},
			prev: []Expense{usdExpense(june, 1, 50)},
			want: expenseSummary{Total: 100, Missing: []money.Currency{money.JPY}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeExpenses(tt.cur, tt.prev); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("summarizeExpenses() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestExpenseRows(t *testing.T) {
	july := expenseMonth(time.July)
	jpy := Expense{Month: july, Item: ExpenseItem{ID: 9, Currency: money.JPY}, Amount: 100}
	rows := expenseRows([]Expense{usdExpense(july, 1, 100), jpy, usdExpense(july, 2, 300), usdExpense(july, 3, -50)})
	var got []int64
	for _, r := range rows {
		got = append(got, r.Expense.Item.ID)
	}
	if want := []int64{2, 1, 3, 9}; !reflect.DeepEqual(got, want) {
		t.Errorf("expenseRows() order = %v, want %v by USD with the row without a rate last", got, want)
	}
}

func TestExpenseTotalOf(t *testing.T) {
	july := expenseMonth(time.July)
	krw := Expense{Month: july, Item: ExpenseItem{ID: 3, Currency: money.KRW}, Amount: 5000, PerUSD: big.NewRat(1000, 1)}
	jpy := Expense{Month: july, Item: ExpenseItem{ID: 4, Currency: money.JPY}, Amount: 1}
	tests := []struct {
		name     string
		expenses []Expense
		want     rowTotal
	}{
		{"one currency sums amounts", []Expense{usdExpense(july, 1, 100), usdExpense(july, 2, -30)}, rowTotal{Amount: 70, Currency: money.USD, HasAmount: true, USD: 70, HasUSD: true}},
		{"mixed currencies sum only USD", []Expense{usdExpense(july, 1, 1000), krw}, rowTotal{Amount: 6000, Currency: money.USD, USD: 1500, HasUSD: true}},
		{"a row without USD hides the USD total", []Expense{usdExpense(july, 1, 100), jpy}, rowTotal{Amount: 101, Currency: money.USD, USD: 100}},
		{"no rows", nil, rowTotal{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expenseTotalOf(expenseRows(tt.expenses)); got != tt.want {
				t.Errorf("expenseTotalOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/finance/ -run 'TestMonthExpenses|TestSummarizeExpenses|TestExpenseRows|TestExpenseTotalOf'`
Expected: FAIL — `undefined: monthExpenses`

- [ ] **Step 3: 구현**

`internal/finance/expense_list.go`:

```go
package finance

import (
	"cmp"
	"slices"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

// expenseSummary values a month's expenses in USD and compares them with the latest earlier
// month that has any. Only Total and Missing are set when a currency has no rate.
type expenseSummary struct {
	Total         int64
	Missing       []money.Currency
	HasChange     bool
	Change        int64
	ChangePercent string
	PreviousMonth time.Time
}

type expenseRow struct {
	Expense Expense
	USD     int64
	HasUSD  bool
}

// monthExpenses splits expenses, which are newest month first, into those of month and those
// of the latest earlier month that has any.
func monthExpenses(expenses []Expense, month time.Time) (cur, prev []Expense) {
	var prevMonth time.Time
	for _, e := range expenses {
		switch {
		case e.Month.Equal(month):
			cur = append(cur, e)
		case e.Month.Before(month) && (prevMonth.IsZero() || e.Month.Equal(prevMonth)):
			prevMonth = e.Month
			prev = append(prev, e)
		}
	}
	return cur, prev
}

func summarizeExpenses(cur, prev []Expense) expenseSummary {
	total, missing := expenseTotal(cur)
	s := expenseSummary{Total: total, Missing: missing}
	if len(missing) > 0 || len(prev) == 0 {
		return s
	}
	prevTotal, prevMissing := expenseTotal(prev)
	if len(prevMissing) > 0 {
		return s
	}
	s.HasChange, s.Change, s.PreviousMonth = true, total-prevTotal, prev[0].Month
	s.ChangePercent = changePercent(s.Change, prevTotal)
	return s
}

// expenseTotal sums expenses in USD cents, leaving out the currencies it returns, which have no rate.
func expenseTotal(expenses []Expense) (int64, []money.Currency) {
	var total int64
	var missing []money.Currency
	for _, e := range expenses {
		usd, ok := e.USD()
		if !ok {
			if !slices.Contains(missing, e.Item.Currency) {
				missing = append(missing, e.Item.Currency)
			}
			continue
		}
		total += usd
	}
	return total, missing
}

// expenseRows orders expenses by USD value, largest first, with those without a USD value last.
func expenseRows(expenses []Expense) []expenseRow {
	rows := make([]expenseRow, len(expenses))
	for i, e := range expenses {
		rows[i] = expenseRow{Expense: e}
		rows[i].USD, rows[i].HasUSD = e.USD()
	}
	slices.SortStableFunc(rows, func(a, b expenseRow) int {
		if a.HasUSD != b.HasUSD {
			if a.HasUSD {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.USD, a.USD)
	})
	return rows
}

func expenseTotalOf(rows []expenseRow) rowTotal {
	if len(rows) == 0 {
		return rowTotal{}
	}
	t := rowTotal{Currency: rows[0].Expense.Item.Currency, HasAmount: true, HasUSD: true}
	for _, r := range rows {
		t.HasAmount = t.HasAmount && r.Expense.Item.Currency == t.Currency
		t.HasUSD = t.HasUSD && r.HasUSD
		t.Amount += r.Expense.Amount
		t.USD += r.USD
	}
	return t
}

func expenseRates(expenses []Expense) []itemRate {
	rates := make([]itemRate, len(expenses))
	for i, e := range expenses {
		rates[i] = itemRate{Currency: e.Item.Currency, Month: e.RateMonth, PerUSD: e.PerUSD}
	}
	return rates
}
```

- [ ] **Step 4: 통과 확인**

Run: `go vet ./internal/finance/ && go test ./internal/finance/`
Expected: `ok`

---

### Task 4: 지출 화면 (핸들러, 라우트, 템플릿, 사이드바)과 통합 테스트

**Files:**
- Create: `internal/finance/expense_handler.go`
- Modify: `internal/finance/handler.go` (라우트 6개)
- Create: `web/templates/expense_list.html`
- Modify: `web/templates/layout.html` (사이드바 링크)
- Test: `cmd/server/expense_test.go`

**Interfaces:**
- Consumes: Task 1–3의 모든 것, 그리고 기존 `handler`, `h.months()`, `h.hasMonth`, `pathID`, `monthFilter`, `monthLayout`, `monthLabelLayout`, `amountExample`, `listView`, `itemForm`, `copyOffer`, `web.Render`, `web.ServerError`
- Produces: 라우트 `GET /finance/expenses`, `POST /finance/expenses/{month}/items/new`, `GET|POST /finance/expenses/{month}/items/{id}/edit`, `POST /finance/expenses/{month}/items/{id}/delete`, `POST /finance/expenses/{month}/copy`

- [ ] **Step 1: 실패하는 통합 테스트 작성**

`cmd/server/expense_test.go`:

```go
package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestExpenses(t *testing.T) {
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
	wantContains := func(t *testing.T, path string, texts ...string) {
		t.Helper()
		got := body(t, path)
		for _, text := range texts {
			if !strings.Contains(got, text) {
				t.Errorf("GET %s does not contain %q:\n%s", path, text, got)
			}
		}
	}
	wantStatus := func(t *testing.T, path string, form url.Values, want int) {
		t.Helper()
		if rec := s.post(t, path, form, session); rec.Code != want {
			t.Errorf("POST %s %v = %d, want %d", path, form, rec.Code, want)
		}
	}
	add := func(t *testing.T, month, name, currency, amount string) {
		t.Helper()
		form := url.Values{"name": {name}, "currency": {currency}, "amount": {amount}}
		wantRedirect(t, s.post(t, "/finance/expenses/"+month+"/items/new", form, session), "/finance/expenses?month="+month)
	}
	itemID := func(t *testing.T, name string) string {
		t.Helper()
		var id int64
		if err := s.db.QueryRowContext(t.Context(), `SELECT id FROM expense_items WHERE name = $1`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return strconv.FormatInt(id, 10)
	}
	count := func(t *testing.T, query string) int {
		t.Helper()
		var n int
		if err := s.db.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("the sidebar links the empty current month", func(t *testing.T) {
		wantContains(t, "/finance/expenses", `href="/finance/expenses" aria-current="page"`, "No expenses in ", "Add to ")
	})

	if _, err := s.db.ExecContext(t.Context(), `
		INSERT INTO exchange_rates (month, currency, per_usd)
		VALUES ('2024-12-01', 'KRW', 1000), ('2024-12-01', 'AED', 4)`); err != nil {
		t.Fatal(err)
	}

	t.Run("items are valued in USD and a refund lowers the total", func(t *testing.T) {
		add(t, "2025-01", "Rent", "AED", "2,000.00")
		add(t, "2025-01", "Groceries", "KRW", "300,000")
		add(t, "2025-01", "Netflix", "USD", "15.49")
		add(t, "2025-01", "Refund", "USD", "-5.49")
		wantContains(t, "/finance/expenses?month=2025-01",
			"AED 2,000.00", "USD 500.00", "KRW 300,000", "USD 300.00", "USD -5.49", "USD 810.00",
			"1 USD = KRW 1,000 (Dec 2024) · AED 4 (Dec 2024)")
	})

	t.Run("invalid items show the form again", func(t *testing.T) {
		for _, form := range []url.Values{
			{"name": {"  "}, "currency": {"USD"}, "amount": {"1"}},
			{"name": {"Gym"}, "currency": {"EUR"}, "amount": {"1"}},
			{"name": {"Gym"}, "currency": {"KRW"}, "amount": {"1.5"}},
			{"name": {"rent"}, "currency": {"AED"}, "amount": {"1"}},
		} {
			wantStatus(t, "/finance/expenses/2025-01/items/new", form, http.StatusUnprocessableEntity)
		}
		rec := s.post(t, "/finance/expenses/2025-02/items/new", url.Values{"name": {" groceries "}, "amount": {"1.5"}}, session)
		if got := rec.Body.String(); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(got, `<select name="currency" disabled>`) || !strings.Contains(got, `<option value="KRW" selected>`) {
			t.Errorf("existing item with an invalid amount = %d, want 422 with its currency fixed:\n%s", rec.Code, got)
		}
	})

	t.Run("an existing name takes the item's currency", func(t *testing.T) {
		add(t, "2025-02", " rent ", "", "2,100.00")
		wantContains(t, "/finance/expenses?month=2025-02", "Rent", "AED 2,100.00", "USD 525.00", "USD -285.00 (-35.2%) vs Jan 2025")
		if n := count(t, `SELECT count(*) FROM expense_items`); n != 4 {
			t.Errorf("expense items = %d, want 4", n)
		}
	})

	t.Run("an empty month copies the latest recorded month once", func(t *testing.T) {
		wantContains(t, "/finance/expenses?month=2025-04", "Copy Feb 2025 (1 items)")
		wantRedirect(t, s.post(t, "/finance/expenses/2025-04/copy", nil, session), "/finance/expenses?month=2025-04")
		wantContains(t, "/finance/expenses?month=2025-04", "AED 2,100.00")
		wantStatus(t, "/finance/expenses/2025-04/copy", nil, http.StatusUnprocessableEntity)
		wantStatus(t, "/finance/expenses/2024-02/copy", nil, http.StatusUnprocessableEntity)
	})

	t.Run("edit changes the name and amount", func(t *testing.T) {
		netflix := itemID(t, "Netflix")
		edit := "/finance/expenses/2025-01/items/" + netflix + "/edit"
		wantContains(t, edit, `form="edit-item"`, `value="15.49"`, `value="Netflix"`)
		wantRedirect(t, s.post(t, edit, url.Values{"name": {"Streaming"}, "amount": {"20.00"}}, session), "/finance/expenses?month=2025-01")
		wantContains(t, "/finance/expenses?month=2025-01", "Streaming", "USD 20.00")
		wantRedirect(t, s.post(t, edit, url.Values{"name": {"STREAMING"}, "amount": {"20.00"}}, session), "/finance/expenses?month=2025-01")
		wantStatus(t, edit, url.Values{"name": {"rent"}, "amount": {"1"}}, http.StatusUnprocessableEntity)
		wantStatus(t, edit, url.Values{"name": {"STREAMING"}, "amount": {"1.555"}}, http.StatusUnprocessableEntity)
		if rec := s.get(t, "/finance/expenses/2025-02/items/"+netflix+"/edit", session); rec.Code != http.StatusNotFound {
			t.Errorf("edit of an item outside the month = %d, want 404", rec.Code)
		}
	})

	t.Run("delete removes the month's row and the unused item", func(t *testing.T) {
		streaming := itemID(t, "STREAMING")
		del := "/finance/expenses/2025-01/items/" + streaming + "/delete"
		wantRedirect(t, s.post(t, del, nil, session), "/finance/expenses?month=2025-01")
		if n := count(t, `SELECT count(*) FROM expense_items WHERE name = 'STREAMING'`); n != 0 {
			t.Errorf("STREAMING is still an item after leaving its only month")
		}
		wantStatus(t, del, nil, http.StatusNotFound)
	})

	t.Run("months outside the range are rejected", func(t *testing.T) {
		for path, want := range map[string]int{
			"/finance/expenses?month=2024-01":        http.StatusNotFound,
			"/finance/expenses?month=2099-01":        http.StatusNotFound,
			"/finance/expenses?month=january":        http.StatusBadRequest,
			"/finance/expenses/2099-01/items/1/edit": http.StatusNotFound,
		} {
			if rec := s.get(t, path, session); rec.Code != want {
				t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
			}
		}
		wantStatus(t, "/finance/expenses/2099-01/items/new", url.Values{"name": {"x"}, "currency": {"USD"}, "amount": {"1"}}, http.StatusNotFound)
	})
}
```

기대값 계산: 1월은 Rent AED 2,000 ÷ 4 = USD 500, Groceries KRW 300,000 ÷ 1,000 = USD 300, Netflix 15.49, Refund −5.49로 합계 USD 810.00이다. 2월은 Rent AED 2,100 ÷ 4 = USD 525.00이고, 1월 대비 525 − 810 = −285, −285 ÷ 810 = −35.2%다. 두 달 모두 환율은 가장 가까운 2024년 12월 값이다.

- [ ] **Step 2: 실패 확인**

Run: `$TDB go test -count=1 -run TestExpenses ./cmd/server/`
Expected: FAIL — `/finance/expenses`가 404

- [ ] **Step 3: 핸들러 구현**

`internal/finance/expense_handler.go`:

```go
package finance

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

type expenseListPage struct {
	Month      time.Time
	MonthOpts  web.Filter
	Summary    expenseSummary
	Rates      []appliedRate
	Rows       []expenseListRow
	Total      rowTotal
	Edit       itemForm
	Add        itemForm
	Candidates []ExpenseItem
	Currencies []money.Currency
	Copy       copyOffer
	Error      string
}

type expenseListRow struct {
	Row       expenseRow
	Editing   bool
	EditURL   string
	DeleteURL string
}

func (h *handler) listExpenses(w http.ResponseWriter, r *http.Request) {
	var month time.Time
	if v := r.URL.Query().Get("month"); v != "" {
		m, err := time.Parse(monthLayout, v)
		if err != nil {
			http.Error(w, "Invalid month.", http.StatusBadRequest)
			return
		}
		month = m
	}
	h.renderExpenses(w, r, http.StatusOK, month, listView{})
}

// renderExpenses shows month's expenses, or the current month's when month is zero.
func (h *handler) renderExpenses(w http.ResponseWriter, r *http.Request, status int, month time.Time, view listView) {
	months := h.months()
	if month.IsZero() {
		month = months[0]
	}
	if !slices.ContainsFunc(months, month.Equal) {
		http.NotFound(w, r)
		return
	}
	items, err := h.store.ExpenseItems(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	expenses, err := h.store.Expenses(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	page, ok := newExpenseListPage(month, months, items, expenses, view)
	if !ok {
		http.NotFound(w, r)
		return
	}
	web.Render(w, r, status, "expense_list", page)
}

// newExpenseListPage returns false when view edits an item that is not in month.
func newExpenseListPage(month time.Time, months []time.Time, items []ExpenseItem, expenses []Expense, view listView) (expenseListPage, bool) {
	cur, prev := monthExpenses(expenses, month)
	rows := expenseRows(cur)
	tableRows, edit, ok := editableExpenseRows(month, rows, view)
	if !ok {
		return expenseListPage{}, false
	}
	page := expenseListPage{
		Month:      month,
		MonthOpts:  monthFilter(months, month),
		Rows:       tableRows,
		Total:      expenseTotalOf(rows),
		Edit:       edit,
		Add:        view.Add,
		Candidates: expenseCandidates(items, cur),
		Currencies: money.Currencies,
		Error:      view.Error,
	}
	page.Add.URL = expenseURL(month, "items/new")
	switch {
	case len(cur) > 0:
		page.Summary = summarizeExpenses(cur, prev)
		page.Rates = appliedRates(month, expenseRates(cur))
	case len(prev) > 0:
		page.Copy = copyOffer{From: prev[0].Month, Count: len(prev), URL: expenseURL(month, "copy")}
	}
	return page, true
}

// editableExpenseRows adds the row actions to rows and returns the form of the row that view
// edits: the input the user submitted, or else the row's current values. It returns false when
// view edits an item that is not in rows.
func editableExpenseRows(month time.Time, rows []expenseRow, view listView) ([]expenseListRow, itemForm, bool) {
	var tableRows []expenseListRow
	edit := view.Edit
	found := view.EditID == 0
	for _, row := range rows {
		item := row.Expense.Item
		id := strconv.FormatInt(item.ID, 10)
		lr := expenseListRow{
			Row:       row,
			Editing:   item.ID == view.EditID,
			EditURL:   expenseURL(month, "items/"+id+"/edit"),
			DeleteURL: expenseURL(month, "items/"+id+"/delete"),
		}
		if lr.Editing {
			found = true
			if !edit.Submitted {
				edit = itemForm{Name: item.Name, Amount: money.Input(item.Currency, row.Expense.Amount)}
			}
			edit.Currency, edit.URL, edit.CancelURL = item.Currency, lr.EditURL, expenseListURL(month)
		}
		tableRows = append(tableRows, lr)
	}
	return tableRows, edit, found
}

// expenseCandidates lists the items that cur does not have yet, for the add form to suggest.
func expenseCandidates(items []ExpenseItem, cur []Expense) []ExpenseItem {
	var out []ExpenseItem
	for _, item := range items {
		if !slices.ContainsFunc(cur, func(e Expense) bool { return e.Item.ID == item.ID }) {
			out = append(out, item)
		}
	}
	return out
}

func (h *handler) createExpense(w http.ResponseWriter, r *http.Request) {
	month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	form := itemForm{
		Name:      r.PostFormValue("name"),
		Currency:  money.Currency(r.PostFormValue("currency")),
		Amount:    r.PostFormValue("amount"),
		Submitted: true,
	}
	items, err := h.store.ExpenseItems(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if item, ok := expenseItemNamed(items, form.Name); ok {
		form.Currency, form.Existing = item.Currency, true
	}
	amount, ok := money.Parse(form.Currency, form.Amount)
	if !ok {
		form.Error = amountExample
		h.renderExpenses(w, r, http.StatusUnprocessableEntity, month, listView{Add: form})
		return
	}
	err = h.store.AddExpense(r.Context(), month, ExpenseInput{Name: form.Name, Currency: form.Currency, Amount: amount})
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name and currency."
	case errors.Is(err, ErrItemExists):
		form.Error = fmt.Sprintf("%s is already in %s.", strings.TrimSpace(form.Name), month.Format(monthLabelLayout))
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, expenseListURL(month), http.StatusSeeOther)
		return
	}
	h.renderExpenses(w, r, http.StatusUnprocessableEntity, month, listView{Add: form})
}

func (h *handler) editExpense(w http.ResponseWriter, r *http.Request) {
	month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderExpenses(w, r, http.StatusOK, month, listView{EditID: id})
}

func (h *handler) updateExpense(w http.ResponseWriter, r *http.Request) {
	month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	items, err := h.store.ExpenseItems(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	i := slices.IndexFunc(items, func(it ExpenseItem) bool { return it.ID == id })
	if i < 0 {
		http.NotFound(w, r)
		return
	}
	form := itemForm{Name: r.PostFormValue("name"), Amount: r.PostFormValue("amount"), Submitted: true}
	view := listView{EditID: id}
	amount, ok := money.Parse(items[i].Currency, form.Amount)
	if !ok {
		form.Error = amountExample
		view.Edit = form
		h.renderExpenses(w, r, http.StatusUnprocessableEntity, month, view)
		return
	}
	err = h.store.UpdateExpense(r.Context(), month, id, ExpenseUpdate{Name: form.Name, Amount: amount})
	switch {
	case errors.Is(err, ErrInvalidItem):
		form.Error = "Enter a name."
	case errors.Is(err, ErrNameTaken):
		form.Error = fmt.Sprintf("Another item is already named %s.", strings.TrimSpace(form.Name))
	case errors.Is(err, ErrItemNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		web.ServerError(w, r, err)
		return
	default:
		http.Redirect(w, r, expenseListURL(month), http.StatusSeeOther)
		return
	}
	view.Edit = form
	h.renderExpenses(w, r, http.StatusUnprocessableEntity, month, view)
}

func (h *handler) deleteExpense(w http.ResponseWriter, r *http.Request) {
	month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.store.DeleteExpense(r.Context(), month, id)
	switch {
	case errors.Is(err, ErrItemNotFound):
		http.NotFound(w, r)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, expenseListURL(month), http.StatusSeeOther)
	}
}

func (h *handler) copyExpenses(w http.ResponseWriter, r *http.Request) {
	month, ok := h.expenseRequest(w, r)
	if !ok {
		return
	}
	err := h.store.CopyPreviousExpenses(r.Context(), month)
	switch {
	case errors.Is(err, ErrMonthNotEmpty):
		h.renderExpenses(w, r, http.StatusUnprocessableEntity, month, listView{Error: "This month already has expenses."})
	case errors.Is(err, ErrNothingToCopy):
		h.renderExpenses(w, r, http.StatusUnprocessableEntity, month, listView{Error: "There is no earlier month to copy."})
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, expenseListURL(month), http.StatusSeeOther)
	}
}

// expenseRequest reads the month of a request on a month's expenses.
// It writes a 404 response and returns false when the month is invalid.
func (h *handler) expenseRequest(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	month, err := time.Parse(monthLayout, r.PathValue("month"))
	if err != nil || !h.hasMonth(month) {
		http.NotFound(w, r)
		return time.Time{}, false
	}
	return month, true
}

func expenseListURL(month time.Time) string {
	return "/finance/expenses?month=" + month.Format(monthLayout)
}

func expenseURL(month time.Time, action string) string {
	return "/finance/expenses/" + month.Format(monthLayout) + "/" + action
}
```

`internal/finance/handler.go`의 `NewHandler`에서 `mux.HandleFunc("POST /finance/snapshots/{month}/note", h.updateNote)` 다음 줄에 추가:

```go
	mux.HandleFunc("GET /finance/expenses", h.listExpenses)
	mux.HandleFunc("POST /finance/expenses/{month}/items/new", h.createExpense)
	mux.HandleFunc("GET /finance/expenses/{month}/items/{id}/edit", h.editExpense)
	mux.HandleFunc("POST /finance/expenses/{month}/items/{id}/edit", h.updateExpense)
	mux.HandleFunc("POST /finance/expenses/{month}/items/{id}/delete", h.deleteExpense)
	mux.HandleFunc("POST /finance/expenses/{month}/copy", h.copyExpenses)
```

- [ ] **Step 4: 템플릿과 사이드바**

`web/templates/expense_list.html`:

```html
{{define "title"}}Expenses{{end}}
{{define "head"}}
<meta name="htmx-config" content='{"scrollIntoViewOnBoost":false,"responseHandling":[{"code":"204","swap":false},{"code":"[23]..","swap":true},{"code":"422","swap":true},{"code":"[45]..","swap":false,"error":true}]}'>
<script src="/static/htmx-2.0.11.min.js" defer></script>
<script src="/static/item_form.js" defer></script>
{{end}}
{{define "content"}}
<header>
  <h1>Expenses</h1>
  {{with .MonthOpts}}
  <form method="get" action="/finance/expenses" hx-boost="true">
    <label>{{.Label}}
      <select name="{{.Name}}" onchange="this.form.requestSubmit()">
        {{range .Options}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
      </select>
    </label>
    <noscript><button type="submit">Show</button></noscript>
  </form>
  {{end}}
</header>
{{with .Error}}<p role="alert">{{.}}</p>{{end}}
{{if .Rows}}
<section aria-label="Summary">
  {{with .Summary}}
  {{if .Missing}}
  <p role="alert">Missing exchange rate for {{range $i, $c := .Missing}}{{if $i}}, {{end}}{{$c}}{{end}}.</p>
  {{else}}
  <dl>
    <div>
      <dt>Total</dt>
      <dd>
        <strong>{{money "USD" .Total}}</strong>
        {{if .HasChange}}<small>{{money "USD" .Change}}{{with .ChangePercent}} ({{.}}){{end}} vs {{.PreviousMonth.Format "Jan 2006"}}</small>{{end}}
      </dd>
    </div>
  </dl>
  {{end}}
  {{end}}
  {{with .Rates}}
  <p>Rates: 1 USD = {{range $i, $r := .}}{{if $i}} · {{end}}{{$r.Currency}} {{$r.PerUSD}}{{if $r.OtherMonth}} ({{$r.Month.Format "Jan 2006"}}){{end}}{{end}}</p>
  {{end}}
</section>
<table hx-boost="true">
  <thead>
    <tr><th>Name</th><th class="num">Amount</th><th class="num">USD</th><th></th></tr>
  </thead>
  <tbody>
    {{range $row := .Rows}}
    {{if $row.Editing}}
    {{with $.Edit}}
    <tr>
      <td><label><span>Name</span><input name="name" value="{{.Name}}" form="edit-item" required></label></td>
      <td class="num"><label><span>Amount in {{.Currency}}</span><input name="amount" value="{{.Amount}}" form="edit-item" inputmode="decimal" required></label></td>
      <td class="num">{{.Currency}}</td>
      <td class="actions">
        <form id="edit-item" method="post" action="{{.URL}}"><button type="submit" class="primary">Save</button></form>
        <a href="{{.CancelURL}}">Cancel</a>
      </td>
    </tr>
    {{with .Error}}<tr><td colspan="4" role="alert">{{.}}</td></tr>{{end}}
    {{end}}
    {{else}}
    {{with $row.Row}}
    <tr>
      <td>{{.Expense.Item.Name}}</td>
      <td class="num">{{money .Expense.Item.Currency .Expense.Amount}}</td>
      <td class="num">{{if .HasUSD}}{{money "USD" .USD}}{{else}}—{{end}}</td>
      <td class="actions">
        <a href="{{$row.EditURL}}" aria-label="Edit {{.Expense.Item.Name}}">Edit</a>
        <form method="post" action="{{$row.DeleteURL}}" hx-confirm="Remove {{.Expense.Item.Name}} from this month?">
          <button type="submit" class="danger" aria-label="Delete {{.Expense.Item.Name}}">Delete</button>
        </form>
      </td>
    </tr>
    {{end}}
    {{end}}
    {{end}}
  </tbody>
  {{with .Total}}
  <tfoot>
    <tr>
      <th scope="row">Total</th>
      <td class="num">{{if .HasAmount}}{{money .Currency .Amount}}{{end}}</td>
      <td class="num">{{if .HasUSD}}{{money "USD" .USD}}{{else}}—{{end}}</td>
      <td></td>
    </tr>
  </tfoot>
  {{end}}
</table>
{{else}}
<p>No expenses in {{.Month.Format "Jan 2006"}} yet.</p>
{{if .Copy.URL}}
<form method="post" action="{{.Copy.URL}}" hx-boost="true">
  <button type="submit">Copy {{.Copy.From.Format "Jan 2006"}} ({{.Copy.Count}} items)</button>
</form>
{{end}}
{{end}}
<form id="add-item" method="post" action="{{.Add.URL}}" hx-boost="true">
  <fieldset>
    <legend>Add to {{.Month.Format "Jan 2006"}}</legend>
    <label>Name
      <input name="name" value="{{.Add.Name}}" list="expense-names" autocomplete="off" required>
    </label>
    <datalist id="expense-names">
      {{range .Candidates}}<option value="{{.Name}}" data-currency="{{.Currency}}">{{.Currency}}</option>{{end}}
    </datalist>
    <label>Currency
      <select name="currency"{{if .Add.Existing}} disabled{{end}}>
        {{range .Currencies}}<option value="{{.}}"{{if eq . $.Add.Currency}} selected{{end}}>{{.}}</option>{{end}}
      </select>
    </label>
    <label>Amount
      <input name="amount" value="{{.Add.Amount}}" inputmode="decimal" placeholder="1,234.56" required>
    </label>
    <button type="submit" class="primary">Add</button>
  </fieldset>
  {{with .Add.Error}}<p role="alert">{{.}}</p>{{end}}
</form>
<footer><a href="https://www.exchangerate-api.com">Rates by Exchange Rate API</a></footer>
{{end}}
```

`web/templates/layout.html`의 Snapshots 링크 줄 다음에 추가:

```html
        <a href="/finance/expenses"{{if current "/finance/expenses"}} aria-current="page"{{end}}>Expenses</a>
```

- [ ] **Step 5: 통과 확인**

Run: `gofmt -l internal cmd; go vet ./... && $TDB go test -count=1 ./...`
Expected: 모든 패키지 `ok`

- [ ] **Step 6: 화면 확인**

`go build -o bin/server ./cmd/server`로 만든 바이너리를 `DATABASE_URL=postgres://postgres:postgres@localhost:5432/monolith?sslmode=disable ADMIN_EMAIL=admin@localhost ADMIN_PASSWORD=admin LISTEN_ADDR=:8081 bin/server`로 띄우고, 헤드리스 Chrome으로 `/finance/expenses`를 데스크톱(1280, 라이트·다크)과 모바일(390)에서 캡처한다. 확인할 것: 헤더의 월 선택 배치, 요약 카드, 표가 모바일에서 무너지지 않는지, 기존 항목 이름을 입력하면 Currency가 잠기고 회색이 되는지. CSS가 필요하면 `:root` 변수만 써서 `web/static/app.css`에 추가한다.

---

### Task 5: 문서와 완료 검사

**Files:**
- Modify: `docs/superpowers/specs/2026-09-25-expenses-design.md` (구현과 다른 점이 생겼으면)
- Modify: `docs/superpowers/specs/2026-09-25-asset-snapshots-design.md` (코드 배치 표: `item_form.js`, 공용 헬퍼)

- [ ] **Step 1: 문서 갱신**

스냅샷 설계 문서의 `snapshot_list.js` 언급을 `item_form.js`로 바꾸고, "코드 배치" 표의 `summary.go` 설명을 `요약과 전월 대비(summarize, changePercent), 적용 환율(appliedRates, snapshotRates)`로 바꾼다. 지출 설계 문서는 구현 중 바뀐 결정이 있으면 반영한다.

- [ ] **Step 2: 완료 기준 전체 실행**

Run:
```bash
test -z "$(gofmt -l .)" && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run && $TDB go test -race -count=1 ./... && go mod tidy -diff
```
Expected: 린트 `0 issues.`, 모든 패키지 `ok`, tidy 출력 없음

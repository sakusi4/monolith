package finance

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/sakusi4/monolith/internal/money"
)

type ExpenseCategory string

const (
	CategoryHousing     ExpenseCategory = "housing"
	CategoryFood        ExpenseCategory = "food"
	CategoryTransport   ExpenseCategory = "transport"
	CategoryBills       ExpenseCategory = "bills"
	CategoryShopping    ExpenseCategory = "shopping"
	CategoryTravel      ExpenseCategory = "travel"
	CategoryFamily      ExpenseCategory = "family"
	CategoryLoanPayment ExpenseCategory = "loan_payment"
	CategoryOther       ExpenseCategory = "other"
)

var expenseCategories = []ExpenseCategory{
	CategoryHousing, CategoryFood, CategoryTransport, CategoryBills, CategoryShopping,
	CategoryTravel, CategoryFamily, CategoryLoanPayment, CategoryOther,
}

func (c ExpenseCategory) valid() bool {
	return slices.Contains(expenseCategories, c)
}

func (c ExpenseCategory) Label() string {
	switch c {
	case CategoryHousing:
		return "Housing"
	case CategoryFood:
		return "Food"
	case CategoryTransport:
		return "Transport"
	case CategoryBills:
		return "Bills"
	case CategoryShopping:
		return "Shopping"
	case CategoryTravel:
		return "Travel"
	case CategoryFamily:
		return "Family & gifts"
	case CategoryLoanPayment:
		return "Loan payments"
	case CategoryOther:
		return "Other"
	}
	return string(c)
}

// Expense is a named amount spent on Date. RateMonth and PerUSD hold the exchange rate of the month
// closest to Date's month; PerUSD is nil for USD and for currencies without any stored rate.
type Expense struct {
	ID        int64
	Date      time.Time
	Name      string
	Category  ExpenseCategory
	Currency  money.Currency
	Amount    int64
	RateMonth time.Time
	PerUSD    *big.Rat
}

// ExpenseInput is one expense of a month, as added or edited.
type ExpenseInput struct {
	Date     time.Time
	Name     string
	Category ExpenseCategory
	Currency money.Currency
	Amount   int64
}

// expenseSuggestion is a name used in an earlier month, with the category and currency it had most recently.
type expenseSuggestion struct {
	Name     string
	Category ExpenseCategory
	Currency money.Currency
}

func (e Expense) USD() (int64, bool) {
	return usdValue(e.Currency, e.Amount, e.PerUSD)
}

func (in ExpenseInput) Clean() (ExpenseInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Date.IsZero():
		return ExpenseInput{}, fmt.Errorf("%w: date is empty", ErrInvalidItem)
	case in.Name == "":
		return ExpenseInput{}, fmt.Errorf("%w: name is empty", ErrInvalidItem)
	case !in.Category.valid():
		return ExpenseInput{}, fmt.Errorf("%w: unknown category %q", ErrInvalidItem, in.Category)
	case !in.Currency.IsValid():
		return ExpenseInput{}, fmt.Errorf("%w: unknown currency %q", ErrInvalidItem, in.Currency)
	}
	return in, nil
}

// Expenses returns the expenses spent from from until before to, by name.
func (s *Store) Expenses(ctx context.Context, from, to time.Time) ([]Expense, error) {
	query := `
		SELECT e.id, e.spent_on, e.name, e.category, e.currency, e.amount, r.month, r.per_usd
		FROM expenses e
		LEFT JOIN LATERAL (
			SELECT er.month, er.per_usd::text AS per_usd
			FROM exchange_rates er
			WHERE er.currency = e.currency
			ORDER BY abs(er.month - date_trunc('month', e.spent_on)::date), er.month
			LIMIT 1
		) r ON true
		WHERE e.spent_on >= $1 AND e.spent_on < $2
		ORDER BY lower(e.name), e.id`
	rows, err := s.db.QueryContext(ctx, query, from, to)
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
		if err := rows.Scan(&e.ID, &e.Date, &e.Name, &e.Category, &e.Currency, &e.Amount, &rateMonth, &perUSD); err != nil {
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

// expenseSuggestions lists every name used in any month once, by name, with its most recent category and currency.
func (s *Store) expenseSuggestions(ctx context.Context) ([]expenseSuggestion, error) {
	query := `
		SELECT DISTINCT ON (lower(name)) name, category, currency
		FROM expenses
		ORDER BY lower(name), spent_on DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query expense suggestions: %w", err)
	}
	defer rows.Close()
	var suggestions []expenseSuggestion
	for rows.Next() {
		var sg expenseSuggestion
		if err := rows.Scan(&sg.Name, &sg.Category, &sg.Currency); err != nil {
			return nil, fmt.Errorf("scan expense suggestion: %w", err)
		}
		suggestions = append(suggestions, sg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query expense suggestions: %w", err)
	}
	return suggestions, nil
}

func (s *Store) AddExpense(ctx context.Context, in ExpenseInput) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	query := `INSERT INTO expenses (spent_on, name, category, currency, amount) VALUES ($1, $2, $3, $4, $5)`
	if _, err := s.db.ExecContext(ctx, query, in.Date, in.Name, in.Category, in.Currency, in.Amount); err != nil {
		return fmt.Errorf("insert expense: %w", err)
	}
	return nil
}

// UpdateExpense replaces the expense id spent in month. It returns ErrItemNotFound when month has no such expense.
func (s *Store) UpdateExpense(ctx context.Context, month time.Time, id int64, in ExpenseInput) error {
	in, err := in.Clean()
	if err != nil {
		return err
	}
	query := `
		UPDATE expenses SET spent_on = $4, name = $5, category = $6, currency = $7, amount = $8, updated_at = now()
		WHERE id = $1 AND spent_on >= $2 AND spent_on < $3`
	res, err := s.db.ExecContext(ctx, query, id, month, month.AddDate(0, 1, 0), in.Date, in.Name, in.Category, in.Currency, in.Amount)
	if err != nil {
		return fmt.Errorf("update expense: %w", err)
	}
	return requireRow(res, ErrItemNotFound)
}

// DeleteExpense removes the expense id spent in month. It returns ErrItemNotFound when month has no such expense.
func (s *Store) DeleteExpense(ctx context.Context, month time.Time, id int64) error {
	query := `DELETE FROM expenses WHERE id = $1 AND spent_on >= $2 AND spent_on < $3`
	res, err := s.db.ExecContext(ctx, query, id, month, month.AddDate(0, 1, 0))
	if err != nil {
		return fmt.Errorf("delete expense: %w", err)
	}
	return requireRow(res, ErrItemNotFound)
}

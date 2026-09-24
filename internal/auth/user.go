package auth

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID    int64
	Email string
}

func (s *Store) SetUser(ctx context.Context, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	query := `
		INSERT INTO users (email, password_hash) VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash`
	if _, err := s.db.ExecContext(ctx, query, normalizeEmail(email), hash); err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

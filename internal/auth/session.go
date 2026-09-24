package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 30 * 24 * time.Hour

const unknownUserHash = "$2a$10$XOPsLG/Zj/yzIMw6c7D2UuFjFprE.dWR7ZN23wysEmEIGDozE2cHa"

var ErrInvalidCredentials = errors.New("invalid credentials")

var ErrNoSession = errors.New("no session")

func (s *Store) Login(ctx context.Context, email, password string) (string, error) {
	var userID int64
	hash := []byte(unknownUserHash)
	err := s.db.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE email = $1`, normalizeEmail(email)).Scan(&userID, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find user: %w", err)
	}
	err = bcrypt.CompareHashAndPassword(hash, []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || userID == 0 {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("compare password: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return "", fmt.Errorf("delete expired sessions: %w", err)
	}
	token := rand.Text()
	query := `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`
	if _, err := s.db.ExecContext(ctx, query, userID, tokenHash(token), time.Now().Add(sessionTTL)); err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return token, nil
}

func (s *Store) UserBySession(ctx context.Context, token string) (User, error) {
	query := `
		SELECT u.id, u.email
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`
	var u User
	err := s.db.QueryRowContext(ctx, query, tokenHash(token)).Scan(&u.ID, &u.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoSession
	}
	if err != nil {
		return User{}, fmt.Errorf("find session: %w", err)
	}
	return u, nil
}

func (s *Store) Logout(ctx context.Context, token string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

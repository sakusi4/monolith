package page

import (
	"context"
	"fmt"
	"strings"
)

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likePattern is the ILIKE pattern that finds q anywhere, taking the wildcards in q as text.
func likePattern(q string) string {
	return "%" + likeEscaper.Replace(q) + "%"
}

// Search returns the pages outside the trash whose title or body contains q, ignoring case: the ones
// with q in the title first, and then the latest changed.
func (s *Store) Search(ctx context.Context, q string) ([]Entry, error) {
	query := `
		SELECT id FROM pages
		WHERE (title ILIKE $1 OR body ILIKE $1) AND trashed_at IS NULL
		ORDER BY title ILIKE $1 DESC, updated_at DESC, id`
	rows, err := s.db.QueryContext(ctx, query, likePattern(q))
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan page: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	return s.describe(ctx, ids, false)
}

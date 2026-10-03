package page

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
)

var pageLink = regexp.MustCompile(`(?:^|[\s(<\[])(?:https?://[^\s/()<>]+)?/(page/pages|task/projects|task/tasks)/(\d+)\b`)

// linkTargets is what a body links to by id: pages, and the pages of projects and tasks.
type linkTargets struct {
	Pages    []int64
	Projects []int64
	Tasks    []int64
}

// linksOf finds the addresses of pages, projects, and tasks in body, with or without a host.
func linksOf(body string) linkTargets {
	var t linkTargets
	for _, m := range pageLink.FindAllStringSubmatch(body, -1) {
		id, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			continue
		}
		switch m[1] {
		case "page/pages":
			t.Pages = append(t.Pages, id)
		case "task/projects":
			t.Projects = append(t.Projects, id)
		case "task/tasks":
			t.Tasks = append(t.Tasks, id)
		}
	}
	return t
}

// writeLinks replaces in tx the links of page id with the pages that body links to.
func writeLinks(ctx context.Context, tx *sql.Tx, id int64, body string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_links WHERE source_id = $1`, id); err != nil {
		return fmt.Errorf("delete page links: %w", err)
	}
	t := linksOf(body)
	query := `
		INSERT INTO page_links (source_id, target_id)
		SELECT $1, id FROM pages WHERE (id = ANY($2) OR project_id = ANY($3) OR task_id = ANY($4)) AND id <> $1
		ON CONFLICT DO NOTHING`
	if _, err := tx.ExecContext(ctx, query, id, t.Pages, t.Projects, t.Tasks); err != nil {
		return fmt.Errorf("insert page links: %w", err)
	}
	return nil
}

// Backlinks returns the pages that link to page id, by title, leaving out the ones in the trash.
func (s *Store) Backlinks(ctx context.Context, id int64) ([]Entry, error) {
	sources, err := s.linkSources(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.describe(ctx, sources, false)
}

// linkSources returns the pages that link to page id by title.
func (s *Store) linkSources(ctx context.Context, id int64) ([]int64, error) {
	query := `
		SELECT l.source_id FROM page_links l JOIN pages p ON p.id = l.source_id
		WHERE l.target_id = $1
		ORDER BY lower(p.title), p.id`
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("query page links: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var source int64
		if err := rows.Scan(&source); err != nil {
			return nil, fmt.Errorf("scan page link: %w", err)
		}
		ids = append(ids, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query page links: %w", err)
	}
	return ids, nil
}

package page

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
)

var pageLink = regexp.MustCompile(`(?:^|[\s(<\[])(?:https?://[^\s/()<>]+)?/(page/pages|task/projects|task/tasks)/(\d+)\b`)

// Links is the pages under a page and the pages that link to it.
type Links struct {
	Children  []Entry
	Backlinks []Entry
}

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

// Subpages returns the pages under page parent outside the trash, by title.
func (s *Store) Subpages(ctx context.Context, parent int64) ([]Entry, error) {
	query := `SELECT id, title FROM pages WHERE parent_id = $1 AND trashed_at IS NULL ORDER BY lower(title), id`
	rows, err := s.db.QueryContext(ctx, query, parent)
	if err != nil {
		return nil, fmt.Errorf("query subpages: %w", err)
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Title); err != nil {
			return nil, fmt.Errorf("scan subpage: %w", err)
		}
		e.Kind, e.URL = kindPage, PageURL(e.ID)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query subpages: %w", err)
	}
	return entries, nil
}

// Links returns the pages under page id and the pages that link to it, by title, leaving out the
// ones in the trash.
func (s *Store) Links(ctx context.Context, id int64) (Links, error) {
	children, err := s.Subpages(ctx, id)
	if err != nil {
		return Links{}, err
	}
	sources, err := s.linkSources(ctx, id)
	if err != nil {
		return Links{}, err
	}
	backlinks, err := s.describe(ctx, sources, false)
	if err != nil {
		return Links{}, err
	}
	return Links{Children: children, Backlinks: backlinks}, nil
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

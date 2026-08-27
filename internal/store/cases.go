package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)


type Case struct {
	ID              int64      `json:"id"`
	GuildID         string     `json:"guild_id"`
	CaseNo          int64      `json:"case_no"`
	Type            string     `json:"type"`
	ModeratorID     string     `json:"moderator_id"`
	TargetID        string     `json:"target_id"`
	Reason          string     `json:"reason"`
	DurationSeconds int64      `json:"duration_seconds"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Active          bool       `json:"active"`
	ActorKind       string     `json:"actor_kind"`
	ReqID           string     `json:"req_id"`
	CreatedAt       time.Time  `json:"created_at"`
}

type CaseNote struct {
	ID        int64     `json:"id"`
	CaseID    int64     `json:"case_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

const caseCols = `id, guild_id, case_no, type, moderator_id, target_id, reason, duration_seconds, expires_at, active, actor_kind, req_id, created_at`

func scanCase(sc interface{ Scan(...any) error }) (*Case, error) {
	c := &Case{}
	var expires sql.NullTime
	var active int
	if err := sc.Scan(&c.ID, &c.GuildID, &c.CaseNo, &c.Type, &c.ModeratorID, &c.TargetID, &c.Reason,
		&c.DurationSeconds, &expires, &active, &c.ActorKind, &c.ReqID, &c.CreatedAt); err != nil {
		return nil, err
	}
	c.Active = active == 1
	if expires.Valid {
		t := expires.Time
		c.ExpiresAt = &t
	}
	return c, nil
}

var ErrCaseNotFound = errors.New("case not found")

func (s *Store) CreateCase(ctx context.Context, gid, caseType, moderatorID, targetID, reason string, durationSeconds int64, expires *time.Time, actorKind, reqID string) (*Case, error) {
	var expiresArg any
	if expires != nil {
		expiresArg = expires.UTC()
	}

	for attempt := 0; attempt < 3; attempt++ {
		res, err := s.db.ExecContext(ctx, `
INSERT INTO mod_cases (guild_id, case_no, type, moderator_id, target_id, reason, duration_seconds, expires_at, active, actor_kind, req_id)
VALUES (?, (SELECT COALESCE(MAX(case_no), 0) + 1 FROM mod_cases WHERE guild_id = ?), ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			gid, gid, caseType, moderatorID, targetID, reason, durationSeconds, expiresArg, actorKind, reqID)
		if err != nil {
			if isUniqueViolation(err) && attempt < 2 {
				continue
			}
			return nil, err
		}
		id, _ := res.LastInsertId()
		row := s.db.QueryRowContext(ctx, `SELECT `+caseCols+` FROM mod_cases WHERE id = ?`, id)
		return scanCase(row)
	}
	return nil, errors.New("case numbering contention")
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *Store) GetCaseByNumber(ctx context.Context, gid string, caseNo int64) (*Case, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+caseCols+` FROM mod_cases WHERE guild_id = ? AND case_no = ?`, gid, caseNo)
	c, err := scanCase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaseNotFound
	}
	return c, err
}

type CaseFilter struct {
	Type        string
	ModeratorID string
}

func (s *Store) ListCases(ctx context.Context, gid, targetID string, limit, offset int) ([]Case, error) {
	return s.ListCasesFiltered(ctx, gid, targetID, CaseFilter{}, limit, offset)
}

func (s *Store) ListCasesFiltered(ctx context.Context, gid, targetID string, f CaseFilter, limit, offset int) ([]Case, error) {
	q := `SELECT ` + caseCols + ` FROM mod_cases WHERE guild_id = ?`
	args := []any{gid}
	if targetID != "" {
		q += ` AND target_id = ?`
		args = append(args, targetID)
	}
	if f.Type != "" {
		q += ` AND type = ?`
		args = append(args, f.Type)
	}
	if f.ModeratorID != "" {
		q += ` AND moderator_id = ?`
		args = append(args, f.ModeratorID)
	}
	q += ` ORDER BY case_no DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Case
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

func (s *Store) CountCases(ctx context.Context, gid, targetID string) (int64, error) {
	q := `SELECT COUNT(1) FROM mod_cases WHERE guild_id = ?`
	args := []any{gid}
	if targetID != "" {
		q += ` AND target_id = ?`
		args = append(args, targetID)
	}
	var n int64
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

func (s *Store) DeactivateCase(ctx context.Context, gid string, caseNo int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE mod_cases SET active = 0 WHERE guild_id = ? AND case_no = ?`, gid, caseNo)
	return err
}

func (s *Store) UpdateCaseReason(ctx context.Context, gid string, caseNo int64, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE mod_cases SET reason = ? WHERE guild_id = ? AND case_no = ?`, reason, gid, caseNo)
	return err
}

type GuildCaseSummary struct {
	GuildID string `json:"guild_id"`
	Count   int64  `json:"count"`
}

func (s *Store) CaseGuildSummaries(ctx context.Context) ([]GuildCaseSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id, COUNT(1) AS n FROM mod_cases GROUP BY guild_id ORDER BY n DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []GuildCaseSummary
	for rows.Next() {
		var g GuildCaseSummary
		if err := rows.Scan(&g.GuildID, &g.Count); err != nil {
			return nil, err
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *Store) AddCaseNote(ctx context.Context, caseID int64, authorID, body string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO case_notes (case_id, author_id, body) VALUES (?, ?, ?)`, caseID, authorID, body)
	return err
}

func (s *Store) ListCaseNotes(ctx context.Context, caseID int64) ([]CaseNote, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, case_id, author_id, body, created_at FROM case_notes WHERE case_id = ? ORDER BY id ASC`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CaseNote
	for rows.Next() {
		var n CaseNote
		if err := rows.Scan(&n.ID, &n.CaseID, &n.AuthorID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}

func (s *Store) ModStats(ctx context.Context, gid, moderatorID string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT type, COUNT(1) FROM mod_cases WHERE guild_id = ? AND moderator_id = ? GROUP BY type`, gid, moderatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var t string
		var n int64
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		stats[t] = n
	}
	return stats, rows.Err()
}


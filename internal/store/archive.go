package store

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)


type archivedCase struct {
	Case  *Case      `json:"case"`
	Notes []CaseNote `json:"notes,omitempty"`
}

func (s *Store) ArchiveOldCases(ctx context.Context, cutoff time.Time, dir string) (string, int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+caseCols+` FROM mod_cases WHERE created_at < ? ORDER BY id ASC`, cutoff.UTC())
	if err != nil {
		return "", 0, err
	}
	var cases []*Case
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			rows.Close()
			return "", 0, err
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", 0, err
	}
	rows.Close()
	if len(cases) == 0 {
		return "", 0, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	name := fmt.Sprintf("vilicus-cases-archive-%s.jsonl.gz", time.Now().UTC().Format("20060102-150405"))

	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", 0, err
	}
	gz := gzip.NewWriter(f)
	enc := json.NewEncoder(gz)

	for _, c := range cases {
		cn, err := s.ListCaseNotes(ctx, c.ID)
		if err != nil {
			gz.Close()
			f.Close()
			os.Remove(filepath.Join(dir, name))
			return "", 0, fmt.Errorf("archive notes for case %d: %w", c.CaseNo, err)
		}
		for i := range cn {
			cn[i].ID = 0
			cn[i].CaseID = 0
		}
		id := c.ID
		c.ID = 0
		if err := enc.Encode(archivedCase{Case: c, Notes: cn}); err != nil {
			gz.Close()
			f.Close()
			os.Remove(filepath.Join(dir, name))
			return "", 0, fmt.Errorf("encode case %d: %w", c.CaseNo, err)
		}
		c.ID = id
	}
	if err := gz.Close(); err != nil {
		f.Close()
		os.Remove(filepath.Join(dir, name))
		return "", 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(filepath.Join(dir, name))
		return "", 0, err
	}

	args := make([]any, len(cases))
	marks := strings.Repeat("?,", len(cases))
	for i, c := range cases {
		args[i] = c.ID
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM case_notes WHERE case_id IN (`+strings.TrimSuffix(marks, ",")+`)`, args...); err != nil {
		return name, 0, fmt.Errorf("file written but note delete failed (re-run will duplicate): %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM mod_cases WHERE id IN (`+strings.TrimSuffix(marks, ",")+`)`, args...); err != nil {
		return name, 0, fmt.Errorf("file written but case delete failed (re-run will duplicate): %w", err)
	}

	return name, int64(len(cases)), nil
}


package store

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveOldCases(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	oldIDs := make([]int64, 0, 3)
	for i := 0; i < 3; i++ {
		c, err := st.CreateCase(ctx, "g1", "warn", "mod1", "target1", "ancient reason "+string(rune('a'+i)), 0, nil, "discord", "")
		if err != nil {
			t.Fatalf("create old case: %v", err)
		}
		oldIDs = append(oldIDs, c.ID)
	}
	if err := st.AddCaseNote(ctx, oldIDs[0], "mod2", "note body one"); err != nil {
		t.Fatalf("add note: %v", err)
	}
	if err := st.AddCaseNote(ctx, oldIDs[0], "mod3", "note body two"); err != nil {
		t.Fatalf("add note: %v", err)
	}
	if err := st.AddCaseNote(ctx, oldIDs[1], "mod2", "orphaned note"); err != nil {
		t.Fatalf("add note: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := st.CreateCase(ctx, "g1", "ban", "mod1", "target2", "recent case", 0, nil, "discord", ""); err != nil {
			t.Fatalf("create recent case: %v", err)
		}
	}

	backdated := strings.Repeat("?,", 3)
	args := []any{oldIDs[0], oldIDs[1], oldIDs[2]}
	if _, err := st.db.ExecContext(ctx,
		`UPDATE mod_cases SET created_at = datetime('now', '-3 years') WHERE id IN (`+strings.TrimSuffix(backdated, ",")+`)`, args...); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	dir := t.TempDir()
	name, n, err := st.ArchiveOldCases(ctx, time.Now().UTC().AddDate(-2, 0, 0), dir)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 3 {
		t.Fatalf("archived %d cases, want 3", n)
	}

	var count int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM mod_cases`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("remaining cases = %d (%v), want 2", count, err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM case_notes`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("remaining notes = %d (%v), want 0", count, err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM cases_fts WHERE src='case'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("fts cases = %d (%v), want 2", count, err)
	}

	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	lines := 0
	noteTotal := 0
	sawZeroID := true
	for scanner.Scan() {
		var ac archivedCase
		if err := json.Unmarshal(scanner.Bytes(), &ac); err != nil {
			t.Fatalf("line %d: %v", lines+1, err)
		}
		lines++
		noteTotal += len(ac.Notes)
		if ac.Case.ID != 0 {
			sawZeroID = false
		}
		for _, note := range ac.Notes {
			if note.ID != 0 || note.CaseID != 0 {
				sawZeroID = false
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if lines != 3 || noteTotal != 3 {
		t.Fatalf("archive has %d lines / %d notes, want 3/3", lines, noteTotal)
	}
	if !sawZeroID {
		t.Error("surrogate IDs must be zeroed in the export")
	}

	before, _ := os.ReadDir(dir)
	name2, n2, err := st.ArchiveOldCases(ctx, time.Now().UTC().AddDate(-2, 0, 0), dir)
	if err != nil || n2 != 0 || name2 != "" {
		t.Fatalf("re-run archived %d to %q (err %v), want 0/empty/nil", n2, name2, err)
	}
	after, _ := os.ReadDir(dir)
	if len(after) != len(before) {
		t.Error("no-op archive must not create a file")
	}
}


package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)


const backupPrefix = "vilicus-"

func (s *Store) RunBackupCycle(ctx context.Context, dir string, keepDaily, keepWeekly int) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("store: backup dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("store: create backup dir: %w", err)
	}

	dest := filepath.Join(dir, fmt.Sprintf("%s%s.db", backupPrefix, time.Now().Format("20060102-150405")))
	if err := s.BackupInto(ctx, dest); err != nil {
		return "", err
	}
	if err := VerifySQLite(dest); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	_, _ = RotateBackups(dir, keepDaily, keepWeekly)
	return dest, nil
}

func (s *Store) BackupInto(ctx context.Context, dest string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dest)
	if err != nil {
		return fmt.Errorf("store: vacuum into %s: %w", dest, err)
	}
	return nil
}

func VerifySQLite(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("store: open backup for verify: %w", err)
	}
	defer db.Close()

	var verdict string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&verdict); err != nil {
		return fmt.Errorf("store: integrity check: %w", err)
	}
	if verdict != "ok" {
		return fmt.Errorf("store: backup failed integrity check: %s", verdict)
	}
	return nil
}

func RotateBackups(dir string, keepDaily, keepWeekly int) ([]string, error) {
	files, err := ListBackupFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) <= keepDaily+keepWeekly {
		return nil, nil
	}

	var removed []string
	seenWeek := make(map[string]bool)
	weeklyKept := 0
	for i, f := range files {
		switch {
		case i < keepDaily:
			continue
		case weeklyKept < keepWeekly:
			y, w := f.ModifiedAt.UTC().ISOWeek()
			week := fmt.Sprintf("%04d-W%02d", y, w)
			if seenWeek[week] {
				_ = os.Remove(filepath.Join(dir, f.Name))
				removed = append(removed, f.Name)
				continue
			}
			seenWeek[week] = true
			weeklyKept++
		default:
			_ = os.Remove(filepath.Join(dir, f.Name))
			removed = append(removed, f.Name)
		}
	}
	return removed, nil
}

type BackupInfo struct {
	Name       string
	SizeBytes  int64
	ModifiedAt time.Time
}

func ListBackupFiles(dir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []BackupInfo
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, backupPrefix) || !strings.HasSuffix(name, ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{Name: name, SizeBytes: info.Size(), ModifiedAt: info.ModTime()})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ModifiedAt.After(out[b].ModifiedAt) })
	return out, nil
}


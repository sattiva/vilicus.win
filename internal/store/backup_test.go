package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunBackupCycleVerifiedCopies(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.CreateCase(ctx, "g1", "warn", "mod1", "t1", "r", 0, nil, "discord", ""); err != nil {
		t.Fatalf("seed case: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "backups")
	p1, err := st.RunBackupCycle(ctx, dir, 7, 4)
	if err != nil {
		t.Fatalf("first cycle: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	p2, err := st.RunBackupCycle(ctx, dir, 7, 4)
	if err != nil {
		t.Fatalf("second cycle: %v", err)
	}
	if p1 == p2 {
		t.Fatalf("cycles produced identical paths: %s", p1)
	}

	files, err := ListBackupFiles(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 backups, got %d", len(files))
	}
	if !strings.HasSuffix(p2, files[0].Name) || files[0].ModifiedAt.Before(files[1].ModifiedAt) {
		t.Fatalf("list not newest-first: %+v", files)
	}
	for _, f := range files {
		if err := VerifySQLite(filepath.Join(dir, f.Name)); err != nil {
			t.Errorf("copy %s not verified: %v", f.Name, err)
		}
		if f.SizeBytes <= 0 {
			t.Errorf("copy %s empty", f.Name)
		}
	}
}

func TestRotateBackupsKeepsDailyPlusWeekly(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 12; i++ {
		stamp := base.Add(-time.Duration(i) * 10 * time.Minute)
		name := filepath.Join(dir, backupPrefix+stamp.Format("20060102-150405")+".db")
		if err := os.WriteFile(name, []byte("stub"), 0o644); err != nil {
			t.Fatalf("write stub: %v", err)
		}
		if err := os.Chtimes(name, stamp, stamp); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	removed, err := RotateBackups(dir, 7, 4)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}

	if len(removed) != 4 {
		t.Fatalf("want 4 removed, got %d (%v)", len(removed), removed)
	}
	files, _ := ListBackupFiles(dir)
	if len(files) != 8 {
		t.Fatalf("want 8 surviving, got %d", len(files))
	}
}

func TestListBackupFilesMissingDir(t *testing.T) {
	files, err := ListBackupFiles(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if files != nil {
		t.Fatalf("want nil, got %+v", files)
	}
}


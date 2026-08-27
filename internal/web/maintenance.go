package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"vilicus/internal/store"
)

func (s *Server) handleMaintenance(w http.ResponseWriter, r *http.Request) {
	files, err := store.ListBackupFiles(s.Config.BackupDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type backupRow struct {
		Name       string
		Size       string
		ModifiedAt string
	}
	rows := make([]backupRow, 0, len(files))
	for _, f := range files {
		rows = append(rows, backupRow{
			Name:       f.Name,
			Size:       fmt.Sprintf("%.2f MB", float64(f.SizeBytes)/1024/1024),
			ModifiedAt: f.ModifiedAt.Format("2006-01-02 15:04:05"),
		})
	}

	s.render(w, r, "maintenance", "Maintenance", "maintenance", map[string]any{
		"Backups":        rows,
		"BackupDir":      s.Config.BackupDir,
		"RetentionUsage": s.Config.RetentionDays,
		"RetentionAudit": s.Config.RetentionAuditDays,
		"Msg":            r.URL.Query().Get("msg"),
		"Failed":         r.URL.Query().Get("ok") == "0",
	})
}

func (s *Server) runMaintenance(w http.ResponseWriter, r *http.Request, purpose, action string, run func(context.Context) (string, error)) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, purpose, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	detail, err := run(r.Context())
	if err != nil {
		s.auditMutation(r, sess, "maintenance_failed", action+": "+err.Error())
		http.Redirect(w, r, "/maintenance?msg="+url.QueryEscape(err.Error())+"&ok=0", http.StatusFound)
		return
	}

	s.auditMutation(r, sess, "maintenance_"+action, detail)
	http.Redirect(w, r, "/maintenance?msg="+url.QueryEscape(detail), http.StatusFound)
}

func (s *Server) handleMaintenanceBackup(w http.ResponseWriter, r *http.Request) {
	s.runMaintenance(w, r, csrfMaintBackup, "backup", func(ctx context.Context) (string, error) {
		dest, err := s.Store.RunBackupCycle(ctx, s.Config.BackupDir, 7, 4)
		if err != nil {
			return "", err
		}
		return "Backup created: " + dest, nil
	})
}

func (s *Server) handleMaintenancePrune(w http.ResponseWriter, r *http.Request) {
	s.runMaintenance(w, r, csrfMaintPrune, "prune", func(ctx context.Context) (string, error) {
		if err := s.Store.Prune(s.Config.RetentionDays, s.Config.RetentionAuditDays); err != nil {
			return "", err
		}
		return fmt.Sprintf("Pruned rows older than %d days (audit trails %d days).",
			s.Config.RetentionDays, s.Config.RetentionAuditDays), nil
	})
}

func (s *Server) handleMaintenanceVacuum(w http.ResponseWriter, r *http.Request) {
	s.runMaintenance(w, r, csrfMaintVacuum, "vacuum", func(ctx context.Context) (string, error) {
		if err := s.Store.Checkpoint(); err != nil {
			return "", err
		}
		if err := s.Store.Vacuum(); err != nil {
			return "", err
		}
		return "WAL checkpointed and database rebuilt with VACUUM.", nil
	})
}

func (s *Server) handleMaintenanceArchive(w http.ResponseWriter, r *http.Request) {
	s.runMaintenance(w, r, csrfMaintArchive, "archive", func(ctx context.Context) (string, error) {
		years, err := strconv.Atoi(r.FormValue("years"))
		if err != nil || years < 1 || years > 20 {
			return "", errors.New("age threshold must be between 1 and 20 years")
		}
		name, n, err := s.Store.ArchiveOldCases(ctx, time.Now().UTC().AddDate(-years, 0, 0), s.Config.BackupDir)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return fmt.Sprintf("No cases older than %d years; nothing archived.", years), nil
		}
		return fmt.Sprintf("Archived %d cases older than %d years to %s/%s.", n, years, s.Config.BackupDir, name), nil
	})
}


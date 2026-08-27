package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"vilicus/internal/store"
)


const (
	csrfGuilds           = "guilds"
	csrfGuildsImport     = "guilds_import"
	csrfCasesNote        = "cases_note"
	csrfCasesDeactivate  = "cases_deactivate"
	csrfAdminsAdd        = "admins_add"
	csrfAdminsDelete     = "admins_delete"
	csrfAdminsRole       = "admins_role"
	csrfLogoutAll        = "logout_all"
	csrfGuildAdminGrant  = "guild_admin_grant"
	csrfGuildAdminRevoke = "guild_admin_revoke"
	csrfSettings         = "settings"
	csrfConsoleExec      = "console_exec"
	csrfMaintBackup      = "maintenance_backup"
	csrfMaintPrune       = "maintenance_prune"
	csrfMaintVacuum      = "maintenance_vacuum"
	csrfMaintArchive     = "maintenance_archive"
)

var csrfPurposes = []string{
	csrfGuilds, csrfGuildsImport, csrfCasesNote, csrfCasesDeactivate,
	csrfAdminsAdd, csrfAdminsDelete, csrfAdminsRole, csrfLogoutAll,
	csrfGuildAdminGrant, csrfGuildAdminRevoke, csrfSettings, csrfConsoleExec,
	csrfMaintBackup, csrfMaintPrune, csrfMaintVacuum, csrfMaintArchive,
}

const csrfGraceWindow = 24 * time.Hour

func csrfTokenFor(secret []byte, sess *store.Session, purpose string) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%d|%s", sess.ID, sess.Epoch, purpose)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) csrfToken(sess *store.Session, purpose string) string {
	return csrfTokenFor([]byte(s.Config.SessionSecret), sess, purpose)
}

func (s *Server) validateCSRF(sess *store.Session, purpose, token string) bool {
	if hmac.Equal([]byte(s.csrfToken(sess, purpose)), []byte(token)) {
		return true
	}
	if old := s.Config.SessionSecretOld; old != "" && time.Since(s.startedAt) < csrfGraceWindow {
		return hmac.Equal([]byte(csrfTokenFor([]byte(old), sess, purpose)), []byte(token))
	}
	return false
}

func (s *Server) csrfMap(sess *store.Session) map[string]string {
	m := make(map[string]string, len(csrfPurposes))
	for _, p := range csrfPurposes {
		m[p] = s.csrfToken(sess, p)
	}
	return m
}


package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)


const maxBundleBytes = 1 << 20

func (s *Server) handleGuildExport(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	gid := strings.TrimSpace(r.URL.Query().Get("guild"))
	if gid == "" || !commands.ValidSnowflake(gid) {
		http.Error(w, "Valid guild ID required", http.StatusBadRequest)
		return
	}
	if !s.canManageGuild(r, gid) {
		forbidden(w)
		return
	}

	b, err := s.Store.ExportGuildBundle(r.Context(), gid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.auditMutation(r, sess, "config_export", fmt.Sprintf("guild=%s sections=guild,protection=%t,starboard=%t,rules=%d",
		gid, b.Protection != nil, b.Starboard != nil, len(b.AutomationRules)))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="vilicus-config-%s.json"`, gid))
	_, _ = w.Write(out)
}

func (s *Server) handleGuildImport(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, csrfGuildsImport, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	dest := strings.TrimSpace(r.FormValue("guild_id"))
	if dest == "" || !commands.ValidSnowflake(dest) {
		http.Redirect(w, r, "/guilds?import_err=Valid+destination+guild+ID+required", http.StatusFound)
		return
	}
	if !s.canManageGuild(r, dest) {
		forbidden(w)
		return
	}

	f, _, err := r.FormFile("bundle")
	if err != nil {
		http.Redirect(w, r, "/guilds?import_err=Attach+a+bundle+.json+file", http.StatusFound)
		return
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, maxBundleBytes+1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(raw) > maxBundleBytes {
		http.Redirect(w, r, "/guilds?import_err=Bundle+exceeds+1+MB", http.StatusFound)
		return
	}

	var b store.GuildConfigBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		msg := strings.ReplaceAll("Not a valid Vilicus config bundle: "+err.Error(), " ", "+")
		http.Redirect(w, r, "/guilds?import_err="+msg, http.StatusFound)
		return
	}

	applied, err := s.Store.ImportGuildBundle(r.Context(), &b, dest)
	if err != nil {
		s.auditMutation(r, sess, "config_import_failed", fmt.Sprintf("guild=%s applied=%v err=%q", dest, applied, err.Error()))
		msg := strings.ReplaceAll("Import failed: "+err.Error(), " ", "+")
		http.Redirect(w, r, "/guilds?import_err="+msg, http.StatusFound)
		return
	}

	s.auditMutation(r, sess, "config_import", fmt.Sprintf("guild=%s sections=%s source_bundle=%s", dest, strings.Join(applied, ","), b.GuildID))
	http.Redirect(w, r, fmt.Sprintf("/guilds?imported=%d", len(applied)), http.StatusFound)
}


package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"vilicus/internal/store"
)


const casesPerPage = 50

type guildSummaryView struct {
	ID   string
	Name string
	N    int64
}

func (s *Server) guildDisplayName(gid string) string {
	if s.Bot != nil && s.Bot.Session != nil {
		if g, _ := s.Bot.Session.State.Guild(gid); g != nil {
			return g.Name
		}
	}
	return gid
}

func (s *Server) handleCases(w http.ResponseWriter, r *http.Request) {
	summaries, err := s.Store.CaseGuildSummaries(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	gviews := make([]guildSummaryView, 0, len(summaries))
	for _, gs := range summaries {
		gviews = append(gviews, guildSummaryView{ID: gs.GuildID, Name: s.guildDisplayName(gs.GuildID), N: gs.Count})
	}

	gid := r.URL.Query().Get("guild")
	if gid == "" && len(gviews) > 0 {
		gid = gviews[0].ID
	}

	var rows []store.Case
	total := int64(0)
	page := 0
	target := r.URL.Query().Get("target")

	if q := r.URL.Query().Get("page"); q != "" {
		if p, err := strconv.Atoi(q); err == nil && p > 0 {
			page = p - 1
		}
	}

	if gid != "" {
		rows, _ = s.Store.ListCases(r.Context(), gid, target, casesPerPage+1, page*casesPerPage)
		total, _ = s.Store.CountCases(r.Context(), gid, target)
	}

	hasNext := false
	if len(rows) > casesPerPage {
		hasNext = true
		rows = rows[:casesPerPage]
	}

	var hits []store.CaseSearchHit
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query != "" {
		hits, _ = s.Store.SearchCases(r.Context(), query, 25)
	}

	s.render(w, r, "cases", "Cases", "cases", map[string]any{
		"GuildSummaries": gviews,
		"SelectedGuild":  gid,
		"GuildName":      s.guildDisplayName(gid),
		"Cases":          rows,
		"Target":         target,
		"Query":          query,
		"Hits":           hits,
		"Total":          total,
		"Page":           page + 1,
		"HasPrev":        page > 0,
		"HasNext":        hasNext,
	})
}

func (s *Server) handleCaseDetail(w http.ResponseWriter, r *http.Request) {
	gid := r.URL.Query().Get("guild")
	no, err := strconv.ParseInt(r.URL.Query().Get("no"), 10, 64)
	if gid == "" || err != nil || no < 1 {
		http.Redirect(w, r, "/cases", http.StatusFound)
		return
	}

	cs, cerr := s.Store.GetCaseByNumber(r.Context(), gid, no)
	if cerr != nil {
		s.render(w, r, "casedetail", "Case Not Found", "cases", map[string]any{
			"MissingNo": no,
		})
		return
	}

	notes, _ := s.Store.ListCaseNotes(r.Context(), cs.ID)

	s.render(w, r, "casedetail", fmt.Sprintf("Case #%d", cs.CaseNo), "cases", map[string]any{
		"Case":        cs,
		"Notes":       notes,
		"GuildID":     gid,
		"GuildName":   s.guildDisplayName(gid),
		"SavedNote":   r.URL.Query().Get("noted") == "1",
		"Deactivated": r.URL.Query().Get("deactivated") == "1",
	})
}

func (s *Server) handleCaseNoteAdd(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, csrfCasesNote, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	gid := r.FormValue("guild")
	no, err := strconv.ParseInt(r.FormValue("number"), 10, 64)
	note := r.FormValue("note")
	if gid == "" || err != nil || no < 1 || note == "" {
		http.Redirect(w, r, "/cases", http.StatusFound)
		return
	}
	if !s.canManageGuild(r, gid) {
		forbidden(w)
		return
	}

	cs, cerr := s.Store.GetCaseByNumber(r.Context(), gid, no)
	if cerr != nil {
		http.Redirect(w, r, "/cases?guild="+gid, http.StatusFound)
		return
	}
	if err := s.Store.AddCaseNote(r.Context(), cs.ID, sess.DiscordUserID, note); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.auditMutation(r, sess, "case_note_add", fmt.Sprintf("guild=%s case=%d", gid, no))
	http.Redirect(w, r, fmt.Sprintf("/cases/view?guild=%s&no=%d&noted=1", gid, no), http.StatusFound)
}

func (s *Server) handleCaseDeactivate(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, csrfCasesDeactivate, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	gid := r.FormValue("guild")
	no, err := strconv.ParseInt(r.FormValue("number"), 10, 64)
	if gid == "" || err != nil || no < 1 {
		http.Redirect(w, r, "/cases", http.StatusFound)
		return
	}
	if !s.canManageGuild(r, gid) {
		forbidden(w)
		return
	}

	if err := s.Store.DeactivateCase(r.Context(), gid, no); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.auditMutation(r, sess, "case_deactivate", fmt.Sprintf("guild=%s case=%d", gid, no))
	http.Redirect(w, r, fmt.Sprintf("/cases/view?guild=%s&no=%d&deactivated=1", gid, no), http.StatusFound)
}


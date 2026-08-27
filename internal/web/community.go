package web

import (
	"net/http"

	"vilicus/internal/store"
)


const (
	mirrorTopPosts = 10
	mirrorXPRows   = 15
)

type guildTabView struct {
	ID      string
	Name    string
	Prefix  string
	Current bool
}

func (s *Server) handleCommunity(w http.ResponseWriter, r *http.Request) {
	cfgs, err := s.Store.ListGuildConfigs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	gid := r.URL.Query().Get("guild")
	tabs := make([]guildTabView, 0, len(cfgs))
	for _, c := range cfgs {
		cur := c.GuildID == gid
		tabs = append(tabs, guildTabView{ID: c.GuildID, Name: s.guildDisplayName(c.GuildID), Prefix: c.Prefix, Current: cur})
		if gid == "" {
			gid = c.GuildID
			tabs[len(tabs)-1].Current = true
		}
	}

	var (
		sbCfg   *store.StarboardConfig
		sbPosts []store.StarboardPost
		xpRows  []store.XPRow
		give    []store.Giveaway
		rules   []store.AutomationRule
		prot    *store.ProtectionConfig
	)

	if gid != "" {
		if c, cerr := s.Store.GetStarboardConfig(r.Context(), gid); cerr == nil {
			sbCfg = c
		}
		sbPosts, _ = s.Store.ListStarboardPosts(r.Context(), gid, mirrorTopPosts)
		xpRows, _ = s.Store.Leaderboard(r.Context(), gid, mirrorXPRows)
		give, _ = s.Store.ListGiveaways(r.Context(), gid)
		rules, _ = s.Store.ListAutomationRules(r.Context(), gid)
		if p, perr := s.Store.GetProtectionConfig(r.Context(), gid); perr == nil {
			prot = p
		}
	}

	s.render(w, r, "community", "Community", "community", map[string]any{
		"GuildTabs":     tabs,
		"SelectedGuild": gid,
		"GuildName":     s.guildDisplayName(gid),
		"StarboardCfg":  sbCfg,
		"StarboardTop":  sbPosts,
		"XPRows":        xpRows,
		"Giveaways":     give,
		"Rules":         rules,
		"Protection":    prot,
	})
}


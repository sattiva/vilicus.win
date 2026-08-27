package web

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/logging"
	"vilicus/internal/sanitize"
	"vilicus/internal/store"
)

func (s *Server) auditMutation(r *http.Request, sess *store.Session, action, detail string) {
	_ = s.Store.LogDashAudit(r.Context(), sess.DiscordUserID, action, detail, s.trustedClientIP(r), logging.GetID(r.Context()))
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := "0s"
	guildCount := 0
	latency := int64(0)
	status := "Offline"

	if s.Bot != nil {
		uptime = time.Since(s.Bot.StartTime).Truncate(time.Second).String()
		if s.Bot.Session != nil && s.Bot.Session.DataReady {
			status = "Online"
			latency = s.Bot.Session.HeartbeatLatency().Milliseconds()
			s.Bot.Session.State.RLock()
			guildCount = len(s.Bot.Session.State.Guilds)
			s.Bot.Session.State.RUnlock()
		}
	}

	allocMB := fmt.Sprintf("%.2f", float64(m.Alloc)/1024/1024)
	sysMB := fmt.Sprintf("%.2f", float64(m.Sys)/1024/1024)

	totalCmds, _ := s.Store.CountLogs(r.Context())
	recentLogs, _ := s.Store.GetRecentLogs(r.Context(), 10)

	s.render(w, r, "dashboard", "Overview", "overview", map[string]any{
		"Status":        status,
		"Uptime":        uptime,
		"Latency":       latency,
		"GuildCount":    guildCount,
		"AllocMB":       allocMB,
		"SysMB":         sysMB,
		"Goroutines":    runtime.NumGoroutine(),
		"TotalCommands": totalCmds,
		"DBConns":       strconv.Itoa(s.Store.MaxConns()),
		"RecentLogs":    recentLogs,
	})
}

func (s *Server) handleGuilds(w http.ResponseWriter, r *http.Request) {
	guilds, _ := s.Store.ListGuildConfigs(r.Context())

	manage := make(map[string]bool, len(guilds))
	for _, g := range guilds {
		manage[g.GuildID] = s.canManageGuild(r, g.GuildID)
	}

	s.render(w, r, "guilds", "Guilds", "guilds", map[string]any{
		"Guilds":    guilds,
		"Manage":    manage,
		"Saved":     r.URL.Query().Get("saved") == "1",
		"Imported":  r.URL.Query().Get("imported"),
		"ImportErr": r.URL.Query().Get("import_err"),
	})
}

func (s *Server) handleGuildsUpdate(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	token := r.FormValue("csrf_token")
	if !s.validateCSRF(sess, csrfGuilds, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	gid := r.FormValue("guild_id")
	if gid == "" {
		http.Error(w, "Guild ID required", http.StatusBadRequest)
		return
	}
	if !s.canManageGuild(r, gid) {
		forbidden(w)
		return
	}

	cfg := &store.GuildConfig{
		GuildID:          gid,
		Prefix:           r.FormValue("prefix"),
		LogChannelID:     r.FormValue("log_channel_id"),
		WelcomeChannelID: r.FormValue("welcome_channel_id"),
		AutoRoleID:       r.FormValue("auto_role_id"),
	}

	if err := s.Store.SaveGuildConfig(r.Context(), cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.auditMutation(r, sess, "guild_update", fmt.Sprintf("guild=%s prefix=%q log=%q welcome=%q autorole=%q",
		gid, cfg.Prefix, cfg.LogChannelID, cfg.WelcomeChannelID, cfg.AutoRoleID))

	http.Redirect(w, r, "/guilds?saved=1", http.StatusFound)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs, _ := s.Store.GetRecentLogs(r.Context(), 50)

	s.render(w, r, "logs", "Logs", "logs", map[string]any{
		"Logs": logs,
	})
}

func (s *Server) handleAdmins(w http.ResponseWriter, r *http.Request) {
	admins, _ := s.Store.ListAdmins(r.Context())
	assignments, _ := s.Store.ListAllGuildAdmins(r.Context())
	cfgs, _ := s.Store.ListGuildConfigs(r.Context())

	guilds := make([]guildTabView, 0, len(cfgs))
	for _, c := range cfgs {
		guilds = append(guilds, guildTabView{ID: c.GuildID, Name: s.guildDisplayName(c.GuildID), Prefix: c.Prefix})
	}

	s.render(w, r, "admins", "Administrators", "admins", map[string]any{
		"Admins":      admins,
		"Assignments": assignments,
		"Guilds":      guilds,
		"Msg":         r.URL.Query().Get("msg"),
	})
}

func (s *Server) handleAdminsAdd(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	token := r.FormValue("csrf_token")
	if !s.validateCSRF(sess, csrfAdminsAdd, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	uid := r.FormValue("discord_user_id")
	name := r.FormValue("username")
	role := store.NormalizeAdminRole(r.FormValue("role"))

	if uid == "" || name == "" {
		http.Error(w, "User ID and Name required", http.StatusBadRequest)
		return
	}

	if err := s.Store.AddAdmin(r.Context(), uid, name, role); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.Store.BumpAllSessionEpochs(r.Context())
	s.auditMutation(r, sess, "admin_add", fmt.Sprintf("user=%s name=%q role=%q", uid, name, role))

	http.Redirect(w, r, "/admins?msg=Admin+added", http.StatusFound)
}

func (s *Server) handleAdminsRole(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, csrfAdminsRole, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	uid := r.FormValue("discord_user_id")
	role := store.NormalizeAdminRole(r.FormValue("role"))
	if uid == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	if err := s.Store.UpdateAdminRole(r.Context(), uid, role); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.Store.BumpAllSessionEpochs(r.Context())
	s.auditMutation(r, sess, "admin_role", fmt.Sprintf("user=%s role=%s", uid, role))
	http.Redirect(w, r, "/admins?msg=Role+updated", http.StatusFound)
}

func (s *Server) handleGuildAdminGrant(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, csrfGuildAdminGrant, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	uid := strings.TrimSpace(r.FormValue("discord_user_id"))
	gid := strings.TrimSpace(r.FormValue("guild_id"))
	if uid == "" || gid == "" || !commands.ValidSnowflake(uid) || !commands.ValidSnowflake(gid) {
		http.Redirect(w, r, "/admins?msg=Valid+user+and+guild+IDs+required", http.StatusFound)
		return
	}

	if err := s.Store.AddGuildAdmin(r.Context(), gid, uid, sess.DiscordUserID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.Store.BumpAllSessionEpochs(r.Context())
	s.auditMutation(r, sess, "guild_admin_grant", fmt.Sprintf("user=%s guild=%s", uid, gid))
	http.Redirect(w, r, "/admins?msg=Guild+access+granted", http.StatusFound)
}

func (s *Server) handleGuildAdminRevoke(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, csrfGuildAdminRevoke, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	uid := r.FormValue("discord_user_id")
	gid := r.FormValue("guild_id")
	if uid == "" || gid == "" {
		http.Redirect(w, r, "/admins?msg=User+and+guild+IDs+required", http.StatusFound)
		return
	}

	if err := s.Store.RemoveGuildAdmin(r.Context(), gid, uid); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.Store.BumpAllSessionEpochs(r.Context())
	s.auditMutation(r, sess, "guild_admin_revoke", fmt.Sprintf("user=%s guild=%s", uid, gid))
	http.Redirect(w, r, "/admins?msg=Guild+access+revoked", http.StatusFound)
}

func (s *Server) handleAdminsDelete(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	token := r.FormValue("csrf_token")
	if !s.validateCSRF(sess, csrfAdminsDelete, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	uid := r.FormValue("discord_user_id")
	if uid == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	if err := s.Store.DeleteAdmin(r.Context(), uid); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.Store.BumpAllSessionEpochs(r.Context())
	s.auditMutation(r, sess, "admin_delete", fmt.Sprintf("user=%s", uid))

	http.Redirect(w, r, "/admins?msg=Admin+removed", http.StatusFound)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	settings := s.Store.GetBotSettings()

	s.render(w, r, "settings", "Settings", "settings", map[string]any{
		"Settings": settings,
		"Saved":    r.URL.Query().Get("saved") == "1",
	})
}

func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	token := r.FormValue("csrf_token")
	if !s.validateCSRF(sess, csrfSettings, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	_ = s.Store.SetSetting("bot_name", r.FormValue("bot_name"))
	_ = s.Store.SetSetting("accent_color", r.FormValue("accent_color"))
	_ = s.Store.SetSetting("footer_text", r.FormValue("footer_text"))
	_ = s.Store.SetSetting("activity_type", r.FormValue("activity_type"))
	_ = s.Store.SetSetting("activity_name", r.FormValue("activity_name"))

	if s.Bot != nil {
		s.Bot.ApplyStatus()
	}

	s.auditMutation(r, sess, "settings_update",
		fmt.Sprintf("bot_name=%q accent=%q activity=%q", r.FormValue("bot_name"), r.FormValue("accent_color"), r.FormValue("activity_name")))

	http.Redirect(w, r, "/settings?saved=1", http.StatusFound)
}

func (s *Server) consoleBase(r *http.Request) map[string]any {
	var guilds []*discordgo.Guild
	botReady := false
	if s.Bot != nil && s.Bot.Session != nil && s.Bot.Session.DataReady {
		botReady = true
		s.Bot.Session.State.RLock()
		guilds = s.Bot.Session.State.Guilds
		s.Bot.Session.State.RUnlock()
	}

	return map[string]any{
		"Guilds":   guilds,
		"BotReady": botReady,
		"Result":   r.URL.Query().Get("res"),
		"Success":  r.URL.Query().Get("ok") == "1",
	}
}

func (s *Server) handleConsole(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "console", "Console", "console", s.consoleBase(r))
}

func (s *Server) renderConsoleStage(w http.ResponseWriter, r *http.Request, extra map[string]any) {
	base := s.consoleBase(r)
	for k, v := range extra {
		base[k] = v
	}
	s.render(w, r, "console", "Console", "console", base)
}

func (s *Server) handleConsoleExec(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	token := r.FormValue("csrf_token")
	if !s.validateCSRF(sess, csrfConsoleExec, token) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if s.Bot == nil || s.Bot.Session == nil {
		http.Redirect(w, r, "/console?res=Bot+gateway+offline", http.StatusFound)
		return
	}

	action := r.FormValue("action")
	gid := strings.TrimSpace(r.FormValue("guild_id"))
	uid := strings.TrimSpace(r.FormValue("user_id"))
	targetCh := strings.TrimSpace(r.FormValue("channel_id"))
	reason := strings.TrimSpace(r.FormValue("reason"))
	textPayload := strings.TrimSpace(r.FormValue("payload"))

	for id, label := range map[string]string{gid: "Guild ID", uid: "User ID", targetCh: "Channel ID"} {
		if id != "" && !commands.ValidSnowflake(id) {
			s.auditMutation(r, sess, "console_rejected", fmt.Sprintf("action=%s bad_%s=%q", action, strings.ToLower(label), id))
			http.Redirect(w, r, fmt.Sprintf("/console?res=%s&ok=0", strings.ReplaceAll(label+" is not a valid ID.", " ", "+")), http.StatusFound)
			return
		}
	}

	destructive := action == "ban" || action == "kick" || action == "unban"

	if destructive && !s.canManageGuild(r, gid) {
		s.auditMutation(r, sess, "console_rejected", fmt.Sprintf("action=%s unscoped_guild=%s", action, gid))
		http.Redirect(w, r, "/console?res=Not+authorized+for+that+guild.&ok=0", http.StatusFound)
		return
	}

	confirmToken := r.FormValue("confirm_token")
	confirmNonce := r.FormValue("confirm_nonce")

	if destructive {
		if reason == "" {
			s.auditMutation(r, sess, "console_rejected", fmt.Sprintf("action=%s missing_reason", action))
			http.Redirect(w, r, "/console?res=A+reason+is+required+for+this+action.&ok=0", http.StatusFound)
			return
		}
		if confirmToken == "" && confirmNonce == "" {
			token, nonce, exp := s.issueConfirmToken(action, gid, uid)
			s.auditMutation(r, sess, "console_staged",
				fmt.Sprintf("action=%s guild=%s user=%s expires=%s", action, gid, uid, exp.Format("15:04:05")))
			s.renderConsoleStage(w, r, map[string]any{
				"Stage": map[string]any{
					"Action": action, "GuildID": gid, "UserID": uid,
					"Reason": reason,
					"Token":  token, "Nonce": nonce,
					"ExpiresAt": exp.Format("15:04:05"),
					"Target":    s.resolveTargetInfo(r.Context(), gid, uid),
				},
			})
			return
		}
		if !s.consumeConfirmToken(confirmToken, confirmNonce, action, gid, uid) {
			s.auditMutation(r, sess, "console_rejected", fmt.Sprintf("action=%s bad_or_used_confirm_token", action))
			http.Redirect(w, r, "/console?res=Confirmation+token+invalid,+expired,+or+already+used.+Start+again.&ok=0", http.StatusFound)
			return
		}
	}
	if !destructive && reason == "" {
		reason = fmt.Sprintf("Admin Panel Action by %s", sess.Username)
	}

	var resMsg string
	ok := true

	switch action {
	case "ban":
		if gid == "" || uid == "" {
			resMsg = "Guild ID and User ID required for ban."
			ok = false
		} else if msg, allowed := s.consoleHierarchyOK(gid, uid); !allowed {
			resMsg = msg
			ok = false
		} else {
			err := s.Bot.Session.GuildBanCreateWithReason(gid, uid,
				fmt.Sprintf("[web:%s] %s", sess.Username, reason), 0)
			if err != nil {
				resMsg = fmt.Sprintf("Ban failed: %s", err.Error())
				ok = false
			} else {
				resMsg = fmt.Sprintf("Successfully banned user %s in guild %s.", uid, gid)
				_ = s.Store.LogAudit(r.Context(), gid, sess.DiscordUserID, uid, "Web Console Ban", reason, "")
			}
		}

	case "unban":
		if gid == "" || uid == "" {
			resMsg = "Guild ID and User ID required for unban."
			ok = false
		} else {
			err := s.Bot.Session.GuildBanDelete(gid, uid)
			if err != nil {
				resMsg = fmt.Sprintf("Unban failed: %s", err.Error())
				ok = false
			} else {
				resMsg = fmt.Sprintf("Successfully unbanned user %s in guild %s.", uid, gid)
				_ = s.Store.LogAudit(r.Context(), gid, sess.DiscordUserID, uid, "Web Console Unban", reason, "")
			}
		}

	case "kick":
		if gid == "" || uid == "" {
			resMsg = "Guild ID and User ID required for kick."
			ok = false
		} else if msg, allowed := s.consoleHierarchyOK(gid, uid); !allowed {
			resMsg = msg
			ok = false
		} else {
			err := s.Bot.Session.GuildMemberDeleteWithReason(gid, uid,
				fmt.Sprintf("[web:%s] %s", sess.Username, reason))
			if err != nil {
				resMsg = fmt.Sprintf("Kick failed: %s", err.Error())
				ok = false
			} else {
				resMsg = fmt.Sprintf("Successfully kicked user %s from guild %s.", uid, gid)
				_ = s.Store.LogAudit(r.Context(), gid, sess.DiscordUserID, uid, "Web Console Kick", reason, "")
			}
		}

	case "lookup_user":
		if uid == "" {
			resMsg = "User ID required."
			ok = false
		} else {
			usr, err := s.Bot.Session.User(uid)
			if err != nil {
				resMsg = fmt.Sprintf("User lookup error: %s", err.Error())
				ok = false
			} else {
				banner := usr.BannerURL("2048")
				if banner == "" {
					banner = "None"
				}
				resMsg = fmt.Sprintf("ID: %s | Tag: %s | Bot: %t | Banner: %s | Avatar: %s", usr.ID, usr.Username, usr.Bot, banner, usr.AvatarURL("1024"))
			}
		}

	case "send_message":
		if targetCh == "" || textPayload == "" {
			resMsg = "Channel ID and Message Text required."
			ok = false
		} else {
			container := s.Bot.Container(
				components.TextDisplay{Content: sanitize.UserText(textPayload)},
			)
			_, err := s.Bot.Session.ChannelMessageSendComplex(targetCh, &discordgo.MessageSend{
				Flags:      components.FlagComponentsV2,
				Components: []discordgo.MessageComponent{container},
			})
			if err != nil {
				resMsg = fmt.Sprintf("Message send failed: %s", err.Error())
				ok = false
			} else {
				resMsg = fmt.Sprintf("Components V2 message sent successfully to channel %s.", targetCh)
			}
		}

	default:
		resMsg = "Unknown console action."
		ok = false
	}

	detail := fmt.Sprintf("action=%s guild=%s user=%s channel=%s result=%t", action, gid, uid, targetCh, ok)
	s.auditMutation(r, sess, "console_exec", detail)

	okParam := "0"
	if ok {
		okParam = "1"
	}
	http.Redirect(w, r, fmt.Sprintf("/console?res=%s&ok=%s", strings.ReplaceAll(resMsg, " ", "+"), okParam), http.StatusFound)
}

func (s *Server) consoleHierarchyOK(gid, targetID string) (string, bool) {
	guild, err := s.Bot.Session.State.Guild(gid)
	if err != nil || guild == nil {
		return "", true
	}
	targetMember, _ := s.Bot.Session.State.Member(gid, targetID)
	if targetMember == nil {
		return "", true
	}
	botMember, _ := s.Bot.Session.State.Member(gid, s.Bot.Session.State.User.ID)
	if botMember == nil {
		return "", true
	}
	if ok, why := commands.CanBotModerate(guild, botMember, targetMember); !ok {
		return fmt.Sprintf("Refused by hierarchy check: %s", why), false
	}
	return "", true
}


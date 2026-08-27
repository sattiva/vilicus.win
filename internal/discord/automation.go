package discord

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/automation"
	"vilicus/internal/discord/commands"
	"vilicus/internal/sanitize"
)


const (
	automationCacheTTL = 15 * time.Second
	automationMaxRoles = 50000
)

type automationEvent = automation.Event

type automationCacheEntry struct {
	rules []*automation.Rule
	at    time.Time
}


type automationWindow struct {
	start time.Time
	count int
}

func (b *Bot) automationGate(ruleID int64, uid string, cooldownSecs int64, limit int, window int64, now time.Time) bool {
	key := automation.Key(ruleID, uid)
	b.automationMu.Lock()
	defer b.automationMu.Unlock()

	if cooldownSecs > 0 {
		if last, ok := b.automationCooldowns[key]; ok && now.Sub(last) < time.Duration(cooldownSecs)*time.Second {
			return false
		}
		b.automationCooldowns[key] = now
	}

	if limit > 0 {
		w := b.automationCounters[key]
		if w.count > 0 && now.Sub(w.start) >= time.Duration(window)*time.Second {
			w = automationWindow{}
		}
		if w.count == 0 {
			w.start = now
		}
		w.count++
		b.automationCounters[key] = w
		if w.count != limit {
			return false
		}
	}
	return true
}


func (b *Bot) automationRulesFor(ctx context.Context, gid string) []*automation.Rule {
	b.automationMu.Lock()
	if e, ok := b.automationCache[gid]; ok && time.Since(e.at) < automationCacheTTL {
		b.automationMu.Unlock()
		return e.rules
	}
	b.automationMu.Unlock()

	rows, err := b.Store.ListAutomationRules(ctx, gid)
	var rules []*automation.Rule
	if err != nil {
		slog.Warn("automation rules load failed", "guild_id", gid, "err", err)
	} else {
		for _, r := range rows {
			if !r.Enabled {
				continue
			}
			rules = append(rules, automation.Compile(r))
		}
	}

	b.automationMu.Lock()
	b.automationCache[gid] = automationCacheEntry{rules: rules, at: time.Now()}
	b.automationMu.Unlock()
	return rules
}

func (b *Bot) InvalidateAutomationRules(gid string) {
	b.automationMu.Lock()
	delete(b.automationCache, gid)
	b.automationMu.Unlock()
}


func (b *Bot) registerAutomationHandlers() {
	b.Session.AddHandler(b.onMessageCreateAutomation)
	b.Session.AddHandler(b.onGuildMemberUpdateAutomation)
}

func (b *Bot) onMessageCreateAutomation(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" || m.Author == nil || m.Author.ID == s.State.User.ID {
		return
	}
	b.safeEvent("automation.message", func(ctx context.Context) {
		b.RunAutomation(ctx, s, b.eventFromMessage(m))
	})
}

func (b *Bot) onGuildMemberUpdateAutomation(s *discordgo.Session, m *discordgo.GuildMemberUpdate) {
	if m.GuildID == "" || m.Member == nil || m.User == nil {
		return
	}
	b.safeEvent("automation.memberUpdate", func(ctx context.Context) {
		prev := b.swapAutomationRoles(m.GuildID, m.User.ID, m.Roles)
		added, removed := diffRoles(prev, m.Roles)
		mem := b.stateMember(m.GuildID, m.User.ID)
		if mem == nil {
			mem = m.Member
		} else if len(mem.Roles) == 0 {
			mem.Roles = m.Roles
		}
		for _, rid := range added {
			b.RunAutomation(ctx, s, b.memberEvent("role_add", m.GuildID, mem, rid))
		}
		for _, rid := range removed {
			b.RunAutomation(ctx, s, b.memberEvent("role_remove", m.GuildID, mem, rid))
		}
	})
}

func (b *Bot) swapAutomationRoles(gid, uid string, roles []string) []string {
	b.automationRoleMu.Lock()
	defer b.automationRoleMu.Unlock()
	if b.automationRoles == nil {
		b.automationRoles = map[string]map[string][]string{}
	}
	if b.automationRoles[gid] == nil {
		b.automationRoles[gid] = map[string][]string{}
	}
	total := len(b.automationRoles)
	for _, g := range b.automationRoles {
		total += len(g)
	}
	if total > automationMaxRoles {
		b.automationRoles = map[string]map[string][]string{}
		b.automationRoles[gid] = map[string][]string{}
	}
	prev := b.automationRoles[gid][uid]
	b.automationRoles[gid][uid] = append([]string(nil), roles...)
	return prev
}

func (b *Bot) PrimeAutomationRoles(gid, uid string, roles []string) {
	b.swapAutomationRoles(gid, uid, roles)
}

func (b *Bot) DropAutomationRoles(gid, uid string) {
	b.automationRoleMu.Lock()
	defer b.automationRoleMu.Unlock()
	if g := b.automationRoles[gid]; g != nil {
		delete(g, uid)
	}
}

func diffRoles(prev, cur []string) (added, removed []string) {
	old := map[string]bool{}
	neu := map[string]bool{}
	for _, r := range prev {
		old[r] = true
	}
	for _, r := range cur {
		neu[r] = true
	}
	for _, r := range cur {
		if !old[r] {
			added = append(added, r)
		}
	}
	for _, r := range prev {
		if !neu[r] {
			removed = append(removed, r)
		}
	}
	return
}

func (b *Bot) stateMember(gid, uid string) *discordgo.Member {
	if mem, _ := b.Session.State.Member(gid, uid); mem != nil {
		return mem
	}
	mem, err := b.Session.GuildMember(gid, uid)
	if err != nil {
		return nil
	}
	return mem
}

func (b *Bot) eventFromMessage(m *discordgo.MessageCreate) automationEvent {
	e := automationEvent{
		Kind:      automation.TriggerMessageCreate,
		GuildID:   m.GuildID,
		GuildName: b.guildName(m.GuildID),
		ChannelID: m.ChannelID,
		MessageID: m.ID,
		UserID:    m.Author.ID,
		Username:  m.Author.Username,
		IsBot:     m.Author.Bot,
		Content:   m.Content,
		Mentions:  len(m.Mentions),
		HasLink:   automation.ContainsLink(m.Content),
	}
	if m.Member != nil {
		e.Roles = m.Member.Roles
	}
	e.AccountAge = accountAge(m.Author.ID)
	if ch, _ := b.Session.State.Channel(m.ChannelID); ch != nil {
		e.ChannelName = ch.Name
	}
	return e
}

func (b *Bot) memberEvent(kind, gid string, mem *discordgo.Member, roleID string) automationEvent {
	e := automationEvent{
		Kind:      kind,
		GuildID:   gid,
		GuildName: b.guildName(gid),
		UserID:    mem.User.ID,
		Username:  mem.User.Username,
		IsBot:     mem.User.Bot,
		Roles:     mem.Roles,
		RoleID:    roleID,
	}
	e.AccountAge = accountAge(mem.User.ID)
	return e
}

func (b *Bot) lifecycleEvent(kind, gid string, u *discordgo.User) automationEvent {
	e := automationEvent{
		Kind:      kind,
		GuildID:   gid,
		GuildName: b.guildName(gid),
		UserID:    u.ID,
		Username:  u.Username,
		IsBot:     u.Bot,
	}
	e.AccountAge = accountAge(u.ID)
	return e
}

func (b *Bot) guildName(gid string) string {
	if g, _ := b.Session.State.Guild(gid); g != nil && g.Name != "" {
		return g.Name
	}
	return gid
}

func accountAge(id string) time.Duration {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Since(time.UnixMilli(n>>22 + 1420070400000))
}


func (b *Bot) RunAutomation(ctx context.Context, s *discordgo.Session, e automationEvent) {
	for _, r := range b.automationRulesFor(ctx, e.GuildID) {
		if r.Trigger != e.Kind {
			continue
		}
		if v := r.Check(e); !v.OK {
			continue
		}
		if !b.automationGate(r.ID, e.UserID, r.CooldownSeconds, r.CounterLimit, r.CounterWindow, time.Now()) {
			continue
		}
		if b.executeAutomation(ctx, s, r, e) {
			break
		}
	}
}

func (b *Bot) executeAutomation(ctx context.Context, s *discordgo.Session, r *automation.Rule, e automationEvent) bool {
	body := sanitize.UserText(automation.ExpandTemplate(r.Template, r.Name, e))
	stopped := false
	for _, act := range automation.ParseActions(r.Actions) {
		if act.Kind == "stop" {
			stopped = true
			continue
		}
		b.runAction(ctx, s, r.Name, e, act, body)
	}
	return stopped
}

func (b *Bot) runAction(ctx context.Context, s *discordgo.Session, ruleName string, e automationEvent, act automation.Action, body string) {
	reason := "Automation rule '" + ruleName + "'"
	switch act.Kind {
	case "delete":
		if e.MessageID != "" {
			_ = s.ChannelMessageDelete(e.ChannelID, e.MessageID)
		}
	case "timeout":
		dur := commands.ParseDurationArg(act.Arg)
		if dur <= 0 || dur > 28*24*time.Hour {
			slog.Warn("automation timeout skipped", "rule", ruleName, "arg", act.Arg)
			return
		}
		until := time.Now().Add(dur).UTC()
		if err := s.GuildMemberTimeout(e.GuildID, e.UserID, &until); err != nil {
			slog.Warn("automation timeout failed", "rule", ruleName, "err", err)
			return
		}
		exp := until
		b.recordProtectionCase(ctx, e.GuildID, e.UserID, "timeout",
			reason+" ("+act.Arg+")", int64(dur.Seconds()), &exp)
	case "ban":
		if err := s.GuildBanCreate(e.GuildID, e.UserID, 0); err != nil {
			slog.Warn("automation ban failed", "rule", ruleName, "err", err)
			return
		}
		b.recordProtectionCase(ctx, e.GuildID, e.UserID, "ban", reason, 0, nil)
	case "kick":
		if err := s.GuildMemberDelete(e.GuildID, e.UserID); err != nil {
			slog.Warn("automation kick failed", "rule", ruleName, "err", err)
			return
		}
		b.recordProtectionCase(ctx, e.GuildID, e.UserID, "kick", reason, 0, nil)
	case "role_add", "role_remove":
		var err error
		if act.Kind == "role_add" {
			err = s.GuildMemberRoleAdd(e.GuildID, e.UserID, act.Arg)
		} else {
			err = s.GuildMemberRoleRemove(e.GuildID, e.UserID, act.Arg)
		}
		if err != nil {
			slog.Warn("automation role action failed", "rule", ruleName, "action", act.Kind, "err", err)
		}
	case "dm":
		if ch, err := s.UserChannelCreate(e.UserID); err == nil {
			sendSoft(b, s, ch.ID, b.Container(TextDisplay(body)))
		}
	case "reply":
		if e.MessageID == "" {
			return
		}
		sendSoft(b, s, e.ChannelID, b.Container(TextDisplay("<@"+e.UserID+"> "+body)))
	case "channel":
		sendSoft(b, s, act.Arg, b.Container(
			TextDisplay("Automation: "+ruleName),
			Sep(),
			Section(body),
		))
	case "log":
		gcfg, err := b.Store.GetGuildConfig(ctx, e.GuildID)
		if err != nil || gcfg.LogChannelID == "" {
			return
		}
		lines := []string{
			"Rule: " + ruleName,
			"Trigger: " + e.Kind,
			"User: <@" + e.UserID + "> (`" + e.UserID + "`)",
		}
		if e.ChannelID != "" {
			lines = append(lines, "Channel: <#"+e.ChannelID+">")
		}
		if body != "" {
			lines = append(lines, truncate(body, 500))
		}
		sendSoft(b, s, gcfg.LogChannelID, b.Container(
			TextDisplay("Automation Fired"),
			Sep(),
			Section(lines...),
		))
	default:
		slog.Warn("unknown automation action", "rule", ruleName, "action", act.Kind)
	}
}


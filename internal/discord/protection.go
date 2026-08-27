package discord

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/automation"
	"vilicus/internal/discord/commands/moderation"
	"vilicus/internal/protection"
)


const (
	protectionCacheTTL   = 15 * time.Second
	spamActionCooldown   = 2 * time.Minute
	noticeSelfDeleteSecs = 6
)

type protectionCacheEntry struct {
	cfg *protectionSettings
	at  time.Time
}

type protectionSettings struct {
	antispam        bool
	msgs, window    int
	antilink        string
	filterWords     []string
	honeypotChannel string
	honeypotAction  string
}

func (b *Bot) registerProtectionHandlers() {
	b.Session.AddHandler(b.onMessageCreateProtection)
}

func (b *Bot) InvalidateProtectionConfig(gid string) {
	b.protectionMu.Lock()
	delete(b.protectionCache, gid)
	b.protectionMu.Unlock()
}

func (b *Bot) protectionFor(ctx context.Context, gid string) *protectionSettings {
	b.protectionMu.Lock()
	if e, ok := b.protectionCache[gid]; ok && time.Since(e.at) < protectionCacheTTL {
		b.protectionMu.Unlock()
		return e.cfg
	}
	b.protectionMu.Unlock()

	cfg, err := b.Store.GetProtectionConfig(ctx, gid)
	var s *protectionSettings
	if err == nil {
		var words []string
		if cfg.FilterWords != "" {
			words = strings.Split(cfg.FilterWords, ",")
		}
		s = &protectionSettings{
			antispam:        cfg.AntispamEnabled,
			msgs:            cfg.AntispamMsgs,
			window:          cfg.AntispamWindow,
			antilink:        cfg.AntilinkMode,
			filterWords:     words,
			honeypotChannel: cfg.HoneypotChannel,
			honeypotAction:  protection.NormalizePunish(cfg.HoneypotAction),
		}
	}

	b.protectionMu.Lock()
	b.protectionCache[gid] = protectionCacheEntry{cfg: s, at: time.Now()}
	b.protectionMu.Unlock()
	return s
}

func (b *Bot) onMessageCreateProtection(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" || m.Author == nil || m.Author.Bot {
		return
	}
	b.safeEvent("protection", func(ctx context.Context) {
		settings := b.protectionFor(ctx, m.GuildID)
		if settings == nil {
			return
		}

		isMod := memberIsMod(s, m.GuildID, m.Author.ID)

		if settings.honeypotChannel == m.ChannelID {
			if !isMod {
				b.tripHoneypot(ctx, s, m)
			}
			return
		}

		if len(m.Content) == 0 {
			return
		}

		if b.checkFilterAndLinks(ctx, s, m, settings, isMod) {
			return
		}
		if settings.antispam {
			b.checkBurst(ctx, s, m, settings, isMod)
		}
	})
}

const honeypotCooldown = time.Minute

func (b *Bot) tripHoneypot(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	key := m.GuildID + ":" + m.Author.ID

	now := time.Now()
	b.antinukeMu.Lock()
	if last, cooling := b.honeypotCooldown[key]; cooling && now.Sub(last) < honeypotCooldown {
		b.antinukeMu.Unlock()
		return
	}
	b.honeypotCooldown[key] = now
	b.antinukeMu.Unlock()

	_ = s.ChannelMessageDelete(m.ChannelID, m.ID)

	gid, uid := m.GuildID, m.Author.ID
	action := protection.NormalizePunish(b.protectionAction(gid))
	var caseType string
	var err error
	switch action {
	case protection.PunishTimeout:
		caseType = "timeout"
		until := now.Add(7 * 24 * time.Hour).UTC()
		err = s.GuildMemberTimeout(gid, uid, &until)
	case protection.PunishKick:
		caseType = "kick"
		err = s.GuildMemberDeleteWithReason(gid, uid, "[Vilicus honeypot] posted in trap channel")
	default:
		caseType = "ban"
		err = s.GuildBanCreateWithReason(gid, uid, "[Vilicus honeypot] posted in trap channel", 1)
	}
	if err != nil {
		slog.Warn("honeypot punishment failed", "guild_id", gid, "user_id", uid, "action", action, "err", err)
		return
	}

	dur := int64(0)
	var expires *time.Time
	if action == protection.PunishTimeout {
		exp := now.Add(7 * 24 * time.Hour).UTC()
		expires = &exp
		dur = int64((7 * 24 * time.Hour).Seconds())
	}
	b.recordProtectionCase(ctx, gid, uid, caseType, "Honeypot: posted in the trap channel", dur, expires)

	if gcfg, gerr := b.Store.GetGuildConfig(ctx, gid); gerr == nil && gcfg.LogChannelID != "" {
		sendSoft(b, s, gcfg.LogChannelID, b.Container(
			TextDisplay("Honeypot Tripped"),
			Sep(),
			Section(
				"Actor: <@"+uid+">",
				"Action: "+action+" (case filed)",
				"Timestamp: "+now.UTC().Format(time.RFC3339),
			),
		))
	}
}

func (b *Bot) protectionAction(gid string) string {
	cfg, err := b.Store.GetProtectionConfig(context.Background(), gid)
	if err != nil {
		return ""
	}
	return cfg.HoneypotAction
}

func memberIsMod(s *discordgo.Session, gid, uid string) bool {
	mem, err := s.State.Member(gid, uid)
	if err != nil || mem == nil {
		return false
	}
	g, err := s.State.Guild(gid)
	if err != nil || g == nil {
		return false
	}
	for _, rid := range mem.Roles {
		for _, r := range g.Roles {
			if r.ID == rid && r.Permissions&(int64(discordgo.PermissionManageMessages)|int64(discordgo.PermissionAdministrator)) != 0 {
				return true
			}
		}
	}
	return g.OwnerID == uid
}

func (b *Bot) checkFilterAndLinks(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, settings *protectionSettings, isMod bool) bool {
	lower := strings.ToLower(m.Content)

	hit := false
	for _, w := range settings.filterWords {
		if w != "" && strings.Contains(lower, w) {
			hit = true
			break
		}
	}
	if !hit && settings.antilink != "off" {
		if !(settings.antilink == "mods" && isMod) {
			hit = automation.ContainsLink(m.Content)
		}
	}
	if !hit {
		return false
	}
	if isMod {
		return false
	}

	_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
	note := "Your message was removed by the server filter."
	if hitByLink(m.Content) {
		note = "Your message was removed: links are not allowed here."
	}
	b.recordProtectionCase(ctx, m.GuildID, m.Author.ID, "warn",
		"Automatic filter: removed message", 0, nil)

	ch, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: "<@" + m.Author.ID + "> " + note,
	})
	if err == nil && ch != nil {
		time.AfterFunc(noticeSelfDeleteSecs*time.Second, func() {
			_ = s.ChannelMessageDelete(m.ChannelID, ch.ID)
		})
	}
	return true
}

func hitByLink(content string) bool { return automation.ContainsLink(content) }

func (b *Bot) recordProtectionCase(ctx context.Context, gid, targetID, caseType, reason string, durationSeconds int64, expires *time.Time) {
	mod := ""
	if b.Session != nil && b.Session.State != nil && b.Session.State.User != nil {
		mod = b.Session.State.User.ID
	}
	moderation.RecordCase(ctx, b, gid, caseType, mod, targetID, reason, durationSeconds, expires)
}

func (b *Bot) checkBurst(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, settings *protectionSettings, isMod bool) {
	if isMod {
		return
	}
	key := m.GuildID + ":" + m.Author.ID

	now := time.Now()
	window := time.Duration(settings.window) * time.Second
	max := settings.msgs

	b.spamMu.Lock()
	times := b.spamWindow[key]
	cut := now.Add(-window)
	kept := times[:0]
	for _, t := range times {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	b.spamWindow[key] = kept
	hits := len(kept)
	lastAct, cooling := b.spamCooldown[key]
	b.spamMu.Unlock()

	if hits < max || (cooling && now.Sub(lastAct) < spamActionCooldown) {
		return
	}

	b.spamMu.Lock()
	b.spamCooldown[key] = now
	delete(b.spamWindow, key)
	b.spamMu.Unlock()

	until := now.Add(time.Minute).UTC()
	if err := s.GuildMemberTimeout(m.GuildID, m.Author.ID, &until); err != nil {
		slog.Warn("antispam timeout failed", "guild_id", m.GuildID, "user_id", m.Author.ID, "err", err)
		return
	}
	expires := until
	b.recordProtectionCase(ctx, m.GuildID, m.Author.ID, "timeout",
		"Antispam: "+itoa(int64(hits))+" messages within "+itoa(int64(window.Seconds()))+"s",
		60, &expires)
	dest := m.ChannelID
	if gcfg, gerr := b.Store.GetGuildConfig(ctx, m.GuildID); gerr == nil && gcfg.LogChannelID != "" {
		dest = gcfg.LogChannelID
	}
	sendSoft(b, s, dest, b.Container(
		TextDisplay("Spam Filter"),
		Sep(),
		Section(
			"<@"+m.Author.ID+"> timed out for 1 minute ("+itoa(int64(hits))+" messages in "+itoa(int64(window.Seconds()))+"s).",
			"Channel: <#"+m.ChannelID+">",
		),
	))
}


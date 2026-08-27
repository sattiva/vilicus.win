package discord

import (
	"context"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/logging"
)


func (b *Bot) registerEventHandlers() {
	b.Session.AddHandler(b.onGuildMemberAdd)
	b.Session.AddHandler(b.onGuildMemberRemove)
	b.Session.AddHandler(b.onMessageDelete)
	b.Session.AddHandler(b.onMessageUpdate)
	b.Session.AddHandler(b.onGuildBanAdd)
	b.Session.AddHandler(b.onGuildBanRemove)

	b.registerStarboardHandlers()
	b.registerProtectionHandlers()
	b.registerLevelHandlers()
	b.registerAutomationHandlers()
}

func (b *Bot) safeEvent(name string, fn func(ctx context.Context)) {
	reqID := logging.NewID()
	ctx := logging.WithID(context.Background(), reqID)
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("event handler panic", "event", name, "err", rec, "req_id", reqID)
		}
	}()
	fn(ctx)
}


func (b *Bot) onGuildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	if m.Member == nil || m.Member.User == nil {
		return
	}
	b.safeEvent("memberAdd", func(ctx context.Context) {
		b.applyAutorole(ctx, s, m.GuildID, m.Member.User.ID)
		b.sendGreeting(ctx, s, m.GuildID, m.Member.User, true)
		logMemberJoin(ctx, b, s, m)
		b.PrimeAutomationRoles(m.GuildID, m.Member.User.ID, m.Member.Roles)
		b.RunAutomation(ctx, s, b.memberEvent("member_join", m.GuildID, m.Member, ""))
	})
}

func (b *Bot) onGuildMemberRemove(s *discordgo.Session, m *discordgo.GuildMemberRemove) {
	if m.Member == nil || m.Member.User == nil {
		return
	}
	b.safeEvent("memberRemove", func(ctx context.Context) {
		b.sendGreeting(ctx, s, m.GuildID, m.Member.User, false)
		logMemberLeave(ctx, b, s, m)
		b.DropAutomationRoles(m.GuildID, m.Member.User.ID)
		b.RunAutomation(ctx, s, b.memberEvent("member_leave", m.GuildID, m.Member, ""))
	})
}

func (b *Bot) applyAutorole(ctx context.Context, s *discordgo.Session, gid, uid string) {
	gcfg, err := b.Store.GetGuildConfig(ctx, gid)
	if err != nil || gcfg.AutoRoleID == "" {
		return
	}
	if err := s.GuildMemberRoleAdd(gid, uid, gcfg.AutoRoleID); err != nil {
		slog.Warn("autorole failed", "guild_id", gid, "user_id", uid, "err", err)
	}
}

func (b *Bot) sendGreeting(ctx context.Context, s *discordgo.Session, gid string, u *discordgo.User, join bool) {
	gcfg, err := b.Store.GetGuildConfig(ctx, gid)
	if err != nil {
		return
	}
	ch := gcfg.WelcomeChannelID
	if ch == "" {
		return
	}

	var title, line string
	if join {
		title = "Member Joined"
		line = "Welcome <@" + u.ID + "> to the server."
	} else {
		title = "Member Left"
		line = "<@" + u.ID + "> has left the server."
	}

	container := b.Container(
		TextDisplay(title),
		Sep(),
		Section(
			line,
			"Account created: <t:"+itoa(snowflakeUnix(u.ID))+":R>",
			"Timestamp: "+time.Now().UTC().Format(time.RFC3339),
		),
	)
	sendSoft(b, s, ch, container)
}


func (b *Bot) onMessageDelete(s *discordgo.Session, m *discordgo.MessageDelete) {
	b.safeEvent("messageDelete", func(ctx context.Context) {
		if m.Message != nil && m.Message.Author != nil && !m.Message.Author.Bot {
			b.snipes.set(m.ChannelID, m.Message.Content, m.Message.Author.ID)
		}
		logMessageDelete(ctx, b, s, m)
	})
}

func (b *Bot) onMessageUpdate(s *discordgo.Session, m *discordgo.MessageUpdate) {
	b.safeEvent("messageUpdate", func(ctx context.Context) {
		if m.Message == nil || m.Author == nil || m.Author.Bot {
			return
		}
		if m.EditedTimestamp == nil || m.Content == "" {
			return
		}
		before := ""
		if prev, ok := b.edits.get(m.ID); ok && prev != m.Content {
			before = prev
		}
		if before == "" {
			return
		}
		logMessageEdit(ctx, b, s, m, before)
	})
}


func (b *Bot) onGuildBanAdd(s *discordgo.Session, m *discordgo.GuildBanAdd) {
	b.safeEvent("banAdd", func(ctx context.Context) {
		if m.User == nil {
			return
		}
		logBan(ctx, b, s, m.GuildID, m.User, true)
		b.RunAutomation(ctx, s, b.lifecycleEvent("member_ban", m.GuildID, m.User))
	})
}

func (b *Bot) onGuildBanRemove(s *discordgo.Session, m *discordgo.GuildBanRemove) {
	b.safeEvent("banRemove", func(ctx context.Context) {
		if m.User == nil {
			return
		}
		logBan(ctx, b, s, m.GuildID, m.User, false)
		b.RunAutomation(ctx, s, b.lifecycleEvent("member_unban", m.GuildID, m.User))
	})
}


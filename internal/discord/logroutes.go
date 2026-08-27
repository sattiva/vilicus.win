package discord

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"
)


func logRoute(ctx context.Context, b *Bot, s *discordgo.Session, gid, title string, lines []string) {
	gcfg, err := b.Store.GetGuildConfig(ctx, gid)
	if err != nil || gcfg.LogChannelID == "" {
		return
	}
	lines = append(lines, "Timestamp: "+time.Now().UTC().Format(time.RFC3339))
	container := b.Container(
		TextDisplay(title),
		Sep(),
		Section(lines...),
	)
	sendSoft(b, s, gcfg.LogChannelID, container)
}

func logMemberJoin(ctx context.Context, b *Bot, s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	if m.Member == nil || m.Member.User == nil {
		return
	}
	u := m.Member.User
	logRoute(ctx, b, s, m.GuildID, "Member Join", []string{
		"User: <@" + u.ID + "> (`" + u.ID + "`)",
		"Username: " + u.Username,
		"Account created: <t:" + itoa(snowflakeUnix(u.ID)) + ":R>",
	})
}

func logMemberLeave(ctx context.Context, b *Bot, s *discordgo.Session, m *discordgo.GuildMemberRemove) {
	if m.Member == nil || m.Member.User == nil {
		return
	}
	u := m.Member.User
	logRoute(ctx, b, s, m.GuildID, "Member Leave", []string{
		"User: <@" + u.ID + "> (`" + u.ID + "`)",
		"Username: " + u.Username,
	})
}

func logMessageDelete(ctx context.Context, b *Bot, s *discordgo.Session, m *discordgo.MessageDelete) {
	if m.Message == nil || m.Message.Author == nil || m.Message.Author.Bot {
		return
	}
	logRoute(ctx, b, s, m.GuildID, "Message Deleted", []string{
		"Author: <@" + m.Message.Author.ID + "> in <#" + m.ChannelID + ">",
		"Content: " + truncate(m.Content, 500),
	})
}

func logMessageEdit(ctx context.Context, b *Bot, s *discordgo.Session, m *discordgo.MessageUpdate, before string) {
	logRoute(ctx, b, s, m.GuildID, "Message Edited", []string{
		"Author: <@" + m.Author.ID + "> in <#" + m.ChannelID + ">",
		"Before: " + truncate(before, 250),
		"After: " + truncate(m.Content, 250),
	})
}

func logBan(ctx context.Context, b *Bot, s *discordgo.Session, gid string, u *discordgo.User, banned bool) {
	title := "User Unbanned"
	if banned {
		title = "User Banned"
	}
	logRoute(ctx, b, s, gid, title, []string{
		"User: <@" + u.ID + "> (`" + u.ID + "`)",
		"Username: " + u.Username,
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		if s == "" {
			return "(empty)"
		}
		return s
	}
	return s[:n] + "..."
}


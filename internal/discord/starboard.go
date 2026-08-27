package discord

import (
	"context"
	"log/slog"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
)


const starEmoji = "\u2b50"

func (b *Bot) registerStarboardHandlers() {
	b.Session.AddHandler(b.onReactionAdd)
	b.Session.AddHandler(b.onReactionRemove)
}

func isStar(e *discordgo.Emoji) bool {
	return e != nil && e.Name == starEmoji && e.ID == ""
}

func (b *Bot) onReactionAdd(s *discordgo.Session, m *discordgo.MessageReactionAdd) {
	if !isStar(&m.Emoji) || m.GuildID == "" || m.UserID == s.State.User.ID {
		return
	}
	if mem, _ := s.State.Member(m.GuildID, m.UserID); mem != nil && mem.User != nil && mem.User.Bot {
		return
	}
	b.safeEvent("starboardAdd", func(ctx context.Context) {
		b.starboardBump(ctx, s, m.GuildID, m.ChannelID, m.MessageID)
	})
}

func (b *Bot) onReactionRemove(s *discordgo.Session, m *discordgo.MessageReactionRemove) {
	if !isStar(&m.Emoji) || m.GuildID == "" {
		return
	}
	b.safeEvent("starboardRemove", func(ctx context.Context) {
		b.starboardDrop(ctx, s, m.GuildID, m.ChannelID, m.MessageID)
	})
}

func (b *Bot) starboardBump(ctx context.Context, s *discordgo.Session, gid, chID, msgID string) {
	cfg, err := b.Store.GetStarboardConfig(ctx, gid)
	if err != nil || !cfg.Enabled || cfg.ChannelID == "" {
		return
	}

	stars, boardMsgID, err := b.Store.AddStar(ctx, gid, msgID)
	if err != nil {
		slog.Warn("starboard add failed", "guild_id", gid, "err", err)
		return
	}

	switch {
	case stars < cfg.Threshold:
	case boardMsgID == "":
		card := b.buildStarCard(s, chID, msgID, stars)
		msg, err := s.ChannelMessageSendComplex(cfg.ChannelID, &discordgo.MessageSend{
			Flags:      components.FlagComponentsV2,
			Components: []discordgo.MessageComponent{card},
		})
		if err != nil {
			slog.Warn("starboard post failed", "guild_id", gid, "err", err)
			return
		}
		_ = b.Store.SetStarboardBoardMessage(ctx, gid, msgID, msg.ID)
	default:
		b.editStarCard(s, cfg.ChannelID, chID, msgID, boardMsgID, stars)
	}
}

func (b *Bot) starboardDrop(ctx context.Context, s *discordgo.Session, gid, chID, msgID string) {
	cfg, err := b.Store.GetStarboardConfig(ctx, gid)
	if err != nil || !cfg.Enabled || cfg.ChannelID == "" {
		return
	}
	stars, boardMsgID, err := b.Store.RemoveStar(ctx, gid, msgID)
	if err != nil || boardMsgID == "" {
		return
	}
	if stars <= 0 {
		_ = s.ChannelMessageDelete(cfg.ChannelID, boardMsgID)
		_ = b.Store.SetStarboardBoardMessage(ctx, gid, msgID, "")
		return
	}
	b.editStarCard(s, cfg.ChannelID, chID, msgID, boardMsgID, stars)
}

func (b *Bot) buildStarCard(s *discordgo.Session, srcChannel, srcMsgID string, stars int) *components.Container {
	content := "(no text content)"
	authorID := ""
	jump := ""
	if msg, err := s.ChannelMessage(srcChannel, srcMsgID); err == nil && msg != nil {
		content = truncate(msg.Content, 1000)
		jump = msgLink(msg.GuildID, srcChannel, srcMsgID)
		if msg.Author != nil {
			authorID = msg.Author.ID
		}
	} else {
		jump = msgLink("", srcChannel, srcMsgID)
	}
	if content == "" {
		content = "(attachment or embed only)"
	}

	header := TextDisplay(starEmoji + " **" + itoa(int64(stars)) + "**  -  <#" + srcChannel + ">")
	details := []string{content}
	if jump != "" {
		details = append(details, "[Jump to message]("+jump+")")
	}
	if authorID != "" {
		details = append(details, "From: <@"+authorID+">")
	}
	return b.Container(header, Sep(), Section(details...))
}

func (b *Bot) editStarCard(s *discordgo.Session, boardChannel, srcChannel, srcMsgID, boardMsgID string, stars int) {
	card := b.buildStarCard(s, srcChannel, srcMsgID, stars)
	_, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         boardMsgID,
		Channel:    boardChannel,
		Components: &[]discordgo.MessageComponent{card},
	})
	if err != nil {
		slog.Warn("starboard card edit failed", "err", err)
	}
}

func msgLink(gid, chID, msgID string) string {
	if gid == "" {
		return "https://discord.com/channels/@me/" + chID + "/" + msgID
	}
	return "https://discord.com/channels/" + gid + "/" + chID + "/" + msgID
}


package discord

import (
	"log/slog"
	"strconv"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
)


func TextDisplay(content string) components.TextDisplay {
	return components.TextDisplay{Content: content}
}

func Sep() components.Separator {
	return components.Separator{Divider: true, Spacing: 1}
}

func Section(lines ...string) components.Section {
	comps := make([]discordgo.MessageComponent, 0, len(lines))
	for _, l := range lines {
		comps = append(comps, TextDisplay(l))
	}
	return components.Section{Components: comps}
}

func snowflakeUnix(id string) int64 {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return (n >> 22) + 1420070400000/1000
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func sendSoft(b *Bot, s *discordgo.Session, channelID string, c *components.Container) {
	if channelID == "" || c == nil {
		return
	}
	if err := components.Validate(c); err != nil {
		slog.Warn("soft send skipped: invalid container", "err", err)
		return
	}
	if _, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Flags:      components.FlagComponentsV2,
		Components: []discordgo.MessageComponent{c},
	}); err != nil {
		slog.Warn("soft send failed", "channel_id", channelID, "err", err)
	}
}


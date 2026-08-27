package moderation

import (
	"context"
	"strconv"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)

type StarboardCmd struct{}

func (c *StarboardCmd) Name() string { return "starboard" }
func (c *StarboardCmd) Description() string {
	return "Configure the starboard (starred messages get reposted)"
}
func (c *StarboardCmd) Category() string  { return "Configuration" }
func (c *StarboardCmd) Aliases() []string { return nil }

func (c *StarboardCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageGuild)
	return &perms
}

var (
	sbThresholdMin = float64(1)
	sbThresholdMax = float64(25)
)

func (c *StarboardCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{Name: "set", Description: "Enable the starboard in a channel", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel starred messages are posted to", Required: true,
				ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText}},
			{Type: discordgo.ApplicationCommandOptionInteger, Name: "threshold", Description: "Stars needed to hit the board (1-25, default 3)", Required: false, MinValue: &sbThresholdMin, MaxValue: sbThresholdMax},
		}},
		{Name: "off", Description: "Disable the starboard", Type: discordgo.ApplicationCommandOptionSubCommand},
		{Name: "show", Description: "Show current starboard settings", Type: discordgo.ApplicationCommandOptionSubCommand},
	}
}

func (c *StarboardCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return c.show(ctx, b, i.GuildID)
	}
	switch opts[0].Name {
	case "set":
		chID, threshold := "", 3
		for _, o := range opts[0].Options {
			switch o.Name {
			case "channel":
				chID = o.ChannelValue(s).ID
			case "threshold":
				threshold = int(o.IntValue())
			}
		}
		if chID == "" {
			return b.Container(components.TextDisplay{Content: "Target channel required."}), nil
		}
		cfg := &store.StarboardConfig{GuildID: i.GuildID, ChannelID: chID, Threshold: threshold, Enabled: true}
		if err := b.GetStore().SaveStarboardConfig(ctx, cfg); err != nil {
			return b.Container(components.TextDisplay{Content: "Failed saving starboard config: " + err.Error()}), nil
		}
		return b.Container(
			components.TextDisplay{Content: "Starboard Enabled"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: "Board channel: <#" + chID + ">"},
					components.TextDisplay{Content: "Threshold: " + strconv.Itoa(threshold) + " stars"},
				},
			},
		), nil
	case "off":
		cfg, err := b.GetStore().GetStarboardConfig(ctx, i.GuildID)
		if err != nil || !cfg.Enabled {
			return b.Container(components.TextDisplay{Content: "Starboard is not enabled."}), nil
		}
		cfg.Enabled = false
		if err := b.GetStore().SaveStarboardConfig(ctx, cfg); err != nil {
			return b.Container(components.TextDisplay{Content: "Failed saving starboard config: " + err.Error()}), nil
		}
		return b.Container(components.TextDisplay{Content: "Starboard disabled. Existing board posts stay put."}), nil
	default:
		return c.show(ctx, b, i.GuildID)
	}
}

func (c *StarboardCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) == 0 || args[0] == "show" {
		return c.show(ctx, b, m.GuildID)
	}
	switch args[0] {
	case "off", "disable":
		cfg, err := b.GetStore().GetStarboardConfig(ctx, m.GuildID)
		if err != nil || !cfg.Enabled {
			return b.Container(components.TextDisplay{Content: "Starboard is not enabled."}), nil
		}
		cfg.Enabled = false
		if err := b.GetStore().SaveStarboardConfig(ctx, cfg); err != nil {
			return b.Container(components.TextDisplay{Content: "Failed saving starboard config: " + err.Error()}), nil
		}
		return b.Container(components.TextDisplay{Content: "Starboard disabled."}), nil
	case "set", "on", "enable":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .starboard set <#channel> [threshold]"}), nil
		}
		chID := commands.ParseMentionID(args[1])
		if len(chID) < 17 {
			if ch, err := s.Channel(args[1]); err == nil && ch != nil {
				chID = ch.ID
			} else {
				return b.Container(components.TextDisplay{Content: "Could not resolve a text channel from that argument."}), nil
			}
		}
		threshold := 3
		if len(args) > 2 {
			if n, err := strconv.Atoi(args[2]); err == nil && n >= 1 && n <= 25 {
				threshold = n
			}
		}
		cfg := &store.StarboardConfig{GuildID: m.GuildID, ChannelID: chID, Threshold: threshold, Enabled: true}
		if err := b.GetStore().SaveStarboardConfig(ctx, cfg); err != nil {
			return b.Container(components.TextDisplay{Content: "Failed saving starboard config: " + err.Error()}), nil
		}
		return b.Container(
			components.TextDisplay{Content: "Starboard Enabled"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: "Board channel: <#" + chID + ">"},
					components.TextDisplay{Content: "Threshold: " + strconv.Itoa(threshold) + " stars"},
				},
			},
		), nil
	default:
		return b.Container(components.TextDisplay{Content: "Usage: .starboard set|off|show"}), nil
	}
}

func (c *StarboardCmd) show(ctx context.Context, b commands.BotInterface, gid string) (*components.Container, error) {
	cfg, err := b.GetStore().GetStarboardConfig(ctx, gid)
	if err != nil || cfg.ChannelID == "" {
		return b.Container(components.TextDisplay{Content: "Starboard is not configured. Use /starboard set to enable it."}), nil
	}
	status := "disabled"
	if cfg.Enabled {
		status = "enabled"
	}
	return b.Container(
		components.TextDisplay{Content: "Starboard Settings"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: "Status: " + status},
				components.TextDisplay{Content: "Board channel: <#" + cfg.ChannelID + ">"},
				components.TextDisplay{Content: "Threshold: " + strconv.Itoa(cfg.Threshold) + " stars"},
			},
		},
	), nil
}


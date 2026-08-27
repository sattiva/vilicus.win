package general

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type ServerInfoCmd struct{}

func (c *ServerInfoCmd) Name() string {
	return "serverinfo"
}

func (c *ServerInfoCmd) Description() string {
	return "Display server configuration and membership metrics"
}

func (c *ServerInfoCmd) Category() string {
	return "General"
}

func (c *ServerInfoCmd) Aliases() []string {
	return []string{"guildinfo", "si", "guild"}
}

func (c *ServerInfoCmd) Options() []*discordgo.ApplicationCommandOption {
	return nil
}

func (c *ServerInfoCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *ServerInfoCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only valid inside a guild."}), nil
	}
	return c.renderServerInfo(b, s, i.GuildID)
}

func (c *ServerInfoCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only valid inside a guild."}), nil
	}
	return c.renderServerInfo(b, s, m.GuildID)
}

func (c *ServerInfoCmd) renderServerInfo(b commands.BotInterface, s *discordgo.Session, gid string) (*components.Container, error) {
	g, err := s.Guild(gid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed fetching guild %s", gid)}), nil
	}

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Server Information: %s", g.Name)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Guild ID: `%s`", g.ID)},
				components.TextDisplay{Content: fmt.Sprintf("Owner: <@%s> (`%s`)", g.OwnerID, g.OwnerID)},
				components.TextDisplay{Content: fmt.Sprintf("Total Members: %d", g.MemberCount)},
				components.TextDisplay{Content: fmt.Sprintf("Total Roles: %d", len(g.Roles))},
				components.TextDisplay{Content: fmt.Sprintf("Total Channels: %d", len(g.Channels))},
				components.TextDisplay{Content: fmt.Sprintf("Premium Tier: %d (Boosts: %d)", g.PremiumTier, g.PremiumSubscriptionCount)},
			},
		},
	), nil
}


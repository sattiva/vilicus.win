package moderation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type ModStatsCmd struct{}

func (c *ModStatsCmd) Name() string        { return "modstats" }
func (c *ModStatsCmd) Description() string { return "Show a moderator's case counts by type" }
func (c *ModStatsCmd) Category() string    { return "Moderation" }
func (c *ModStatsCmd) Aliases() []string   { return []string{"mystats"} }

func (c *ModStatsCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionModerateMembers)
	return &perms
}

func (c *ModStatsCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "moderator",
			Description: "Moderator to inspect (default: yourself)",
			Required:    false,
		},
	}
}

func (c *ModStatsCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	modID := i.Member.User.ID
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "moderator" {
			if u := o.UserValue(s); u != nil {
				modID = u.ID
			}
		}
	}
	return c.render(ctx, b, i.GuildID, modID)
}

func (c *ModStatsCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	modID := m.Author.ID
	if m.Member != nil && m.Member.User != nil {
		modID = m.Member.User.ID
	}
	if len(args) >= 1 {
		if id := commands.ParseMentionID(args[0]); id != "" {
			modID = id
		} else {
			return b.Container(components.TextDisplay{Content: "Could not resolve a moderator from the first argument."}), nil
		}
	}
	return c.render(ctx, b, m.GuildID, modID)
}

func (c *ModStatsCmd) render(ctx context.Context, b commands.BotInterface, gid, modID string) (*components.Container, error) {
	stats, err := b.GetStore().ModStats(ctx, gid, modID)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed loading stats: %s", err.Error())}), nil
	}

	if len(stats) == 0 {
		return b.Container(components.TextDisplay{
			Content: fmt.Sprintf("No recorded cases for <@%s> in this server.", modID),
		}), nil
	}

	total := int64(0)
	types := make([]string, 0, len(stats))
	for t, n := range stats {
		total += n
		types = append(types, t)
	}
	sort.Strings(types)

	lines := make([]string, 0, len(types))
	for _, t := range types {
		lines = append(lines, fmt.Sprintf("%s - %d", t, stats[t]))
	}

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Moderation Stats: <@%s>", modID)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: strings.Join(lines, "\n")},
				components.TextDisplay{Content: fmt.Sprintf("Total: %d", total)},
			},
		},
	), nil
}


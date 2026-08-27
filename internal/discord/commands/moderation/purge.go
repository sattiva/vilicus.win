package moderation

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type PurgeCmd struct{}

func (c *PurgeCmd) Name() string {
	return "purge"
}

func (c *PurgeCmd) Description() string {
	return "Bulk delete up to 100 recent messages from the current channel"
}

func (c *PurgeCmd) Category() string {
	return "Moderation"
}

func (c *PurgeCmd) Aliases() []string {
	return []string{"clear", "clean"}
}

func (c *PurgeCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageMessages)
	return &perms
}

func (c *PurgeCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "count",
			Description: "Number of messages to delete (1-100)",
			Required:    true,
		},
	}
}

func (c *PurgeCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return b.Container(components.TextDisplay{Content: "Count parameter required."}), nil
	}

	count := int(opts[0].IntValue())
	return c.handlePurge(ctx, b, s, i.GuildID, i.ChannelID, i.Member.User.ID, count)
}

func (c *PurgeCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if len(args) == 0 {
		return b.Container(components.TextDisplay{Content: "Usage: `.purge <count (1-100)>`"}), nil
	}

	count, err := strconv.Atoi(args[0])
	if err != nil || count < 1 || count > 100 {
		return b.Container(components.TextDisplay{Content: "Count must be between 1 and 100."}), nil
	}

	return c.handlePurge(ctx, b, s, m.GuildID, m.ChannelID, m.Author.ID, count)
}

func (c *PurgeCmd) handlePurge(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, chID, modID string, count int) (*components.Container, error) {
	if count < 1 || count > 100 {
		return b.Container(components.TextDisplay{Content: "Count must be between 1 and 100."}), nil
	}

	msgs, err := s.ChannelMessages(chID, count, "", "", "")
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed fetching messages: %s", err.Error())}), nil
	}

	var msgIDs []string
	for _, m := range msgs {
		msgIDs = append(msgIDs, m.ID)
	}

	if len(msgIDs) > 0 {
		err = s.ChannelMessagesBulkDelete(chID, msgIDs)
		if err != nil {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed purging messages: %s", err.Error())}), nil
		}
	}

	if gid != "" {
		commands.DispatchAudit(ctx, b, s, gid, modID, chID, "Purge", "Bulk message deletion", fmt.Sprintf("Deleted: %d messages", len(msgIDs)))
	}

	return b.Container(
		components.TextDisplay{Content: "Messages Purged"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Channel: <#%s>", chID)},
				components.TextDisplay{Content: fmt.Sprintf("Messages Deleted: %d", len(msgIDs))},
				components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", modID)},
			},
		},
	), nil
}


package general

import (
	"context"
	"strconv"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type SnipeCmd struct{}

func (c *SnipeCmd) Name() string { return "snipe" }
func (c *SnipeCmd) Description() string {
	return "Show the most recently deleted message in this channel"
}
func (c *SnipeCmd) Category() string  { return "General" }
func (c *SnipeCmd) Aliases() []string { return []string{"s"} }
func (c *SnipeCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageMessages)
	return &perms
}

func (c *SnipeCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionChannel,
			Name:        "channel",
			Description: "Channel to snipe (default: current)",
			Required:    false,
			ChannelTypes: []discordgo.ChannelType{
				discordgo.ChannelTypeGuildText,
			},
		},
	}
}

func (c *SnipeCmd) FastPath() bool { return true }

func (c *SnipeCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	channelID := i.ChannelID
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "channel" {
			channelID = o.ChannelValue(s).ID
		}
	}
	return renderSnipe(b, channelID), nil
}

func (c *SnipeCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	return renderSnipe(b, m.ChannelID), nil
}

func renderSnipe(b commands.BotInterface, channelID string) *components.Container {
	reader, ok := b.(commands.SnipeReader)
	if !ok {
		return b.Container(components.TextDisplay{Content: "Snipe is not available."})
	}
	content, authorID, at, found := reader.LatestSnipe(channelID)
	if !found {
		return b.Container(components.TextDisplay{Content: "Nothing to snipe  -  no deleted messages here in the last 5 minutes."})
	}
	return b.Container(
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: "Last Deleted Message"},
				components.TextDisplay{Content: "Author: <@" + authorID + ">"},
				components.TextDisplay{Content: content},
				components.TextDisplay{Content: "Deleted: <t:" + strconv.FormatInt(at.Unix(), 10) + ":R>"},
			},
		},
	)
}


package general

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type RemindCmd struct{}

func (c *RemindCmd) Name() string        { return "remind" }
func (c *RemindCmd) Description() string { return "Set a reminder for later in this channel" }
func (c *RemindCmd) Category() string    { return "General" }
func (c *RemindCmd) Aliases() []string   { return []string{"remindme", "reminder"} }

func (c *RemindCmd) RequiredPermissions() *int64 { return nil }

func (c *RemindCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "in",
			Description: "When to remind you, e.g. 1h30m (max 365d)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "body",
			Description: "What to remind you about",
			Required:    true,
		},
	}
}

func (c *RemindCmd) FastPath() bool { return true }

func (c *RemindCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	inRaw, body := "", ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "in":
			inRaw = o.StringValue()
		case "body":
			body = o.StringValue()
		}
	}
	return c.handle(ctx, b, i.GuildID, i.ChannelID, i.Member.User.ID, inRaw, body)
}

func (c *RemindCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .remind <1h30m> <what to remind you about>"}), nil
	}
	userID := m.Author.ID
	if m.Member != nil && m.Member.User != nil {
		userID = m.Member.User.ID
	}
	return c.handle(ctx, b, m.GuildID, m.ChannelID, userID, args[0], strings.Join(args[1:], " "))
}

func (c *RemindCmd) handle(ctx context.Context, b commands.BotInterface, gid, channelID, userID, inRaw, body string) (*components.Container, error) {
	if strings.TrimSpace(body) == "" {
		return b.Container(components.TextDisplay{Content: "A reminder body is required."}), nil
	}
	d := commands.ParseDurationArg(inRaw)
	if d <= 0 {
		return b.Container(components.TextDisplay{Content: "Invalid duration. Use forms like 30m, 2h, 1d2h (max 365d)."}), nil
	}

	dueAt := time.Now().UTC().Add(d)
	if _, err := b.GetStore().CreateReminder(ctx, userID, gid, channelID, body, dueAt); err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed saving reminder: %s", err.Error())}), nil
	}

	lines := []discordgo.MessageComponent{
		components.TextDisplay{Content: "Reminder Set"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Fires: <t:%d:R>", dueAt.Unix())},
				components.TextDisplay{Content: truncateText(body, 400)},
			},
		},
	}
	if gid == "" {
		lines = append(lines, components.TextDisplay{Content: "(DM reminders fire here.)"})
	}
	return b.Container(lines...), nil
}


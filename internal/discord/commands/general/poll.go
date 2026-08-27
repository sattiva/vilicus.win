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

type PollCmd struct{}

func (c *PollCmd) Name() string        { return "poll" }
func (c *PollCmd) Description() string { return "Run a reaction-style vote with live counts" }
func (c *PollCmd) Category() string    { return "General" }
func (c *PollCmd) Aliases() []string   { return []string{"vote"} }

func (c *PollCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *PollCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "question",
			Description: "The poll question",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "options",
			Description: "Comma-separated choices (2-10)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "duration",
			Description: "How long the poll runs, e.g. 2h (default 1h, max 7d)",
			Required:    false,
		},
	}
}

func (c *PollCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	question, rawOpts, durRaw := "", "", ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "question":
			question = o.StringValue()
		case "options":
			rawOpts = o.StringValue()
		case "duration":
			durRaw = o.StringValue()
		}
	}
	channelID := i.ChannelID
	gid := i.GuildID
	return c.start(ctx, b, s, gid, channelID, question, rawOpts, durRaw)
}

func (c *PollCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .poll <question> <option, option, ...> [duration]"}), nil
	}
	durRaw := ""
	if commands.ParseDurationArg(args[len(args)-1]) > 0 && len(args) >= 3 {
		durRaw = args[len(args)-1]
		args = args[:len(args)-1]
	}
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .poll <question> <option, option, ...> [duration]"}), nil
	}
	return c.start(ctx, b, s, m.GuildID, m.ChannelID, args[0], strings.Join(args[1:], " "), durRaw)
}

func (c *PollCmd) start(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, channelID, question, rawOpts, durRaw string) (*components.Container, error) {
	starter, ok := b.(commands.PollStarter)
	if !ok {
		return b.Container(components.TextDisplay{Content: "Polls are not available."}), nil
	}

	question = strings.TrimSpace(question)
	if question == "" {
		return b.Container(components.TextDisplay{Content: "A question is required."}), nil
	}

	opts := make([]string, 0, 10)
	for _, part := range strings.Split(rawOpts, ",") {
		if t := strings.TrimSpace(part); t != "" {
			opts = append(opts, t)
		}
	}
	if len(opts) < 2 || len(opts) > 10 {
		return b.Container(components.TextDisplay{Content: "Polls need between 2 and 10 comma-separated options."}), nil
	}

	duration := time.Hour
	if durRaw != "" {
		d := commands.ParseDurationArg(durRaw)
		if d <= 0 {
			return b.Container(components.TextDisplay{Content: "Invalid duration. Use forms like 30m, 2h, 1d (max 7d)."}), nil
		}
		if d > 7*24*time.Hour {
			return b.Container(components.TextDisplay{Content: "Polls run at most 7 days."}), nil
		}
		duration = d
	}

	msgID, err := starter.StartPoll(s, gid, channelID, question, opts, duration)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed to start poll: %s", err.Error())}), nil
	}

	lines := []discordgo.MessageComponent{
		components.TextDisplay{Content: "Poll Started"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Question: %s", question)},
				components.TextDisplay{Content: fmt.Sprintf("Choices: %d", len(opts))},
			},
		},
	}
	if msgID != "" && gid != "" {
		lines = append(lines,
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: "[Open the poll](https://discord.com/channels/" + gid + "/" + channelID + "/" + msgID + ")"},
				},
			},
		)
	}
	return b.Container(lines...), nil
}


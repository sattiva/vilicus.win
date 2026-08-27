package general

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type ChooseCmd struct{}

func (c *ChooseCmd) Name() string                { return "choose" }
func (c *ChooseCmd) Description() string         { return "Pick one of the given options at random" }
func (c *ChooseCmd) Category() string            { return "General" }
func (c *ChooseCmd) Aliases() []string           { return []string{"pick", "decide"} }
func (c *ChooseCmd) RequiredPermissions() *int64 { return nil }

func (c *ChooseCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "options",
			Description: "Comma-separated options to choose from",
			Required:    true,
		},
	}
}

func (c *ChooseCmd) FastPath() bool { return true }

func (c *ChooseCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	raw := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "options" {
			raw = o.StringValue()
		}
	}
	return renderChoose(b, raw), nil
}

func (c *ChooseCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	return renderChoose(b, strings.Join(args, " ")), nil
}

func renderChoose(b commands.BotInterface, raw string) *components.Container {
	parts := strings.Split(raw, ",")
	opts := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			opts = append(opts, t)
		}
	}
	if len(opts) < 2 {
		return b.Container(components.TextDisplay{Content: "Give me at least two options separated by commas."})
	}
	if len(opts) > 25 {
		return b.Container(components.TextDisplay{Content: "Twenty-five options is plenty."})
	}
	pick := opts[rand.Intn(len(opts))]
	return b.Container(
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Options considered: %d", len(opts))},
				components.TextDisplay{Content: fmt.Sprintf("Decision: **%s**", truncateText(pick, 200))},
			},
		},
	)
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}


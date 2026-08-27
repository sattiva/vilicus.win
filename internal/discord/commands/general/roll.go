package general

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type RollCmd struct{}

func (c *RollCmd) Name() string        { return "roll" }
func (c *RollCmd) Description() string { return "Roll dice in NdM notation (e.g. 2d6), default 1d20" }
func (c *RollCmd) Category() string    { return "General" }
func (c *RollCmd) Aliases() []string   { return []string{"dice", "d"} }
func (c *RollCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *RollCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "notation",
			Description: "Dice notation, e.g. 2d6 (default 1d20)",
			Required:    false,
		},
	}
}

func (c *RollCmd) FastPath() bool { return true }

func (c *RollCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	raw := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "notation" {
			raw = o.StringValue()
		}
	}
	return renderRoll(b, raw), nil
}

func (c *RollCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	return renderRoll(b, strings.Join(args, "")), nil
}

func renderRoll(b commands.BotInterface, raw string) *components.Container {
	count, sides := 1, 20
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw != "" {
		parts := strings.SplitN(strings.TrimPrefix(raw, "d"), "d", 2)
		n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || n < 1 {
			return usageRoll(b)
		}
		count = n
		sides = 20
		if len(parts) == 2 {
			m, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || m < 1 {
				return usageRoll(b)
			}
			sides = m
		}
	}
	if count > 25 || sides > 1000 {
		return b.Container(components.TextDisplay{Content: "Limits: 25 dice, 1000 sides."})
	}

	total := 0
	rolls := make([]string, 0, count)
	for k := 0; k < count; k++ {
		v := 1 + rand.Intn(sides)
		total += v
		rolls = append(rolls, strconv.Itoa(v))
	}

	detail := fmt.Sprintf("Rolls: %s", strings.Join(rolls, " + "))
	if count == 1 {
		detail = fmt.Sprintf("Rolled: %d", total)
	}
	return b.Container(
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("%dd%d", count, sides)},
				components.TextDisplay{Content: detail},
				components.TextDisplay{Content: fmt.Sprintf("Total: **%d**", total)},
			},
		},
	)
}

func usageRoll(b commands.BotInterface) *components.Container {
	return b.Container(components.TextDisplay{Content: "Usage: .roll 2d6 (default 1d20)"})
}


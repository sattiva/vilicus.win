package general

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type PingCmd struct{}

func (c *PingCmd) Name() string {
	return "ping"
}

func (c *PingCmd) Description() string {
	return "Check bot latency and gateway status"
}

func (c *PingCmd) Category() string {
	return "General"
}

func (c *PingCmd) Aliases() []string {
	return []string{"latency", "p"}
}

func (c *PingCmd) Options() []*discordgo.ApplicationCommandOption {
	return nil
}

func (c *PingCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *PingCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	hb := s.HeartbeatLatency()
	return b.Container(
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: "Status: Online"},
				components.TextDisplay{Content: fmt.Sprintf("Heartbeat: %d ms", hb.Milliseconds())},
			},
		},
	), nil
}

func (c *PingCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	hb := s.HeartbeatLatency()
	return b.Container(
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: "Status: Online"},
				components.TextDisplay{Content: fmt.Sprintf("Heartbeat: %d ms", hb.Milliseconds())},
			},
		},
	), nil
}


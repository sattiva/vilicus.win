package general

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type AboutCmd struct{}

func (c *AboutCmd) Name() string        { return "about" }
func (c *AboutCmd) Description() string { return "Bot version, uptime, and runtime info" }
func (c *AboutCmd) Category() string    { return "General" }
func (c *AboutCmd) Aliases() []string   { return []string{"info"} }
func (c *AboutCmd) Options() []*discordgo.ApplicationCommandOption {
	return nil
}
func (c *AboutCmd) RequiredPermissions() *int64 { return nil }

func (c *AboutCmd) FastPath() bool { return true }

func (c *AboutCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	return c.render(b), nil
}

func (c *AboutCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	return c.render(b), nil
}

func (c *AboutCmd) render(b commands.BotInterface) *components.Container {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return b.Container(
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: "Vilicus Framework"},
				components.TextDisplay{Content: fmt.Sprintf("Uptime: %s", time.Since(b.GetStartTime()).Truncate(time.Second).String())},
				components.TextDisplay{Content: fmt.Sprintf("Commands loaded: %d", len(b.GetCommands()))},
				components.TextDisplay{Content: fmt.Sprintf("Memory: %.1f MB alloc / %.1f MB sys",
					float64(m.Alloc)/1024/1024, float64(m.Sys)/1024/1024)},
			},
		},
	)
}


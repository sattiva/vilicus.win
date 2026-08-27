package general

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type AvatarCmd struct{}

func (c *AvatarCmd) Name() string {
	return "avatar"
}

func (c *AvatarCmd) Description() string {
	return "Display user profile picture asset URL"
}

func (c *AvatarCmd) Category() string {
	return "General"
}

func (c *AvatarCmd) Aliases() []string {
	return []string{"av", "pfp"}
}

func (c *AvatarCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *AvatarCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "Target user",
			Required:    false,
		},
	}
}

func (c *AvatarCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	targetUser := i.Member.User
	if i.User != nil {
		targetUser = i.User
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) > 0 && opts[0].UserValue(s) != nil {
		targetUser = opts[0].UserValue(s)
	}

	return c.renderAvatar(b, s, targetUser.ID)
}

func (c *AvatarCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	targetID := m.Author.ID
	if len(args) > 0 {
		raw := args[0]
		raw = strings.TrimPrefix(raw, "<@")
		raw = strings.TrimPrefix(raw, "!")
		raw = strings.TrimSuffix(raw, ">")
		if raw != "" {
			targetID = raw
		}
	}
	return c.renderAvatar(b, s, targetID)
}

func (c *AvatarCmd) renderAvatar(b commands.BotInterface, s *discordgo.Session, uid string) (*components.Container, error) {
	u, err := s.User(uid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("User %s not found.", uid)}), nil
	}

	avURL := u.AvatarURL("1024")
	if avURL == "" {
		avURL = "https://cdn.discordapp.com/embed/avatars/0.png"
	}

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Avatar: %s", u.Username)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Direct URL: %s", avURL)},
			},
		},
	), nil
}


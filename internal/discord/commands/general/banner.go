package general

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type BannerCmd struct{}

func (c *BannerCmd) Name() string {
	return "banner"
}

func (c *BannerCmd) Description() string {
	return "Display user profile banner asset URL"
}

func (c *BannerCmd) Category() string {
	return "General"
}

func (c *BannerCmd) Aliases() []string {
	return []string{"ubanner", "userbanner"}
}

func (c *BannerCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *BannerCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "Target user",
			Required:    false,
		},
	}
}

func (c *BannerCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	targetUser := i.Member.User
	if i.User != nil {
		targetUser = i.User
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) > 0 && opts[0].UserValue(s) != nil {
		targetUser = opts[0].UserValue(s)
	}

	return c.renderBanner(b, s, targetUser.ID)
}

func (c *BannerCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
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
	return c.renderBanner(b, s, targetID)
}

func (c *BannerCmd) renderBanner(b commands.BotInterface, s *discordgo.Session, uid string) (*components.Container, error) {
	u, err := s.User(uid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("User %s not found.", uid)}), nil
	}

	if u.Banner == "" {
		return b.Container(
			components.TextDisplay{Content: fmt.Sprintf("%s has no custom profile banner configured.", u.Username)},
		), nil
	}

	bannerURL := u.BannerURL("2048")
	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Banner: %s", u.Username)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Direct URL: %s", bannerURL)},
			},
		},
	), nil
}


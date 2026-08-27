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

type UserInfoCmd struct{}

func (c *UserInfoCmd) Name() string {
	return "userinfo"
}

func (c *UserInfoCmd) Description() string {
	return "Inspect Discord user account and server member details"
}

func (c *UserInfoCmd) Category() string {
	return "General"
}

func (c *UserInfoCmd) Aliases() []string {
	return []string{"whois", "ui", "user"}
}

func (c *UserInfoCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *UserInfoCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "Target user",
			Required:    false,
		},
	}
}

func (c *UserInfoCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	targetUser := i.Member.User
	if i.User != nil {
		targetUser = i.User
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) > 0 && opts[0].UserValue(s) != nil {
		targetUser = opts[0].UserValue(s)
	}

	return c.renderUserInfo(b, s, i.GuildID, targetUser.ID)
}

func (c *UserInfoCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
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
	return c.renderUserInfo(b, s, m.GuildID, targetID)
}

func (c *UserInfoCmd) renderUserInfo(b commands.BotInterface, s *discordgo.Session, gid, uid string) (*components.Container, error) {
	u, err := s.User(uid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("User %s not found.", uid)}), nil
	}

	roleList := "None"
	joinedAt := "N/A"
	if gid != "" {
		if mem, err := s.GuildMember(gid, uid); err == nil && mem != nil {
			if len(mem.Roles) > 0 {
				var roleTags []string
				for _, r := range mem.Roles {
					roleTags = append(roleTags, fmt.Sprintf("<@&%s>", r))
				}
				roleList = strings.Join(roleTags, ", ")
			}
			if mem.JoinedAt.Year() > 2000 {
				joinedAt = mem.JoinedAt.Format(time.RFC3339)
			}
		}
	}

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("User Information: %s", u.Username)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("ID: `%s`", u.ID)},
				components.TextDisplay{Content: fmt.Sprintf("Username: %s", u.Username)},
				components.TextDisplay{Content: fmt.Sprintf("Bot Account: %t", u.Bot)},
				components.TextDisplay{Content: fmt.Sprintf("Server Joined: %s", joinedAt)},
				components.TextDisplay{Content: fmt.Sprintf("Assigned Roles: %s", roleList)},
			},
		},
	), nil
}


package general

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)

type PrefixCmd struct{}

func (c *PrefixCmd) Name() string {
	return "prefix"
}

func (c *PrefixCmd) Description() string {
	return "View or configure server and personal user prefixes"
}

func (c *PrefixCmd) Category() string {
	return "General"
}

func (c *PrefixCmd) Aliases() []string {
	return []string{"setprefix", "pfx"}
}

func (c *PrefixCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *PrefixCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "get",
			Description: "View active server and personal prefixes",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "set",
			Description: "Set server-wide prefix (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "prefix",
					Description: "New server prefix (1-5 characters)",
					Required:    true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "self",
			Description: "Set your personal custom prefix",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "prefix",
					Description: "Personal prefix (1-5 characters)",
					Required:    true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "reset",
			Description: "Reset your personal custom prefix",
		},
	}
}

func (c *PrefixCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	gid := i.GuildID
	uid := ""
	if i.Member != nil && i.Member.User != nil {
		uid = i.Member.User.ID
	} else if i.User != nil {
		uid = i.User.ID
	}

	opts := i.ApplicationCommandData().Options
	sub := "get"
	val := ""
	if len(opts) > 0 {
		sub = opts[0].Name
		if len(opts[0].Options) > 0 {
			val = opts[0].Options[0].StringValue()
		}
	}

	return c.handleAction(ctx, b, s, gid, uid, i.Member, sub, val)
}

func (c *PrefixCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if len(args) == 0 {
		return c.handleAction(ctx, b, s, m.GuildID, m.Author.ID, m.Member, "get", "")
	}

	sub := strings.ToLower(args[0])
	val := ""
	if len(args) > 1 {
		val = args[1]
	}

	switch sub {
	case "set":
		return c.handleAction(ctx, b, s, m.GuildID, m.Author.ID, m.Member, "set", val)
	case "self":
		return c.handleAction(ctx, b, s, m.GuildID, m.Author.ID, m.Member, "self", val)
	case "reset":
		return c.handleAction(ctx, b, s, m.GuildID, m.Author.ID, m.Member, "reset", "")
	default:
		return c.handleAction(ctx, b, s, m.GuildID, m.Author.ID, m.Member, "get", "")
	}
}

func (c *PrefixCmd) handleAction(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, uid string, member *discordgo.Member, action, val string) (*components.Container, error) {
	st := b.GetStore()

	serverPrefix := "."
	if gid != "" {
		if gcfg, err := st.GetGuildConfig(ctx, gid); err == nil && gcfg.Prefix != "" {
			serverPrefix = gcfg.Prefix
		}
	}

	userPrefix := "None"
	if ucfg, err := st.GetUserConfig(ctx, uid); err == nil && ucfg.Prefix != "" {
		userPrefix = ucfg.Prefix
	}

	switch action {
	case "get":
		return b.Container(
			components.TextDisplay{Content: "Prefix Configuration"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: fmt.Sprintf("Server Prefix: `%s`", serverPrefix)},
					components.TextDisplay{Content: fmt.Sprintf("Personal Self-Prefix: `%s`", userPrefix)},
					components.TextDisplay{Content: "Usage:\n- Set server prefix (Admin): `.prefix set <prefix>`\n- Set personal prefix: `.prefix self <prefix>`\n- Reset personal prefix: `.prefix reset`"},
				},
			},
		), nil

	case "set":
		if gid == "" {
			return b.Container(components.TextDisplay{Content: "Server prefix can only be changed inside a server."}), nil
		}
		if member != nil && member.Permissions&discordgo.PermissionAdministrator == 0 {
			return b.Container(components.TextDisplay{Content: "Administrator permissions required to update server prefix."}), nil
		}
		val = strings.TrimSpace(val)
		if len(val) == 0 || len(val) > 5 {
			return b.Container(components.TextDisplay{Content: "Prefix must be between 1 and 5 characters."}), nil
		}

		gcfg, _ := st.GetGuildConfig(ctx, gid)
		gcfg.Prefix = val
		if err := st.SaveGuildConfig(ctx, gcfg); err != nil {
			return nil, err
		}

		return b.Container(
			components.TextDisplay{Content: "Server Prefix Updated"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: fmt.Sprintf("New Server Prefix: `%s`", val)},
					components.TextDisplay{Content: fmt.Sprintf("All server members can now execute commands with `%shelp`", val)},
				},
			},
		), nil

	case "self":
		val = strings.TrimSpace(val)
		if len(val) == 0 || len(val) > 5 {
			return b.Container(components.TextDisplay{Content: "Personal prefix must be between 1 and 5 characters."}), nil
		}

		ucfg := &store.UserConfig{UserID: uid, Prefix: val}
		if err := st.SaveUserConfig(ctx, ucfg); err != nil {
			return nil, err
		}

		return b.Container(
			components.TextDisplay{Content: "Personal Prefix Set"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: fmt.Sprintf("Your Personal Prefix: `%s`", val)},
					components.TextDisplay{Content: fmt.Sprintf("You can now trigger commands in any server using `%shelp`", val)},
				},
			},
		), nil

	case "reset":
		_ = st.DeleteUserConfig(ctx, uid)
		return b.Container(
			components.TextDisplay{Content: "Personal Prefix Reset"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: "Your personal prefix has been cleared. Defaulting to server prefix."},
				},
			},
		), nil
	}

	return b.Container(components.TextDisplay{Content: "Unknown prefix action."}), nil
}


package config

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)

type ConfigCmd struct{}

func (c *ConfigCmd) Name() string {
	return "config"
}

func (c *ConfigCmd) Description() string {
	return "Inspect or update server configuration channels and roles"
}

func (c *ConfigCmd) Category() string {
	return "Configuration"
}

func (c *ConfigCmd) Aliases() []string {
	return []string{"cfg", "settings"}
}

func (c *ConfigCmd) RequiredPermissions() *int64 {
	adminPerms := int64(discordgo.PermissionAdministrator)
	return &adminPerms
}

func (c *ConfigCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "get",
			Description: "View current guild settings",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "set",
			Description: "Update guild settings",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "prefix",
					Description: "Command prefix",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "log_channel",
					Description: "Audit logging channel",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "welcome_channel",
					Description: "Welcome greeting channel",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "auto_role",
					Description: "Automatic member role",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "jail_role",
					Description: "Holding role applied by jail/unjail",
					Required:    false,
				},
			},
		},
	}
}

func (c *ConfigCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	gid := i.GuildID
	if gid == "" {
		return b.Container(components.TextDisplay{Content: "Configuration is only available inside a guild."}), nil
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return b.Container(components.TextDisplay{Content: "Subcommand missing."}), nil
	}

	sub := opts[0].Name
	cfg, err := b.GetStore().GetGuildConfig(ctx, gid)
	if err != nil {
		return nil, err
	}

	if sub == "get" {
		return c.renderConfig(b, gid, cfg)
	}

	if sub == "set" {
		for _, o := range opts[0].Options {
			switch o.Name {
			case "prefix":
				cfg.Prefix = o.StringValue()
			case "log_channel":
				cfg.LogChannelID = o.ChannelValue(s).ID
			case "welcome_channel":
				cfg.WelcomeChannelID = o.ChannelValue(s).ID
			case "auto_role":
				cfg.AutoRoleID = o.RoleValue(s, gid).ID
			case "jail_role":
				cfg.JailRoleID = o.RoleValue(s, gid).ID
			}
		}

		if err := b.GetStore().SaveGuildConfig(ctx, cfg); err != nil {
			return nil, err
		}

		return c.renderConfig(b, gid, cfg)
	}

	return b.Container(components.TextDisplay{Content: "Unknown subcommand."}), nil
}

func (c *ConfigCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Configuration is only available inside a guild."}), nil
	}

	cfg, err := b.GetStore().GetGuildConfig(ctx, m.GuildID)
	if err != nil {
		return nil, err
	}

	return c.renderConfig(b, m.GuildID, cfg)
}

func (c *ConfigCmd) renderConfig(b commands.BotInterface, gid string, cfg *store.GuildConfig) (*components.Container, error) {
	logCh := cfg.LogChannelID
	if logCh == "" {
		logCh = "None"
	}
	welCh := cfg.WelcomeChannelID
	if welCh == "" {
		welCh = "None"
	}
	role := cfg.AutoRoleID
	if role == "" {
		role = "None"
	}
	jail := cfg.JailRoleID
	if jail == "" {
		jail = "None"
	}

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Guild Settings [%s]", gid)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Prefix: `%s`", cfg.Prefix)},
				components.TextDisplay{Content: fmt.Sprintf("Log Channel: %s", logCh)},
				components.TextDisplay{Content: fmt.Sprintf("Welcome Channel: %s", welCh)},
				components.TextDisplay{Content: fmt.Sprintf("Auto Role: %s", role)},
				components.TextDisplay{Content: fmt.Sprintf("Jail Role: %s", jail)},
				components.TextDisplay{Content: fmt.Sprintf("Last Modified: %s", cfg.UpdatedAt.Format(time.RFC3339))},
			},
		},
	), nil
}


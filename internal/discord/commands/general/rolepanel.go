package general

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

var errPanelUnsupported = errors.New("role panels are not available on this bot instance")

const usageRolePanel = "Usage: .rolepanel create <#channel> <title> <@role...> | .rolepanel delete <#channel> <message_id>"

type RolePanelCmd struct{}

func (c *RolePanelCmd) Name() string        { return "rolepanel" }
func (c *RolePanelCmd) Description() string { return "Create or remove self-serve role button panels" }
func (c *RolePanelCmd) Category() string    { return "General" }
func (c *RolePanelCmd) Aliases() []string   { return []string{"rpanel"} }

func (c *RolePanelCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageRoles)
	return &perms
}

func (c *RolePanelCmd) Options() []*discordgo.ApplicationCommandOption {
	opts := []*discordgo.ApplicationCommandOption{
		{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel the panel is posted in", Required: true,
			ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText}},
		{Type: discordgo.ApplicationCommandOptionString, Name: "title", Description: "Panel title (max 200 chars)", Required: true},
	}
	for n := 1; n <= 10; n++ {
		opts = append(opts, &discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionRole,
			Name:        "role" + strconv.Itoa(n),
			Description: "Role on the panel",
			Required:    n == 1,
		})
	}
	return []*discordgo.ApplicationCommandOption{
		{Name: "create", Description: "Post a new role panel", Type: discordgo.ApplicationCommandOptionSubCommand, Options: opts},
		{Name: "delete", Description: "Remove a panel and its buttons", Type: discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "message_id", Description: "ID of the panel message", Required: true},
				{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel the panel lives in", Required: true,
					ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText}},
			}},
	}
}

func (c *RolePanelCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	starter, ok := b.(commands.PanelStarter)
	if !ok {
		return nil, errPanelUnsupported
	}
	if i.Member == nil || i.Member.User == nil {
		return b.Container(components.TextDisplay{Content: "Could not resolve the invoking member."}), nil
	}
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return b.Container(components.TextDisplay{Content: usageRolePanel}), nil
	}

	switch opts[0].Name {
	case "create":
		channelID := ""
		title := "Pick your roles"
		var roles []string
		for _, o := range opts[0].Options {
			switch {
			case o.Name == "channel":
				if ch := o.ChannelValue(s); ch != nil {
					channelID = ch.ID
				}
			case o.Name == "title":
				title = o.StringValue()
			case strings.HasPrefix(o.Name, "role"):
				if r := o.RoleValue(s, i.GuildID); r != nil && r.ID != "" {
					roles = append(roles, r.ID)
				}
			}
		}
		if channelID == "" || len(roles) == 0 {
			return b.Container(components.TextDisplay{Content: usageRolePanel}), nil
		}
		return starter.PostRolePanel(ctx, s, i.GuildID, channelID, truncateTitle(title), i.Member.User.ID, roles)

	default:
		msgID, channelID := "", ""
		for _, o := range opts[0].Options {
			switch o.Name {
			case "message_id":
				msgID = o.StringValue()
			case "channel":
				if ch := o.ChannelValue(s); ch != nil {
					channelID = ch.ID
				}
			}
		}
		if msgID == "" || channelID == "" {
			return b.Container(components.TextDisplay{Content: usageRolePanel}), nil
		}
		n, err := starter.DeleteRolePanel(s, i.GuildID, channelID, msgID)
		if err != nil {
			return b.Container(components.TextDisplay{Content: "Failed removing panel: " + err.Error()}), nil
		}
		if n == 0 {
			return b.Container(components.TextDisplay{Content: "No panel found with that message id."}), nil
		}
		return b.Container(components.TextDisplay{
			Content: fmt.Sprintf("Role panel removed (%d bindings cleared).", n),
		}), nil
	}
}

func (c *RolePanelCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	starter, ok := b.(commands.PanelStarter)
	if !ok {
		return nil, errPanelUnsupported
	}
	if len(args) == 0 {
		return b.Container(components.TextDisplay{Content: usageRolePanel}), nil
	}

	switch args[0] {
	case "create":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .rolepanel create <#channel> <title> <@role> [@role...]"}), nil
		}
		channelID := commands.ParseIDArg(args[1])
		if channelID == "" {
			return b.Container(components.TextDisplay{Content: "Could not resolve a channel from that argument."}), nil
		}
		var roles []string
		titleWords := make([]string, 0, len(args))
		for _, a := range args[2:] {
			if rid := parseRoleArg(s, m.GuildID, a); rid != "" {
				roles = append(roles, rid)
			} else if len(roles) == 0 {
				titleWords = append(titleWords, a)
			}
		}
		if len(roles) == 0 {
			return b.Container(components.TextDisplay{Content: "No roles resolved. Mention them like @Role."}), nil
		}
		title := truncateTitle(strings.Join(titleWords, " "))
		return starter.PostRolePanel(ctx, s, m.GuildID, channelID, title, m.Author.ID, roles)

	case "delete":
		if len(args) < 3 {
			return b.Container(components.TextDisplay{Content: "Usage: .rolepanel delete <#channel> <message_id>"}), nil
		}
		channelID := commands.ParseIDArg(args[1])
		msgID := commands.ParseIDArg(args[2])
		if channelID == "" || msgID == "" {
			return b.Container(components.TextDisplay{Content: "Could not resolve the channel or message id."}), nil
		}
		n, err := starter.DeleteRolePanel(s, m.GuildID, channelID, msgID)
		if err != nil {
			return b.Container(components.TextDisplay{Content: "Failed removing panel: " + err.Error()}), nil
		}
		if n == 0 {
			return b.Container(components.TextDisplay{Content: "No panel found with that message id."}), nil
		}
		return b.Container(components.TextDisplay{Content: "Role panel removed."}), nil

	default:
		return b.Container(components.TextDisplay{Content: usageRolePanel}), nil
	}
}

func truncateTitle(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		t = "Pick your roles"
	}
	if len(t) > 200 {
		t = t[:200]
	}
	return t
}

func parseRoleArg(s *discordgo.Session, gid, arg string) string {
	if id := commands.ParseIDArg(arg); id != "" {
		return id
	}
	g, err := s.State.Guild(gid)
	if err != nil || g == nil {
		return ""
	}
	lower := strings.ToLower(strings.TrimPrefix(arg, "@"))
	for _, r := range g.Roles {
		if strings.ToLower(r.Name) == lower {
			return r.ID
		}
	}
	return ""
}


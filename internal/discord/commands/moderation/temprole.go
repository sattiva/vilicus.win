package moderation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type TempRoleCmd struct{}

func (c *TempRoleCmd) Name() string        { return "temprole" }
func (c *TempRoleCmd) Description() string { return "Grant a role that auto-expires (e.g. 1h30m)" }
func (c *TempRoleCmd) Category() string    { return "Moderation" }
func (c *TempRoleCmd) Aliases() []string   { return []string{"trole", "tr"} }

func (c *TempRoleCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageRoles)
	return &perms
}

func (c *TempRoleCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "User to receive the role",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionRole,
			Name:        "role",
			Description: "Role to grant temporarily",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "duration",
			Description: "How long the role lasts, e.g. 1h30m (max 365d)",
			Required:    true,
		},
	}
}

func (c *TempRoleCmd) CooldownClass() string { return "danger" }

func (c *TempRoleCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	var targetID, roleID, durRaw string
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "target":
			if u := o.UserValue(s); u != nil {
				targetID = u.ID
			}
		case "role":
			if r := o.RoleValue(s, i.GuildID); r != nil {
				roleID = r.ID
			}
		case "duration":
			durRaw = o.StringValue()
		}
	}
	if targetID == "" || roleID == "" {
		return b.Container(components.TextDisplay{Content: "Target user and role are required."}), nil
	}
	d := commands.ParseDurationArg(durRaw)
	if d <= 0 {
		return b.Container(components.TextDisplay{Content: "Invalid duration. Use forms like 30m, 2h, 1h30m, 3d (max 365d)."}), nil
	}
	return c.handleTempRole(ctx, b, s, i.GuildID, i.Member, targetID, roleID, d)
}

func (c *TempRoleCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 3 {
		return b.Container(components.TextDisplay{Content: "Usage: .temprole <@user> <@role|role_id|name> <duration>"}), nil
	}
	targetID := commands.ParseMentionID(args[0])
	if targetID == "" {
		return b.Container(components.TextDisplay{Content: "Could not resolve a user from the first argument."}), nil
	}
	guild, err := s.Guild(m.GuildID)
	if err != nil || guild == nil {
		return b.Container(components.TextDisplay{Content: "Failed to load guild."}), nil
	}
	role := resolveRoleArg(guild, args[1])
	if role == nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("No role matches `%s`.", args[1])}), nil
	}
	d := commands.ParseDurationArg(args[2])
	if d <= 0 {
		return b.Container(components.TextDisplay{Content: "Invalid duration. Use forms like 30m, 2h, 1h30m, 3d (max 365d)."}), nil
	}
	return c.handleTempRole(ctx, b, s, m.GuildID, m.Member, targetID, role.ID, d)
}

func (c *TempRoleCmd) handleTempRole(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID, roleID string, d time.Duration) (*components.Container, error) {
	guild, err := s.Guild(gid)
	if err != nil || guild == nil {
		return b.Container(components.TextDisplay{Content: "Failed to load guild."}), nil
	}
	var role *discordgo.Role
	for _, r := range guild.Roles {
		if r.ID == roleID {
			role = r
			break
		}
	}
	if role == nil {
		return b.Container(components.TextDisplay{Content: "Role not found in this guild."}), nil
	}

	botMember, err := s.GuildMember(gid, s.State.User.ID)
	if err != nil {
		return nil, err
	}

	if ok, r := commands.CanManageRole(guild, caller, botMember, role); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", r)}), nil
	}

	expiresAt := time.Now().UTC().Add(d)
	rowID, err := b.GetStore().AddTempRole(ctx, gid, targetID, roleID, expiresAt, caller.User.ID)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed scheduling removal: %s", err.Error())}), nil
	}

	if err := s.GuildMemberRoleAdd(gid, targetID, roleID); err != nil {
		_ = b.GetStore().MarkTempRoleRemoved(ctx, rowID)
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed granting role: %s", err.Error())}), nil
	}

	reason := fmt.Sprintf("temporary role for %s", commands.FormatDuration(d))
	commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "TempRole",
		fmt.Sprintf("role %s (%s)", role.Name, reason), fmt.Sprintf("expires <t:%d:R>", expiresAt.Unix()))

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Temporary Role Granted (row #%d)", rowID)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (`%s`)", targetID, targetID)},
				components.TextDisplay{Content: fmt.Sprintf("Role: <@&%s> (`%s`)", roleID, role.Name)},
				components.TextDisplay{Content: fmt.Sprintf("Duration: %s", commands.FormatDuration(d))},
				components.TextDisplay{Content: fmt.Sprintf("Removal scheduled: <t:%d:R>", expiresAt.Unix())},
			},
		},
	), nil
}

func resolveRoleArg(g *discordgo.Guild, arg string) *discordgo.Role {
	arg = strings.TrimSpace(arg)
	id := strings.TrimPrefix(strings.TrimPrefix(arg, "<@&"), ">")
	for _, r := range g.Roles {
		if r.ID == id {
			return r
		}
	}
	lower := strings.ToLower(arg)
	for _, r := range g.Roles {
		if strings.ToLower(r.Name) == lower {
			return r
		}
	}
	return nil
}


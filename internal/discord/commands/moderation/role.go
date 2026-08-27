package moderation

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type RoleCmd struct{}

func (c *RoleCmd) Name() string {
	return "role"
}

func (c *RoleCmd) Description() string {
	return "Assign or remove a role with hierarchy validation and audit logging"
}

func (c *RoleCmd) Category() string {
	return "Moderation"
}

func (c *RoleCmd) Aliases() []string {
	return []string{"r"}
}

func (c *RoleCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageRoles)
	return &perms
}

func (c *RoleCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "add",
			Description: "Assign a role to target member",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "target",
					Description: "Target member",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to grant",
					Required:    true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "remove",
			Description: "Remove a role from target member",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "target",
					Description: "Target member",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to revoke",
					Required:    true,
				},
			},
		},
	}
}

func (c *RoleCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return b.Container(components.TextDisplay{Content: "Subcommand missing."}), nil
	}

	sub := opts[0].Name
	var targetUser *discordgo.User
	var targetRole *discordgo.Role

	for _, o := range opts[0].Options {
		if o.Name == "target" {
			targetUser = o.UserValue(s)
		} else if o.Name == "role" {
			targetRole = o.RoleValue(s, i.GuildID)
		}
	}

	if targetUser == nil || targetRole == nil {
		return b.Container(components.TextDisplay{Content: "Target user and role required."}), nil
	}

	return c.handleRole(ctx, b, s, i.GuildID, i.Member, targetUser.ID, targetRole.ID, sub)
}

func (c *RoleCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	if len(args) < 3 {
		return b.Container(components.TextDisplay{Content: "Usage: `.role <add|remove> <@user> <@role|role_id>`"}), nil
	}

	sub := strings.ToLower(args[0])
	targetID := parseMentionID(args[1])
	roleID := parseMentionID(args[2])

	return c.handleRole(ctx, b, s, m.GuildID, m.Member, targetID, roleID, sub)
}

func (c *RoleCmd) handleRole(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID, roleID, action string) (*components.Container, error) {
	guild, err := s.Guild(gid)
	if err != nil {
		return nil, err
	}

	botMember, err := s.GuildMember(gid, s.State.User.ID)
	if err != nil {
		return nil, err
	}

	targetMember, err := s.GuildMember(gid, targetID)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Target member %s not found in guild.", targetID)}), nil
	}

	var role *discordgo.Role
	for _, r := range guild.Roles {
		if r.ID == roleID {
			role = r
			break
		}
	}
	if role == nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Role %s not found in guild.", roleID)}), nil
	}

	if ok, reason := commands.CanManageRole(guild, caller, botMember, role); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", reason)}), nil
	}

	if ok, reason := commands.CanModerate(guild, caller, targetMember); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", reason)}), nil
	}

	if action == "add" {
		err = s.GuildMemberRoleAdd(gid, targetID, roleID)
		if err != nil {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed granting role: %s", err.Error())}), nil
		}

		commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "Role Add", "Manual role assignment", fmt.Sprintf("Role: %s (%s)", role.Name, role.ID))

		return b.Container(
			components.TextDisplay{Content: "Role Granted"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: fmt.Sprintf("Member: <@%s>", targetID)},
					components.TextDisplay{Content: fmt.Sprintf("Role: <@&%s>", roleID)},
					components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
				},
			},
		), nil
	}

	if action == "remove" {
		err = s.GuildMemberRoleRemove(gid, targetID, roleID)
		if err != nil {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed revoking role: %s", err.Error())}), nil
		}

		commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "Role Remove", "Manual role revocation", fmt.Sprintf("Role: %s (%s)", role.Name, role.ID))

		return b.Container(
			components.TextDisplay{Content: "Role Revoked"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: fmt.Sprintf("Member: <@%s>", targetID)},
					components.TextDisplay{Content: fmt.Sprintf("Role: <@&%s>", roleID)},
					components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
				},
			},
		), nil
	}

	return b.Container(components.TextDisplay{Content: "Unknown action. Use 'add' or 'remove'."}), nil
}

func parseMentionID(raw string) string {
	raw = strings.TrimPrefix(raw, "<@&")
	raw = strings.TrimPrefix(raw, "<@")
	raw = strings.TrimPrefix(raw, "!")
	raw = strings.TrimSuffix(raw, ">")
	return strings.TrimSpace(raw)
}


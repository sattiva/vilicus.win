package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)


type JailCmd struct{}

func (c *JailCmd) Name() string {
	return "jail"
}

func (c *JailCmd) Description() string {
	return "Move a member to the configured holding role, backing up their roles"
}

func (c *JailCmd) Category() string {
	return "Moderation"
}

func (c *JailCmd) Aliases() []string {
	return []string{"lockup"}
}

func (c *JailCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageRoles)
	return &perms
}

func (c *JailCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "Target member to jail",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Reason for jailing",
			Required:    false,
		},
	}
}

func (c *JailCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	var targetID, reason string
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "target":
			targetID = o.UserValue(s).ID
		case "reason":
			reason = o.StringValue()
		}
	}
	if targetID == "" {
		return b.Container(components.TextDisplay{Content: "Target user required."}), nil
	}

	return handleJail(ctx, b, s, i.GuildID, i.Member, targetID, reason)
}

func (c *JailCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 1 {
		return b.Container(components.TextDisplay{Content: "Usage: `.jail <@user|user_id> [reason]`"}), nil
	}

	targetID := parseMentionID(args[0])
	reason := "No reason provided"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	return handleJail(ctx, b, s, m.GuildID, m.Member, targetID, reason)
}

func handleJail(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID, reason string) (*components.Container, error) {
	gcfg, err := b.GetStore().GetGuildConfig(ctx, gid)
	if err != nil {
		return nil, err
	}
	if gcfg.JailRoleID == "" {
		return b.Container(components.TextDisplay{Content: "No jail role configured. Set one with `/config set jail_role` first."}), nil
	}

	guild, err := s.Guild(gid)
	if err != nil {
		return nil, err
	}
	var jailRole *discordgo.Role
	for _, r := range guild.Roles {
		if r.ID == gcfg.JailRoleID {
			jailRole = r
			break
		}
	}
	if jailRole == nil {
		return b.Container(components.TextDisplay{Content: "Configured jail role no longer exists. Set a new one with `/config set jail_role`."}), nil
	}

	botMember, err := s.GuildMember(gid, s.State.User.ID)
	if err != nil {
		return nil, err
	}
	targetMember, err := s.GuildMember(gid, targetID)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Target member %s not found in guild.", targetID)}), nil
	}

	if ok, rReason := commands.CanModerate(guild, caller, targetMember); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", rReason)}), nil
	}
	if ok, rReason := commands.CanBotModerate(guild, botMember, targetMember); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", rReason)}), nil
	}
	if ok, rReason := commands.CanManageRole(guild, caller, botMember, jailRole); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Cannot apply jail role: %s", rReason)}), nil
	}

	if _, err := b.GetStore().GetJailBackup(ctx, gid, targetID); err == nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("<@%s> is already jailed.", targetID)}), nil
	} else if !errors.Is(err, store.ErrJailBackupNotFound) {
		return nil, err
	}

	var stripped []string
	for _, rid := range targetMember.Roles {
		if rid == guild.ID || rid == jailRole.ID {
			continue
		}
		stripped = append(stripped, rid)
	}
	if len(stripped) > 0 {
		roleMap := make(map[string]*discordgo.Role, len(guild.Roles))
		for _, r := range guild.Roles {
			roleMap[r.ID] = r
		}
		kept := stripped[:0]
		for _, rid := range stripped {
			if r, ok := roleMap[rid]; ok && !r.Managed {
				kept = append(kept, rid)
			}
		}
		stripped = kept
	}
	if err := b.GetStore().SaveJailBackup(ctx, gid, targetID, caller.User.ID, reason, stripped); err != nil {
		return nil, err
	}

	if len(stripped) > 0 {
		var removed []string
		for _, rid := range stripped {
			if err := s.GuildMemberRoleRemove(gid, targetID, rid); err != nil {
				for _, done := range removed {
					_ = s.GuildMemberRoleAdd(gid, targetID, done)
				}
				_ = b.GetStore().DeleteJailBackup(ctx, gid, targetID)
				return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed stripping roles: %s", err.Error())}), nil
			}
			removed = append(removed, rid)
		}
	}
	if err := s.GuildMemberRoleAdd(gid, targetID, jailRole.ID); err != nil {
		if len(stripped) > 0 {
			for _, rid := range stripped {
				_ = s.GuildMemberRoleAdd(gid, targetID, rid)
			}
			_ = b.GetStore().DeleteJailBackup(ctx, gid, targetID)
		}
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed applying jail role: %s", err.Error())}), nil
	}

	caseLabel := recordCase(ctx, b, gid, "jail", caller.User.ID, targetID, reason, 0, nil)
	extra := fmt.Sprintf("Roles held: %d", len(stripped))
	if caseLabel != "" {
		extra += ", " + strings.ToLower(caseLabel)
	}
	commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "Member Jailed", reason, extra)

	title := "Member Jailed"
	if caseLabel != "" {
		title = fmt.Sprintf("Member Jailed (%s)", caseLabel)
	}
	return b.Container(
		components.TextDisplay{Content: title},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (`%s`)", targetID, targetID)},
				components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
				components.TextDisplay{Content: fmt.Sprintf("Roles Held: %d", len(stripped))},
				components.TextDisplay{Content: fmt.Sprintf("Reason: %s", reason)},
			},
		},
	), nil
}

type UnjailCmd struct{}

func (c *UnjailCmd) Name() string {
	return "unjail"
}

func (c *UnjailCmd) Description() string {
	return "Release a jailed member and restore their previous roles"
}

func (c *UnjailCmd) Category() string {
	return "Moderation"
}

func (c *UnjailCmd) Aliases() []string {
	return []string{"release"}
}

func (c *UnjailCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageRoles)
	return &perms
}

func (c *UnjailCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "Target member to release",
			Required:    true,
		},
	}
}

func (c *UnjailCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	targetID := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "target" {
			targetID = o.UserValue(s).ID
		}
	}
	if targetID == "" {
		return b.Container(components.TextDisplay{Content: "Target user required."}), nil
	}

	return handleUnjail(ctx, b, s, i.GuildID, i.Member, targetID)
}

func (c *UnjailCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 1 {
		return b.Container(components.TextDisplay{Content: "Usage: `.unjail <@user|user_id>`"}), nil
	}

	return handleUnjail(ctx, b, s, m.GuildID, m.Member, parseMentionID(args[0]))
}

func handleUnjail(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID string) (*components.Container, error) {
	bk, err := b.GetStore().GetJailBackup(ctx, gid, targetID)
	if err != nil {
		if errors.Is(err, store.ErrJailBackupNotFound) {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("<@%s> has no jail record.", targetID)}), nil
		}
		return nil, err
	}

	gcfg, err := b.GetStore().GetGuildConfig(ctx, gid)
	if err != nil {
		return nil, err
	}

	member, memberErr := s.GuildMember(gid, targetID)

	if memberErr != nil || member == nil {
		_ = b.GetStore().DeleteJailBackup(ctx, gid, targetID)
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("<@%s> left the guild; jail record cleared.", targetID)}), nil
	}

	if gcfg.JailRoleID != "" {
		if err := s.GuildMemberRoleRemove(gid, targetID, gcfg.JailRoleID); err != nil {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed removing jail role: %s", err.Error())}), nil
		}
	}

	var failed []string
	for _, rid := range bk.Roles {
		if err := s.GuildMemberRoleAdd(gid, targetID, rid); err != nil {
			failed = append(failed, rid)
		}
	}
	if len(failed) == 0 {
		_ = b.GetStore().DeleteJailBackup(ctx, gid, targetID)
	}

	caseLabel := recordCase(ctx, b, gid, "unjail", caller.User.ID, targetID, "Released from jail", 0, nil)
	extra := fmt.Sprintf("Roles restored: %d/%d", len(bk.Roles)-len(failed), len(bk.Roles))
	if caseLabel != "" {
		extra += ", " + strings.ToLower(caseLabel)
	}
	commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "Member Released", "Released from jail", extra)

	body := []discordgo.MessageComponent{
		components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (`%s`)", targetID, targetID)},
		components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
		components.TextDisplay{Content: fmt.Sprintf("Roles Restored: %d/%d", len(bk.Roles)-len(failed), len(bk.Roles))},
	}
	status := "Member Released"
	if caseLabel != "" {
		status = fmt.Sprintf("Member Released (%s)", caseLabel)
	}
	if len(failed) > 0 {
		status += " - Partial"
		body = append(body, components.TextDisplay{
			Content: fmt.Sprintf("Restore failed for %d role(s); the backup was kept so you can retry.", len(failed)),
		})
	}

	return b.Container(
		components.TextDisplay{Content: status},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: body},
	), nil
}


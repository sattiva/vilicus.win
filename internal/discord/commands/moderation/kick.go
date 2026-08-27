package moderation

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type KickCmd struct{}

func (c *KickCmd) Name() string {
	return "kick"
}

func (c *KickCmd) Description() string {
	return "Kick a member from the server with role hierarchy verification"
}

func (c *KickCmd) Category() string {
	return "Moderation"
}

func (c *KickCmd) Aliases() []string {
	return []string{"k"}
}

func (c *KickCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionKickMembers)
	return &perms
}

func (c *KickCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "Target member to kick",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Kick reason",
			Required:    false,
		},
	}
}

func (c *KickCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	opts := i.ApplicationCommandData().Options
	var targetUser *discordgo.User
	reason := "No reason provided"

	for _, o := range opts {
		if o.Name == "target" {
			targetUser = o.UserValue(s)
		} else if o.Name == "reason" {
			reason = o.StringValue()
		}
	}

	if targetUser == nil {
		return b.Container(components.TextDisplay{Content: "Target user required."}), nil
	}

	return c.handleKick(ctx, b, s, i.GuildID, i.Member, targetUser.ID, reason)
}

func (c *KickCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	if len(args) == 0 {
		return b.Container(components.TextDisplay{Content: "Usage: `.kick <@user|user_id> [reason]`"}), nil
	}

	targetID := parseMentionID(args[0])
	reason := "No reason provided"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	return c.handleKick(ctx, b, s, m.GuildID, m.Member, targetID, reason)
}

func (c *KickCmd) handleKick(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID, reason string) (*components.Container, error) {
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

	if ok, rReason := commands.CanModerate(guild, caller, targetMember); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", rReason)}), nil
	}

	if ok, rReason := commands.CanBotModerate(guild, botMember, targetMember); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", rReason)}), nil
	}

	err = s.GuildMemberDeleteWithReason(gid, targetID, fmt.Sprintf("[%s] %s", caller.User.Username, reason))
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed kicking member: %s", err.Error())}), nil
	}

	caseLabel := recordCase(ctx, b, gid, "kick", caller.User.ID, targetID, reason, 0, nil)
	if caseLabel != "" {
		commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "Kick", reason, strings.ToLower(caseLabel))
	} else {
		commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "Kick", reason, "")
	}

	title := "Member Kicked"
	if caseLabel != "" {
		title = fmt.Sprintf("Member Kicked (%s)", caseLabel)
	}

	return b.Container(
		components.TextDisplay{Content: title},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (`%s`)", targetID, targetID)},
				components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
				components.TextDisplay{Content: fmt.Sprintf("Reason: %s", reason)},
			},
		},
	), nil
}


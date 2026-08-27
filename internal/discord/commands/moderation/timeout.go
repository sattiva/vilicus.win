package moderation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type TimeoutCmd struct{}

func (c *TimeoutCmd) Name() string {
	return "timeout"
}

func (c *TimeoutCmd) Description() string {
	return "Timeout a member for a specified duration in minutes"
}

func (c *TimeoutCmd) Category() string {
	return "Moderation"
}

func (c *TimeoutCmd) Aliases() []string {
	return []string{"mute", "to"}
}

func (c *TimeoutCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionModerateMembers)
	return &perms
}

func (c *TimeoutCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "Target member to timeout",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "minutes",
			Description: "Duration in minutes (0 to remove timeout, max 40320)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Reason for timeout",
			Required:    false,
		},
	}
}

func (c *TimeoutCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	opts := i.ApplicationCommandData().Options
	var targetUser *discordgo.User
	var mins int64
	reason := "No reason provided"

	for _, o := range opts {
		if o.Name == "target" {
			targetUser = o.UserValue(s)
		} else if o.Name == "minutes" {
			mins = o.IntValue()
		} else if o.Name == "reason" {
			reason = o.StringValue()
		}
	}

	if targetUser == nil {
		return b.Container(components.TextDisplay{Content: "Target user required."}), nil
	}

	return c.handleTimeout(ctx, b, s, i.GuildID, i.Member, targetUser.ID, int(mins), reason)
}

func (c *TimeoutCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}

	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: `.timeout <@user|user_id> <minutes> [reason]`"}), nil
	}

	targetID := parseMentionID(args[0])
	mins, err := strconv.Atoi(args[1])
	if err != nil || mins < 0 {
		return b.Container(components.TextDisplay{Content: "Invalid duration minutes."}), nil
	}

	reason := "No reason provided"
	if len(args) > 2 {
		reason = strings.Join(args[2:], " ")
	}

	return c.handleTimeout(ctx, b, s, m.GuildID, m.Member, targetID, mins, reason)
}

func (c *TimeoutCmd) handleTimeout(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID string, mins int, reason string) (*components.Container, error) {
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

	var until *time.Time
	var durationSeconds int64
	if mins > 0 {
		t := time.Now().Add(time.Duration(mins) * time.Minute)
		until = &t
		durationSeconds = int64(mins) * 60
	}

	err = s.GuildMemberTimeout(gid, targetID, until)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed modifying timeout: %s", err.Error())}), nil
	}

	actionLabel := "Timeout Applied"
	caseType := "timeout"
	if mins == 0 {
		actionLabel = "Timeout Removed"
		caseType = "untimeout"
	}

	caseLabel := recordCase(ctx, b, gid, caseType, caller.User.ID, targetID, reason, durationSeconds, until)
	extra := fmt.Sprintf("Duration: %d mins", mins)
	if caseLabel != "" {
		extra += ", " + strings.ToLower(caseLabel)
	}
	commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, actionLabel, reason, extra)

	title := fmt.Sprintf("Member %s", actionLabel)
	if caseLabel != "" {
		title = fmt.Sprintf("Member %s (%s)", actionLabel, caseLabel)
	}

	return b.Container(
		components.TextDisplay{Content: title},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (`%s`)", targetID, targetID)},
				components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
				components.TextDisplay{Content: fmt.Sprintf("Duration: %d minutes", mins)},
				components.TextDisplay{Content: fmt.Sprintf("Reason: %s", reason)},
			},
		},
	), nil
}


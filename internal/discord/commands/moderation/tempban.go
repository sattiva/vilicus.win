package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/logging"
	"vilicus/internal/store"
)

type TempBanCmd struct{}

func (c *TempBanCmd) Name() string { return "tempban" }
func (c *TempBanCmd) Description() string {
	return "Ban a user for a set time, auto-unbanned after (e.g. 2d)"
}
func (c *TempBanCmd) Category() string  { return "Moderation" }
func (c *TempBanCmd) Aliases() []string { return []string{"tban"} }

func (c *TempBanCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionBanMembers)
	return &perms
}

func (c *TempBanCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "User to ban temporarily",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "duration",
			Description: "Ban length, e.g. 12h or 3d (max 365d)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Reason for the temporary ban",
			Required:    false,
		},
	}
}

func (c *TempBanCmd) CooldownClass() string { return "danger" }

func (c *TempBanCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	var targetID, durRaw, reason = "", "", "No reason provided"
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "target":
			if u := o.UserValue(s); u != nil {
				targetID = u.ID
			}
		case "duration":
			durRaw = o.StringValue()
		case "reason":
			reason = o.StringValue()
		}
	}
	if targetID == "" {
		return b.Container(components.TextDisplay{Content: "Target user required."}), nil
	}
	d := commands.ParseDurationArg(durRaw)
	if d <= 0 {
		return b.Container(components.TextDisplay{Content: "Invalid duration. Use forms like 12h, 3d, 1d12h (max 365d)."}), nil
	}
	return c.handle(ctx, b, s, i.GuildID, i.Member, targetID, d, reason)
}

func (c *TempBanCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .tempban <@user|user_id> <duration> [reason]"}), nil
	}
	targetID := commands.ParseMentionID(args[0])
	if targetID == "" {
		return b.Container(components.TextDisplay{Content: "Could not resolve a user from the first argument."}), nil
	}
	d := commands.ParseDurationArg(args[1])
	if d <= 0 {
		return b.Container(components.TextDisplay{Content: "Invalid duration. Use forms like 12h, 3d, 1d12h (max 365d)."}), nil
	}
	reason := "No reason provided"
	if len(args) > 2 {
		reason = strings.Join(args[2:], " ")
	}
	return c.handle(ctx, b, s, m.GuildID, m.Member, targetID, d, reason)
}

func (c *TempBanCmd) handle(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID string, d time.Duration, reason string) (*components.Container, error) {
	guild, err := s.Guild(gid)
	if err != nil || guild == nil {
		return b.Container(components.TextDisplay{Content: "Failed to load guild."}), nil
	}

	botMember, err := s.GuildMember(gid, s.State.User.ID)
	if err != nil {
		return nil, err
	}

	if targetMember, err := s.GuildMember(gid, targetID); err == nil && targetMember != nil {
		if ok, r := commands.CanModerate(guild, caller, targetMember); !ok {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", r)}), nil
		}
		if ok, r := commands.CanBotModerate(guild, botMember, targetMember); !ok {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", r)}), nil
		}
	} else if ok, r := commands.CanBotModerate(guild, botMember,
		&discordgo.Member{User: &discordgo.User{ID: targetID}}); !ok {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Hierarchy violation: %s", r)}), nil
	}

	expiresAt := time.Now().UTC().Add(d)

	reqID := logging.GetID(ctx)
	caseRow, err := b.GetStore().CreateCase(ctx, gid, "tempban", caller.User.ID, targetID, reason,
		int64(d.Seconds()), &expiresAt, "discord", reqID)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed recording case: %s", err.Error())}), nil
	}

	err = s.GuildBanCreateWithReason(gid, targetID,
		fmt.Sprintf("[%s] tempban %s: %s", caller.User.Username, commands.FormatDuration(d), reason), 0)
	if err != nil {
		_ = b.GetStore().DeactivateCase(ctx, gid, caseRow.CaseNo)
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed banning user: %s", err.Error())}), nil
	}

	if err := b.GetStore().CreateTempBan(ctx, gid, targetID, reason, expiresAt, caller.User.ID, caseRow.CaseNo); err != nil {
		if errors.Is(err, store.ErrActiveTempBan) {
			return b.Container(components.TextDisplay{
				Content: fmt.Sprintf("User banned, but they already had an active tempban scheduled  -  the earliest expiry wins."),
			}), nil
		}
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("User banned but unban scheduling failed: %s", err.Error())}), nil
	}

	commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "TempBan",
		reason, fmt.Sprintf("case #%d, expires <t:%d:R>", caseRow.CaseNo, expiresAt.Unix()))

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Temporary Ban Issued (Case #%d)", caseRow.CaseNo)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (`%s`)", targetID, targetID)},
				components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
				components.TextDisplay{Content: fmt.Sprintf("Duration: %s", commands.FormatDuration(d))},
				components.TextDisplay{Content: fmt.Sprintf("Auto-unban: <t:%d:R>", expiresAt.Unix())},
				components.TextDisplay{Content: fmt.Sprintf("Reason: %s", truncateReason(reason, 300))},
			},
		},
	), nil
}


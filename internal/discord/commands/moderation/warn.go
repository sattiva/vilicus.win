package moderation

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/logging"
)

type WarnCmd struct{}

func (c *WarnCmd) Name() string        { return "warn" }
func (c *WarnCmd) Description() string { return "Warn a user and record a moderation case" }
func (c *WarnCmd) Category() string    { return "Moderation" }
func (c *WarnCmd) Aliases() []string   { return []string{"w"} }

func (c *WarnCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionModerateMembers)
	return &perms
}

func (c *WarnCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "User to warn",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Reason for the warning",
			Required:    true,
		},
	}
}

func (c *WarnCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	var target *discordgo.User
	reason := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "target" {
			target = o.UserValue(s)
		} else if o.Name == "reason" {
			reason = o.StringValue()
		}
	}
	if target == nil {
		return b.Container(components.TextDisplay{Content: "Target user required."}), nil
	}
	if reason == "" {
		return b.Container(components.TextDisplay{Content: "A reason is required for warnings."}), nil
	}
	return c.handleWarn(ctx, b, s, i.GuildID, i.Member, target.ID, reason)
}

func (c *WarnCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .warn <@user|user_id> <reason>"}), nil
	}
	targetID := commands.ParseMentionID(args[0])
	reason := strings.Join(args[1:], " ")
	if targetID == "" {
		return b.Container(components.TextDisplay{Content: "Could not resolve a user from the first argument."}), nil
	}
	return c.handleWarn(ctx, b, s, m.GuildID, m.Member, targetID, reason)
}

func (c *WarnCmd) handleWarn(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, caller *discordgo.Member, targetID, reason string) (*components.Container, error) {
	guild, err := s.Guild(gid)
	if err != nil {
		return nil, err
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
	}

	reqID := logging.GetID(ctx)
	caseRow, err := b.GetStore().CreateCase(ctx, gid, "warn", caller.User.ID, targetID, reason, 0, nil, "discord", reqID)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed recording case: %s", err.Error())}), nil
	}

	dmCh, err := s.UserChannelCreate(targetID)
	if err == nil && dmCh != nil {
		dm := b.Container(
			components.TextDisplay{Content: "You have been warned in " + guild.Name},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{
				Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: fmt.Sprintf("Reason: %s", reason)},
					components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", caller.User.ID)},
					components.TextDisplay{Content: fmt.Sprintf("Case: #%d", caseRow.CaseNo)},
				},
			},
		)
		_, _ = s.ChannelMessageSendComplex(dmCh.ID, &discordgo.MessageSend{
			Flags:      components.FlagComponentsV2,
			Components: []discordgo.MessageComponent{dm},
		})
	}

	commands.DispatchAudit(ctx, b, s, gid, caller.User.ID, targetID, "Warn", reason, fmt.Sprintf("case #%d", caseRow.CaseNo))

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Warning Issued (Case #%d)", caseRow.CaseNo)},
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


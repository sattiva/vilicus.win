package moderation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type ReasonCmd struct{}

func (c *ReasonCmd) Name() string        { return "reason" }
func (c *ReasonCmd) Description() string { return "Update the reason on a moderation case" }
func (c *ReasonCmd) Category() string    { return "Moderation" }
func (c *ReasonCmd) Aliases() []string   { return []string{"setreason"} }

func (c *ReasonCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionModerateMembers)
	return &perms
}

func (c *ReasonCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "number",
			Description: "Case number to update",
			Required:    true,
			MinValue:    &[]float64{1}[0],
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "New reason text",
			Required:    true,
		},
	}
}

func (c *ReasonCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	var num int64
	reason := ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "number":
			num = o.IntValue()
		case "reason":
			reason = o.StringValue()
		}
	}
	return c.handle(ctx, b, s, i.GuildID, i.Member.User.ID, num, reason)
}

func (c *ReasonCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .reason <case_number> <new reason>"}), nil
	}
	num, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || num < 1 {
		return b.Container(components.TextDisplay{Content: "Case number must be a positive integer."}), nil
	}
	userID := m.Author.ID
	if m.Member != nil && m.Member.User != nil {
		userID = m.Member.User.ID
	}
	return c.handle(ctx, b, s, m.GuildID, userID, num, strings.Join(args[1:], " "))
}

func (c *ReasonCmd) handle(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, modID string, num int64, reason string) (*components.Container, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return b.Container(components.TextDisplay{Content: "The new reason cannot be empty."}), nil
	}
	cs, err := b.GetStore().GetCaseByNumber(ctx, gid, num)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("No case #%d in this server.", num)}), nil
	}
	if err := b.GetStore().UpdateCaseReason(ctx, gid, num, reason); err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed updating case: %s", err.Error())}), nil
	}

	commands.DispatchAudit(ctx, b, s, gid, modID, cs.TargetID, "Case Reason Updated",
		truncateReason(reason, 300), fmt.Sprintf("case #%d (%s)", cs.CaseNo, cs.Type))

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Reason Updated on Case #%d", cs.CaseNo)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Type: %s", cs.Type)},
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s>", cs.TargetID)},
				components.TextDisplay{Content: fmt.Sprintf("New reason: %s", truncateReason(reason, 400))},
			},
		},
	), nil
}


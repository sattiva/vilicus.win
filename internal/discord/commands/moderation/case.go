package moderation

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type CaseCmd struct{}

func (c *CaseCmd) Name() string        { return "case" }
func (c *CaseCmd) Description() string { return "Show a single moderation case with its notes" }
func (c *CaseCmd) Category() string    { return "Moderation" }
func (c *CaseCmd) Aliases() []string   { return nil }

func (c *CaseCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageMessages)
	return &perms
}

func (c *CaseCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "number",
			Description: "Case number to inspect",
			Required:    true,
			MinValue:    &[]float64{1}[0],
		},
	}
}

func (c *CaseCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	var num int64
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "number" {
			num = o.IntValue()
		}
	}
	return c.render(ctx, b, i.GuildID, num)
}

func (c *CaseCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 1 {
		return b.Container(components.TextDisplay{Content: "Usage: .case <case_number>"}), nil
	}
	num, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || num < 1 {
		return b.Container(components.TextDisplay{Content: "Case number must be a positive integer."}), nil
	}
	return c.render(ctx, b, m.GuildID, num)
}

func (c *CaseCmd) render(ctx context.Context, b commands.BotInterface, gid string, num int64) (*components.Container, error) {
	cs, err := b.GetStore().GetCaseByNumber(ctx, gid, num)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("No case #%d in this server.", num)}), nil
	}

	status := "active"
	if !cs.Active {
		status = "inactive"
	}

	detail := ""
	if cs.DurationSeconds > 0 {
		detail = fmt.Sprintf("\nDuration: %ds", cs.DurationSeconds)
	}
	if cs.ExpiresAt != nil {
		detail += fmt.Sprintf("\nExpires: <t:%d:R>", cs.ExpiresAt.Unix())
	}

	lines := []discordgo.MessageComponent{
		components.TextDisplay{Content: fmt.Sprintf("Case #%d - %s (%s)", cs.CaseNo, cs.Type, status)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (`%s`)", cs.TargetID, cs.TargetID)},
				components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s>", cs.ModeratorID)},
				components.TextDisplay{Content: fmt.Sprintf("Reason: %s", truncateReason(cs.Reason, 500))},
				components.TextDisplay{Content: fmt.Sprintf("Created: <t:%d:R>%s", cs.CreatedAt.Unix(), detail)},
			},
		},
	}

	notes, err := b.GetStore().ListCaseNotes(ctx, cs.ID)
	if err == nil && len(notes) > 0 {
		lines = append(lines, components.Separator{Divider: true, Spacing: 1})
		for _, n := range notes {
			lines = append(lines, components.TextDisplay{
				Content: fmt.Sprintf("<@%s>: %s (<t:%d:R>)", n.AuthorID, truncateReason(n.Body, 200), n.CreatedAt.Unix()),
			})
		}
	}

	return b.Container(lines...), nil
}


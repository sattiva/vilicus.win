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

type CaseNoteCmd struct{}

func (c *CaseNoteCmd) Name() string        { return "casenote" }
func (c *CaseNoteCmd) Description() string { return "Add a staff note to a moderation case" }
func (c *CaseNoteCmd) Category() string    { return "Moderation" }
func (c *CaseNoteCmd) Aliases() []string   { return []string{"note"} }

func (c *CaseNoteCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionModerateMembers)
	return &perms
}

func (c *CaseNoteCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "number",
			Description: "Case number to annotate",
			Required:    true,
			MinValue:    &[]float64{1}[0],
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "note",
			Description: "Note body",
			Required:    true,
		},
	}
}

func (c *CaseNoteCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	var num int64
	note := ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "number":
			num = o.IntValue()
		case "note":
			note = o.StringValue()
		}
	}
	return c.handle(ctx, b, i.GuildID, i.Member.User.ID, num, note)
}

func (c *CaseNoteCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .casenote <case_number> <note text>"}), nil
	}
	num, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || num < 1 {
		return b.Container(components.TextDisplay{Content: "Case number must be a positive integer."}), nil
	}
	userID := m.Author.ID
	if m.Member != nil && m.Member.User != nil {
		userID = m.Member.User.ID
	}
	return c.handle(ctx, b, m.GuildID, userID, num, strings.Join(args[1:], " "))
}

func (c *CaseNoteCmd) handle(ctx context.Context, b commands.BotInterface, gid, authorID string, num int64, note string) (*components.Container, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return b.Container(components.TextDisplay{Content: "The note cannot be empty."}), nil
	}
	cs, err := b.GetStore().GetCaseByNumber(ctx, gid, num)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("No case #%d in this server.", num)}), nil
	}
	if err := b.GetStore().AddCaseNote(ctx, cs.ID, authorID, note); err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed adding note: %s", err.Error())}), nil
	}

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Note Added to Case #%d", cs.CaseNo)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s>", cs.TargetID)},
				components.TextDisplay{Content: truncateReason(note, 400)},
			},
		},
	), nil
}


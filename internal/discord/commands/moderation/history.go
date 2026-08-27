package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/logging"
	"vilicus/internal/store"
)

type HistoryCmd struct{}

func (c *HistoryCmd) Name() string { return "history" }
func (c *HistoryCmd) Description() string {
	return "List moderation cases, optionally filtered by user, type, or moderator"
}
func (c *HistoryCmd) Category() string  { return "Moderation" }
func (c *HistoryCmd) Aliases() []string { return []string{"cases", "hist"} }

func (c *HistoryCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageMessages)
	return &perms
}

type CaseListFilter struct {
	Type string
	Mod  string
}

func (f CaseListFilter) empty() bool { return f.Type == "" && f.Mod == "" }

func (c *HistoryCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "target",
			Description: "User whose history to show",
			Required:    false,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "type",
			Description: "Filter by case type: ban, kick, timeout, warn, tempban, temprole, ...",
			Required:    false,
		},
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "moderator",
			Description: "Filter by the moderator who issued the case",
			Required:    false,
		},
	}
}

func (c *HistoryCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	var target, moderator *discordgo.User
	var caseType string
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "target":
			target = o.UserValue(s)
		case "type":
			caseType = normalizeCaseType(o.StringValue())
		case "moderator":
			moderator = o.UserValue(s)
		}
	}
	if target == nil && caseType == "" && moderator == nil {
		return b.Container(components.TextDisplay{
			Content: "Give a target, a case type, or a moderator to list something.",
		}), nil
	}
	f := CaseListFilter{Type: caseType}
	if moderator != nil {
		f.Mod = moderator.ID
	}
	targetID := ""
	if target != nil {
		targetID = target.ID
	}
	return RenderHistoryPage(b, i.GuildID, targetID, 0, f), nil
}

func (c *HistoryCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	f := CaseListFilter{}
	targetID := ""
	for _, a := range args {
		key, val, hasEq := strings.Cut(a, "=")
		if !hasEq {
			if targetID == "" {
				if id := commands.ParseMentionID(a); id != "" {
					targetID = id
					continue
				}
			}
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "type", "t":
			f.Type = normalizeCaseType(val)
		case "mod", "moderator", "by":
			f.Mod = commands.ParseMentionID(val)
		}
	}
	if targetID == "" && f.empty() {
		return b.Container(components.TextDisplay{
			Content: "Usage: .history [@user] [type=<type>] [mod=@user]",
		}), nil
	}
	return RenderHistoryPage(b, m.GuildID, targetID, 0, f), nil
}

func normalizeCaseType(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

type casesPayload struct {
	G  string `json:"g"`
	T  string `json:"t"`
	P  int    `json:"p"`
	Ct string `json:"ct,omitempty"`
	Cm string `json:"cm,omitempty"`
}

const historyPerPage = 5

func recordCase(ctx context.Context, b commands.BotInterface, gid, caseType, modID, targetID, reason string, durationSeconds int64, expires *time.Time) string {
	caseRow, err := b.GetStore().CreateCase(ctx, gid, caseType, modID, targetID, reason, durationSeconds, expires, "discord", logging.GetID(ctx))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("Case #%d", caseRow.CaseNo)
}

func RecordCase(ctx context.Context, b commands.BotInterface, gid, caseType, modID, targetID, reason string, durationSeconds int64, expires *time.Time) string {
	return recordCase(ctx, b, gid, caseType, modID, targetID, reason, durationSeconds, expires)
}

func RenderHistoryPage(b commands.BotInterface, gid, targetID string, page int, f CaseListFilter) *components.Container {
	ctx := context.Background()
	cases, err := b.GetStore().ListCasesFiltered(ctx, gid, targetID, store.CaseFilter{
		Type:        f.Type,
		ModeratorID: f.Mod,
	}, historyPerPage+1, page*historyPerPage)
	if err != nil {
		return b.Container(components.TextDisplay{Content: fmt.Sprintf("Failed loading cases: %s", err.Error())})
	}

	hasNext := false
	if len(cases) > historyPerPage {
		hasNext = true
		cases = cases[:historyPerPage]
	}
	if len(cases) == 0 && page == 0 {
		return b.Container(components.TextDisplay{
			Content: fmt.Sprintf("No moderation cases match (%s).", describeScope(targetID, f)),
		})
	}

	title := fmt.Sprintf("Case History: <@%s>", targetID)
	if targetID == "" {
		title = "Moderation Cases"
	}
	lines := []discordgo.MessageComponent{
		components.TextDisplay{Content: title},
		components.Separator{Divider: true, Spacing: 1},
	}
	if !f.empty() {
		scope := describeScope(targetID, f)
		lines = append(lines,
			components.TextDisplay{Content: "Filter: " + scope},
			components.Separator{Divider: false, Spacing: 1})
	}
	for _, cs := range cases {
		status := ""
		if !cs.Active {
			status = " [inactive]"
		}
		target := ""
		if targetID == "" {
			target = "<@" + cs.TargetID + "> "
		}
		lines = append(lines, components.TextDisplay{
			Content: fmt.Sprintf("#%d %s%s %s- <@%s>: %s (<t:%d:R>)",
				cs.CaseNo, cs.Type, status, target, cs.ModeratorID, truncateReason(cs.Reason, 120), cs.CreatedAt.Unix()),
		})
	}

	expiry := time.Now().Add(15 * time.Minute)
	buttons := make([]discordgo.MessageComponent, 0, 2)
	if page > 0 {
		buttons = append(buttons, pageButton(gid, targetID, page-1, f, "Prev", expiry))
	}
	if hasNext {
		buttons = append(buttons, pageButton(gid, targetID, page+1, f, "Next", expiry))
	}

	children := []discordgo.MessageComponent{b.Container(lines...)}
	if len(buttons) > 0 {
		children = append(children, discordgo.ActionsRow{Components: buttons})
	}
	return b.Container(children...)
}

func describeScope(targetID string, f CaseListFilter) string {
	parts := make([]string, 0, 3)
	switch {
	case targetID != "":
		parts = append(parts, "user <@"+targetID+">")
	case !f.empty():
		parts = append(parts, "all users")
	default:
		parts = append(parts, "everything")
	}
	if f.Type != "" {
		parts = append(parts, "type "+f.Type)
	}
	if f.Mod != "" {
		parts = append(parts, "mod <@"+f.Mod+">")
	}
	return strings.Join(parts, ", ")
}

func pageButton(gid, targetID string, page int, f CaseListFilter, label string, expiry time.Time) discordgo.Button {
	payload, _ := json.Marshal(casesPayload{G: gid, T: targetID, P: page, Ct: f.Type, Cm: f.Mod})
	return discordgo.Button{
		Label:    label,
		Style:    discordgo.SecondaryButton,
		CustomID: components.BuildCustomID("cases", "page", payload, expiry),
	}
}

func truncateReason(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func DecodeCasesPayload(raw []byte) (gid, targetID string, f CaseListFilter, page int, ok bool) {
	var p casesPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", CaseListFilter{}, 0, false
	}
	if p.G == "" {
		return "", "", CaseListFilter{}, 0, false
	}
	return p.G, p.T, CaseListFilter{Type: p.Ct, Mod: p.Cm}, p.P, true
}


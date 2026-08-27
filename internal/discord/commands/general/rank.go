package general

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)

type RankCmd struct{}

func (c *RankCmd) Name() string        { return "rank" }
func (c *RankCmd) Description() string { return "Show your (or another user's) level and XP" }
func (c *RankCmd) Category() string    { return "General" }
func (c *RankCmd) Aliases() []string   { return []string{"level", "xp"} }

func (c *RankCmd) RequiredPermissions() *int64 { return nil }

func (c *RankCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "user",
			Description: "Member to inspect (default: yourself)",
			Required:    false,
		},
	}
}

func (c *RankCmd) FastPath() bool { return true }

func (c *RankCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	target := interactionUser(i)
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "user" {
			if u := o.UserValue(s); u != nil {
				target = u
			}
		}
	}
	gid := i.GuildID
	if gid == "" || target == nil {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	return renderRank(b, gid, target.ID)
}

func (c *RankCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	uid := m.Author.ID
	if len(args) > 0 {
		if parsed := commands.ParseMentionID(args[0]); parsed != "" {
			uid = parsed
		}
	}
	return renderRank(b, m.GuildID, uid)
}

func renderRank(b commands.BotInterface, gid, uid string) (*components.Container, error) {
	row, err := b.GetStore().GetUserXP(context.Background(), gid, uid)
	if err != nil {
		return nil, err
	}
	if row == nil || row.XP == 0 {
		return b.Container(components.TextDisplay{Content: "<@" + uid + "> has not earned any XP yet."}), nil
	}

	toNext := store.XPToNext(row.XP)
	bar := progressBar(row.XP, toNext)

	return b.Container(
		components.TextDisplay{Content: "Rank  -  <@" + uid + ">"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: []discordgo.MessageComponent{
			components.TextDisplay{Content: fmt.Sprintf("Level %d", row.Level)},
			components.TextDisplay{Content: fmt.Sprintf("Total XP: %d", row.XP)},
			components.TextDisplay{Content: fmt.Sprintf("%d XP to level %d", toNext, row.Level+1)},
			components.TextDisplay{Content: bar},
		}},
	), nil
}

func progressBar(xp, toNext int64) string {
	const width = 10
	total := xp + toNext
	filled := 0
	if total > 0 {
		filled = int((xp * width) / total)
	}
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	if i.User != nil {
		return i.User
	}
	return nil
}

type LeaderboardCmd struct{}

func (c *LeaderboardCmd) Name() string        { return "leaderboard" }
func (c *LeaderboardCmd) Description() string { return "Show the top 10 members by XP" }
func (c *LeaderboardCmd) Category() string    { return "General" }
func (c *LeaderboardCmd) Aliases() []string   { return []string{"lb", "top"} }

func (c *LeaderboardCmd) RequiredPermissions() *int64 { return nil }
func (c *LeaderboardCmd) Options() []*discordgo.ApplicationCommandOption {
	return nil
}

func (c *LeaderboardCmd) FastPath() bool { return true }

func (c *LeaderboardCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	return renderLeaderboard(b, i.GuildID)
}

func (c *LeaderboardCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	return renderLeaderboard(b, m.GuildID)
}

func renderLeaderboard(b commands.BotInterface, gid string) (*components.Container, error) {
	rows, err := b.GetStore().Leaderboard(context.Background(), gid, 10)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return b.Container(components.TextDisplay{Content: "No XP earned on this server yet."}), nil
	}

	lines := make([]string, 0, len(rows))
	for idx, r := range rows {
		lines = append(lines, fmt.Sprintf("**%d.** <@%s>  -  Level %d (%s XP)",
			idx+1, r.UserID, r.Level, strconv.FormatInt(r.XP, 10)))
	}

	comps := make([]discordgo.MessageComponent, 0, len(lines))
	for _, l := range lines {
		comps = append(comps, components.TextDisplay{Content: l})
	}
	return b.Container(
		components.TextDisplay{Content: "Server Leaderboard"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: comps},
	), nil
}


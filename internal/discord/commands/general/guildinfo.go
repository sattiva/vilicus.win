package general

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)


const discordEpochMS = 1420070400000

func snowflakeTime(id string) time.Time {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(n>>22 + discordEpochMS)
}

func sectionLines(lines ...string) components.Section {
	comps := make([]discordgo.MessageComponent, 0, len(lines))
	for _, l := range lines {
		comps = append(comps, components.TextDisplay{Content: l})
	}
	return components.Section{Components: comps}
}

func guildOnly(b commands.BotInterface, gid string) (*components.Container, bool) {
	if gid == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), false
	}
	return nil, true
}


type MemberCountCmd struct{}

func (c *MemberCountCmd) Name() string        { return "membercount" }
func (c *MemberCountCmd) Description() string { return "Server membership snapshot" }
func (c *MemberCountCmd) Category() string    { return "General" }
func (c *MemberCountCmd) Aliases() []string   { return []string{"mc"} }
func (c *MemberCountCmd) FastPath() bool      { return true }

func (c *MemberCountCmd) RequiredPermissions() *int64 { return nil }

func (c *MemberCountCmd) Options() []*discordgo.ApplicationCommandOption { return nil }

func (c *MemberCountCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if res, ok := guildOnly(b, i.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, i.GuildID)
}

func (c *MemberCountCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if res, ok := guildOnly(b, m.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, m.GuildID)
}

func (c *MemberCountCmd) render(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string) (*components.Container, error) {
	g, err := s.Guild(gid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Could not load this server: " + err.Error()}), nil
	}

	bots := 0
	if cached := stateGuild(s, gid); cached != nil {
		for _, mem := range cached.Members {
			if mem.User != nil && mem.User.Bot {
				bots++
			}
		}
	}

	lines := []string{
		"Members: " + strconv.Itoa(int(g.MemberCount)),
		fmt.Sprintf("Bots (cached roster): %d", bots),
		"Created: <t:" + itoa64(snowflakeTime(g.ID).Unix()) + ":R>",
	}
	if g.PremiumSubscriptionCount > 0 {
		tier := "none"
		switch g.PremiumTier {
		case 1:
			tier = "level 1"
		case 2:
			tier = "level 2"
		case 3:
			tier = "level 3"
		}
		lines = append(lines,
			"Boosts: "+strconv.Itoa(int(g.PremiumSubscriptionCount))+" ("+tier+")",
		)
	}
	return b.Container(
		components.TextDisplay{Content: "Member Count"},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines(lines...),
	), nil
}


type BotsCmd struct{}

func (c *BotsCmd) Name() string        { return "bots" }
func (c *BotsCmd) Description() string { return "List this server's bot members" }
func (c *BotsCmd) Category() string    { return "General" }
func (c *BotsCmd) Aliases() []string   { return nil }
func (c *BotsCmd) FastPath() bool      { return true }

func (c *BotsCmd) RequiredPermissions() *int64 { return nil }

func (c *BotsCmd) Options() []*discordgo.ApplicationCommandOption { return nil }

func (c *BotsCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if res, ok := guildOnly(b, i.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, i.GuildID)
}

func (c *BotsCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if res, ok := guildOnly(b, m.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, m.GuildID)
}

func (c *BotsCmd) render(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string) (*components.Container, error) {
	g := stateGuild(s, gid)
	if g == nil {
		return b.Container(components.TextDisplay{Content: "Member roster is still loading  -  try again in a moment."}), nil
	}

	const cap = 50
	var names []string
	for _, mem := range g.Members {
		if mem.User == nil || !mem.User.Bot {
			continue
		}
		entry := "<@" + mem.User.ID + ">"
		if len(names) < cap {
			names = append(names, entry)
		}
	}
	if len(names) == 0 {
		return b.Container(components.TextDisplay{Content: "No bot members found in the cached roster."}), nil
	}

	suffix := ""
	total := botCount(g)
	if total > cap {
		suffix = fmt.Sprintf("\n...and %d more", total-cap)
	}
	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Bot Members (%d known)", total)},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines(strings.Join(names, ", ")+suffix),
	), nil
}

func botCount(g *discordgo.Guild) int {
	n := 0
	for _, mem := range g.Members {
		if mem.User != nil && mem.User.Bot {
			n++
		}
	}
	return n
}

func stateGuild(s *discordgo.Session, gid string) *discordgo.Guild {
	g, err := s.State.Guild(gid)
	if err != nil {
		return nil
	}
	return g
}


type ChannelInfoCmd struct{}

func (c *ChannelInfoCmd) Name() string        { return "channelinfo" }
func (c *ChannelInfoCmd) Description() string { return "Details about a channel" }
func (c *ChannelInfoCmd) Category() string    { return "General" }
func (c *ChannelInfoCmd) Aliases() []string   { return []string{"ci", "chaninfo"} }
func (c *ChannelInfoCmd) FastPath() bool      { return true }

func (c *ChannelInfoCmd) RequiredPermissions() *int64 { return nil }

func (c *ChannelInfoCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionChannel,
			Name:        "channel",
			Description: "Channel to inspect",
			Required:    false,
			ChannelTypes: []discordgo.ChannelType{
				discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildVoice,
				discordgo.ChannelTypeGuildCategory, discordgo.ChannelTypeGuildForum,
				discordgo.ChannelTypeGuildNews,
			},
		},
	}
}

func (c *ChannelInfoCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if res, ok := guildOnly(b, i.GuildID); !ok {
		return res, nil
	}
	chID := i.ChannelID
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "channel" {
			if ch := o.ChannelValue(s); ch != nil {
				chID = ch.ID
			}
		}
	}
	return c.render(ctx, b, s, i.GuildID, chID)
}

func (c *ChannelInfoCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if res, ok := guildOnly(b, m.GuildID); !ok {
		return res, nil
	}
	chID := m.ChannelID
	if len(args) > 0 {
		if parsed := commands.ParseIDArg(args[0]); parsed != "" {
			chID = parsed
		} else {
			return b.Container(components.TextDisplay{Content: "Could not resolve a channel from that argument."}), nil
		}
	}
	return c.render(ctx, b, s, m.GuildID, chID)
}

func (c *ChannelInfoCmd) render(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, chID string) (*components.Container, error) {
	ch, err := s.State.Channel(chID)
	if err != nil || ch == nil {
		ch, err = s.Channel(chID)
		if err != nil || ch == nil {
			return b.Container(components.TextDisplay{Content: "Could not load that channel."}), nil
		}
	}

	typeNames := map[discordgo.ChannelType]string{
		discordgo.ChannelTypeGuildText:          "text",
		discordgo.ChannelTypeDM:                 "dm",
		discordgo.ChannelTypeGuildVoice:         "voice",
		discordgo.ChannelTypeGuildCategory:      "category",
		discordgo.ChannelTypeGuildNews:          "announcement",
		discordgo.ChannelTypeGuildStore:         "store",
		discordgo.ChannelTypeGuildNewsThread:    "news thread",
		discordgo.ChannelTypeGuildPublicThread:  "public thread",
		discordgo.ChannelTypeGuildPrivateThread: "private thread",
		discordgo.ChannelTypeGuildStageVoice:    "stage",
		discordgo.ChannelTypeGuildDirectory:     "directory",
		discordgo.ChannelTypeGuildForum:         "forum",
	}
	typeName, ok := typeNames[ch.Type]
	if !ok {
		typeName = fmt.Sprintf("type %d", ch.Type)
	}

	lines := []string{
		"Type: " + typeName,
		"Created: <t:" + itoa64(snowflakeTime(ch.ID).Unix()) + ":R>",
	}
	if ch.Topic != "" {
		lines = append(lines, "Topic: "+truncateStr(ch.Topic, 300))
	}
	if ch.ParentID != "" {
		lines = append(lines, "Category: <#"+ch.ParentID+">")
	}
	if ch.Position > 0 {
		lines = append(lines, "Position: "+strconv.Itoa(ch.Position))
	}
	if ch.Type == discordgo.ChannelTypeGuildText || ch.Type == discordgo.ChannelTypeGuildNews {
		if ch.NSFW {
			lines = append(lines, "Age-restricted: yes")
		}
		if ch.RateLimitPerUser > 0 {
			lines = append(lines, "Slowmode: "+FormatSeconds(int(ch.RateLimitPerUser)))
		}
	}
	if ch.Type == discordgo.ChannelTypeGuildVoice && ch.Bitrate > 0 {
		lines = append(lines, fmt.Sprintf("Bitrate: %dkbps", ch.Bitrate/1000))
	}
	if ch.UserLimit > 0 {
		lines = append(lines, "User limit: "+strconv.Itoa(ch.UserLimit))
	}

	title := "#" + ch.Name
	if ch.Type == discordgo.ChannelTypeGuildVoice {
		title = ch.Name
	}
	return b.Container(
		components.TextDisplay{Content: "Channel: " + title},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines(lines...),
	), nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func FormatSeconds(secs int) string {
	if secs%3600 == 0 && secs >= 3600 {
		return strconv.Itoa(secs/3600) + "h"
	}
	if secs%60 == 0 && secs >= 60 {
		return strconv.Itoa(secs/60) + "m"
	}
	return strconv.Itoa(secs) + "s"
}


type RolesCmd struct{}

func (c *RolesCmd) Name() string        { return "roles" }
func (c *RolesCmd) Description() string { return "Role tree sorted by position with member counts" }
func (c *RolesCmd) Category() string    { return "General" }
func (c *RolesCmd) Aliases() []string   { return []string{"roletree"} }
func (c *RolesCmd) FastPath() bool      { return true }

func (c *RolesCmd) RequiredPermissions() *int64 { return nil }

func (c *RolesCmd) Options() []*discordgo.ApplicationCommandOption { return nil }

func (c *RolesCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if res, ok := guildOnly(b, i.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, i.GuildID)
}

func (c *RolesCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if res, ok := guildOnly(b, m.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, m.GuildID)
}

func (c *RolesCmd) render(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string) (*components.Container, error) {
	g := stateGuild(s, gid)
	if g == nil {
		return b.Container(components.TextDisplay{Content: "Could not load this server from cache."}), nil
	}

	counts := make(map[string]int)
	for _, mem := range g.Members {
		for _, rid := range mem.Roles {
			counts[rid]++
		}
	}

	roles := make([]*discordgo.Role, len(g.Roles))
	copy(roles, g.Roles)
	sort.SliceStable(roles, func(a, b int) bool { return roles[a].Position > roles[b].Position })

	const maxShow = 45
	var lines []string
	shown := 0
	for _, r := range roles {
		if shown >= maxShow {
			break
		}
		name := r.Name
		if r.ID == gid {
			name = "@everyone"
		}
		entry := name
		if r.ID != gid && counts[r.ID] > 0 {
			entry += fmt.Sprintf("  -  %d", counts[r.ID])
		}
		if r.Managed {
			entry += " (managed)"
		}
		if r.Color != 0 {
			entry = fmt.Sprintf("[#%06X] ", r.Color) + entry
		}
		lines = append(lines, entry)
		shown++
	}
	suffix := ""
	if len(roles) > maxShow {
		suffix = fmt.Sprintf("\n...and %d more roles", len(roles)-maxShow)
	}

	blocks := chunkLines(strings.Join(lines, "\n")+suffix, 3800)
	children := make([]discordgo.MessageComponent, 0, len(blocks)+2)
	children = append(children,
		components.TextDisplay{Content: fmt.Sprintf("Role Tree (%d roles)", len(roles))},
		components.Separator{Divider: true, Spacing: 1},
	)
	for _, blk := range blocks {
		children = append(children, components.TextDisplay{Content: blk})
	}
	return b.Container(children...), nil
}


type InvitesCmd struct{}

func (c *InvitesCmd) Name() string        { return "invites" }
func (c *InvitesCmd) Description() string { return "List active server invites" }
func (c *InvitesCmd) Category() string    { return "General" }
func (c *InvitesCmd) Aliases() []string   { return nil }

func (c *InvitesCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageGuild)
	return &perms
}

func (c *InvitesCmd) FastPath() bool { return false }

func (c *InvitesCmd) Options() []*discordgo.ApplicationCommandOption { return nil }

func (c *InvitesCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if res, ok := guildOnly(b, i.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, i.GuildID)
}

func (c *InvitesCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if res, ok := guildOnly(b, m.GuildID); !ok {
		return res, nil
	}
	return c.render(ctx, b, s, m.GuildID)
}

func (c *InvitesCmd) render(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string) (*components.Container, error) {
	invites, err := s.GuildInvites(gid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Could not list invites: " + err.Error()}), nil
	}
	if len(invites) == 0 {
		return b.Container(components.TextDisplay{Content: "No active invites."}), nil
	}

	const maxShow = 20
	var lines []string
	for n, inv := range invites {
		if n >= maxShow {
			break
		}
		line := "`" + inv.Code + "`"
		if inv.Inviter != nil {
			line += " by <@" + inv.Inviter.ID + ">"
		}
		if inv.Uses > 0 || inv.MaxUses > 0 {
			line += fmt.Sprintf("  -  used %d/%s", inv.Uses, usesCap(inv))
		}
		if inv.Channel != nil && inv.Channel.ID != "" {
			line += " -> <#" + inv.Channel.ID + ">"
		}
		lines = append(lines, line)
	}
	suffix := ""
	if len(invites) > maxShow {
		suffix = fmt.Sprintf("\n...and %d more", len(invites)-maxShow)
	}

	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Active Invites (%d)", len(invites))},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines(strings.Join(lines, "\n")+suffix),
	), nil
}

func usesCap(inv *discordgo.Invite) string {
	if inv.MaxUses == 0 {
		return ""
	}
	return strconv.Itoa(inv.MaxUses)
}


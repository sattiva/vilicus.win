package general

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

var errGiveawayUnsupported = errors.New("giveaways are not available on this bot instance")

const usageGiveaway = "Usage: .giveaway <duration> [winners] <prize...> | .greroll <message_id> [winners]"

type GiveawayCmd struct{}

func (c *GiveawayCmd) Name() string        { return "giveaway" }
func (c *GiveawayCmd) Description() string { return "Start a giveaway with an enter button" }
func (c *GiveawayCmd) Category() string    { return "General" }
func (c *GiveawayCmd) Aliases() []string   { return []string{"gstart"} }

func (c *GiveawayCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageGuild)
	return &perms
}

func (c *GiveawayCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "duration",
			Description: "How long it runs, e.g. 2h or 3d (max 30d)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "winners",
			Description: "Number of winners (default 1, max 20)",
			Required:    false,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "prize",
			Description: "What the winners get",
			Required:    true,
		},
	}
}

func (c *GiveawayCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" || i.Member == nil || i.Member.User == nil {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	starter, ok := b.(commands.GiveawayStarter)
	if !ok {
		return nil, errGiveawayUnsupported
	}
	durRaw, prize := "", ""
	winners := 1
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "duration":
			durRaw = o.StringValue()
		case "winners":
			winners = int(o.IntValue())
		case "prize":
			prize = o.StringValue()
		}
	}
	return c.start(ctx, b, starter, s, i.GuildID, i.ChannelID, i.Member.User.ID, durRaw, winners, prize)
}

func (c *GiveawayCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	starter, ok := b.(commands.GiveawayStarter)
	if !ok {
		return nil, errGiveawayUnsupported
	}
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: usageGiveaway}), nil
	}
	durRaw := args[0]
	winners := 1
	prizeWords := args[1:]
	if len(args) > 2 {
		if n, err := strconv.Atoi(args[1]); err == nil && n >= 1 && n <= 20 {
			winners = n
			prizeWords = args[2:]
		}
	}
	return c.start(ctx, b, starter, s, m.GuildID, m.ChannelID, m.Author.ID, durRaw, winners, strings.Join(prizeWords, " "))
}

func (c *GiveawayCmd) start(ctx context.Context, b commands.BotInterface, starter commands.GiveawayStarter, s *discordgo.Session, gid, channelID, hostedBy, durRaw string, winners int, prize string) (*components.Container, error) {
	prize = strings.TrimSpace(prize)
	if prize == "" {
		return b.Container(components.TextDisplay{Content: "A prize is required."}), nil
	}
	if len(prize) > 200 {
		prize = prize[:200]
	}
	d := commands.ParseDurationArg(durRaw)
	if d <= 0 {
		return b.Container(components.TextDisplay{Content: "Invalid duration. Use forms like 2h, 3d, 1d12h (max 30d)."}), nil
	}
	if d > 30*24*time.Hour {
		return b.Container(components.TextDisplay{Content: "Giveaways can run at most 30 days."}), nil
	}
	return starter.StartGiveaway(ctx, s, gid, channelID, prize, hostedBy, winners, d)
}

type GiveawayRerollCmd struct{}

func (c *GiveawayRerollCmd) Name() string        { return "greroll" }
func (c *GiveawayRerollCmd) Description() string { return "Reroll winners of an ended giveaway" }
func (c *GiveawayRerollCmd) Category() string    { return "General" }
func (c *GiveawayRerollCmd) Aliases() []string   { return []string{"grerolls"} }

func (c *GiveawayRerollCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageGuild)
	return &perms
}

func (c *GiveawayRerollCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "message_id",
			Description: "Message ID of the giveaway panel",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "winners",
			Description: "How many new winners to draw (default 1)",
			Required:    false,
		},
	}
}

func (c *GiveawayRerollCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" || i.Member == nil || i.Member.User == nil {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	starter, ok := b.(commands.GiveawayStarter)
	if !ok {
		return nil, errGiveawayUnsupported
	}
	msgID, extra := "", 1
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "message_id":
			msgID = o.StringValue()
		case "winners":
			extra = int(o.IntValue())
		}
	}
	return reroll(ctx, b, starter, s, i.GuildID, i.Member.User.ID, msgID, extra)
}

func (c *GiveawayRerollCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	starter, ok := b.(commands.GiveawayStarter)
	if !ok {
		return nil, errGiveawayUnsupported
	}
	if len(args) < 1 {
		return b.Container(components.TextDisplay{Content: usageGiveaway}), nil
	}
	extra := 1
	if len(args) > 1 {
		if n, err := strconv.Atoi(args[1]); err == nil && n >= 1 {
			extra = n
		}
	}
	return reroll(ctx, b, starter, s, m.GuildID, m.Author.ID, args[0], extra)
}

func reroll(ctx context.Context, b commands.BotInterface, starter commands.GiveawayStarter, s *discordgo.Session, gid, actorID, msgID string, extra int) (*components.Container, error) {
	if !commands.ValidSnowflake(msgID) {
		return b.Container(components.TextDisplay{Content: "That does not look like a message id."}), nil
	}
	return starter.RerollGiveaway(ctx, s, gid, msgID, actorID, extra)
}


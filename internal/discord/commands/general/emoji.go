package general

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)


const emojiCDN = "https://cdn.discordapp.com/emojis/%s.%s?size=128&quality=lossless"

var emojiTagRe = regexp.MustCompile(`^<?(a)?:?([a-zA-Z0-9_]{2,32}):(\d{15,21})>?$`)
var emojiNameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{2,32}$`)

var emojiHTTPClient = &http.Client{Timeout: 10 * time.Second}

type EmojiCmd struct{}

func (c *EmojiCmd) Name() string        { return "emoji" }
func (c *EmojiCmd) Description() string { return "Jumbo, steal, list, or delete custom emojis" }
func (c *EmojiCmd) Category() string    { return "General" }
func (c *EmojiCmd) Aliases() []string   { return nil }
func (c *EmojiCmd) FastPath() bool      { return false }

func (c *EmojiCmd) RequiredPermissions() *int64 { return nil }

func (c *EmojiCmd) Options() []*discordgo.ApplicationCommandOption {
	emojiOpt := &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionString,
		Name:        "emoji",
		Description: "A custom emoji (paste it) or its name",
		Required:    true,
	}
	return []*discordgo.ApplicationCommandOption{
		{Name: "jumbo", Description: "Show a custom emoji at full size", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{emojiOpt}},
		{Name: "steal", Description: "Copy an emoji from another server into this one", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			emojiOpt,
			{Name: "name", Description: "Custom name (default: source name)", Required: false},
		}},
		{Name: "list", Description: "List this server's custom emojis", Type: discordgo.ApplicationCommandOptionSubCommand},
		{Name: "delete", Description: "Delete a custom emoji by name", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{emojiOpt}},
	}
}

type parsedEmoji struct {
	ID       string
	Name     string
	Animated bool
}

func parseEmojiTag(raw string) *parsedEmoji {
	m := emojiTagRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return nil
	}
	return &parsedEmoji{ID: m[3], Name: m[2], Animated: m[1] == "a"}
}

func resolveEmoji(s *discordgo.Session, gid, raw string) *parsedEmoji {
	if p := parseEmojiTag(raw); p != nil {
		return p
	}
	g, err := s.State.Guild(gid)
	if err != nil || g == nil {
		return nil
	}
	if commands.ValidSnowflake(raw) {
		for _, e := range g.Emojis {
			if e.ID == raw {
				return &parsedEmoji{ID: e.ID, Name: e.Name, Animated: e.Animated}
			}
		}
		return nil
	}
	lower := strings.ToLower(strings.Trim(raw, ":"))
	for _, e := range g.Emojis {
		if strings.EqualFold(e.Name, lower) {
			return &parsedEmoji{ID: e.ID, Name: e.Name, Animated: e.Animated}
		}
	}
	return nil
}

func emojiURL(p *parsedEmoji) string {
	ext := "png"
	if p.Animated {
		ext = "gif"
	}
	return fmt.Sprintf(emojiCDN, p.ID, ext)
}

func (c *EmojiCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return b.Container(components.TextDisplay{Content: "Usage: /emoji jumbo|steal|list|delete"}), nil
	}
	uid := ""
	if i.Member != nil && i.Member.User != nil {
		uid = i.Member.User.ID
	}
	switch opts[0].Name {
	case "jumbo":
		raw := subString(opts[0].Options, "emoji")
		return c.jumbo(ctx, b, s, i.GuildID, raw)
	case "steal":
		raw := subString(opts[0].Options, "emoji")
		name := subString(opts[0].Options, "name")
		if !runtimePerm(s, i.GuildID, uid, i.Member, discordgo.PermissionManageGuildExpressions) {
			return b.Container(components.TextDisplay{Content: "You need the Manage Guild Expressions permission to steal emojis."}), nil
		}
		return c.steal(ctx, b, s, i.GuildID, raw, name)
	case "list":
		return c.list(ctx, b, s, i.GuildID)
	default:
		raw := subString(opts[0].Options, "emoji")
		if !runtimePerm(s, i.GuildID, uid, i.Member, discordgo.PermissionManageGuildExpressions) {
			return b.Container(components.TextDisplay{Content: "You need the Manage Guild Expressions permission to delete emojis."}), nil
		}
		return c.del(ctx, b, s, i.GuildID, raw)
	}
}

func (c *EmojiCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) == 0 {
		return b.Container(components.TextDisplay{Content: "Usage: .emoji jumbo|steal|list|delete"}), nil
	}
	uid := ""
	if m.Author != nil {
		uid = m.Author.ID
	}
	switch args[0] {
	case "jumbo", "big":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Paste the emoji to jumbo, e.g. `.emoji jumbo :pog:1234567890`"}), nil
		}
		return c.jumbo(ctx, b, s, m.GuildID, args[1])
	case "steal":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .emoji steal <:name:id> [new_name]"}), nil
		}
		if !runtimePerm(s, m.GuildID, uid, m.Member, discordgo.PermissionManageGuildExpressions) {
			return b.Container(components.TextDisplay{Content: "You need the Manage Guild Expressions permission to steal emojis."}), nil
		}
		name := ""
		if len(args) > 2 {
			name = args[2]
		}
		return c.steal(ctx, b, s, m.GuildID, args[1], name)
	case "list", "all":
		return c.list(ctx, b, s, m.GuildID)
	case "delete", "del", "rm":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .emoji delete <emoji_or_name>"}), nil
		}
		if !runtimePerm(s, m.GuildID, uid, m.Member, discordgo.PermissionManageGuildExpressions) {
			return b.Container(components.TextDisplay{Content: "You need the Manage Guild Expressions permission to delete emojis."}), nil
		}
		return c.del(ctx, b, s, m.GuildID, args[1])
	default:
		return b.Container(components.TextDisplay{Content: "Usage: .emoji jumbo|steal|list|delete"}), nil
	}
}

func (c *EmojiCmd) jumbo(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, raw string) (*components.Container, error) {
	p := resolveEmoji(s, gid, raw)
	if p == nil {
		return b.Container(components.TextDisplay{Content: "Only custom server emojis can be jumbod  -  paste one like `<:name:id>`."}), nil
	}
	return b.Container(
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: ":" + p.Name + ":  -  " + p.ID},
			},
			Accessory: &discordgo.Thumbnail{Media: discordgo.UnfurledMediaItem{URL: emojiURL(p)}},
		},
		components.MediaGallery{Items: []components.MediaGalleryItem{{Media: components.MediaItem{URL: emojiURL(p)}}}},
	), nil
}

func (c *EmojiCmd) steal(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, raw, name string) (*components.Container, error) {
	p := parseEmojiTag(raw)
	if p == nil {
		return b.Container(components.TextDisplay{Content: "Stealing needs the full emoji tag  -  paste it like `<:name:id>` from the other server."}), nil
	}
	if name == "" {
		name = p.Name
	}
	name = strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	if !emojiNameRe.MatchString(name) {
		return b.Container(components.TextDisplay{Content: "Names must be 2-32 letters, numbers, or underscores."}), nil
	}

	existing, err := s.GuildEmojis(gid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Could not read this server's emojis: " + err.Error()}), nil
	}
	if len(existing) >= 50 {
		return b.Container(components.TextDisplay{Content: "This server is at its emoji slot limit."}), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, emojiURL(p), nil)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Bad emoji source URL."}), nil
	}
	resp, err := emojiHTTPClient.Do(req)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Emoji download failed: " + err.Error()}), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return b.Container(components.TextDisplay{Content: "Emoji download failed (HTTP " + strconv.Itoa(resp.StatusCode) + ")."}), nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Emoji download failed: " + err.Error()}), nil
	}
	if len(body) > 256*1024 {
		return b.Container(components.TextDisplay{Content: "Emoji image exceeds the 256KB Discord limit."}), nil
	}

	mime := "image/png"
	if p.Animated {
		mime = "image/gif"
	}
	dataURI := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(body)
	created, err := s.GuildEmojiCreate(gid, &discordgo.EmojiParams{Name: name, Image: dataURI})
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Emoji upload rejected: " + err.Error()}), nil
	}
	render := "<:" + created.Name + ":" + created.ID + ">"
	if created.Animated {
		render = "<a:" + created.Name + ":" + created.ID + ">"
	}
	return b.Container(
		components.TextDisplay{Content: "Emoji Stolen"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: "Added as " + render + " (`:" + created.Name + ":`)"},
			},
			Accessory: &discordgo.Thumbnail{Media: discordgo.UnfurledMediaItem{URL: emojiURL(&parsedEmoji{ID: created.ID, Name: created.Name, Animated: created.Animated})}},
		},
	), nil
}

func (c *EmojiCmd) list(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string) (*components.Container, error) {
	emojis, err := s.GuildEmojis(gid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Could not read this server's emojis: " + err.Error()}), nil
	}
	if len(emojis) == 0 {
		return b.Container(components.TextDisplay{Content: "This server has no custom emojis."}), nil
	}

	var sb strings.Builder
	for n, e := range emojis {
		if n > 0 && n%25 == 0 {
			sb.WriteString("\n")
		}
		if e.Animated {
			sb.WriteString("<a:" + e.Name + ":" + e.ID + ">")
		} else {
			sb.WriteString("<:" + e.Name + ":" + e.ID + ">")
		}
	}
	blocks := chunkLines(sb.String(), 3800)
	children := make([]discordgo.MessageComponent, 0, len(blocks)+2)
	children = append(children,
		components.TextDisplay{Content: fmt.Sprintf("Custom Emojis (%d)", len(emojis))},
		components.Separator{Divider: true, Spacing: 1},
	)
	for _, blk := range blocks {
		children = append(children, components.TextDisplay{Content: blk})
	}
	return b.Container(children...), nil
}

func (c *EmojiCmd) del(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, raw string) (*components.Container, error) {
	p := resolveEmoji(s, gid, raw)
	if p == nil {
		return b.Container(components.TextDisplay{Content: "No emoji matching that name or id in this server."}), nil
	}
	if err := s.GuildEmojiDelete(gid, p.ID); err != nil {
		return b.Container(components.TextDisplay{Content: "Delete failed: " + err.Error()}), nil
	}
	return b.Container(components.TextDisplay{Content: "Deleted `:" + p.Name + ":`."}), nil
}

func runtimePerm(s *discordgo.Session, gid, uid string, member *discordgo.Member, perm int64) bool {
	if uid == "" {
		return false
	}
	if member == nil || member.Permissions == 0 {
		if mem, _ := s.State.Member(gid, uid); mem != nil {
			member = mem
		}
	}
	if member == nil || member.Permissions == 0 {
		if fetched, err := s.GuildMember(gid, uid); err == nil {
			member = fetched
		}
	}
	if member == nil {
		return false
	}
	return member.Permissions&(perm|discordgo.PermissionAdministrator) != 0
}

func subString(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range opts {
		if o.Name == name {
			return o.StringValue()
		}
	}
	return ""
}

func chunkLines(s string, max int) []string {
	if len(s) <= max {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, line := range strings.SplitAfter(s, "\n") {
		for len(line) > max {
			flush()
			out = append(out, line[:max])
			line = line[max:]
		}
		if cur.Len()+len(line) > max {
			flush()
		}
		cur.WriteString(line)
	}
	flush()
	return out
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }


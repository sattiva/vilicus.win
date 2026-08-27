package general

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)

type HelpCmd struct {
	mu         sync.RWMutex
	cachedCats map[string][]cachedCmd
	cachedKeys []string
}

type cachedCmd struct {
	name string
	desc string
	pfx  string
}

func (c *HelpCmd) Name() string {
	return "help"
}

func (c *HelpCmd) Description() string {
	return "List all available bot commands by category"
}

func (c *HelpCmd) Category() string {
	return "General"
}

func (c *HelpCmd) Aliases() []string {
	return []string{"commands", "cmds", "h"}
}

func (c *HelpCmd) Options() []*discordgo.ApplicationCommandOption {
	return nil
}

func (c *HelpCmd) RequiredPermissions() *int64 {
	return nil
}

func (c *HelpCmd) initCache(b commands.BotInterface) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedCats != nil {
		return
	}

	c.cachedCats = make(map[string][]cachedCmd)
	for _, cmd := range b.GetCommands() {
		cat := cmd.Category()
		if cat == "" {
			cat = "General"
		}
		aliasStr := ""
		if len(cmd.Aliases()) > 0 {
			aliasStr = fmt.Sprintf(" [%s]", strings.Join(cmd.Aliases(), ", "))
		}
		c.cachedCats[cat] = append(c.cachedCats[cat], cachedCmd{
			name: cmd.Name(),
			desc: cmd.Description(),
			pfx:  aliasStr,
		})
	}

	for k := range c.cachedCats {
		c.cachedKeys = append(c.cachedKeys, k)
	}
	sort.Strings(c.cachedKeys)
}

func (c *HelpCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	gid := i.GuildID
	uid := ""
	if i.Member != nil && i.Member.User != nil {
		uid = i.Member.User.ID
	} else if i.User != nil {
		uid = i.User.ID
	}
	return c.buildHelp(ctx, b, gid, uid)
}

func (c *HelpCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	return c.buildHelp(ctx, b, m.GuildID, m.Author.ID)
}

func (c *HelpCmd) buildHelp(ctx context.Context, b commands.BotInterface, gid, uid string) (*components.Container, error) {
	c.initCache(b)

	botName := b.GetStore().GetSetting("bot_name", "Vilicus")
	p := b.GetStore().ResolvePrefix(ctx, gid, uid)

	c.mu.RLock()
	defer c.mu.RUnlock()

	var compList []discordgo.MessageComponent
	compList = append(compList, components.TextDisplay{Content: fmt.Sprintf("%s Command Manual | Active Prefix: `%s`", botName, p)})
	compList = append(compList, components.Separator{Divider: true, Spacing: 1})

	for _, cat := range c.cachedKeys {
		cmds := c.cachedCats[cat]
		var lines []string
		for _, item := range cmds {
			lines = append(lines, fmt.Sprintf("%s%s%s - %s", p, item.name, item.pfx, item.desc))
		}

		compList = append(compList, components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("**%s**\n%s", strings.ToUpper(cat), strings.Join(lines, "\n"))},
			},
		})
	}

	return b.Container(compList...), nil
}


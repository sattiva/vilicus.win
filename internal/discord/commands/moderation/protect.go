package moderation

import (
	"context"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/protection"
	"vilicus/internal/store"
)

type ProtectCmd struct{}

func (c *ProtectCmd) Name() string { return "protect" }
func (c *ProtectCmd) Description() string {
	return "Configure antispam, link blocking, the word filter, honeypot, and antinuke"
}
func (c *ProtectCmd) Category() string  { return "Configuration" }
func (c *ProtectCmd) Aliases() []string { return []string{"protection"} }

func (c *ProtectCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageGuild)
	return &perms
}

func punishChoices() []*discordgo.ApplicationCommandOptionChoice {
	return []*discordgo.ApplicationCommandOptionChoice{
		{Name: "timeout", Value: "timeout"},
		{Name: "kick", Value: "kick"},
		{Name: "ban", Value: "ban"},
	}
}

func (c *ProtectCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{Name: "antispam", Description: "Toggle the burst detector", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "state", Description: "on or off", Required: true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{{Name: "on", Value: "on"}, {Name: "off", Value: "off"}}},
			{Type: discordgo.ApplicationCommandOptionInteger, Name: "messages", Description: "Messages within the window that count as spam (3-30, default 6)", Required: false},
			{Type: discordgo.ApplicationCommandOptionInteger, Name: "window_seconds", Description: "Rolling window in seconds (2-30, default 5)", Required: false},
		}},
		{Name: "antilink", Description: "Block links", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "mode", Description: "Who may still post links", Required: true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "off", Value: "off"},
					{Name: "mods only may post links", Value: "mods"},
					{Name: "everyone blocked", Value: "on"},
				}},
		}},
		{Name: "filter", Description: "Manage the blocked word list", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "action", Description: "add, remove, clear, or list", Required: true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "add", Value: "add"}, {Name: "remove", Value: "remove"},
					{Name: "clear", Value: "clear"}, {Name: "list", Value: "list"},
				}},
			{Type: discordgo.ApplicationCommandOptionString, Name: "word", Description: "Word to add or remove", Required: false},
		}},
		{Name: "honeypot", Description: "Configure the honeypot trap channel", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "state", Description: "on or off", Required: false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{{Name: "on", Value: "on"}, {Name: "off", Value: "off"}}},
			{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel that becomes the trap", Required: false,
				ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText}},
			{Type: discordgo.ApplicationCommandOptionString, Name: "action", Description: "What happens to anyone posting there", Required: false,
				Choices: punishChoices()},
		}},
		{Name: "antinuke", Description: "Configure the audit-log threat watcher", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "state", Description: "on or off", Required: false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{{Name: "on", Value: "on"}, {Name: "off", Value: "off"}}},
			{Type: discordgo.ApplicationCommandOptionString, Name: "punish", Description: "Punishment when an actor crosses the threshold", Required: false,
				Choices: punishChoices()},
			{Type: discordgo.ApplicationCommandOptionInteger, Name: "threshold", Description: "Threat score that triggers punishment (20-1000, default 100)", Required: false},
			{Type: discordgo.ApplicationCommandOptionInteger, Name: "window_seconds", Description: "Sliding scoring window in seconds (10-300, default 60)", Required: false},
			{Type: discordgo.ApplicationCommandOptionString, Name: "whitelist_action", Description: "Whitelist management", Required: false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "add", Value: "add"}, {Name: "remove", Value: "remove"}, {Name: "clear", Value: "clear"},
				}},
			{Type: discordgo.ApplicationCommandOptionUser, Name: "whitelist_user", Description: "Trusted actor to add or remove", Required: false},
			{Type: discordgo.ApplicationCommandOptionChannel, Name: "alert_channel", Description: "Alerts go here (default: mod log)", Required: false,
				ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText}},
		}},
		{Name: "show", Description: "Show current protection settings", Type: discordgo.ApplicationCommandOptionSubCommand},
	}
}

func (c *ProtectCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return c.show(ctx, b, i.GuildID)
	}

	switch opts[0].Name {
	case "honeypot":
		return c.honeypot(ctx, b, s, i.GuildID, opts[0].Options)
	case "antinuke":
		return c.antinuke(ctx, b, s, i.GuildID, opts[0].Options)
	}

	cfg := c.load(ctx, b, i.GuildID)
	switch opts[0].Name {
	case "antispam":
		state := ""
		for _, o := range opts[0].Options {
			switch o.Name {
			case "state":
				state = o.StringValue()
			case "messages":
				cfg.AntispamMsgs = int(o.IntValue())
			case "window_seconds":
				cfg.AntispamWindow = int(o.IntValue())
			}
		}
		cfg.AntispamEnabled = state == "on"
	case "antilink":
		if len(opts[0].Options) > 0 {
			cfg.AntilinkMode = opts[0].Options[0].StringValue()
		}
	case "filter":
		action, word := "list", ""
		if len(opts[0].Options) > 0 {
			action = opts[0].Options[0].StringValue()
		}
		if len(opts[0].Options) > 1 {
			word = strings.ToLower(strings.TrimSpace(opts[0].Options[1].StringValue()))
		}
		return c.filter(ctx, b, cfg, action, word)
	default:
		return c.show(ctx, b, i.GuildID)
	}

	if err := b.GetStore().SaveProtectionConfig(ctx, cfg); err != nil {
		return b.Container(components.TextDisplay{Content: "Failed saving protection config: " + err.Error()}), nil
	}
	return c.show(ctx, b, i.GuildID)
}

func (c *ProtectCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) == 0 || args[0] == "show" {
		return c.show(ctx, b, m.GuildID)
	}

	switch args[0] {
	case "honeypot", "trap":
		return c.prefixHoneypot(ctx, b, s, m.GuildID, args[1:])
	case "antinuke":
		return c.prefixAntinuke(ctx, b, s, m.GuildID, args[1:])
	case "antispam", "spam":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .protect antispam on|off [msgs] [window_seconds]"}), nil
		}
		cfg := c.load(ctx, b, m.GuildID)
		switch args[1] {
		case "on", "enable":
			cfg.AntispamEnabled = true
		case "off", "disable":
			cfg.AntispamEnabled = false
		default:
			return b.Container(components.TextDisplay{Content: "State must be on or off."}), nil
		}
		if len(args) > 2 {
			if n, err := strconv.Atoi(args[2]); err == nil {
				cfg.AntispamMsgs = n
			}
		}
		if len(args) > 3 {
			if n, err := strconv.Atoi(args[3]); err == nil {
				cfg.AntispamWindow = n
			}
		}
		if err := b.GetStore().SaveProtectionConfig(ctx, cfg); err != nil {
			return b.Container(components.TextDisplay{Content: "Failed saving protection config: " + err.Error()}), nil
		}
		return c.show(ctx, b, m.GuildID)
	case "antilink", "links":
		cfg := c.load(ctx, b, m.GuildID)
		mode := "on"
		if len(args) > 1 {
			switch args[1] {
			case "off", "disable":
				mode = "off"
			case "mods", "modonly":
				mode = "mods"
			}
		}
		cfg.AntilinkMode = mode
		if err := b.GetStore().SaveProtectionConfig(ctx, cfg); err != nil {
			return b.Container(components.TextDisplay{Content: "Failed saving protection config: " + err.Error()}), nil
		}
		return c.show(ctx, b, m.GuildID)
	case "filter":
		action := "list"
		word := ""
		if len(args) > 1 {
			action = args[1]
		}
		if len(args) > 2 {
			word = strings.ToLower(strings.TrimSpace(strings.Join(args[2:], " ")))
		}
		cfg := c.load(ctx, b, m.GuildID)
		return c.filter(ctx, b, cfg, action, word)
	default:
		return b.Container(components.TextDisplay{Content: "Usage: .protect show|antispam|antilink|filter|honeypot|antinuke"}), nil
	}
}


func (c *ProtectCmd) honeypot(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, opts []*discordgo.ApplicationCommandInteractionDataOption) (*components.Container, error) {
	cfg := c.load(ctx, b, gid)
	if len(opts) == 0 {
		return c.show(ctx, b, gid)
	}
	for _, o := range opts {
		switch o.Name {
		case "channel":
			if ch := o.ChannelValue(s); ch != nil && ch.ID != "" {
				cfg.HoneypotChannel = ch.ID
			}
		case "state":
			if o.StringValue() == "off" {
				cfg.HoneypotChannel = ""
			}
		case "action":
			cfg.HoneypotAction = o.StringValue()
		}
	}
	if err := b.GetStore().SaveProtectionConfig(ctx, cfg); err != nil {
		return b.Container(components.TextDisplay{Content: "Failed saving protection config: " + err.Error()}), nil
	}
	bustProtectionCache(b, gid)
	return c.show(ctx, b, gid)
}

func (c *ProtectCmd) prefixHoneypot(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, args []string) (*components.Container, error) {
	cfg := c.load(ctx, b, gid)
	if len(args) == 0 {
		return c.show(ctx, b, gid)
	}
	switch args[0] {
	case "off", "disable":
		cfg.HoneypotChannel = ""
	case "action":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .protect honeypot action timeout|kick|ban"}), nil
		}
		cfg.HoneypotAction = args[1]
	default:
		chID := commands.ParseMentionID(args[0])
		if len(chID) < 17 {
			if ch, err := s.Channel(args[0]); err == nil && ch != nil {
				chID = ch.ID
			} else {
				return b.Container(components.TextDisplay{Content: "Usage: .protect honeypot <#channel>|off [action]"}), nil
			}
		}
		cfg.HoneypotChannel = chID
		if len(args) > 1 && !isPunishWord(args[1]) {
			return b.Container(components.TextDisplay{Content: "Action must be timeout, kick, or ban."}), nil
		}
		if len(args) > 1 {
			cfg.HoneypotAction = args[1]
		}
	}
	if err := b.GetStore().SaveProtectionConfig(ctx, cfg); err != nil {
		return b.Container(components.TextDisplay{Content: "Failed saving protection config: " + err.Error()}), nil
	}
	bustProtectionCache(b, gid)
	return c.show(ctx, b, gid)
}

func isPunishWord(s string) bool {
	return protection.ValidPunish(strings.ToLower(s))
}

func bustProtectionCache(b commands.BotInterface, gid string) {
	type cacheBuster interface{ InvalidateProtectionConfig(gid string) }
	if cb, ok := b.(cacheBuster); ok {
		cb.InvalidateProtectionConfig(gid)
	}
}


func (c *ProtectCmd) antinuke(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, opts []*discordgo.ApplicationCommandInteractionDataOption) (*components.Container, error) {
	cfg := c.loadAntinuke(ctx, b, gid)
	if len(opts) == 0 {
		return c.show(ctx, b, gid)
	}

	wlAction, wlUser := "", ""
	for _, o := range opts {
		switch o.Name {
		case "state":
			cfg.Enabled = o.StringValue() == "on"
		case "punish":
			cfg.Punish = o.StringValue()
		case "threshold":
			cfg.Threshold = int(o.IntValue())
		case "window_seconds":
			cfg.WindowSeconds = int(o.IntValue())
		case "alert_channel":
			if ch := o.ChannelValue(s); ch != nil && ch.ID != "" {
				cfg.AlertChannelID = ch.ID
			}
		case "whitelist_action":
			wlAction = o.StringValue()
		case "whitelist_user":
			if u := o.UserValue(s); u != nil {
				wlUser = u.ID
			}
		}
	}

	switch {
	case wlAction == "add":
		if wlUser == "" {
			return b.Container(components.TextDisplay{Content: "Pick a whitelist_user to add."}), nil
		}
		cfg.Whitelist = appendWhitelist(cfg.Whitelist, wlUser)
	case wlAction == "remove":
		if wlUser == "" {
			return b.Container(components.TextDisplay{Content: "Pick a whitelist_user to remove."}), nil
		}
		cfg.Whitelist = removeWhitelist(cfg.Whitelist, wlUser)
	case wlAction == "clear":
		cfg.Whitelist = ""
	}

	if err := b.GetStore().SaveAntinukeConfig(ctx, cfg); err != nil {
		return b.Container(components.TextDisplay{Content: "Failed saving antinuke config: " + err.Error()}), nil
	}
	return c.show(ctx, b, gid)
}

func (c *ProtectCmd) prefixAntinuke(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid string, args []string) (*components.Container, error) {
	if len(args) == 0 {
		return c.show(ctx, b, gid)
	}
	cfg := c.loadAntinuke(ctx, b, gid)

	switch args[0] {
	case "on", "enable", "off", "disable":
		cfg.Enabled = args[0] == "on" || args[0] == "enable"
		if len(args) > 2 {
			if n, err := strconv.Atoi(args[2]); err == nil {
				cfg.Threshold = n
			}
		}
		if len(args) > 3 {
			if n, err := strconv.Atoi(args[3]); err == nil {
				cfg.WindowSeconds = n
			}
		}
	case "punish":
		if len(args) < 2 || !isPunishWord(args[1]) {
			return b.Container(components.TextDisplay{Content: "Usage: .protect antinuke punish timeout|kick|ban"}), nil
		}
		cfg.Punish = strings.ToLower(args[1])
	case "threshold":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Current threshold usage: .protect antinuke threshold <20-1000> [window_seconds]"}), nil
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return b.Container(components.TextDisplay{Content: "Threshold must be a number between 20 and 1000."}), nil
		}
		cfg.Threshold = n
		if len(args) > 2 {
			if w, err := strconv.Atoi(args[2]); err == nil {
				cfg.WindowSeconds = w
			}
		}
	case "alert":
		if len(args) < 2 || args[1] == "off" {
			cfg.AlertChannelID = ""
		} else {
			chID := commands.ParseMentionID(args[1])
			if len(chID) < 17 {
				if ch, err := s.Channel(args[1]); err == nil && ch != nil {
					chID = ch.ID
				}
			}
			if len(chID) < 17 {
				return b.Container(components.TextDisplay{Content: "Could not resolve a text channel from that argument."}), nil
			}
			cfg.AlertChannelID = chID
		}
	case "whitelist":
		sub := "list"
		if len(args) > 1 {
			sub = args[1]
		}
		switch sub {
		case "add", "remove":
			if len(args) < 3 {
				return b.Container(components.TextDisplay{Content: "Mention the user to " + sub + ": .protect antinuke whitelist " + sub + " @user"}), nil
			}
			uid := commands.ParseMentionID(args[2])
			if uid == "" {
				return b.Container(components.TextDisplay{Content: "Could not resolve that user mention."}), nil
			}
			if sub == "add" {
				cfg.Whitelist = appendWhitelist(cfg.Whitelist, uid)
			} else {
				cfg.Whitelist = removeWhitelist(cfg.Whitelist, uid)
			}
		case "clear":
			cfg.Whitelist = ""
		default:
			list := "(empty)"
			if cfg.Whitelist != "" {
				list = renderIDs(cfg.Whitelist)
			}
			return b.Container(
				components.TextDisplay{Content: "Antinuke Whitelist"},
				components.Separator{Divider: true, Spacing: 1},
				components.Section{Components: []discordgo.MessageComponent{
					components.TextDisplay{Content: list},
				}},
			), nil
		}
	default:
		return b.Container(components.TextDisplay{Content: "Usage: .protect antinuke on|off|punish|threshold|alert|whitelist"}), nil
	}

	if err := b.GetStore().SaveAntinukeConfig(ctx, cfg); err != nil {
		return b.Container(components.TextDisplay{Content: "Failed saving antinuke config: " + err.Error()}), nil
	}
	return c.show(ctx, b, gid)
}


func appendWhitelist(csv, id string) string {
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) == id {
			return csv
		}
	}
	if csv == "" {
		return id
	}
	return csv + "," + id
}

func removeWhitelist(csv, id string) string {
	parts := strings.Split(csv, ",")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != id {
			kept = append(kept, strings.TrimSpace(part))
		}
	}
	return strings.Join(kept, ",")
}

func renderIDs(csv string) string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, "<@"+p+">")
		}
	}
	return strings.Join(out, ", ")
}

func (c *ProtectCmd) loadAntinuke(ctx context.Context, b commands.BotInterface, gid string) *store.AntinukeConfig {
	cfg, err := b.GetStore().GetAntinukeConfig(ctx, gid)
	if err != nil {
		cfg = &store.AntinukeConfig{
			GuildID:       gid,
			Punish:        "ban",
			Threshold:     100,
			WindowSeconds: 60,
		}
	}
	return cfg
}

func (c *ProtectCmd) load(ctx context.Context, b commands.BotInterface, gid string) *store.ProtectionConfig {
	cfg, err := b.GetStore().GetProtectionConfig(ctx, gid)
	if err != nil {
		cfg = &store.ProtectionConfig{
			GuildID:      gid,
			AntispamMsgs: 6, AntispamWindow: 5,
			AntilinkMode:   "off",
			HoneypotAction: "ban",
		}
	}
	return cfg
}

func (c *ProtectCmd) filter(ctx context.Context, b commands.BotInterface, cfg *store.ProtectionConfig, action, word string) (*components.Container, error) {
	var words []string
	if cfg.FilterWords != "" {
		words = strings.Split(cfg.FilterWords, ",")
	}

	switch action {
	case "add":
		if word == "" {
			return b.Container(components.TextDisplay{Content: "Give a word to add."}), nil
		}
		for _, w := range words {
			if w == word {
				return b.Container(components.TextDisplay{Content: "`" + word + "` is already filtered."}), nil
			}
		}
		words = append(words, word)
	case "remove":
		if word == "" {
			return b.Container(components.TextDisplay{Content: "Give a word to remove."}), nil
		}
		kept := words[:0]
		for _, w := range words {
			if w != word {
				kept = append(kept, w)
			}
		}
		words = kept
	case "clear":
		words = nil
	default:
		if len(words) == 0 {
			return b.Container(components.TextDisplay{Content: "The filter list is empty."}), nil
		}
		return b.Container(
			components.TextDisplay{Content: "Filtered Words (" + strconv.Itoa(len(words)) + ")"},
			components.Separator{Divider: true, Spacing: 1},
			components.Section{Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: strings.Join(words, ", ")},
			}},
		), nil
	}

	cfg.FilterWords = strings.Join(words, ",")
	if err := b.GetStore().SaveProtectionConfig(ctx, cfg); err != nil {
		return b.Container(components.TextDisplay{Content: "Failed saving filter: " + err.Error()}), nil
	}
	status := "empty"
	if len(words) > 0 {
		status = strconv.Itoa(len(words)) + " words: " + strings.Join(words, ", ")
	}
	return b.Container(
		components.TextDisplay{Content: "Filter Updated"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: []discordgo.MessageComponent{
			components.TextDisplay{Content: status},
		}},
	), nil
}

func (c *ProtectCmd) show(ctx context.Context, b commands.BotInterface, gid string) (*components.Container, error) {
	lines := make([]string, 0, 8)

	cfg, err := b.GetStore().GetProtectionConfig(ctx, gid)
	if err != nil {
		cfg = &store.ProtectionConfig{AntilinkMode: "off"}
	}
	spam := "off"
	if cfg.AntispamEnabled {
		spam = "on (" + strconv.Itoa(cfg.AntispamMsgs) + " msgs / " + strconv.Itoa(cfg.AntispamWindow) + "s)"
	}
	filterCount := 0
	if cfg.FilterWords != "" {
		filterCount = len(strings.Split(cfg.FilterWords, ","))
	}
	trap := "not set"
	if cfg.HoneypotChannel != "" {
		trap = "<#" + cfg.HoneypotChannel + "> (" + cfg.HoneypotAction + ")"
	}
	lines = append(lines,
		"Antispam: "+spam,
		"Antilink mode: "+cfg.AntilinkMode,
		"Filtered words: "+strconv.Itoa(filterCount),
		"Honeypot: "+trap,
	)

	acfg, err := b.GetStore().GetAntinukeConfig(ctx, gid)
	if err != nil {
		acfg = &store.AntinukeConfig{}
	}
	state := "off"
	if acfg.Enabled {
		state = "on"
	}
	wl := 0
	if acfg.Whitelist != "" {
		wl = len(strings.Split(acfg.Whitelist, ","))
	}
	alert := "mod log"
	if acfg.AlertChannelID != "" {
		alert = "<#" + acfg.AlertChannelID + ">"
	}
	lines = append(lines,
		"Antinuke: "+state+" ("+acfg.Punish+", score "+strconv.Itoa(acfg.Threshold)+" / "+strconv.Itoa(acfg.WindowSeconds)+"s)",
		"Antinuke whitelist: "+strconv.Itoa(wl)+" trusted",
		"Antinuke alerts: "+alert,
	)

	comps := make([]discordgo.MessageComponent, 0, len(lines))
	for _, l := range lines {
		comps = append(comps, components.TextDisplay{Content: l})
	}
	return b.Container(
		components.TextDisplay{Content: "Protection Settings"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: comps},
	), nil
}


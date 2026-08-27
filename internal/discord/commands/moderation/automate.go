package moderation

import (
	"context"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/automation"
	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)

type AutomateCmd struct{}

func (c *AutomateCmd) Name() string { return "automate" }
func (c *AutomateCmd) Description() string {
	return "Manage automation rules (trigger -> conditions -> actions)"
}
func (c *AutomateCmd) Category() string  { return "Configuration" }
func (c *AutomateCmd) Aliases() []string { return nil }

func (c *AutomateCmd) RequiredPermissions() *int64 {
	perms := int64(discordgo.PermissionManageGuild)
	return &perms
}

var automateTriggerChoices = []*discordgo.ApplicationCommandOptionChoice{
	{Name: "message_create", Value: "message_create"},
	{Name: "member_join", Value: "member_join"},
	{Name: "member_leave", Value: "member_leave"},
	{Name: "member_ban", Value: "member_ban"},
	{Name: "member_unban", Value: "member_unban"},
	{Name: "role_add", Value: "role_add"},
	{Name: "role_remove", Value: "role_remove"},
	{Name: "interval", Value: "interval"},
}

const automateUsage = `Usage:
.automate add <name> <trigger> key=value ...
Keys: interval=<dur> role=<id> channels=<ids|-ids> actors=any|bots|humans age=<dur>
 require=<ids> forbid=<ids> phrases=<a,b,c> pattern=<regex> links=bool mentions=<n>
 cooldown=<dur> counter=<n>/<window> actions=<list> template=<text>
Triggers: message_create, member_join, member_leave, member_ban, member_unban,
 role_add, role_remove, interval
Actions: delete, timeout:<1m-28d>, ban, kick, role_add:<id>, role_remove:<id>,
 dm, reply, channel:<id>, log, stop`

func (c *AutomateCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{Name: "add", Description: "Create a rule", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Name: "name", Description: "Rule name (letters, digits, _ or -)", Type: discordgo.ApplicationCommandOptionString, Required: true},
			{Name: "trigger", Description: "What fires the rule", Type: discordgo.ApplicationCommandOptionString, Required: true, Choices: automateTriggerChoices},
			{Name: "spec", Description: "key=value pairs, e.g. phrases=spam actions=timeout:10m,delete template=stop it", Type: discordgo.ApplicationCommandOptionString, Required: true},
		}},
		{Name: "list", Description: "List this server's rules", Type: discordgo.ApplicationCommandOptionSubCommand},
		{Name: "show", Description: "Show one rule in full", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Name: "name", Description: "Rule name", Type: discordgo.ApplicationCommandOptionString, Required: true},
		}},
		{Name: "enable", Description: "Enable a rule", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Name: "name", Description: "Rule name", Type: discordgo.ApplicationCommandOptionString, Required: true},
		}},
		{Name: "disable", Description: "Disable a rule without deleting it", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Name: "name", Description: "Rule name", Type: discordgo.ApplicationCommandOptionString, Required: true},
		}},
		{Name: "delete", Description: "Delete a rule", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Name: "name", Description: "Rule name", Type: discordgo.ApplicationCommandOptionString, Required: true},
		}},
		{Name: "test", Description: "Dry-run a rule against sample text (fires nothing)", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
			{Name: "name", Description: "Rule name", Type: discordgo.ApplicationCommandOptionString, Required: true},
			{Name: "text", Description: "Sample message content to evaluate", Type: discordgo.ApplicationCommandOptionString, Required: false},
		}},
	}
}

func (c *AutomateCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	if i.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return c.list(ctx, b, i.GuildID)
	}
	sub := opts[0]
	optStr := func(name string) string {
		for _, o := range sub.Options {
			if o.Name == name {
				return o.StringValue()
			}
		}
		return ""
	}
	switch sub.Name {
	case "add":
		uid := ""
		if i.Member != nil && i.Member.User != nil {
			uid = i.Member.User.ID
		}
		return c.add(ctx, b, i.GuildID, uid, optStr("name"), optStr("trigger"), optStr("spec"))
	case "show":
		return c.show(ctx, b, i.GuildID, optStr("name"))
	case "enable":
		return c.setEnabled(ctx, b, i.GuildID, optStr("name"), true)
	case "disable":
		return c.setEnabled(ctx, b, i.GuildID, optStr("name"), false)
	case "delete":
		return c.delete(ctx, b, i.GuildID, optStr("name"))
	case "test":
		uid := ""
		if i.Member != nil && i.Member.User != nil {
			uid = i.Member.User.ID
		}
		var roles []string
		if i.Member != nil {
			roles = i.Member.Roles
		}
		return c.test(ctx, b, s, i.GuildID, i.ChannelID, uid, roles, optStr("name"), optStr("text"))
	default:
		return c.list(ctx, b, i.GuildID)
	}
}

func (c *AutomateCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if m.GuildID == "" {
		return b.Container(components.TextDisplay{Content: "Command only available in guilds."}), nil
	}
	if len(args) == 0 || args[0] == "list" {
		return c.list(ctx, b, m.GuildID)
	}
	switch args[0] {
	case "add":
		if len(args) < 4 {
			return b.Container(components.TextDisplay{Content: automateUsage}), nil
		}
		uid := ""
		if m.Author != nil {
			uid = m.Author.ID
		}
		return c.add(ctx, b, m.GuildID, uid, args[1], args[2], strings.Join(args[3:], " "))
	case "show":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .automate show <name>"}), nil
		}
		return c.show(ctx, b, m.GuildID, args[1])
	case "enable", "on":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .automate enable <name>"}), nil
		}
		return c.setEnabled(ctx, b, m.GuildID, args[1], true)
	case "disable", "off":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .automate disable <name>"}), nil
		}
		return c.setEnabled(ctx, b, m.GuildID, args[1], false)
	case "delete", "del", "rm":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .automate delete <name>"}), nil
		}
		return c.delete(ctx, b, m.GuildID, args[1])
	case "test":
		if len(args) < 2 {
			return b.Container(components.TextDisplay{Content: "Usage: .automate test <name> [text]"}), nil
		}
		uid := ""
		var roles []string
		if m.Member != nil {
			roles = m.Member.Roles
		}
		if m.Author != nil {
			uid = m.Author.ID
		}
		return c.test(ctx, b, s, m.GuildID, m.ChannelID, uid, roles, args[1], strings.Join(args[2:], " "))
	default:
		return b.Container(components.TextDisplay{Content: automateUsage}), nil
	}
}


func parseSpec(input string) (*store.AutomationRule, error) {
	r := &store.AutomationRule{}
	rest := strings.TrimSpace(strings.Join(strings.Fields(input), " "))
	set := func(key, val string) error {
		switch key {
		case "interval", "every":
			r.TriggerArg = val
		case "role":
			r.TriggerArg = commands.ParseMentionID(val)
		case "channels", "channel":
			r.Channels = normalizeIDs(val)
		case "actors":
			r.Actors = val
		case "age":
			d := commands.ParseDurationArg(val)
			if d <= 0 {
				return errBad("age")
			}
			r.MinAccountAge = int64(d.Seconds())
		case "require":
			r.RequireRoles = normalizeIDs(val)
		case "forbid":
			r.ForbidRoles = normalizeIDs(val)
		case "phrases":
			r.Phrases = strings.ToLower(strings.Join(automation.SplitCSV(val), ","))
		case "pattern":
			r.Pattern = val
		case "links":
			v, err := strconv.ParseBool(val)
			if err != nil {
				return errBad("links")
			}
			r.Links = v
		case "mentions":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return errBad("mentions")
			}
			r.MinMentions = n
		case "cooldown":
			d := commands.ParseDurationArg(val)
			if d <= 0 {
				return errBad("cooldown")
			}
			r.CooldownSeconds = int64(d.Seconds())
		case "counter":
			nSlashW := strings.SplitN(val, "/", 2)
			if len(nSlashW) != 2 {
				return errorsNew("counter wants <n>/<window>, e.g. counter=5/30s")
			}
			n, err := strconv.Atoi(nSlashW[0])
			if err != nil {
				return errBad("counter")
			}
			w := commands.ParseDurationArg(nSlashW[1])
			if w <= 0 {
				return errBad("counter window")
			}
			r.CounterLimit, r.CounterWindow = n, int64(w.Seconds())
		case "actions":
			r.Actions = strings.Join(automation.SplitCSV(val), ",")
		default:
			return errorsNew("unknown key " + key + "=")
		}
		return nil
	}

	for rest != "" {
		sp := strings.IndexByte(rest, ' ')
		tok := rest
		if sp >= 0 {
			tok, rest = rest[:sp], strings.TrimSpace(rest[sp+1:])
		} else {
			rest = ""
		}
		eq := strings.IndexByte(tok, '=')
		if eq <= 0 {
			return nil, errorsNew("expected key=value, got " + tok)
		}
		key, val := tok[:eq], tok[eq+1:]
		if key == "template" {
			if val == "" && rest != "" {
				val, rest = rest, ""
			}
			r.Template = val
			continue
		}
		if val == "" {
			return nil, errorsNew("key " + key + "= needs a value")
		}
		if err := set(key, val); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func normalizeIDs(val string) string {
	parts := automation.SplitCSV(val)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		ignore := strings.HasPrefix(p, "-")
		id := commands.ParseMentionID(strings.TrimPrefix(p, "-"))
		if ignore {
			id = "-" + id
		}
		out = append(out, id)
	}
	return strings.Join(out, ",")
}

type specError struct{ msg string }

func (e specError) Error() string { return e.msg }

func errBad(key string) error { return specError{"bad value for " + key + "="} }

func errorsNew(msg string) error { return specError{msg} }


func (c *AutomateCmd) add(ctx context.Context, b commands.BotInterface, gid, uid, name, trigger, specInput string) (*components.Container, error) {
	rule, err := parseSpec(specInput)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Spec error: " + err.Error()}), nil
	}
	rule.GuildID = gid
	rule.Name = strings.TrimSpace(name)
	rule.Trigger = trigger
	rule.Enabled = true
	rule.CreatedBy = uid

	if trigger == automation.TriggerInterval {
		rule.TriggerArg = strconv.FormatInt(int64(commands.ParseDurationArg(rule.TriggerArg).Seconds()), 10)
	}

	if err := automation.Validate(rule); err != nil {
		return b.Container(components.TextDisplay{Content: "Invalid rule: " + err.Error()}), nil
	}
	if err := b.GetStore().CreateAutomationRule(ctx, rule); err != nil {
		return b.Container(components.TextDisplay{Content: "Failed saving rule: " + err.Error()}), nil
	}
	bustCache(b, gid)

	lines := []string{
		"Trigger: " + rule.Trigger,
		"Actions: " + rule.Actions,
	}
	if rule.Template != "" {
		lines = append(lines, "Template: "+rule.Template)
	}
	return b.Container(
		components.TextDisplay{Content: "Rule Created: " + rule.Name},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: textDisplays(lines)},
	), nil
}

func (c *AutomateCmd) setEnabled(ctx context.Context, b commands.BotInterface, gid, name string, enabled bool) (*components.Container, error) {
	word := "disabled"
	if enabled {
		word = "enabled"
	}
	if err := b.GetStore().SetAutomationRuleEnabled(ctx, gid, name, enabled); err != nil {
		return b.Container(components.TextDisplay{Content: "No rule named `" + name + "`."}), nil
	}
	bustCache(b, gid)
	return b.Container(components.TextDisplay{Content: "Rule `" + name + "` " + word + "."}), nil
}

func (c *AutomateCmd) delete(ctx context.Context, b commands.BotInterface, gid, name string) (*components.Container, error) {
	if err := b.GetStore().DeleteAutomationRule(ctx, gid, name); err != nil {
		return b.Container(components.TextDisplay{Content: "No rule named `" + name + "`."}), nil
	}
	bustCache(b, gid)
	return b.Container(components.TextDisplay{Content: "Rule `" + name + "` deleted."}), nil
}

func (c *AutomateCmd) list(ctx context.Context, b commands.BotInterface, gid string) (*components.Container, error) {
	rules, err := b.GetStore().ListAutomationRules(ctx, gid)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Failed reading rules: " + err.Error()}), nil
	}
	if len(rules) == 0 {
		return b.Container(components.TextDisplay{Content: "No automation rules yet. See /automate add."}), nil
	}
	lines := make([]string, 0, len(rules))
	for _, r := range rules {
		status := "off"
		if r.Enabled {
			status = "on"
		}
		line := "- `" + r.Name + "` " + r.Trigger + " (" + status + ")"
		if r.Trigger == "interval" {
			line += " every " + r.TriggerArg + "s"
		}
		lines = append(lines, line)
	}
	return b.Container(
		components.TextDisplay{Content: "Automation Rules (" + strconv.Itoa(len(rules)) + ")"},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: textDisplays(lines),
		},
	), nil
}

func (c *AutomateCmd) show(ctx context.Context, b commands.BotInterface, gid, name string) (*components.Container, error) {
	r, err := b.GetStore().GetAutomationRule(ctx, gid, name)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Failed reading rules: " + err.Error()}), nil
	}
	if r == nil {
		return b.Container(components.TextDisplay{Content: "No rule named `" + name + "`."}), nil
	}

	var lines []string
	addLine := func(label, val string) {
		if val != "" && val != "0" {
			lines = append(lines, label+": "+val)
		}
	}
	status := "disabled"
	if r.Enabled {
		status = "enabled"
	}
	lines = append(lines,
		"Status: "+status,
		"Trigger: "+r.Trigger,
	)
	if r.TriggerArg != "" {
		if r.Trigger == "interval" {
			lines = append(lines, "Every: "+r.TriggerArg+"s")
		} else {
			lines = append(lines, "Pinned role: <@&"+r.TriggerArg+">")
		}
	}
	addLine("Channels", r.Channels)
	addLine("Actors", r.Actors)
	if r.MinAccountAge > 0 {
		lines = append(lines, "Min account age: "+strconv.FormatInt(r.MinAccountAge, 10)+"s")
	}
	addLine("Require roles", r.RequireRoles)
	addLine("Forbid roles", r.ForbidRoles)
	addLine("Phrases", r.Phrases)
	addLine("Pattern", "`"+r.Pattern+"`")
	if r.Links {
		lines = append(lines, "Requires link: yes")
	}
	if r.MinMentions > 0 {
		lines = append(lines, "Min mentions: "+strconv.Itoa(r.MinMentions))
	}
	if r.CooldownSeconds > 0 {
		lines = append(lines, "Cooldown: "+strconv.FormatInt(r.CooldownSeconds, 10)+"s per user")
	}
	if r.CounterLimit > 0 {
		lines = append(lines, "Counter: "+strconv.Itoa(r.CounterLimit)+" per "+strconv.FormatInt(r.CounterWindow, 10)+"s")
	}
	lines = append(lines, "Actions: "+r.Actions)
	if r.Template != "" {
		lines = append(lines, "Template: "+r.Template)
	}
	return b.Container(
		components.TextDisplay{Content: "Rule: " + r.Name},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: textDisplays(lines)},
	), nil
}

func (c *AutomateCmd) test(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, chID, uid string, roles []string, name, content string) (*components.Container, error) {
	row, err := b.GetStore().GetAutomationRule(ctx, gid, name)
	if err != nil {
		return b.Container(components.TextDisplay{Content: "Failed reading rules: " + err.Error()}), nil
	}
	if row == nil {
		return b.Container(components.TextDisplay{Content: "No rule named `" + name + "`."}), nil
	}

	e := automation.Event{
		Kind:      row.Trigger,
		GuildID:   gid,
		ChannelID: chID,
		UserID:    uid,
		Roles:     roles,
		Content:   content,
		Mentions:  strings.Count(content, "<@"),
		HasLink:   automation.ContainsLink(content),
	}
	if ch, _ := s.State.Channel(chID); ch != nil {
		e.ChannelName = ch.Name
	}

	verdict := automation.Compile(*row).Check(e)
	result := "MATCH  -  actions would fire: " + row.Actions
	if !verdict.OK {
		result = "no match: " + verdict.Reason
	}
	lines := []string{result}
	if content != "" {
		lines = append(lines, "Evaluated content: "+content)
	}
	lines = append(lines,
		"Gates are not consumed by tests.",
		"Interval rules fire from the scheduler, not from events.",
	)
	return b.Container(
		components.TextDisplay{Content: "Test: " + name},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{Components: textDisplays(lines)},
	), nil
}

func bustCache(b commands.BotInterface, gid string) {
	type cacheBuster interface{ InvalidateAutomationRules(gid string) }
	if cb, ok := b.(cacheBuster); ok {
		cb.InvalidateAutomationRules(gid)
	}
}

func textDisplays(lines []string) []discordgo.MessageComponent {
	comps := make([]discordgo.MessageComponent, 0, len(lines))
	for _, l := range lines {
		comps = append(comps, components.TextDisplay{Content: l})
	}
	return comps
}


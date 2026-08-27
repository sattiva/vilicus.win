package automation

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)

const (
	TriggerMessageCreate = "message_create"
	TriggerMemberJoin    = "member_join"
	TriggerMemberLeave   = "member_leave"
	TriggerMemberBan     = "member_ban"
	TriggerMemberUnban   = "member_unban"
	TriggerRoleAdd       = "role_add"
	TriggerRoleRemove    = "role_remove"
	TriggerInterval      = "interval"
)

type Event struct {
	Kind        string
	GuildID     string
	GuildName   string
	ChannelID   string
	ChannelName string
	MessageID   string
	UserID      string
	Username    string
	IsBot       bool
	Roles       []string
	Content     string
	Mentions    int
	HasLink     bool
	AccountAge  time.Duration
	RoleID      string
}

type Rule struct {
	store.AutomationRule
	includeChans map[string]bool
	ignoreChans  map[string]bool
	phraseSet    []string
	requireRoles map[string]bool
	forbidRoles  map[string]bool
	pattern      *regexp.Regexp
}

func Compile(r store.AutomationRule) *Rule {
	cr := &Rule{AutomationRule: r}
	for _, c := range SplitCSV(r.Channels) {
		if strings.HasPrefix(c, "-") {
			if cr.ignoreChans == nil {
				cr.ignoreChans = map[string]bool{}
			}
			cr.ignoreChans[strings.TrimPrefix(c, "-")] = true
		} else {
			if cr.includeChans == nil {
				cr.includeChans = map[string]bool{}
			}
			cr.includeChans[c] = true
		}
	}
	cr.phraseSet = lowerAll(SplitCSV(r.Phrases))
	for _, id := range SplitCSV(r.RequireRoles) {
		if cr.requireRoles == nil {
			cr.requireRoles = map[string]bool{}
		}
		cr.requireRoles[id] = true
	}
	for _, id := range SplitCSV(r.ForbidRoles) {
		if cr.forbidRoles == nil {
			cr.forbidRoles = map[string]bool{}
		}
		cr.forbidRoles[id] = true
	}
	if r.Pattern != "" {
		if re, err := regexp.Compile(r.Pattern); err == nil {
			cr.pattern = re
		}
	}
	return cr
}

func SplitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

type Verdict struct {
	OK     bool
	Reason string
}

func (r *Rule) Check(e Event) Verdict {
	if r.Trigger != e.Kind {
		return Verdict{false, "trigger " + r.Trigger + " != event " + e.Kind}
	}
	if (e.Kind == TriggerRoleAdd || e.Kind == TriggerRoleRemove) && r.TriggerArg != "" && r.TriggerArg != e.RoleID {
		return Verdict{false, "role " + e.RoleID + " not pinned role " + r.TriggerArg}
	}

	switch {
	case r.includeChans != nil && !r.includeChans[e.ChannelID]:
		return Verdict{false, "channel not in include list"}
	case r.ignoreChans[e.ChannelID]:
		return Verdict{false, "channel ignored"}
	}

	switch r.Actors {
	case "bots":
		if !e.IsBot {
			return Verdict{false, "actor is human"}
		}
	case "humans":
		if e.IsBot {
			return Verdict{false, "actor is a bot"}
		}
	}

	if r.MinAccountAge > 0 && e.AccountAge < time.Duration(r.MinAccountAge)*time.Second {
		return Verdict{false, "account age below minimum"}
	}

	if r.requireRoles != nil || r.forbidRoles != nil {
		var have map[string]bool
		if len(e.Roles) > 0 {
			have = map[string]bool{}
			for _, id := range e.Roles {
				have[id] = true
			}
		}
		for id := range r.requireRoles {
			if have == nil || !have[id] {
				return Verdict{false, "missing required role " + id}
			}
		}
		for id := range r.forbidRoles {
			if have != nil && have[id] {
				return Verdict{false, "has forbidden role " + id}
			}
		}
	}

	lower := strings.ToLower(e.Content)
	if len(r.phraseSet) > 0 && !containsAnyPhrase(lower, r.phraseSet) {
		return Verdict{false, "no phrase matched"}
	}
	if r.pattern != nil && !r.pattern.MatchString(e.Content) {
		return Verdict{false, "pattern did not match"}
	}
	if r.Links && !e.HasLink {
		return Verdict{false, "no link or invite present"}
	}
	if r.MinMentions > 0 && e.Mentions < r.MinMentions {
		return Verdict{false, "mention count below minimum"}
	}
	return Verdict{OK: true}
}

func containsAnyPhrase(lower string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}


type Action struct {
	Kind string
	Arg  string
}

func ParseActions(spec string) []Action {
	var out []Action
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		a := Action{}
		if i := strings.Index(part, ":"); i >= 0 {
			a.Kind, a.Arg = part[:i], part[i+1:]
		} else {
			a.Kind = part
		}
		out = append(out, a)
	}
	return out
}


var automationNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

var snowflakeRe = regexp.MustCompile(`^\d{15,21}$`)

var linkRe = regexp.MustCompile(`(?i)(discord\.gg/|discord(?:app)?\.com/invite/|https?://\S+)`)

func ContainsLink(content string) bool { return linkRe.MatchString(content) }

const (
	maxTimeout   = 28 * 24 * time.Hour
	maxAccAge    = 365 * 24 * time.Hour
	counterLimit = 100
)

func Validate(r *store.AutomationRule) error {
	if !store.ValidAutomationTriggers[r.Trigger] {
		return fmt.Errorf("unknown trigger %q", r.Trigger)
	}
	if r.Name == "" || r.GuildID == "" {
		return errors.New("rule needs guild and name")
	}
	if !automationNameRe.MatchString(r.Name) {
		return errors.New("name must be 1-32 chars: letters, digits, _ or -")
	}
	switch r.Actors {
	case "", "any", "bots", "humans":
	default:
		return fmt.Errorf("actors must be any|bots|humans, got %q", r.Actors)
	}

	isMsg := r.Trigger == TriggerMessageCreate
	isInterval := r.Trigger == TriggerInterval

	if isInterval {
		secs, ok := parseSecondsArg(r.TriggerArg)
		if !ok || secs < 15 {
			return errors.New("interval rules need interval=15s or longer")
		}
	} else if r.TriggerArg != "" &&
		!(r.Trigger == TriggerRoleAdd || r.Trigger == TriggerRoleRemove) {
		return fmt.Errorf("trigger %s takes no argument", r.Trigger)
	}
	if (r.Trigger == TriggerRoleAdd || r.Trigger == TriggerRoleRemove) && r.TriggerArg != "" && !snowflakeRe.MatchString(r.TriggerArg) {
		return errors.New("pinned role must be a role id")
	}

	if !isMsg {
		if r.Phrases != "" || r.Pattern != "" || r.Links || r.MinMentions > 0 {
			return errors.New("phrases/pattern/links/mentions apply only to message_create rules")
		}
	}

	for _, field := range [][]string{SplitCSV(r.Channels), SplitCSV(r.RequireRoles), SplitCSV(r.ForbidRoles)} {
		for _, v := range field {
			id := strings.TrimPrefix(v, "-")
			if !snowflakeRe.MatchString(id) {
				return fmt.Errorf("%q is not a snowflake id", v)
			}
		}
	}

	if r.Pattern != "" {
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("pattern does not compile: %v", err)
		}
	}
	if r.MinAccountAge < 0 || time.Duration(r.MinAccountAge)*time.Second > maxAccAge {
		return errors.New("account age minimum must be 0 to 365d")
	}

	if isInterval {
		for _, a := range ParseActions(r.Actions) {
			switch a.Kind {
			case "channel", "log":
				if a.Kind == "channel" && !snowflakeRe.MatchString(a.Arg) {
					return errors.New("channel:<id> needs a channel id")
				}
			default:
				return fmt.Errorf("interval rules support channel:<id> and log actions only, got %q", a.Kind)
			}
		}
		return validateGates(r)
	}

	sawOutput := false
	for _, a := range ParseActions(r.Actions) {
		sawOutput = true
		switch a.Kind {
		case "delete", "reply":
			if !isMsg {
				return fmt.Errorf("action %q applies only to message_create rules", a.Kind)
			}
		case "timeout":
			dur := commands.ParseDurationArg(a.Arg)
			if dur <= 0 || dur > maxTimeout {
				return errors.New("timeout duration must be between 1m and 28d")
			}
		case "ban", "kick", "dm", "log", "stop":
		case "role_add", "role_remove":
			if !snowflakeRe.MatchString(a.Arg) {
				return errors.New(a.Kind + ":<role_id> needs a role id")
			}
		case "channel":
			if !snowflakeRe.MatchString(a.Arg) {
				return errors.New("channel:<id> needs a channel id")
			}
		default:
			return fmt.Errorf("unknown action %q", a.Kind)
		}
	}
	if !sawOutput {
		return errors.New("rule needs at least one action")
	}
	return validateGates(r)
}

func validateGates(r *store.AutomationRule) error {
	if r.CooldownSeconds < 0 || r.CooldownSeconds > 7*24*3600 {
		return errors.New("cooldown must be 0 to 7d")
	}
	if r.CounterLimit != 0 {
		if r.CounterLimit < 2 || r.CounterLimit > counterLimit {
			return fmt.Errorf("counter limit must be 2..%d", counterLimit)
		}
		w := r.CounterWindow
		if w < 10 || w > 7*24*3600 {
			return errors.New("counter window must be 10s..7d")
		}
	}
	if len(r.Template) > 1500 {
		return errors.New("template too long (max 1500 chars)")
	}
	return nil
}

func parseSecondsArg(s string) (int64, bool) {
	d := commands.ParseDurationArg(s)
	if d <= 0 {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return 0, false
		}
		return int64(n), true
	}
	return int64(d.Seconds()), true
}

func Key(ruleID int64, uid string) string {
	return strconv.FormatInt(ruleID, 10) + ":" + uid
}

func ExpandTemplate(tpl, ruleName string, e Event) string {
	rep := []string{
		"{user}", e.Username,
		"{user.name}", e.Username,
		"{user.id}", e.UserID,
		"{user.mention}", "<@" + e.UserID + ">",
		"{guild}", e.GuildName,
		"{guild.name}", e.GuildName,
		"{guild.id}", e.GuildID,
		"{channel.name}", e.ChannelName,
		"{channel.id}", e.ChannelID,
		"{channel.mention}", "<#" + e.ChannelID + ">",
		"{message.id}", e.MessageID,
		"{content}", e.Content,
		"{rule}", ruleName,
		"{timestamp}", strconv.FormatInt(time.Now().Unix(), 10),
		"{role.id}", e.RoleID,
	}
	return strings.NewReplacer(rep...).Replace(tpl)
}


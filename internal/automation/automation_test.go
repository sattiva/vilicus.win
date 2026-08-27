package automation

import (
	"strings"
	"testing"
	"time"

	"vilicus/internal/store"
)

func rule(t *testing.T, mutate func(*store.AutomationRule)) *Rule {
	t.Helper()
	r := store.AutomationRule{
		GuildID: "g1", Name: "t", Enabled: true,
		Trigger: TriggerMessageCreate,
		Actions: "log",
	}
	if mutate != nil {
		mutate(&r)
	}
	if err := Validate(&r); err != nil {
		t.Fatalf("validate: %v", err)
	}
	return Compile(r)
}

var baseEvent = Event{
	Kind: "message_create", GuildID: "g1", ChannelID: "111111111111111111",
	UserID: "u1", Username: "joe",
}

func TestCheckOrdering(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*store.AutomationRule)
		event  func(*Event)
		wantOK bool
		reason string
	}{
		{"clean match", nil, nil, true, ""},
		{"wrong trigger", nil, func(e *Event) { e.Kind = "member_join" }, false, "trigger"},
		{
			"ignore channel",
			func(r *store.AutomationRule) { r.Channels = "-111111111111111111" },
			nil, false, "ignored",
		},
		{
			"include channel miss",
			func(r *store.AutomationRule) { r.Channels = "222222222222222222" },
			nil, false, "include list",
		},
		{
			"include channel hit",
			func(r *store.AutomationRule) { r.Channels = "111111111111111111" },
			nil, true, "",
		},
		{
			"bots only vs human",
			func(r *store.AutomationRule) { r.Actors = "bots" },
			nil, false, "human",
		},
		{
			"humans only rejects bot",
			func(r *store.AutomationRule) { r.Actors = "humans" },
			func(e *Event) { e.IsBot = true }, false, "actor is a bot",
		},
		{
			"account too young",
			func(r *store.AutomationRule) { r.MinAccountAge = 86400 },
			func(e *Event) { e.AccountAge = time.Hour }, false, "account age",
		},
		{
			"account old enough",
			func(r *store.AutomationRule) { r.MinAccountAge = 3600 },
			func(e *Event) { e.AccountAge = 2 * time.Hour }, true, "",
		},
		{
			"missing required role fails closed without roles",
			func(r *store.AutomationRule) { r.RequireRoles = "555555555555555555" },
			nil, false, "required role",
		},
		{
			"required role present",
			func(r *store.AutomationRule) { r.RequireRoles = "555555555555555555" },
			func(e *Event) { e.Roles = []string{"555555555555555555"} }, true, "",
		},
		{
			"forbidden role blocks",
			func(r *store.AutomationRule) { r.ForbidRoles = "666666666666666666" },
			func(e *Event) { e.Roles = []string{"666666666666666666"} }, false, "forbidden role",
		},
		{
			"phrase case-insensitive",
			func(r *store.AutomationRule) { r.Phrases = "SPAM" },
			func(e *Event) { e.Content = "total Spam here" }, true, "",
		},
		{
			"phrase miss",
			func(r *store.AutomationRule) { r.Phrases = "spam" },
			func(e *Event) { e.Content = "hello world" }, false, "no phrase",
		},
		{
			"pattern hit",
			func(r *store.AutomationRule) { r.Pattern = `(?i)free\s+nitro` },
			func(e *Event) { e.Content = "get FREE Nitro now" }, true, "",
		},
		{
			"pattern miss",
			func(r *store.AutomationRule) { r.Pattern = `free\s+nitro` },
			func(e *Event) { e.Content = "nothing to see" }, false, "pattern",
		},
		{
			"link required but absent",
			func(r *store.AutomationRule) { r.Links = true },
			nil, false, "no link",
		},
		{
			"link present",
			func(r *store.AutomationRule) { r.Links = true },
			func(e *Event) { e.Content = "join https://x.example now"; e.HasLink = true }, true, "",
		},
		{
			"mentions below minimum",
			func(r *store.AutomationRule) { r.MinMentions = 3 },
			func(e *Event) { e.Mentions = 2 }, false, "mention",
		},
		{
			"role trigger pin mismatch",
			func(r *store.AutomationRule) {
				r.Trigger = TriggerRoleAdd
				r.TriggerArg = "777777777777777777"
				r.Actions = "dm"
			},
			func(e *Event) { e.Kind = TriggerRoleAdd; e.RoleID = "888888888888888888" },
			false, "pinned role",
		},
	}
	for _, tc := range cases {
		r := rule(t, tc.mutate)
		e := baseEvent
		if tc.event != nil {
			tc.event(&e)
		}
		got := r.Check(e)
		if got.OK != tc.wantOK || (tc.reason != "" && !strings.Contains(got.Reason, tc.reason)) {
			t.Errorf("%s: got ok=%v reason=%q, want ok=%v reason~%q",
				tc.name, got.OK, got.Reason, tc.wantOK, tc.reason)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	bad := []struct {
		name   string
		mutate func(*store.AutomationRule)
	}{
		{"dead trigger (the reference bot's bug)", func(r *store.AutomationRule) { r.Trigger = "message_delete" }},
		{"unknown action", func(r *store.AutomationRule) { r.Actions = "nuke" }},
		{"no actions", func(r *store.AutomationRule) { r.Actions = "" }},
		{"reply on member event", func(r *store.AutomationRule) { r.Trigger = "member_join"; r.Actions = "reply" }},
		{"phrases on ban event", func(r *store.AutomationRule) { r.Trigger = "member_ban"; r.Phrases = "x" }},
		{"bad pattern", func(r *store.AutomationRule) { r.Pattern = "(unclosed" }},
		{"timeout over 28d", func(r *store.AutomationRule) { r.Actions = "timeout:400d" }},
		{"interval too short", func(r *store.AutomationRule) {
			r.Trigger = "interval"
			r.TriggerArg = "5s"
			r.Actions = "channel:111111111111111111"
		}},
		{"ban on interval", func(r *store.AutomationRule) { r.Trigger = "interval"; r.TriggerArg = "60s"; r.Actions = "ban" }},
		{"counter limit 1", func(r *store.AutomationRule) { r.CounterLimit = 1; r.CounterWindow = 30 }},
		{"bad name", func(r *store.AutomationRule) { r.Name = "has space" }},
		{"bad channel id", func(r *store.AutomationRule) { r.Channels = "abc" }},
	}
	for _, tc := range bad {
		r := store.AutomationRule{GuildID: "g1", Name: "ok-name-1", Trigger: TriggerMessageCreate, Actions: "log"}
		tc.mutate(&r)
		if err := Validate(&r); err == nil {
			t.Errorf("%s: expected rejection", tc.name)
		}
	}

	good := store.AutomationRule{GuildID: "g1", Name: "fine", Trigger: TriggerInterval, TriggerArg: "10m", Actions: "channel:111111111111111111"}
	if err := Validate(&good); err != nil {
		t.Errorf("valid interval rejected: %v", err)
	}
}

func TestParseAndExpand(t *testing.T) {
	acts := ParseActions("delete, timeout:10m , role_add:123456789012345678")
	if len(acts) != 3 || acts[0].Kind != "delete" || acts[1].Arg != "10m" || acts[2].Kind != "role_add" {
		t.Fatalf("parseActions wrong: %+v", acts)
	}

	e := Event{
		GuildName: "Testers", ChannelID: "42", UserID: "7", Username: "jo",
		MessageID: "9", Content: "hi there", RoleID: "5",
	}
	got := ExpandTemplate("{user.mention} welcome to {guild} in #{channel.name} ({rule}) msg={message.id} role={role.id}", "welcome", e)
	want := "<@7> welcome to Testers in # (welcome) msg=9 role=5"
	if got != want {
		t.Fatalf("expand:\n got %q\nwant %q", got, want)
	}
	if ExpandTemplate("{nope}", "r", e) != "{nope}" {
		t.Fatal("unknown tokens must pass through")
	}
}

func TestKeyStable(t *testing.T) {
	if Key(12, "u") != "12:u" || Key(12, "u") == Key(21, "u") {
		t.Fatal("key layout broken")
	}
}


package store

import (
	"context"
	"testing"
	"time"
)

func TestAutomationRuleCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	r := &AutomationRule{
		GuildID:   "g1",
		Name:      "welcome",
		Enabled:   true,
		Trigger:   "member_join",
		Actions:   "channel:123",
		Template:  "hi {user.mention}",
		Channels:  "-999",
		Actors:    "humans",
		CreatedBy: "mod1",
	}
	if err := st.CreateAutomationRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	if r.ID == 0 {
		t.Fatal("expected inserted id")
	}

	err := st.CreateAutomationRule(ctx, &AutomationRule{GuildID: "g1", Name: "welcome", Trigger: "member_join"})
	if err == nil {
		t.Fatal("duplicate rule name should fail")
	}

	got, err := st.GetAutomationRule(ctx, "g1", "welcome")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.TriggerArg != "" || !got.Enabled || got.Actors != "humans" || got.Channels != "-999" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.LastRun != nil {
		t.Fatal("fresh rule must have nil LastRun")
	}

	if err := st.CreateAutomationRule(ctx, &AutomationRule{GuildID: "g1", Name: "bad", Trigger: "message_delete"}); err == nil {
		t.Fatal("dead trigger must be rejected")
	}

	if err := st.SetAutomationRuleEnabled(ctx, "g1", "welcome", false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetAutomationRuleEnabled(ctx, "g1", "missing", true); err == nil {
		t.Fatal("enable of missing rule should fail")
	}

	rules, err := st.ListAutomationRules(ctx, "g1")
	if err != nil || len(rules) != 1 {
		t.Fatalf("list: %d rules, err=%v", len(rules), err)
	}
	if rules[0].Enabled {
		t.Fatal("rule should be disabled")
	}

	if err := st.DeleteAutomationRule(ctx, "g1", "welcome"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAutomationRule(ctx, "g1", "welcome"); err == nil {
		t.Fatal("double delete should fail")
	}
}

func TestDueIntervalRules(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	mk := func(name, period string, ago time.Duration) *AutomationRule {
		r := &AutomationRule{
			GuildID: "g1", Name: name, Enabled: true,
			Trigger: "interval", TriggerArg: period,
			Actions: "channel:42", Template: "daily ping",
		}
		if err := st.CreateAutomationRule(ctx, r); err != nil {
			t.Fatal(err)
		}
		if ago > 0 {
			past := time.Now().UTC().Add(-ago)
			if _, err := st.db.Exec(`UPDATE automation_rules SET last_run = ? WHERE id = ?`, past, r.ID); err != nil {
				t.Fatal(err)
			}
		}
		return r
	}

	due := mk("every-run", "15", 0)
	mk("stale", "60", 5*time.Minute)
	mk("recent", "3600", time.Minute)
	mk("broken", "abc", 0)

	got, err := st.DueIntervalRules(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range got {
		names[r.Name] = true
	}
	if len(got) != 2 || !names["every-run"] || !names["stale"] {
		t.Fatalf("want [every-run stale], got %v", names)
	}

	if err := st.TouchAutomationRuleRun(ctx, due.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := st.GetAutomationRule(ctx, "g1", "every-run")
	if fresh.LastRun == nil || time.Since(*fresh.LastRun) > time.Minute {
		t.Fatalf("last_run not stamped: %v", fresh.LastRun)
	}
}


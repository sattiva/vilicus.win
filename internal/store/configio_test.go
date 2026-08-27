package store

import (
	"context"
	"testing"
)

func TestGuildBundleRoundtrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if err := st.SaveGuildConfig(ctx, &GuildConfig{
		GuildID: "g1", Prefix: "!", LogChannelID: "c-log", WelcomeChannelID: "c-welcome", AutoRoleID: "r-auto", JailRoleID: "r-jail",
	}); err != nil {
		t.Fatalf("save guild: %v", err)
	}
	if err := st.SaveProtectionConfig(ctx, &ProtectionConfig{
		GuildID: "g1", AntispamEnabled: true, AntispamMsgs: 6, AntispamWindow: 10,
		AntilinkMode: "mods", FilterWords: "Scam, Spam", HoneypotChannel: "c-honey", HoneypotAction: "kick",
	}); err != nil {
		t.Fatalf("save protection: %v", err)
	}
	if err := st.SaveStarboardConfig(ctx, &StarboardConfig{GuildID: "g1", ChannelID: "c-star", Threshold: 3, Enabled: true}); err != nil {
		t.Fatalf("save starboard: %v", err)
	}
	rule := &AutomationRule{GuildID: "g1", Name: "invite-block", Enabled: true, Trigger: "message_create",
		Links: true, Actions: "delete,warn", CreatedBy: "mod1"}
	if err := st.CreateAutomationRule(ctx, rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	b, err := st.ExportGuildBundle(ctx, "g1")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if b.Version != BundleVersion || b.Guild == nil || b.Protection == nil || b.Starboard == nil || len(b.AutomationRules) != 1 {
		t.Fatalf("bundle missing sections: %+v", b)
	}

	applied, err := st.ImportGuildBundle(ctx, b, "g2")
	if err != nil {
		t.Fatalf("import: %v (applied=%v)", err, applied)
	}
	if len(applied) != 4 {
		t.Fatalf("applied = %v, want all four sections", applied)
	}

	g2, _ := st.GetGuildConfig(ctx, "g2")
	if g2.Prefix != "!" || g2.LogChannelID != "c-log" || g2.JailRoleID != "r-jail" {
		t.Fatalf("guild section not cloned: %+v", g2)
	}
	p2, err := st.GetProtectionConfig(ctx, "g2")
	if err != nil || !p2.AntispamEnabled || p2.FilterWords != "scam,spam" {
		t.Fatalf("protection section not cloned: %+v err=%v", p2, err)
	}
	sb2, err := st.GetStarboardConfig(ctx, "g2")
	if err != nil || sb2.ChannelID != "c-star" || sb2.Threshold != 3 {
		t.Fatalf("starboard section not cloned: %+v err=%v", sb2, err)
	}
	rules2, _ := st.ListAutomationRules(ctx, "g2")
	if len(rules2) != 1 || rules2[0].Name != "invite-block" || rules2[0].Trigger != "message_create" || !rules2[0].Links {
		t.Fatalf("automation section not cloned: %+v", rules2)
	}

	b.AutomationRules = nil
	if _, err := st.ImportGuildBundle(ctx, b, "g2"); err != nil {
		t.Fatalf("re-import without rules: %v", err)
	}
	rules2, _ = st.ListAutomationRules(ctx, "g2")
	if len(rules2) != 0 {
		t.Fatalf("replace-all failed, still have: %+v", rules2)
	}
}

func TestGuildBundleRejectsBadRules(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if err := st.CreateAutomationRule(ctx, &AutomationRule{GuildID: "g1", Name: "keepme", Trigger: "member_join", Actions: "log"}); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	b := &GuildConfigBundle{Version: BundleVersion, GuildID: "g9", AutomationRules: []AutomationRule{
		{Name: "bad", Trigger: "message_delete", Actions: "log"},
	}}

	if _, err := st.ImportGuildBundle(ctx, b, "g1"); err == nil {
		t.Fatal("import with unknown trigger should fail")
	}
	rules, _ := st.ListAutomationRules(ctx, "g1")
	if len(rules) != 1 || rules[0].Name != "keepme" {
		t.Fatalf("validation must be pre-delete, dest rules now: %+v", rules)
	}

	b2 := &GuildConfigBundle{Version: 99, GuildID: "g9"}
	if _, err := st.ImportGuildBundle(ctx, b2, "g1"); err == nil {
		t.Fatal("import with future version should fail")
	}
}


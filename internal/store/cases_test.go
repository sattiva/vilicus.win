package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestMigrateFreshDB(t *testing.T) {
	st := openTestStore(t)

	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != 10 {
		t.Fatalf("want user_version 10, got %d", version)
	}

	for _, table := range []string{
		"mod_cases", "case_notes", "dashboard_audit_log", "reminders", "temp_roles", "temp_bans",
		"starboard_config", "starboard_posts", "role_bindings", "protection_config",
		"user_xp", "giveaways", "giveaway_entries", "automation_rules", "antinuke_config",
		"jail_backups", "guild_admins", "cases_fts",
	} {
		var name string
		err := st.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing after migrate: %v", table, err)
		}
	}
}

func TestCaseNumberingAndRetrieval(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	gid := "g1"

	for i := 0; i < 5; i++ {
		if _, err := st.CreateCase(ctx, gid, "warn", "mod1", "target1", "r", 0, nil, "discord", ""); err != nil {
			t.Fatalf("create case %d: %v", i, err)
		}
	}

	cs, err := st.GetCaseByNumber(ctx, gid, 3)
	if err != nil {
		t.Fatalf("get case 3: %v", err)
	}
	if cs.CaseNo != 3 || cs.Type != "warn" || cs.TargetID != "target1" || !cs.Active {
		t.Fatalf("unexpected case row: %+v", cs)
	}

	if _, err := st.GetCaseByNumber(ctx, gid, 99); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("want ErrCaseNotFound, got %v", err)
	}

	if _, err := st.CreateCase(ctx, "g2", "ban", "mod2", "t2", "", 0, nil, "discord", ""); err != nil {
		t.Fatal(err)
	}
	if cs, _ := st.GetCaseByNumber(ctx, "g2", 1); cs == nil || cs.CaseNo != 1 {
		t.Fatal("second guild should start numbering at 1")
	}
}

func TestCaseNotesReasonDeactivate(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	cs, err := st.CreateCase(ctx, "g1", "tempban", "mod1", "t1", "old reason", 3600,
		nil, "discord", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.AddCaseNote(ctx, cs.ID, "staffer", "first note"); err != nil {
		t.Fatalf("add note: %v", err)
	}
	notes, err := st.ListCaseNotes(ctx, cs.ID)
	if err != nil || len(notes) != 1 || notes[0].Body != "first note" {
		t.Fatalf("notes = %+v err = %v", notes, err)
	}

	if err := st.UpdateCaseReason(ctx, "g1", cs.CaseNo, "new reason"); err != nil {
		t.Fatalf("update reason: %v", err)
	}
	cs, _ = st.GetCaseByNumber(ctx, "g1", cs.CaseNo)
	if cs.Reason != "new reason" {
		t.Fatalf("reason not updated: %q", cs.Reason)
	}

	if err := st.DeactivateCase(ctx, "g1", cs.CaseNo); err != nil {
		t.Fatal(err)
	}
	cs, _ = st.GetCaseByNumber(ctx, "g1", cs.CaseNo)
	if cs.Active {
		t.Fatal("case should be inactive")
	}
}

func TestCaseGuildSummaries(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		st.CreateCase(ctx, "ga", "warn", "m", "x", "", 0, nil, "discord", "")
	}
	st.CreateCase(ctx, "gb", "kick", "m", "x", "", 0, nil, "discord", "")

	sums, err := st.CaseGuildSummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 2 || sums[0].GuildID != "ga" || sums[0].Count != 3 || sums[1].GuildID != "gb" {
		t.Fatalf("unexpected summaries: %+v", sums)
	}
}

func TestListCasesFiltered(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	st.CreateCase(ctx, "g1", "ban", "modA", "u1", "", 0, nil, "discord", "")
	st.CreateCase(ctx, "g1", "warn", "modA", "u2", "", 0, nil, "discord", "")
	st.CreateCase(ctx, "g1", "warn", "modB", "u1", "", 0, nil, "discord", "")
	st.CreateCase(ctx, "g2", "warn", "modA", "u1", "", 0, nil, "discord", "")

	all, err := st.ListCasesFiltered(ctx, "g1", "", CaseFilter{}, 100, 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("guild-wide unfiltered: %d %v", len(all), err)
	}

	byType, _ := st.ListCasesFiltered(ctx, "g1", "", CaseFilter{Type: "warn"}, 100, 0)
	if len(byType) != 2 {
		t.Fatalf("type filter: want 2 got %d", len(byType))
	}

	byMod, _ := st.ListCasesFiltered(ctx, "g1", "", CaseFilter{ModeratorID: "modA"}, 100, 0)
	if len(byMod) != 2 {
		t.Fatalf("mod filter: want 2 got %d", len(byMod))
	}

	both, _ := st.ListCasesFiltered(ctx, "g1", "", CaseFilter{Type: "ban", ModeratorID: "modA"}, 100, 0)
	if len(both) != 1 || both[0].TargetID != "u1" {
		t.Fatalf("combined filter: %+v", both)
	}

	byUser, _ := st.ListCasesFiltered(ctx, "g1", "u1", CaseFilter{Type: "warn"}, 100, 0)
	if len(byUser) != 1 || byUser[0].ModeratorID != "modB" {
		t.Fatalf("target+type filter: %+v", byUser)
	}

	legacy, _ := st.ListCases(ctx, "g1", "u2", 100, 0)
	if len(legacy) != 1 {
		t.Fatalf("legacy ListCases: %d", len(legacy))
	}
}

func TestJailBackupCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.GetJailBackup(ctx, "g1", "u1"); !errors.Is(err, ErrJailBackupNotFound) {
		t.Fatalf("want ErrJailBackupNotFound, got %v", err)
	}

	if err := st.SaveJailBackup(ctx, "g1", "u1", "mod1", "raiding", []string{"r1", " r2 ", "", "r3"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	bk, err := st.GetJailBackup(ctx, "g1", "u1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if bk.JailedBy != "mod1" || bk.Reason != "raiding" || len(bk.Roles) != 3 {
		t.Fatalf("unexpected backup: %+v", bk)
	}
	if bk.Roles[0] != "r1" || bk.Roles[1] != "r2" || bk.Roles[2] != "r3" {
		t.Fatalf("roles not trimmed: %+v", bk.Roles)
	}

	if err := st.SaveJailBackup(ctx, "g1", "u1", "mod2", "again", []string{"only"}); err != nil {
		t.Fatal(err)
	}
	bk, _ = st.GetJailBackup(ctx, "g1", "u1")
	if len(bk.Roles) != 1 || bk.Roles[0] != "only" || bk.JailedBy != "mod2" {
		t.Fatalf("replace failed: %+v", bk)
	}

	if err := st.SaveJailBackup(ctx, "g2", "u1", "m", "", nil); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListJailBackups(ctx, "g1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list g1: %d %v", len(list), err)
	}

	if err := st.DeleteJailBackup(ctx, "g1", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetJailBackup(ctx, "g1", "u1"); !errors.Is(err, ErrJailBackupNotFound) {
		t.Fatalf("want not-found after delete, got %v", err)
	}
}

func TestGuildConfigJailRoleRoundtrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	cfg, err := st.GetGuildConfig(ctx, "g9")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JailRoleID != "" {
		t.Fatalf("default jail role should be empty, got %q", cfg.JailRoleID)
	}

	cfg.Prefix = "."
	cfg.LogChannelID = "log"
	cfg.JailRoleID = "role-jail"
	if err := st.SaveGuildConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	cfg, _ = st.GetGuildConfig(ctx, "g9")
	if cfg.Prefix != "." || cfg.LogChannelID != "log" || cfg.JailRoleID != "role-jail" {
		t.Fatalf("roundtrip lost fields: %+v", cfg)
	}

	cfg.JailRoleID = ""
	if err := st.SaveGuildConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	cfg, _ = st.GetGuildConfig(ctx, "g9")
	if cfg.JailRoleID != "" || cfg.Prefix != "." || cfg.LogChannelID != "log" {
		t.Fatalf("clearing jail role disturbed siblings: %+v", cfg)
	}
}

func TestMirrorQueries(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	for range 4 {
		st.AddStar(ctx, "g1", "m1")
	}
	st.AddStar(ctx, "g1", "m2")
	st.AddStar(ctx, "g1", "m3")
	st.RemoveStar(ctx, "g1", "m3")

	posts, err := st.ListStarboardPosts(ctx, "g1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 || posts[0].SourceID != "m1" || posts[0].Stars != 4 || posts[1].SourceID != "m2" {
		t.Fatalf("unexpected posts: %+v", posts)
	}

	top1, _ := st.ListStarboardPosts(ctx, "g1", 1)
	if len(top1) != 1 || top1[0].SourceID != "m1" {
		t.Fatalf("limit not applied: %+v", top1)
	}

	if other, _ := st.ListStarboardPosts(ctx, "g2", 10); len(other) != 0 {
		t.Fatalf("guild isolation broken: %+v", other)
	}

	exp := time.Now().UTC().Add(time.Hour)
	gOpen, err := st.CreateGiveaway(ctx, "g1", "c1", "keycaps", 2, exp, "host1")
	if err != nil {
		t.Fatal(err)
	}
	gOld, err := st.CreateGiveaway(ctx, "g1", "c1", "sticker", 1, exp, "host1")
	if err != nil {
		t.Fatal(err)
	}
	st.SetGiveawayWinners(ctx, gOld.ID, []string{"u9"})
	st.MarkGiveawayDrawn(ctx, gOld.ID)

	list, err := st.ListGiveaways(ctx, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 giveaways, got %d", len(list))
	}
	if list[0].ID != gOpen.ID || list[0].Drawn {
		t.Fatalf("running giveaway not first/intact: %+v", list[0])
	}
	if !list[1].Drawn || len(list[1].WinnerIDs) != 1 || list[1].WinnerIDs[0] != "u9" {
		t.Fatalf("drawn giveaway lost winners: %+v", list[1])
	}

	if empty, _ := st.ListGiveaways(ctx, "g2"); len(empty) != 0 {
		t.Fatalf("guild isolation broken: %+v", empty)
	}
}

func TestGuildAdminCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if st.IsGuildAdmin(ctx, "g1", "u1") {
		t.Fatal("no mapping should exist yet")
	}

	if err := st.AddGuildAdmin(ctx, "g1", "u1", "boss"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddGuildAdmin(ctx, "g1", "u1", "boss2"); err != nil {
		t.Fatal(err)
	}
	if !st.IsGuildAdmin(ctx, "g1", "u1") || st.IsGuildAdmin(ctx, "g2", "u1") || st.IsGuildAdmin(ctx, "g1", "u2") {
		t.Fatal("scoping wrong after grant")
	}

	if err := st.AddGuildAdmin(ctx, "g2", "u1", "boss"); err != nil {
		t.Fatal(err)
	}
	guilds, err := st.ListGuildAdminGuilds(ctx, "u1")
	if err != nil || len(guilds) != 2 || guilds[0] != "g1" || guilds[1] != "g2" {
		t.Fatalf("user guilds: %v %v", guilds, err)
	}

	list, err := st.ListGuildAdmins(ctx, "g1")
	if err != nil || len(list) != 1 || list[0].GrantedBy != "boss2" {
		t.Fatalf("guild admins: %+v %v", list, err)
	}

	if err := st.RemoveGuildAdmin(ctx, "g1", "u1"); err != nil {
		t.Fatal(err)
	}
	if st.IsGuildAdmin(ctx, "g1", "u1") {
		t.Fatal("mapping should be gone")
	}

	if err := st.DeleteAdmin(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	left, _ := st.ListGuildAdminGuilds(ctx, "u1")
	if len(left) != 0 {
		t.Fatalf("scopes survived admin deletion: %v", left)
	}
}

func TestNormalizeAdminRole(t *testing.T) {
	for raw, want := range map[string]string{
		"superadmin": RoleSuperadmin,
		"admin":      RoleAdmin,
		"viewer":     RoleViewer,
		"":           RoleAdmin,
		"hacker":     RoleAdmin,
	} {
		if got := NormalizeAdminRole(raw); got != want {
			t.Errorf("NormalizeAdminRole(%q) = %q, want %q", raw, got, want)
		}
	}
	if ValidAdminRole("") || ValidAdminRole("hacker") {
		t.Error("ValidAdminRole must reject non-tiers")
	}
	if !ValidAdminRole(RoleViewer) {
		t.Error("ValidAdminRole must accept viewer")
	}
}

func TestTempBanLifecycle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	exp := time.Now().UTC().Add(time.Hour)

	if err := st.CreateTempBan(ctx, "g1", "u1", "reason", exp, "mod", 7); err != nil {
		t.Fatalf("create tempban: %v", err)
	}
	if err := st.CreateTempBan(ctx, "g1", "u1", "again", exp, "mod", 8); !errors.Is(err, ErrActiveTempBan) {
		t.Fatalf("want ErrActiveTempBan, got %v", err)
	}

	due, err := st.DueTempBans(ctx, time.Now(), 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("nothing due yet, got %+v err %v", due, err)
	}

	future := exp.Add(time.Minute)
	due, err = st.DueTempBans(ctx, future, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("want 1 due tempban, got %+v err %v", due, err)
	}
	if due[0].CaseNo != 7 {
		t.Fatalf("case number not carried through: %+v", due[0])
	}

	if err := st.MarkTempBanUnbanned(ctx, due[0].ID); err != nil {
		t.Fatal(err)
	}
	due, _ = st.DueTempBans(ctx, future, 10)
	if len(due) != 0 {
		t.Fatalf("row should be consumed, got %+v", due)
	}
}


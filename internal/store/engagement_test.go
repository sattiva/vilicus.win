package store

import (
	"context"
	"errors"
	"testing"
	"time"
)


func TestXPCurve(t *testing.T) {
	cases := []struct {
		xp   int64
		lvl  int64
		next int64
	}{
		{0, 0, 100},
		{99, 0, 1},
		{100, 1, 155},
		{254, 1, 1},
		{255, 2, 220},
	}
	for _, c := range cases {
		if got := XPLevelFor(c.xp); got != c.lvl {
			t.Errorf("XPLevelFor(%d) = %d, want %d", c.xp, got, c.lvl)
		}
		if got := XPToNext(c.xp); got != c.next {
			t.Errorf("XPToNext(%d) = %d, want %d", c.xp, got, c.next)
		}
	}
}

func TestAwardXPCooldownAndUpsert(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	xp, lvl, up, err := st.AwardXP(ctx, "g", "u", 120)
	if err != nil || xp != 120 || lvl != 1 || !up {
		t.Fatalf("first award: xp=%d lvl=%d up=%v err=%v", xp, lvl, up, err)
	}

	if _, _, _, err := st.AwardXP(ctx, "g", "u", 50); !errors.Is(err, ErrXPCooldown) {
		t.Fatalf("want ErrXPCooldown, got %v", err)
	}

	row, err := st.GetUserXP(ctx, "g", "u")
	if err != nil || row == nil {
		t.Fatalf("get user xp: %v (%v)", row, err)
	}
	if row.XP != 120 {
		t.Fatalf("cooldown path wrote anyway: xp=%d", row.XP)
	}

	if _, _, _, err := st.AwardXP(ctx, "g2", "u", 40); err != nil {
		t.Fatalf("second guild first contact: %v", err)
	}

	users, err := st.Leaderboard(ctx, "g", 10)
	if err != nil || len(users) != 1 {
		t.Fatalf("leaderboard: %v rows=%v err=%v", users, len(users), err)
	}
}

func TestStarboardLedger(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	cfg := &StarboardConfig{GuildID: "g", ChannelID: "c1", Threshold: 30, Enabled: true}
	if err := st.SaveStarboardConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Threshold != 25 {
		t.Fatalf("threshold clamp failed: %d", cfg.Threshold)
	}
	got, err := st.GetStarboardConfig(ctx, "g")
	if err != nil || got.ChannelID != "c1" || got.Threshold != 25 || !got.Enabled {
		t.Fatalf("roundtrip: %+v err=%v", got, err)
	}

	stars, board, err := st.AddStar(ctx, "g", "m1")
	if err != nil || stars != 1 || board != "" {
		t.Fatalf("first star: stars=%d board=%q err=%v", stars, board, err)
	}
	for i := 0; i < 2; i++ {
		if stars, _, err = st.AddStar(ctx, "g", "m1"); err != nil || stars != i+2 {
			t.Fatalf("star %d: stars=%d err=%v", i+2, stars, err)
		}
	}
	if err := st.SetStarboardBoardMessage(ctx, "g", "m1", "bm"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if _, _, err := st.RemoveStar(ctx, "g", "m1"); err != nil {
			t.Fatal(err)
		}
	}
	stars, board, err = st.RemoveStar(ctx, "g", "m1")
	if err != nil || stars != 0 || board != "bm" {
		t.Fatalf("floored removal: stars=%d board=%q err=%v", stars, board, err)
	}
	if stars, _, err := st.RemoveStar(ctx, "g", "missing"); err != nil || stars != 0 {
		t.Fatalf("unknown message removal: stars=%d err=%v", stars, err)
	}
}

func TestGiveawayLifecycleCAS(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	g, err := st.CreateGiveaway(ctx, "g", "ch", "prize", 99, future, "host")
	if err != nil {
		t.Fatal(err)
	}
	if g.Winners != 20 {
		t.Fatalf("winners clamp: %d", g.Winners)
	}
	if _, err := st.GetGiveawayByMessage(ctx, "g", "nope"); !errors.Is(err, ErrGiveawayNotFound) {
		t.Fatalf("want ErrGiveawayNotFound, got %v", err)
	}

	if fresh, err := st.AddGiveawayEntry(ctx, g.ID, "a"); err != nil || !fresh {
		t.Fatalf("entry a: fresh=%v err=%v", fresh, err)
	}
	if fresh, err := st.AddGiveawayEntry(ctx, g.ID, "a"); err != nil || fresh {
		t.Fatalf("duplicate entry a: fresh=%v err=%v", fresh, err)
	}
	for _, u := range []string{"b", "c"} {
		if _, err := st.AddGiveawayEntry(ctx, g.ID, u); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := st.ListGiveawayEntries(ctx, g.ID)
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries: %v err=%v", entries, err)
	}

	due, err := st.DueGiveaways(ctx, time.Now(), 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("premature due sweep: %v err=%v", due, err)
	}

	if !st.MarkGiveawayDrawn(ctx, g.ID) {
		t.Fatal("first claim should win")
	}
	if st.MarkGiveawayDrawn(ctx, g.ID) {
		t.Fatal("second claim must lose")
	}
	if due, _ := st.DueGiveaways(ctx, past, 10); len(due) != 0 {
		t.Fatal("claimed giveaway still appears due")
	}

	winners := []string{"b"}
	if err := st.SetGiveawayWinners(ctx, g.ID, winners); err != nil {
		t.Fatal(err)
	}
	if err := st.SetGiveawayMessage(ctx, g.ID, "panelmsg"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetGiveawayByMessage(ctx, "g", "panelmsg")
	if err != nil || got == nil {
		t.Fatalf("by message: %v err=%v", got, err)
	}
	if !got.Drawn || len(got.WinnerIDs) != 1 || got.WinnerIDs[0] != "b" {
		t.Fatalf("persisted winners wrong: drawn=%v ids=%v", got.Drawn, got.WinnerIDs)
	}
}

func TestRoleBindingsCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	bindings := []RoleBinding{
		{RoleID: "r1", Label: "One"},
		{RoleID: "r2"},
	}
	if err := st.AddRoleBindings(ctx, "g", "msg", "staff", bindings); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListRoleBindings(ctx, "g", "msg")
	if err != nil || len(rows) != 2 {
		t.Fatalf("list after add: %v err=%v", rows, err)
	}
	if rows[0].RoleID != "r1" || rows[0].Label != "One" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}

	if err := st.AddRoleBindings(ctx, "g", "msg", "staff", []RoleBinding{{RoleID: "r1"}}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := st.ListRoleBindings(ctx, "g", "msg"); len(rows) != 2 {
		t.Fatalf("re-add duplicated rows: %d", len(rows))
	}

	n, err := st.DeleteRoleBindings(ctx, "g", "msg")
	if err != nil || n != 2 {
		t.Fatalf("delete: n=%d err=%v", n, err)
	}
	n, _ = st.DeleteRoleBindings(ctx, "g", "msg")
	if n != 0 {
		t.Fatalf("second delete removed %d", n)
	}
}

func TestProtectionConfigRoundtrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.GetProtectionConfig(ctx, "g"); !errors.Is(err, ErrProtectionUnconfigured) {
		t.Fatalf("want ErrProtectionUnconfigured, got %v", err)
	}

	cfg := &ProtectionConfig{
		GuildID: "g", AntispamEnabled: true,
		AntispamMsgs: 999, AntispamWindow: 1,
		AntilinkMode: "nonsense",
		FilterWords:  " Bad Word , ,SPAM ",
	}
	if err := st.SaveProtectionConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AntispamMsgs != 30 || cfg.AntispamWindow != 2 || cfg.AntilinkMode != "off" {
		t.Fatalf("clamps failed: %+v", cfg)
	}
	if cfg.FilterWords != "bad word,spam" {
		t.Fatalf("filter normalization failed: %q", cfg.FilterWords)
	}

	got, err := st.GetProtectionConfig(ctx, "g")
	if err != nil || !got.AntispamEnabled || got.AntispamMsgs != 30 || got.FilterWords != "bad word,spam" {
		t.Fatalf("roundtrip: %+v err=%v", got, err)
	}
}


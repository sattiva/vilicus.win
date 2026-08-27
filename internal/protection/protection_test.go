package protection

import (
	"testing"
	"time"
)

func TestScorePrunesAndSums(t *testing.T) {
	now := time.Now()
	events := []Event{
		{At: now.Add(-2 * time.Hour), Weight: 35, Module: ModuleBanAdd},
		{At: now.Add(-30 * time.Second), Weight: 25, Module: ModuleBotAdd},
		{At: now.Add(-5 * time.Second), Weight: 30, Module: ModuleRoleDelete},
	}

	score, kept := Score(events, now, time.Minute)
	if score != 55 {
		t.Fatalf("want 55, got %d", score)
	}
	if len(kept) != 2 || kept[0].Module != ModuleBotAdd {
		t.Fatalf("pruned slice wrong: %+v", kept)
	}
	if &kept[0] != &events[0] {
		t.Fatal("kept should reuse the caller's backing array")
	}
}

func TestScoreEmptyWindow(t *testing.T) {
	score, kept := Score(nil, time.Now(), time.Minute)
	if score != 0 || len(kept) != 0 {
		t.Fatalf("empty input should score 0, got %d / %+v", score, kept)
	}
}

func TestNormalizePunishDefaultsToBan(t *testing.T) {
	cases := map[string]string{
		"ban":     PunishBan,
		"kick":    PunishKick,
		"timeout": PunishTimeout,
		"":        PunishBan,
		"BAN":     PunishBan,
		"nuke":    PunishBan,
	}
	for in, want := range cases {
		if got := NormalizePunish(in); got != want {
			t.Errorf("NormalizePunish(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidPunish(t *testing.T) {
	for _, ok := range []string{PunishBan, PunishKick, PunishTimeout} {
		if !ValidPunish(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "Ban", "warn"} {
		if ValidPunish(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestModuleWeightsCoverLabels(t *testing.T) {
	for mod := range ModuleWeights {
		if ModuleLabels[mod] == "" {
			t.Errorf("module %d has a weight but no label", mod)
		}
	}
	for mod := range ModuleLabels {
		if _, ok := ModuleWeights[mod]; !ok {
			t.Errorf("module %d has a label but no weight", mod)
		}
	}
}


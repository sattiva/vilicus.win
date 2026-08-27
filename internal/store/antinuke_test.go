package store

import (
	"context"
	"errors"
	"testing"
)

func TestAntinukeConfigCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.GetAntinukeConfig(ctx, "g1"); !errors.Is(err, ErrAntinukeUnconfigured) {
		t.Fatalf("want unconfigured, got %v", err)
	}

	cfg := &AntinukeConfig{
		GuildID: "g1", Enabled: true,
		Punish:    "nuke",
		Threshold: 5, WindowSeconds: 9999,
		Whitelist: " 222 , 333 , 222 ,, ",
	}
	if err := st.SaveAntinukeConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Punish != "ban" {
		t.Errorf("punish not normalized: %q", cfg.Punish)
	}
	if cfg.Threshold != 20 || cfg.WindowSeconds != 300 {
		t.Errorf("clamps wrong: threshold %d window %d", cfg.Threshold, cfg.WindowSeconds)
	}
	if cfg.Whitelist != "222,333" {
		t.Errorf("whitelist not normalized: %q", cfg.Whitelist)
	}

	got, err := st.GetAntinukeConfig(ctx, "g1")
	if err != nil || !got.Enabled || got.Whitelist != "222,333" {
		t.Fatalf("roundtrip mismatch: %+v err %v", got, err)
	}

	got.Enabled = false
	got.AlertChannelID = "chan1"
	if err := st.SaveAntinukeConfig(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetAntinukeConfig(ctx, "g1")
	if got.Enabled || got.AlertChannelID != "chan1" {
		t.Fatalf("update failed: %+v", got)
	}

	enabled, err := st.EnabledAntinukeGuilds(ctx)
	if err != nil || len(enabled) != 0 {
		t.Fatalf("disabled guild should not be listed: %+v err %v", enabled, err)
	}

	if err := st.SaveAntinukeConfig(ctx, &AntinukeConfig{GuildID: "g2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveAntinukeConfig(ctx, &AntinukeConfig{GuildID: "g3", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	enabled, err = st.EnabledAntinukeGuilds(ctx)
	if err != nil || len(enabled) != 2 {
		t.Fatalf("want 2 enabled guilds, got %+v err %v", enabled, err)
	}
}

func TestProtectionHoneypotColumns(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	cfg := &ProtectionConfig{
		GuildID:         "g1",
		HoneypotChannel: "ch99",
		HoneypotAction:  "yeet",
	}
	if err := st.SaveProtectionConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.HoneypotAction != "ban" {
		t.Fatalf("honeypot action not normalized: %q", cfg.HoneypotAction)
	}
	got, err := st.GetProtectionConfig(ctx, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if got.HoneypotChannel != "ch99" || got.HoneypotAction != "ban" {
		t.Fatalf("honeypot roundtrip mismatch: %+v", got)
	}

	cfg.HoneypotAction = "kick"
	if err := st.SaveProtectionConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.GetProtectionConfig(ctx, "g1"); got.HoneypotAction != "kick" {
		t.Fatalf("update failed: %+v", got)
	}
}


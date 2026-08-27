package discord

import (
	"testing"

	"vilicus/internal/config"
)

func TestCommandRegistryBoots(t *testing.T) {
	b, err := New(&config.Config{BotToken: "offline-test"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if len(b.cmdList) == 0 || len(b.commands) < len(b.cmdList) {
		t.Fatalf("registry inconsistent: %d commands, %d lookup entries", len(b.cmdList), len(b.commands))
	}

	seen := make(map[string]bool, len(b.cmdList))
	for _, c := range b.cmdList {
		name := c.Name()
		if seen[name] {
			t.Errorf("duplicate command name %q", name)
		}
		seen[name] = true

		for _, a := range c.Aliases() {
			key := "alias:" + a
			if seen[key] && a != name {
				t.Errorf("duplicate alias %q", a)
			}
			seen[key] = true
		}
	}

	defs := b.Definitions()
	if len(defs) != len(b.cmdList) {
		t.Fatalf("Definitions() built %d, want %d", len(defs), len(b.cmdList))
	}
}


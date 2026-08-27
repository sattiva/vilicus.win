package discord

import (
	"context"
	"testing"

	"vilicus/internal/config"
	"vilicus/internal/lava"
)

func newTestMusicBot(t *testing.T) *Bot {
	t.Helper()
	b, err := New(&config.Config{
		BotToken:     "offline-test",
		LavalinkHost: "127.0.0.1",
		LavalinkPort: 1,
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

func mkQT(encoded string) lava.QueuedTrack {
	return lava.QueuedTrack{Track: lava.Track{Encoded: encoded, Info: lava.TrackInfo{Title: encoded}}}
}

func TestOnTrackEndKeepsCurrentForReplaceStop(t *testing.T) {
	b := newTestMusicBot(t)
	p := b.playerFor("g")
	cur := mkQT("a")
	p.current = &cur

	b.onTrackEnd("g", "REPLACED")
	b.onTrackEnd("g", "STOPPED")

	if p.current == nil || p.current.Track.Encoded != "a" {
		t.Fatalf("REPLACED/STOPPED must not clear current, got %+v", p.current)
	}
}

func TestOnTrackEndFinishEmptiesQueue(t *testing.T) {
	b := newTestMusicBot(t)
	p := b.playerFor("g")
	cur := mkQT("a")
	p.current = &cur

	b.onTrackEnd("g", "FINISHED")

	if p.current != nil {
		t.Fatal("FINISHED should clear current")
	}
	if p.queue.Len() != 0 {
		t.Fatalf("empty queue stays empty, got %d", p.queue.Len())
	}
}

func TestOnTrackEndQueueLoopRequeuesFinisher(t *testing.T) {
	b := newTestMusicBot(t)
	p := b.playerFor("g")
	cur := mkQT("playing")
	p.current = &cur
	p.loop = "queue"
	p.queue.Add(mkQT("next"))

	b.onTrackEnd("g", "FINISHED")

	if p.queue.Len() != 1 {
		t.Fatalf("want finisher left after consumed advance, got %d", p.queue.Len())
	}
	left, _ := p.queue.Pop()
	if left.Track.Encoded != "playing" {
		t.Fatalf("finisher must be the leftover entry, got %q", left.Track.Encoded)
	}
}

func TestHandleLavaMessagePositionAndDrop(t *testing.T) {
	b := newTestMusicBot(t)
	p := b.playerFor("g")
	cur := mkQT("a")
	p.current = &cur

	b.handleLavaMessage(lava.ServerMessage{
		Op:      "playerUpdate",
		GuildID: "g",
		State: &struct {
			Time      int64 `json:"time"`
			Position  int64 `json:"position"`
			Connected bool  `json:"connected"`
		}{Position: 42000},
	})
	if p.positionMS != 42000 {
		t.Fatalf("position not stamped: %d", p.positionMS)
	}

	b.handleLavaMessage(lava.ServerMessage{Op: "disconnect"})
	select {
	case <-b.lavaDrop:
	default:
		t.Fatal("disconnect op must signal the drop channel")
	}
}

func TestMusicModeSettersOffline(t *testing.T) {
	b := newTestMusicBot(t)

	if err := b.MusicLoop(context.TODO(), "g", "bogus"); err == nil {
		t.Fatal("invalid loop mode must be rejected")
	}
	if err := b.MusicLoop(context.TODO(), "g", "track"); err != nil {
		t.Fatalf("valid loop mode rejected: %v", err)
	}
	p := b.players["g"]
	if p.loop != "track" {
		t.Fatalf("loop not persisted: %q", p.loop)
	}

	on, err := b.MusicShuffle(context.TODO(), "g")
	if err != nil || !on {
		t.Fatalf("first shuffle toggle want on/nil, got %v %v", on, err)
	}
	on, _ = b.MusicShuffle(context.TODO(), "g")
	if on {
		t.Fatal("second shuffle toggle want off")
	}

	n, err := b.MusicClear(context.TODO(), "missing-guild")
	if err != nil || n != 0 {
		t.Fatalf("clear on unknown guild want 0/nil, got %d %v", n, err)
	}
}

func TestMusicEnabledGatesOnConfig(t *testing.T) {
	withNode := newTestMusicBot(t)
	if !withNode.MusicEnabled() {
		t.Fatal("configured host must report music enabled")
	}

	dormant, err := New(&config.Config{BotToken: "offline-test"}, nil)
	if err != nil {
		t.Fatalf("New dormant: %v", err)
	}
	if dormant.MusicEnabled() {
		t.Fatal("no host configured must report music disabled")
	}
}

func TestMusicSnapshotCopies(t *testing.T) {
	b := newTestMusicBot(t)
	p := b.playerFor("g")
	p.voiceChannelID = "vc"
	p.volume = 80
	p.queue.Add(mkQT("one"))
	p.queue.Add(mkQT("two"))

	snap, ok := b.MusicSnapshot("g")
	if !ok {
		t.Fatal("snapshot missing for known guild")
	}
	if snap.VoiceChannelID != "vc" || snap.Volume != 80 || len(snap.Upcoming) != 2 {
		t.Fatalf("snapshot fields wrong: %+v", snap)
	}

	snap.Upcoming[0].Track.Encoded = "mutated"
	if again, _ := b.MusicSnapshot("g"); again.Upcoming[0].Track.Encoded != "one" {
		t.Fatal("snapshot leaked live queue backing array")
	}

	if _, ok := b.MusicSnapshot("other"); ok {
		t.Fatal("snapshot reported unknown guild")
	}
}


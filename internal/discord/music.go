package discord

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/discord/commands"
	"vilicus/internal/lava"
)


const (
	musicDialRetry  = 5 * time.Second
	musicIdleSweep  = 5 * time.Minute
	musicIdleTTL    = 15 * time.Minute
	musicMaxVolume  = 150
	musicDefaultVol = 100
)

var (
	errNoResults   = errors.New("no tracks found for that query")
	errNotPlaying  = errors.New("nothing is playing")
	errNotSeekable = errors.New("this track cannot be seeked")
	errNoNode      = errors.New("music is not enabled on this instance")
	errNotInVoice  = errors.New("join a voice channel first")
)

type musicPlayer struct {
	guildID        string
	textChannelID  string
	voiceChannelID string

	mu         sync.Mutex
	queue      lava.Queue
	current    *lava.QueuedTrack
	positionMS int64
	positionAt time.Time
	paused     bool
	loop       string
	shuffle    bool
	volume     int
	lastActive time.Time

	voiceSessionID string
	voiceToken     string
	voiceEndpoint  string
}

func (p *musicPlayer) positionLocked() time.Duration {
	if p.current == nil || p.paused {
		return time.Duration(p.positionMS) * time.Millisecond
	}
	return time.Duration(p.positionMS)*time.Millisecond + time.Since(p.positionAt)
}

func (b *Bot) playerFor(gid string) *musicPlayer {
	b.musicMu.Lock()
	defer b.musicMu.Unlock()
	p, ok := b.players[gid]
	if !ok {
		p = &musicPlayer{guildID: gid, loop: "off", volume: musicDefaultVol}
		b.players[gid] = p
	}
	p.mu.Lock()
	p.lastActive = time.Now()
	p.mu.Unlock()
	return p
}

func (b *Bot) lookupPlayer(gid string) (*musicPlayer, bool) {
	b.musicMu.Lock()
	defer b.musicMu.Unlock()
	p, ok := b.players[gid]
	return p, ok
}


func (b *Bot) startMusic(ctx context.Context) {
	go func() {
		for ctx.Err() == nil {
			if err := b.lava.Dial(ctx); err != nil {
				slog.Warn("lavalink dial failed", "host", b.Config.LavalinkHost, "err", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(musicDialRetry):
				}
				continue
			}
			slog.Info("lavalink connected", "host", b.Config.LavalinkHost)
			select {
			case <-ctx.Done():
				return
			case <-b.lavaDrop:
			}
			slog.Warn("lavalink disconnected; retrying", "host", b.Config.LavalinkHost)
			b.lava.Close()
			for {
				select {
				case <-b.lavaDrop:
					continue
				default:
				}
				break
			}
		}
	}()

	go func() {
		t := time.NewTicker(musicIdleSweep)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				b.sweepIdlePlayers()
			}
		}
	}()
}

func (b *Bot) sweepIdlePlayers() {
	cutoff := time.Now().Add(-musicIdleTTL)
	b.musicMu.Lock()
	for gid, p := range b.players {
		p.mu.Lock()
		idle := p.current == nil && p.queue.Len() == 0 &&
			p.voiceSessionID == "" && p.lastActive.Before(cutoff)
		p.mu.Unlock()
		if idle {
			delete(b.players, gid)
		}
	}
	b.musicMu.Unlock()
}

func (b *Bot) registerMusicHandlers() {
	b.Session.AddHandler(b.onVoiceStateForMusic)
	b.Session.AddHandler(b.onVoiceServerForMusic)
}


func (b *Bot) onVoiceStateForMusic(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {
	if b.lava == nil || m.VoiceState == nil || m.UserID != b.selfUserID() {
		return
	}
	b.safeEvent("voiceStateMusic", func(ctx context.Context) {
		p, ok := b.lookupPlayer(m.GuildID)
		if !ok {
			return
		}
		p.mu.Lock()
		if m.ChannelID == "" {
			p.voiceSessionID = ""
			p.mu.Unlock()
			return
		}
		p.voiceSessionID = m.SessionID
		p.voiceChannelID = m.ChannelID
		vsid, token, ep := p.voiceSessionID, p.voiceToken, p.voiceEndpoint
		p.mu.Unlock()

		b.sendVoiceIfReady(p, vsid, token, ep)
	})
}

func (b *Bot) onVoiceServerForMusic(s *discordgo.Session, m *discordgo.VoiceServerUpdate) {
	if b.lava == nil {
		return
	}
	b.safeEvent("voiceServerMusic", func(ctx context.Context) {
		p, ok := b.lookupPlayer(m.GuildID)
		if !ok {
			return
		}
		p.mu.Lock()
		p.voiceToken = m.Token
		p.voiceEndpoint = m.Endpoint
		vsid := p.voiceSessionID
		p.mu.Unlock()
		b.sendVoiceIfReady(p, vsid, m.Token, m.Endpoint)
	})
}

func (b *Bot) sendVoiceIfReady(p *musicPlayer, sessionID, token, endpoint string) {
	if sessionID == "" || token == "" || endpoint == "" || !b.lava.Connected() {
		return
	}
	if err := b.lava.SendVoiceUpdate(p.guildID, sessionID, token, endpoint); err != nil {
		slog.Warn("lavalink voiceUpdate failed", "guild_id", p.guildID, "err", err)
	}
}

func (b *Bot) selfUserID() string {
	if b.Session.State != nil && b.Session.State.User != nil {
		return b.Session.State.User.ID
	}
	return ""
}


func (b *Bot) handleLavaMessage(msg lava.ServerMessage) {
	switch msg.Op {
	case "disconnect":
		select {
		case b.lavaDrop <- struct{}{}:
		default:
		}
	case "ready":
		go b.resyncAllPlayers()
	case "playerUpdate":
		if msg.State == nil {
			return
		}
		if p, ok := b.lookupPlayer(msg.GuildID); ok {
			p.mu.Lock()
			p.positionMS = msg.State.Position
			p.positionAt = time.Now()
			p.mu.Unlock()
		}
	case "event":
		if msg.Type != "TrackEndEvent" && msg.Type != "WebSocketClosedEvent" {
			return
		}
		go b.safeEvent("lavaEvent", func(ctx context.Context) {
			if msg.Type == "WebSocketClosedEvent" {
				slog.Warn("lavalink voice websocket closed",
					"guild_id", msg.GuildID, "code", msg.Code,
					"reason", msg.Reason, "by_remote", msg.ByRemote)
				return
			}
			b.onTrackEnd(msg.GuildID, msg.Reason)
		})
	}
}

func (b *Bot) resyncAllPlayers() {
	b.musicMu.Lock()
	all := make([]*musicPlayer, 0, len(b.players))
	for _, p := range b.players {
		all = append(all, p)
	}
	b.musicMu.Unlock()

	for _, p := range all {
		p.mu.Lock()
		sessionID, token, ep := p.voiceSessionID, p.voiceToken, p.voiceEndpoint
		cur := p.current
		pos := p.positionLocked().Milliseconds()
		paused := p.paused
		vol := p.volume
		p.mu.Unlock()

		if sessionID != "" && token != "" && ep != "" {
			b.sendVoiceIfReady(p, sessionID, token, ep)
		}
		if cur == nil || !b.lava.Connected() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := b.lava.UpdatePlayer(ctx, p.guildID, lava.PlayerPatch{
			Track:    &lava.EncodedTrack{Encoded: cur.Track.Encoded},
			Volume:   &vol,
			Paused:   &paused,
			Position: &pos,
		})
		cancel()
		if err != nil {
			slog.Warn("lavalink player resync failed", "guild_id", p.guildID, "err", err)
		}
	}
}

func (b *Bot) onTrackEnd(gid, reason string) {
	p, ok := b.lookupPlayer(gid)
	if !ok {
		return
	}
	switch reason {
	case "REPLACED", "STOPPED":
		return
	}

	finished := p.takeCurrent()
	if finished == nil {
		return
	}

	p.mu.Lock()
	loop, shuffle := p.loop, p.shuffle
	textCh := p.textChannelID
	p.mu.Unlock()

	if loop == "track" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := b.startTrack(ctx, p, *finished, false)
		cancel()
		if err != nil {
			slog.Warn("loop-track restart failed", "guild_id", gid, "err", err)
		}
		return
	}

	p.mu.Lock()
	if loop == "queue" {
		p.queue.Add(*finished)
	}
	if shuffle {
		p.queue.Shuffle()
	}
	next, has := p.queue.Pop()
	p.mu.Unlock()

	if !has {
		b.softAnnounce(textCh, "Queue Finished", "That was the last track.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := b.startTrack(ctx, p, next, true); err != nil {
		slog.Warn("auto-advance failed", "guild_id", gid, "err", err)
	}
}


func (b *Bot) startTrack(ctx context.Context, p *musicPlayer, qt lava.QueuedTrack, announce bool) error {
	p.mu.Lock()
	vol, paused := p.volume, p.paused
	p.mu.Unlock()

	patch := lava.PlayerPatch{
		Track:  &lava.EncodedTrack{Encoded: qt.Track.Encoded},
		Volume: &vol,
		Paused: &paused,
	}
	if err := b.lava.UpdatePlayer(ctx, p.guildID, patch); err != nil {
		return err
	}

	p.mu.Lock()
	p.current = &qt
	p.positionMS = 0
	p.positionAt = time.Now()
	p.lastActive = time.Now()
	queued := p.queue.Len()
	textCh := p.textChannelID
	p.mu.Unlock()

	if announce {
		b.softAnnounce(textCh, "Now Playing",
			trackLine(qt), "Up next: "+itoa(int64(queued))+" queued")
	}
	return nil
}

func (b *Bot) stopPlayback(ctx context.Context, p *musicPlayer) {
	p.mu.Lock()
	p.current = nil
	p.positionMS = 0
	p.paused = false
	p.mu.Unlock()
	if err := b.lava.DestroyPlayer(ctx, p.guildID); err != nil {
		slog.Debug("lavalink destroy skipped", "guild_id", p.guildID, "err", err)
	}
}

func (b *Bot) softAnnounce(channelID, title string, lines ...string) {
	if channelID == "" {
		return
	}
	sendSoft(b, b.Session, channelID, b.Container(
		TextDisplay(title),
		Sep(),
		Section(lines...),
	))
}


func (b *Bot) MusicEnabled() bool { return b.lava != nil }

var _ commands.MusicController = (*Bot)(nil)

func (b *Bot) MusicJoin(gid, voiceChannelID, textChannelID string) error {
	if b.lava == nil {
		return errNoNode
	}
	if voiceChannelID == "" {
		return errNotInVoice
	}
	p := b.playerFor(gid)
	p.mu.Lock()
	p.voiceChannelID = voiceChannelID
	if textChannelID != "" {
		p.textChannelID = textChannelID
	}
	p.mu.Unlock()
	return b.Session.ChannelVoiceJoinManual(gid, voiceChannelID, false, true)
}

func (b *Bot) MusicLeave(ctx context.Context, gid string) error {
	if b.lava == nil {
		return errNoNode
	}
	if p, ok := b.lookupPlayer(gid); ok {
		b.stopPlayback(ctx, p)
		p.mu.Lock()
		p.queue.Clear()
		p.textChannelID = ""
		p.voiceToken = ""
		p.voiceEndpoint = ""
		p.voiceSessionID = ""
		p.voiceChannelID = ""
		p.loop = "off"
		p.shuffle = false
		p.mu.Unlock()
	}
	return b.Session.ChannelVoiceJoinManual(gid, "", false, false)
}

func (b *Bot) MusicPlay(ctx context.Context, gid, query, requesterID string, top bool) (*commands.MusicPlayResult, error) {
	if b.lava == nil {
		return nil, errNoNode
	}
	res, err := b.lava.LoadTracks(ctx, lava.SearchIdentifier(query))
	if err != nil {
		return nil, err
	}
	tracks := res.AllTracks()
	if len(tracks) == 0 {
		return nil, errNoResults
	}
	if res.LoadType == "SEARCH_RESULT" {
		tracks = tracks[:1]
	}

	p := b.playerFor(gid)
	added := make([]lava.QueuedTrack, 0, len(tracks))
	for _, t := range tracks {
		added = append(added, lava.QueuedTrack{Track: t, RequesterID: requesterID})
	}

	p.mu.Lock()
	first := added[0]
	if top {
		p.queue.AddNext(first)
	} else {
		p.queue.Add(first)
	}
	for _, qt := range added[1:] {
		p.queue.Add(qt)
	}
	idle := p.current == nil
	p.mu.Unlock()

	out := &commands.MusicPlayResult{Queued: len(tracks)}
	if idle {
		started, _ := p.popNext()
		out.Started = &started.Track
		if err := b.startTrack(ctx, p, started, false); err != nil {
			return out, err
		}
	} else {
		out.Enqueued = &first.Track
	}
	return out, nil
}

func (b *Bot) MusicPause(ctx context.Context, gid string, paused bool) error {
	if b.lava == nil {
		return errNoNode
	}
	p, ok := b.lookupPlayer(gid)
	if !ok {
		return errNotPlaying
	}
	p.mu.Lock()
	hasCurrent := p.current != nil
	p.mu.Unlock()
	if !hasCurrent {
		return errNotPlaying
	}
	if err := b.lava.UpdatePlayer(ctx, gid, lava.PlayerPatch{Paused: &paused}); err != nil {
		return err
	}
	p.mu.Lock()
	p.paused = paused
	if paused {
		p.positionMS = p.positionLocked().Milliseconds()
	}
	p.positionAt = time.Now()
	p.mu.Unlock()
	return nil
}

func (b *Bot) MusicStop(ctx context.Context, gid string) error {
	if b.lava == nil {
		return errNoNode
	}
	p, ok := b.lookupPlayer(gid)
	if !ok {
		return nil
	}
	b.stopPlayback(ctx, p)
	p.mu.Lock()
	p.queue.Clear()
	p.mu.Unlock()
	return nil
}

func (b *Bot) MusicSkip(ctx context.Context, gid string) error {
	if b.lava == nil {
		return errNoNode
	}
	p, ok := b.lookupPlayer(gid)
	if !ok {
		return errNotPlaying
	}
	p.mu.Lock()
	hasCurrent := p.current != nil
	p.mu.Unlock()
	if !hasCurrent {
		return errNotPlaying
	}

	next, has := p.popNext()
	if !has {
		b.stopPlayback(ctx, p)
		return nil
	}
	return b.startTrack(ctx, p, next, true)
}

func (b *Bot) MusicSeek(ctx context.Context, gid string, positionSeconds int64) error {
	if b.lava == nil {
		return errNoNode
	}
	p, ok := b.lookupPlayer(gid)
	if !ok {
		return errNotPlaying
	}
	p.mu.Lock()
	cur := p.current
	p.mu.Unlock()
	if cur == nil {
		return errNotPlaying
	}
	if cur.Track.Info.IsStream || !cur.Track.Info.IsSeekable {
		return errNotSeekable
	}
	ms := positionSeconds * 1000
	if err := b.lava.UpdatePlayer(ctx, gid, lava.PlayerPatch{Position: &ms}); err != nil {
		return err
	}
	p.mu.Lock()
	p.positionMS = ms
	p.positionAt = time.Now()
	p.mu.Unlock()
	return nil
}

func (b *Bot) MusicVolume(ctx context.Context, gid string, volume int) error {
	if b.lava == nil {
		return errNoNode
	}
	if volume < 0 {
		volume = 0
	}
	if volume > musicMaxVolume {
		volume = musicMaxVolume
	}
	p := b.playerFor(gid)
	if err := b.lava.UpdatePlayer(ctx, gid, lava.PlayerPatch{Volume: &volume}); err != nil {
		return err
	}
	p.mu.Lock()
	p.volume = volume
	p.mu.Unlock()
	return nil
}

func (b *Bot) MusicLoop(ctx context.Context, gid, mode string) error {
	switch mode {
	case "off", "track", "queue":
	default:
		return errors.New("loop mode must be off, track, or queue")
	}
	p := b.playerFor(gid)
	p.mu.Lock()
	p.loop = mode
	p.mu.Unlock()
	return nil
}

func (b *Bot) MusicShuffle(ctx context.Context, gid string) (bool, error) {
	p := b.playerFor(gid)
	p.mu.Lock()
	p.shuffle = !p.shuffle
	on := p.shuffle
	p.mu.Unlock()
	return on, nil
}

func (b *Bot) MusicClear(ctx context.Context, gid string) (int, error) {
	p, ok := b.lookupPlayer(gid)
	if !ok {
		return 0, nil
	}
	p.mu.Lock()
	n := p.queue.Len()
	p.queue.Clear()
	p.mu.Unlock()
	return n, nil
}

func (b *Bot) MusicSnapshot(gid string) (snap commands.MusicSnapshot, ok bool) {
	p, found := b.lookupPlayer(gid)
	if !found {
		return snap, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	snap.VoiceChannelID = p.voiceChannelID
	snap.TextChannelID = p.textChannelID
	cur := p.current
	snap.Current = cur
	snap.Position = p.positionLocked()
	snap.Paused = p.paused
	snap.Loop = p.loop
	snap.Shuffle = p.shuffle
	snap.Volume = p.volume
	snap.Connected = p.voiceSessionID != ""
	if n := p.queue.Len(); n > 0 {
		snap.Upcoming = p.queue.Page(1, n)
	}
	return snap, true
}


func (p *musicPlayer) popNext() (lava.QueuedTrack, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queue.Pop()
}

func (p *musicPlayer) takeCurrent() *lava.QueuedTrack {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.current
	p.current = nil
	p.positionMS = 0
	return cur
}

func trackLine(qt lava.QueuedTrack) string {
	line := qt.Track.Info.Title
	if qt.Track.Info.Author != "" {
		line += " - " + qt.Track.Info.Author
	}
	line += " [" + lava.FormatMillis(qt.Track.Info.Length) + "]"
	return line + "\nRequested by <@" + qt.RequesterID + ">"
}


package general

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/lava"
)

type MusicCmd struct {
	Kind string
}

func (c *MusicCmd) Name() string { return c.Kind }

func (c *MusicCmd) Category() string { return "Music" }

func (c *MusicCmd) RequiredPermissions() *int64 { return nil }

func (c *MusicCmd) Description() string {
	switch c.Kind {
	case "play":
		return "Play a track or playlist from a URL or search text"
	case "pause":
		return "Pause the current track"
	case "resume":
		return "Resume playback"
	case "skip":
		return "Skip to the next track in the queue"
	case "stop":
		return "Stop playback and clear the queue (stays in voice)"
	case "queue":
		return "Show the current queue"
	case "np":
		return "Show the track that is playing now"
	case "seek":
		return "Seek within the current track (seconds or m:ss)"
	case "volume":
		return "Set playback volume (0-150)"
	case "loop":
		return "Set loop mode: off, track, or queue"
	case "shuffle":
		return "Toggle shuffle for the queue"
	case "clear":
		return "Remove every track from the queue"
	}
	return ""
}

func (c *MusicCmd) Aliases() []string {
	switch c.Kind {
	case "play":
		return nil
	case "volume":
		return []string{"vol"}
	case "loop":
		return []string{"repeat"}
	case "np":
		return []string{"nowplaying"}
	}
	return nil
}

const musicPageSz = 10

func (c *MusicCmd) Options() []*discordgo.ApplicationCommandOption {
	switch c.Kind {
	case "play":
		return []*discordgo.ApplicationCommandOption{
			{Name: "query", Description: "URL or search text", Type: discordgo.ApplicationCommandOptionString, Required: true},
			{Name: "top", Description: "Queue ahead of everything else", Type: discordgo.ApplicationCommandOptionBoolean},
		}
	case "seek":
		return []*discordgo.ApplicationCommandOption{
			{Name: "position", Description: "Seconds or m:ss into the track", Type: discordgo.ApplicationCommandOptionString, Required: true},
		}
	case "volume":
		return []*discordgo.ApplicationCommandOption{
			{Name: "level", Description: "0-150", Type: discordgo.ApplicationCommandOptionInteger, Required: true,
				MinValue: f64Ptr(0), MaxValue: 150},
		}
	case "loop":
		return []*discordgo.ApplicationCommandOption{
			{Name: "mode", Description: "Loop mode", Type: discordgo.ApplicationCommandOptionString, Required: true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "off", Value: "off"},
					{Name: "track", Value: "track"},
					{Name: "queue", Value: "queue"},
				}},
		}
	case "queue":
		return []*discordgo.ApplicationCommandOption{
			{Name: "page", Description: "Page number", Type: discordgo.ApplicationCommandOptionInteger},
		}
	}
	return nil
}

func f64Ptr(v float64) *float64 { return &v }

func (c *MusicCmd) controller(b commands.BotInterface, s *discordgo.Session, gid, callerID string) (commands.MusicController, *components.Container, bool) {
	mc, ok := b.(commands.MusicController)
	if !ok || !mc.MusicEnabled() {
		return nil, simpleCard(b, "Music is not enabled on this instance."), false
	}
	if !commands.CanControlMusic(s, gid, callerID) {
		return nil, simpleCard(b, "Control the player from its voice channel or with moderator permissions."), false
	}
	return mc, nil, true
}

func simpleCard(b commands.BotInterface, lines ...string) *components.Container {
	comps := make([]discordgo.MessageComponent, 0, len(lines))
	for _, l := range lines {
		comps = append(comps, components.TextDisplay{Content: l})
	}
	return b.Container(comps...)
}

func trackLabel(t *lava.Track) string {
	s := t.Info.Title
	if t.Info.Author != "" {
		s += " - " + t.Info.Author
	}
	if t.Info.IsStream {
		return s + " [live]"
	}
	if t.Info.Length > 0 {
		s += " [" + lava.FormatMillis(t.Info.Length) + "]"
	}
	return s
}

func npBody(snap *commands.MusicSnapshot) []string {
	if snap.Current == nil {
		return []string{"Nothing is playing."}
	}
	lines := []string{trackLabel(&snap.Current.Track)}
	if !snap.Current.Track.Info.IsStream && snap.Current.Track.Info.Length > 0 {
		bar := commands.MusicBar(snap.Position, timeFromMillis(snap.Current.Track.Info.Length), 16)
		lines = append(lines, bar+" "+lava.FormatMillis(snap.Position.Milliseconds())+" / "+
			lava.FormatMillis(snap.Current.Track.Info.Length))
	} else {
		lines = append(lines, "Position: "+lava.FormatMillis(snap.Position.Milliseconds()))
	}
	mode := "Loop " + snap.Loop
	if snap.Shuffle {
		mode += " | Shuffle on"
	}
	mode += fmt.Sprintf(" | Volume %d%%", snap.Volume)
	if snap.Paused {
		mode += " | Paused"
	}
	lines = append(lines, mode)
	if len(snap.Upcoming) > 0 {
		lines = append(lines, fmt.Sprintf("Up next: %s", trackLabel(&snap.Upcoming[0].Track)))
	}
	return lines
}

func (c *MusicCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	opts := i.ApplicationCommandData().Options
	args := make([]string, 0, len(opts))
	for _, o := range opts {
		switch o.Type {
		case discordgo.ApplicationCommandOptionString:
			args = append(args, o.StringValue())
		case discordgo.ApplicationCommandOptionInteger:
			args = append(args, strconv.FormatInt(o.IntValue(), 10))
		case discordgo.ApplicationCommandOptionBoolean:
			if o.BoolValue() {
				args = append(args, "true")
			} else {
				args = append(args, "false")
			}
		}
	}
	return c.run(ctx, b, s, i.GuildID, i.ChannelID, i.Member.User.ID, args)
}

func (c *MusicCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	return c.run(ctx, b, s, m.GuildID, m.ChannelID, m.Author.ID, args)
}

func (c *MusicCmd) run(ctx context.Context, b commands.BotInterface, s *discordgo.Session, gid, chID, callerID string, args []string) (*components.Container, error) {
	mc, card, ok := c.controller(b, s, gid, callerID)
	if !ok {
		return card, nil
	}

	switch c.Kind {
	case "play":
		if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
			return simpleCard(b, "Give me a URL or search text: `.play <query>`"), nil
		}
		vc := commands.AuthorVoiceChannel(s, gid, callerID)
		if vc == "" {
			return simpleCard(b, "Join a voice channel first, then play."), nil
		}
		top := len(args) > 1 && args[1] == "true"
		if err := mc.MusicJoin(gid, vc, chID); err != nil {
			return nil, fmt.Errorf("voice join failed: %w", err)
		}
		res, err := mc.MusicPlay(ctx, gid, args[0], callerID, top)
		if err != nil {
			return nil, err
		}
		if res.Started != nil {
			snap := mustSnapshot(mc, gid)
			return simpleCard(b, append([]string{"Now Playing"}, npBody(&snap)...)...), nil
		}
		if res.Enqueued != nil && res.Queued > 1 {
			return simpleCard(b, fmt.Sprintf("Queued %d tracks, first: %s", res.Queued, trackLabel(res.Enqueued))), nil
		}
		return simpleCard(b, "Queued: "+trackLabel(res.Enqueued)), nil

	case "pause":
		if err := mc.MusicPause(ctx, gid, true); err != nil {
			return nil, err
		}
		return simpleCard(b, "Paused."), nil

	case "resume":
		if err := mc.MusicPause(ctx, gid, false); err != nil {
			return nil, err
		}
		return simpleCard(b, "Resumed."), nil

	case "skip":
		if err := mc.MusicSkip(ctx, gid); err != nil {
			return nil, err
		}
		snap := mustSnapshot(mc, gid)
		if snap.Current == nil {
			return simpleCard(b, "Skipped. Queue is empty."), nil
		}
		return simpleCard(b, append([]string{"Skipped. Now Playing"}, npBody(&snap)...)...), nil

	case "stop":
		if err := mc.MusicStop(ctx, gid); err != nil {
			return nil, err
		}
		return simpleCard(b, "Stopped. Queue cleared."), nil

	case "queue":
		return c.queueCard(b, mc, gid, args), nil

	case "np":
		snap := mustSnapshot(mc, gid)
		return simpleCard(b, append([]string{"Now Playing"}, npBody(&snap)...)...), nil

	case "seek":
		if len(args) == 0 {
			return simpleCard(b, "Give a position: seconds or m:ss."), nil
		}
		sec, ok := lava.ParseTimestamp(args[0])
		if !ok {
			return simpleCard(b, "That is not a position I can read: use seconds or m:ss."), nil
		}
		if err := mc.MusicSeek(ctx, gid, sec); err != nil {
			return nil, err
		}
		snap := mustSnapshot(mc, gid)
		return simpleCard(b, append([]string{"Seeked"}, npBody(&snap)...)...), nil

	case "volume":
		if len(args) == 0 {
			return simpleCard(b, "Give a level between 0 and 150."), nil
		}
		lvl, err := strconv.Atoi(args[0])
		if err != nil || lvl < 0 || lvl > 150 {
			return simpleCard(b, "Level must be a number between 0 and 150."), nil
		}
		if err := mc.MusicVolume(ctx, gid, lvl); err != nil {
			return nil, err
		}
		return simpleCard(b, fmt.Sprintf("Volume set to %d%%.", lvl)), nil

	case "loop":
		if len(args) == 0 {
			return simpleCard(b, "Mode must be off, track, or queue."), nil
		}
		mode := strings.ToLower(strings.TrimSpace(args[0]))
		if err := mc.MusicLoop(ctx, gid, mode); err != nil {
			return nil, err
		}
		return simpleCard(b, "Loop mode: "+mode), nil

	case "shuffle":
		on, err := mc.MusicShuffle(ctx, gid)
		if err != nil {
			return nil, err
		}
		state := "off"
		if on {
			state = "on"
		}
		return simpleCard(b, "Shuffle "+state), nil

	case "clear":
		n, err := mc.MusicClear(ctx, gid)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return simpleCard(b, "Queue is already empty."), nil
		}
		return simpleCard(b, fmt.Sprintf("Cleared %d tracks.", n)), nil
	}
	return simpleCard(b, "Unknown music subcommand."), nil
}

func (c *MusicCmd) queueCard(b commands.BotInterface, mc commands.MusicController, gid string, args []string) *components.Container {
	snap, ok := mc.MusicSnapshot(gid)
	if !ok || (snap.Current == nil && len(snap.Upcoming) == 0) {
		return simpleCard(b, "Queue is empty. Start something with `.play <query>`.")
	}

	page := 1
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			page = n
		}
	}
	pages := (len(snap.Upcoming) + musicPageSz - 1) / musicPageSz
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}

	lines := []string{fmt.Sprintf("Queue - %d track(s), page %d/%d", len(snap.Upcoming), page, pages)}
	if snap.Current != nil && page == 1 {
		lines = append(lines, "Now: "+trackLabel(&snap.Current.Track))
	}
	start := (page - 1) * musicPageSz
	end := start + musicPageSz
	if end > len(snap.Upcoming) {
		end = len(snap.Upcoming)
	}
	for idx := start; idx < end; idx++ {
		qt := snap.Upcoming[idx]
		lines = append(lines, fmt.Sprintf("%d. %s <@%s>", idx+1, trackLabel(&qt.Track), qt.RequesterID))
	}
	if len(lines) > 25 {
		lines = lines[:25]
	}
	return simpleCard(b, lines...)
}

func mustSnapshot(mc commands.MusicController, gid string) commands.MusicSnapshot {
	snap, _ := mc.MusicSnapshot(gid)
	return snap
}

func timeFromMillis(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}


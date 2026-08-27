package commands

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/lava"
)

type MusicController interface {
	MusicEnabled() bool

	MusicJoin(gid, voiceChannelID, textChannelID string) error
	MusicLeave(ctx context.Context, gid string) error

	MusicPlay(ctx context.Context, gid, query, requesterID string, top bool) (*MusicPlayResult, error)

	MusicPause(ctx context.Context, gid string, paused bool) error
	MusicStop(ctx context.Context, gid string) error
	MusicSkip(ctx context.Context, gid string) error
	MusicSeek(ctx context.Context, gid string, positionSeconds int64) error
	MusicVolume(ctx context.Context, gid string, volume int) error
	MusicLoop(ctx context.Context, gid, mode string) error
	MusicShuffle(ctx context.Context, gid string) (bool, error)
	MusicClear(ctx context.Context, gid string) (int, error)

	MusicSnapshot(gid string) (MusicSnapshot, bool)
}

type MusicPlayResult struct {
	Started  *lava.Track
	Queued   int
	Enqueued *lava.Track
}

type MusicSnapshot struct {
	VoiceChannelID string
	TextChannelID  string
	Current        *lava.QueuedTrack
	Position       time.Duration
	Paused         bool
	Loop           string
	Shuffle        bool
	Volume         int
	Upcoming       []lava.QueuedTrack
	Connected      bool
}

func CanControlMusic(s *discordgo.Session, gid, callerID string) bool {
	modPerms := int64(discordgo.PermissionManageMessages | discordgo.PermissionManageGuild)
	if member, _ := s.State.Member(gid, callerID); member != nil && member.Permissions&modPerms != 0 {
		return true
	}
	vs, err := s.State.VoiceState(gid, callerID)
	return err == nil && vs != nil && vs.ChannelID != ""
}

func AuthorVoiceChannel(s *discordgo.Session, gid, callerID string) string {
	vs, err := s.State.VoiceState(gid, callerID)
	if err != nil || vs == nil {
		return ""
	}
	return vs.ChannelID
}

func MusicBar(position, total time.Duration, width int) string {
	if width < 4 {
		width = 4
	}
	filled := 0
	if total > 0 {
		pct := float64(position) / float64(total)
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}
		filled = int(pct * float64(width))
	}
	bar := make([]byte, 0, width+2)
	for i := 0; i < width; i++ {
		if i < filled {
			bar = append(bar, '#')
		} else {
			bar = append(bar, '-')
		}
	}
	return "[" + string(bar) + "]"
}


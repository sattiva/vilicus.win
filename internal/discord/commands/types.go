package commands

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/config"
	"vilicus/internal/store"
)

type BotInterface interface {
	GetStore() *store.Store
	GetConfig() *config.Config
	GetStartTime() time.Time
	Container(children ...discordgo.MessageComponent) *components.Container
	GetCommands() []Command
	ApplyStatus()
}

type SnipeReader interface {
	LatestSnipe(channelID string) (content, authorID string, at time.Time, ok bool)
}

type PollStarter interface {
	StartPoll(s *discordgo.Session, gid, channelID, question string, options []string, duration time.Duration) (string, error)
}

type PanelStarter interface {
	PostRolePanel(ctx context.Context, s *discordgo.Session, gid, channelID, title, createdBy string, roles []string) (*components.Container, error)
	DeleteRolePanel(s *discordgo.Session, gid, channelID, messageID string) (int64, error)
}

type GiveawayStarter interface {
	StartGiveaway(ctx context.Context, s *discordgo.Session, gid, channelID, prize, hostedBy string, winners int, d time.Duration) (*components.Container, error)
	RerollGiveaway(ctx context.Context, s *discordgo.Session, gid, messageID, actorID string, extraWinners int) (*components.Container, error)
}

type Command interface {
	Name() string
	Description() string
	Category() string
	Aliases() []string
	Options() []*discordgo.ApplicationCommandOption
	RequiredPermissions() *int64
	ExecuteSlash(ctx context.Context, b BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error)
	ExecutePrefix(ctx context.Context, b BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error)
}


package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/config"
	"vilicus/internal/discord/commands"
	cmdConfig "vilicus/internal/discord/commands/config"
	"vilicus/internal/discord/commands/general"
	"vilicus/internal/discord/commands/moderation"
	"vilicus/internal/lava"
	"vilicus/internal/logging"
	"vilicus/internal/store"
)

type FastPather interface {
	FastPath() bool
}

type CooldownClasser interface {
	CooldownClass() string
}

type Bot struct {
	Session   *discordgo.Session
	Store     *store.Store
	Config    *config.Config
	StartTime time.Time
	commands  map[string]commands.Command
	cmdList   []commands.Command

	compRouter *componentRouter
	cooldowns  *Cooldowns
	snipes     *snipeStore
	edits      *editCache
	polls      *pollStore

	protectionMu    sync.Mutex
	protectionCache map[string]protectionCacheEntry
	spamMu          sync.Mutex
	spamWindow      map[string][]time.Time
	spamCooldown    map[string]time.Time

	xpMu   sync.Mutex
	xpGate map[string]xpGateEntry

	automationMu        sync.Mutex
	automationCache     map[string]automationCacheEntry
	automationCooldowns map[string]time.Time
	automationCounters  map[string]automationWindow
	automationRoleMu    sync.Mutex
	automationRoles     map[string]map[string][]string

	antinukeMu       sync.Mutex
	antinuke         map[string]*antinukeState
	honeypotCooldown map[string]time.Time

	musicMu  sync.Mutex
	players  map[string]*musicPlayer
	lava     *lava.Client
	lavaDrop chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg *config.Config, st *store.Store) (*Bot, error) {
	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, err
	}

	dg.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuildMessageReactions
	if cfg.LavalinkHost != "" {
		dg.Identify.Intents |= discordgo.IntentsGuildVoiceStates
	}

	ctx, cancel := context.WithCancel(context.Background())

	b := &Bot{
		Session:          dg,
		Store:            st,
		Config:           cfg,
		StartTime:        time.Now(),
		commands:         make(map[string]commands.Command),
		compRouter:       newComponentRouter(),
		cooldowns:        NewCooldowns(),
		snipes:           newSnipeStore(),
		edits:            newEditCache(),
		polls:            newPollStore(),
		protectionCache:  make(map[string]protectionCacheEntry),
		spamWindow:       make(map[string][]time.Time),
		spamCooldown:     make(map[string]time.Time),
		xpGate:           make(map[string]xpGateEntry),
		automationCache:  make(map[string]automationCacheEntry),
		antinuke:         make(map[string]*antinukeState),
		honeypotCooldown: make(map[string]time.Time),
		players:          make(map[string]*musicPlayer),
		lavaDrop:         make(chan struct{}, 1),

		ctx:    ctx,
		cancel: cancel,
	}

	if cfg.LavalinkHost != "" {
		b.lava = lava.NewClient(cfg.LavalinkHost, cfg.LavalinkPort,
			cfg.LavalinkPassword, cfg.LavalinkSecure, "", "vilicus")
		b.lava.OnEvent = b.handleLavaMessage
		b.registerMusicHandlers()
	}

	b.registerDefaults()
	b.registerComponentHandlers()
	b.registerEventHandlers()
	dg.AddHandler(b.handleInteraction)
	dg.AddHandler(b.handleMessage)

	return b, nil
}

func (b *Bot) registerDefaults() {
	b.Register(
		&general.PingCmd{},
		&general.BannerCmd{},
		&general.HelpCmd{},
		&general.PrefixCmd{},
		&general.UserInfoCmd{},
		&general.ServerInfoCmd{},
		&general.AvatarCmd{},
		&general.AboutCmd{},
		&general.ChooseCmd{},
		&general.RollCmd{},
		&general.SnipeCmd{},
		&general.PollCmd{},
		&general.RemindCmd{},
		&general.RankCmd{},
		&general.LeaderboardCmd{},
		&general.RolePanelCmd{},
		&general.GiveawayCmd{},
		&general.GiveawayRerollCmd{},
		&general.EmojiCmd{},
		&general.MemberCountCmd{},
		&general.BotsCmd{},
		&general.ChannelInfoCmd{},
		&general.RolesCmd{},
		&general.InvitesCmd{},
		&general.UUIDCmd{},
		&general.HashCmd{},
		&general.CodecCmd{},
		&general.CodecCmd{Decode: true},
		&general.GenCmd{},
		&general.EntropyCmd{},
		&moderation.RoleCmd{},
		&moderation.KickCmd{},
		&moderation.BanCmd{},
		&moderation.TimeoutCmd{},
		&moderation.PurgeCmd{},
		&moderation.WarnCmd{},
		&moderation.HistoryCmd{},
		&moderation.TempRoleCmd{},
		&moderation.JailCmd{},
		&moderation.UnjailCmd{},
		&moderation.CaseCmd{},
		&moderation.CaseNoteCmd{},
		&moderation.ReasonCmd{},
		&moderation.ModStatsCmd{},
		&moderation.TempBanCmd{},
		&moderation.StarboardCmd{},
		&moderation.ProtectCmd{},
		&moderation.AutomateCmd{},
		&cmdConfig.ConfigCmd{},
		&general.MusicCmd{Kind: "play"},
		&general.MusicCmd{Kind: "pause"},
		&general.MusicCmd{Kind: "resume"},
		&general.MusicCmd{Kind: "skip"},
		&general.MusicCmd{Kind: "stop"},
		&general.MusicCmd{Kind: "queue"},
		&general.MusicCmd{Kind: "np"},
		&general.MusicCmd{Kind: "seek"},
		&general.MusicCmd{Kind: "volume"},
		&general.MusicCmd{Kind: "loop"},
		&general.MusicCmd{Kind: "shuffle"},
		&general.MusicCmd{Kind: "clear"},
	)
}

func (b *Bot) Register(cmds ...commands.Command) {
	for _, c := range cmds {
		b.commands[c.Name()] = c
		b.cmdList = append(b.cmdList, c)
		for _, a := range c.Aliases() {
			b.commands[strings.ToLower(a)] = c
		}
	}
}

func (b *Bot) GetStore() *store.Store {
	return b.Store
}

func (b *Bot) GetConfig() *config.Config {
	return b.Config
}

func (b *Bot) GetStartTime() time.Time {
	return b.StartTime
}

func (b *Bot) GetCommands() []commands.Command {
	return b.cmdList
}

func (b *Bot) LatestSnipe(channelID string) (content, authorID string, at time.Time, ok bool) {
	e, found := b.snipes.latest(channelID)
	return e.content, e.authorID, e.at, found
}

func (b *Bot) Start() error {
	if err := b.Session.Open(); err != nil {
		return err
	}

	slog.Info("gateway connected", "user", b.Session.State.User.Username)
	b.ApplyStatus()
	b.startSweeper(b.ctx)

	if b.lava != nil {
		b.lava.UserID = b.Session.State.User.ID
		b.startMusic(b.ctx)
	}

	appID := b.Config.AppID
	if appID == "" {
		appID = b.Session.State.User.ID
	}

	defs := b.Definitions()
	if b.Config.GuildID != "" {
		_, err := b.Session.ApplicationCommandBulkOverwrite(appID, b.Config.GuildID, defs)
		if err != nil {
			slog.Warn("guild command registration failed", "err", err)
		} else {
			slog.Info("guild commands registered", "guild_id", b.Config.GuildID)
		}
	} else {
		_, err := b.Session.ApplicationCommandBulkOverwrite(appID, "", defs)
		if err != nil {
			slog.Warn("global command registration failed", "err", err)
		} else {
			slog.Info("global commands registered")
		}
	}

	return nil
}

func (b *Bot) Stop() {
	b.cancel()
	b.cooldowns.Stop()
	if b.lava != nil {
		b.lava.Close()
	}
	if b.Session != nil {
		_ = b.Session.Close()
	}
}

func (b *Bot) ApplyStatus() {
	if b.Session == nil {
		return
	}
	actName := b.Store.GetSetting("activity_name", ".help | Vilicus")
	_ = b.Session.UpdateGameStatus(0, actName)
}

func (b *Bot) Container(children ...discordgo.MessageComponent) *components.Container {
	col := b.Store.ParseAccentColor()
	footer := b.Store.GetSetting("footer_text", "")
	return components.NewCustomContainer(col, footer, children...)
}

func (b *Bot) Definitions() []*discordgo.ApplicationCommand {
	defs := make([]*discordgo.ApplicationCommand, 0, len(b.cmdList))
	for _, c := range b.cmdList {
		def := &discordgo.ApplicationCommand{
			Name:                     c.Name(),
			Description:              c.Description(),
			Options:                  c.Options(),
			DefaultMemberPermissions: c.RequiredPermissions(),
		}
		if c.RequiredPermissions() != nil {
			dm := false
			def.DMPermission = &dm
		}
		defs = append(defs, def)
	}
	return defs
}


func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		b.handleSlashCommand(s, i)
	case discordgo.InteractionMessageComponent:
		b.handleComponent(s, i)
	case discordgo.InteractionModalSubmit:
		b.handleModal(s, i)
	}
}

func (b *Bot) handleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	start := time.Now()
	reqID := logging.NewID()
	ctx := logging.WithID(context.Background(), reqID)

	cmdName := i.ApplicationCommandData().Name
	gid := i.GuildID
	uid := interactionUserID(i)

	slog.Info("slash interaction received", "cmd", cmdName, "guild_id", gid, "user_id", uid, "req_id", reqID)

	cmd, exists := b.commands[cmdName]
	var resContainer *components.Container
	var execErr error
	var spans store.Spans = store.NoSpans
	noteAck := func() { spans.AckMS = time.Since(start).Milliseconds() }
	noteSend := func() { spans.SendMS = time.Since(start).Milliseconds() }

	finish := func() {
		edit, valErr := components.NewWebhookEdit(resContainer)
		if valErr != nil {
			slog.Error("container validation failed", "err", valErr, "req_id", reqID)
			edit, _ = components.NewWebhookEdit(b.Container(
				components.TextDisplay{Content: "Internal response error."},
			))
		}
		if _, editErr := s.InteractionResponseEdit(i.Interaction, edit); editErr != nil {
			slog.Error("interaction response edit failed", "cmd", cmdName, "err", editErr, "req_id", reqID)
		}
	}

	if !exists {
		resContainer = b.Container(
			components.TextDisplay{Content: fmt.Sprintf("Unknown command: %s", cmdName)},
		)
		finish()
	} else {
		ackDone := false

		if fp, ok := cmd.(FastPather); ok && fp.FastPath() {
			resContainer, execErr = cmd.ExecuteSlash(ctx, b, s, i)
			if execErr != nil {
				slog.Warn("command handler error", "cmd", cmdName, "err", execErr, "req_id", reqID)
				resContainer = b.Container(
					components.TextDisplay{Content: fmt.Sprintf("Command error: %s", execErr.Error())},
				)
			}
			if resp, respErr := components.NewResponse(resContainer); respErr == nil {
				if err := s.InteractionRespond(i.Interaction, resp); err == nil {
					ackDone = true
					noteAck()
					noteSend()
				}
			}
		}

		if !ackDone {
			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags: components.FlagComponentsV2,
				},
			}); err != nil {
				slog.Error("interaction ack failed", "cmd", cmdName, "err", err, "req_id", reqID)
				b.logCommand(cmdName, gid, uid, "error", time.Since(start), store.NoSpans)
				return
			}
			noteAck()

			resContainer, execErr = cmd.ExecuteSlash(ctx, b, s, i)
			if execErr != nil {
				slog.Warn("command handler error", "cmd", cmdName, "err", execErr, "req_id", reqID)
				resContainer = b.Container(
					components.TextDisplay{Content: fmt.Sprintf("Command error: %s", execErr.Error())},
				)
			}
			finish()
			noteSend()
		}
	}

	status := "success"
	if execErr != nil {
		status = "error"
	}
	b.logCommand(cmdName, gid, uid, status, time.Since(start), spans)
	slog.Info("slash interaction finished", "cmd", cmdName, "status", status,
		"ms", time.Since(start).Milliseconds(), "ack_ms", spans.AckMS, "send_ms", spans.SendMS, "req_id", reqID)
}

func (b *Bot) logCommand(name, gid, uid, status string, d time.Duration, spans ...store.Spans) {
	_ = b.Store.LogCommand(context.Background(), name, gid, uid, status, d.Milliseconds(), spans...)
}


var dangerousPrefixCommands = map[string]bool{
	"ban": true, "kick": true, "timeout": true, "purge": true,
	"warn": true, "temprole": true, "role": true, "unbanall": true,
	"tempban": true, "starboard": true, "protect": true, "automate": true,
	"rolepanel": true, "giveaway": true, "gstart": true, "greroll": true,
}

func (b *Bot) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}

	content := strings.TrimSpace(m.Content)
	if len(content) == 0 {
		return
	}

	reqID := logging.NewID()

	b.edits.set(m.ID, content)

	p := b.resolvePrefixFast(content, m.GuildID, m.Author.ID)
	if p == "" {
		return
	}

	rawCmd := strings.TrimPrefix(content, p)
	parts := strings.Fields(rawCmd)
	if len(parts) == 0 {
		return
	}

	name := strings.ToLower(parts[0])
	args := parts[1:]

	cmd, exists := b.commands[name]
	if !exists {
		return
	}

	if m.GuildID == "" && cmd.RequiredPermissions() != nil {
		res := b.Container(components.TextDisplay{Content: "This command is only available in servers."})
		_, _ = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Flags:      components.FlagComponentsV2,
			Components: []discordgo.MessageComponent{res},
		})
		return
	}

	class := ""
	if cc, ok := cmd.(CooldownClasser); ok {
		class = cc.CooldownClass()
	} else if dangerousPrefixCommands[name] {
		class = "danger"
	}
	if !b.cooldowns.Allow(m.Author.ID+":cmd:"+name, class) {
		res := b.Container(components.TextDisplay{Content: "Slow down a moment."})
		_, _ = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Flags:      components.FlagComponentsV2,
			Components: []discordgo.MessageComponent{res},
		})
		return
	}

	if perm := cmd.RequiredPermissions(); perm != nil && m.GuildID != "" {
		member := m.Member
		if member == nil || member.Permissions == 0 {
			if sm, _ := s.State.Member(m.GuildID, m.Author.ID); sm != nil {
				member = sm
			}
		}
		if member == nil {
			fetched, err := s.GuildMember(m.GuildID, m.Author.ID)
			if err == nil {
				member = fetched
			}
		}
		if member != nil {
			if member.Permissions&*perm == 0 && member.Permissions&discordgo.PermissionAdministrator == 0 {
				res := b.Container(components.TextDisplay{Content: "You lack the required permissions for this command."})
				_, _ = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
					Flags:      components.FlagComponentsV2,
					Components: []discordgo.MessageComponent{res},
				})
				return
			}
		}
	}

	start := time.Now()
	slog.Info("prefix message command received", "cmd", name, "guild_id", m.GuildID, "user_id", m.Author.ID, "req_id", reqID)
	ctx := logging.WithID(context.Background(), reqID)

	resContainer, execErr := cmd.ExecutePrefix(ctx, b, s, m, args)
	if execErr != nil {
		slog.Warn("prefix command execution error", "cmd", name, "err", execErr, "req_id", reqID)
		resContainer = b.Container(
			components.TextDisplay{Content: fmt.Sprintf("Command error: %s", execErr.Error())},
		)
	}

	_, sendErr := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Flags:      components.FlagComponentsV2,
		Components: []discordgo.MessageComponent{resContainer},
	})
	if sendErr != nil {
		slog.Error("prefix response send failed", "cmd", name, "err", sendErr, "req_id", reqID)
	}

	durationMS := time.Since(start)
	status := "success"
	if execErr != nil || sendErr != nil {
		status = "error"
	}

	b.logCommand(name, m.GuildID, m.Author.ID, status, durationMS,
		store.Spans{AckMS: -1, SendMS: durationMS.Milliseconds()})
	slog.Info("prefix command finished", "cmd", name, "status", status, "ms", durationMS.Milliseconds(), "req_id", reqID)
}

func (b *Bot) resolvePrefixFast(content, gid, uid string) string {
	if strings.HasPrefix(content, ".") {
		return "."
	}
	if gid != "" {
		if g, err := b.Store.GetGuildConfig(context.Background(), gid); err == nil {
			if g.Prefix != "" && g.Prefix != "." && strings.HasPrefix(content, g.Prefix) {
				return g.Prefix
			}
		}
	}
	if uid != "" {
		if u, err := b.Store.GetUserConfig(context.Background(), uid); err == nil && u.Prefix != "" && u.Prefix != "." && strings.HasPrefix(content, u.Prefix) {
			return u.Prefix
		}
	}
	return ""
}


package discord

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/discord/commands"
	"vilicus/internal/protection"
	"vilicus/internal/store"
)


const (
	antinukeFetchLimit     = 50
	antinukePunishCooldown = 5 * time.Minute
	antinukeTimeout        = time.Hour
	antinukeWarnThrottle   = 5 * time.Minute
)

type antinukeState struct {
	seenID  string
	events  map[string][]protection.Event
	actedAt map[string]time.Time
	warnAt  time.Time
	ownerID string
}

func (b *Bot) antinukeFor(gid string) *antinukeState {
	b.antinukeMu.Lock()
	defer b.antinukeMu.Unlock()
	st, ok := b.antinuke[gid]
	if !ok {
		st = &antinukeState{
			events:  make(map[string][]protection.Event),
			actedAt: make(map[string]time.Time),
		}
		b.antinuke[gid] = st
	}
	return st
}

func (b *Bot) sweepAntinuke(ctx context.Context) {
	cfgs, err := b.Store.EnabledAntinukeGuilds(ctx)
	if err != nil {
		slog.Warn("antinuke sweep failed", "err", err)
		return
	}
	for _, cfg := range cfgs {
		if ctx.Err() != nil {
			return
		}
		b.pollAntinuke(ctx, cfg)
	}
}

func (b *Bot) pollAntinuke(ctx context.Context, cfg *store.AntinukeConfig) {
	st := b.antinukeFor(cfg.GuildID)

	al, err := b.Session.GuildAuditLog(cfg.GuildID, "", "", 0, antinukeFetchLimit)
	if err != nil {
		if time.Since(st.warnAt) >= antinukeWarnThrottle {
			st.warnAt = time.Now()
			slog.Warn("antinuke audit fetch failed", "guild_id", cfg.GuildID, "err", err)
		}
		return
	}

	entries := al.AuditLogEntries
	now := time.Now().UTC()

	if st.seenID == "" {
		if len(entries) > 0 {
			st.seenID = entries[0].ID
		}
		return
	}

	fresh := make([]*discordgo.AuditLogEntry, 0, len(entries))
	maxID := st.seenID
	for _, e := range entries {
		if snowflakeGreater(e.ID, maxID) {
			maxID = e.ID
		}
		if snowflakeGreater(e.ID, st.seenID) {
			fresh = append(fresh, e)
		}
	}
	st.seenID = maxID
	if len(fresh) == 0 {
		return
	}

	owner := b.antinukeOwner(cfg.GuildID, st)
	botID := ""
	if b.Session.State.User != nil {
		botID = b.Session.State.User.ID
	}
	whitelisted := whitelistSet(cfg.Whitelist)

	for i := len(fresh) - 1; i >= 0; i-- {
		e := fresh[i]
		if e.ActionType == nil {
			continue
		}
		weight, ok := protection.ModuleWeights[int(*e.ActionType)]
		if !ok {
			continue
		}
		actor := e.UserID
		if actor == "" || actor == botID || actor == owner || whitelisted[actor] {
			continue
		}
		st.events[actor] = append(st.events[actor], protection.Event{
			At:     time.Unix(snowflakeUnix(e.ID), 0).UTC(),
			Weight: weight,
			Module: int(*e.ActionType),
			Target: e.TargetID,
		})
	}

	window := time.Duration(cfg.WindowSeconds) * time.Second
	for actor, evs := range st.events {
		score, kept := protection.Score(evs, now, window)
		st.events[actor] = kept
		if score < cfg.Threshold || len(kept) == 0 {
			continue
		}
		if last, acted := st.actedAt[actor]; acted && now.Sub(last) < antinukePunishCooldown {
			continue
		}
		b.punishAntinuke(ctx, cfg, st, actor, score, kept, now)
	}
}

func (b *Bot) punishAntinuke(ctx context.Context, cfg *store.AntinukeConfig, st *antinukeState, actorID string, score int, events []protection.Event, now time.Time) {
	st.actedAt[actorID] = now
	delete(st.events, actorID)

	gid := cfg.GuildID
	summary := moduleSummary(events)
	reason := "Antinuke: score " + itoa(int64(score)) + " (" + summary + ")"

	var caseType string
	var err error
	switch cfg.Punish {
	case protection.PunishTimeout:
		caseType = "timeout"
		err = b.antinukeTimeout(gid, actorID)
	case protection.PunishKick:
		caseType = "kick"
		err = b.antinukeKick(gid, actorID)
	default:
		caseType = "ban"
		err = b.antinukeBan(gid, actorID)
	}
	if err != nil {
		slog.Warn("antinuke punishment failed", "guild_id", gid, "user_id", actorID,
			"punish", cfg.Punish, "err", err)
		return
	}

	expires := now.Add(antinukeTimeout)
	dur := int64(0)
	var expPtr *time.Time
	if cfg.Punish == protection.PunishTimeout {
		dur = int64(antinukeTimeout.Seconds())
		expPtr = &expires
	}
	b.recordProtectionCase(ctx, gid, actorID, caseType, reason, dur, expPtr)
	slog.Info("antinuke punished", "guild_id", gid, "user_id", actorID,
		"punish", cfg.Punish, "score", score)

	b.antinukeAlert(ctx, cfg, actorID, caseType, score, reason)
}

func (b *Bot) antinukeTimeout(gid, uid string) error {
	until := time.Now().UTC().Add(antinukeTimeout)
	return b.Session.GuildMemberTimeout(gid, uid, &until)
}

func (b *Bot) antinukeKick(gid, uid string) error {
	if ok := b.antinukeCanAct(gid, uid); !ok {
		return errNotModeratable
	}
	err := b.Session.GuildMemberDeleteWithReason(gid, uid, "[Vilicus antinuke] threat threshold crossed")
	return err
}

func (b *Bot) antinukeBan(gid, uid string) error {
	if ok := b.antinukeCanAct(gid, uid); !ok {
		return errNotModeratable
	}
	return b.Session.GuildBanCreateWithReason(gid, uid, "[Vilicus antinuke] threat threshold crossed", 0)
}

var errNotModeratable = errHierarchySkip{}

type errHierarchySkip struct{}

func (errHierarchySkip) Error() string { return "target outranks the bot or owns the guild" }

func (b *Bot) antinukeCanAct(gid, uid string) bool {
	g, _ := b.Session.State.Guild(gid)
	if g == nil {
		var err error
		if g, err = b.Session.Guild(gid); err != nil {
			return false
		}
	}
	if uid == g.OwnerID || uid == b.Session.State.User.ID {
		return false
	}
	target, err := b.Session.GuildMember(gid, uid)
	if err != nil {
		return false
	}
	bot, err := b.Session.GuildMember(gid, b.Session.State.User.ID)
	if err != nil {
		return false
	}
	ok, _ := commands.CanBotModerate(g, bot, target)
	return ok
}

func (b *Bot) antinukeOwner(gid string, st *antinukeState) string {
	if st.ownerID != "" {
		return st.ownerID
	}
	if g, err := b.Session.State.Guild(gid); err == nil && g.OwnerID != "" {
		st.ownerID = g.OwnerID
		return st.ownerID
	}
	if g, err := b.Session.Guild(gid); err == nil {
		st.ownerID = g.OwnerID
	}
	return st.ownerID
}

func (b *Bot) antinukeAlert(ctx context.Context, cfg *store.AntinukeConfig, actorID, action string, score int, reason string) {
	ch := cfg.AlertChannelID
	if ch == "" {
		gcfg, err := b.Store.GetGuildConfig(ctx, cfg.GuildID)
		if err != nil {
			return
		}
		ch = gcfg.LogChannelID
	}
	sendSoft(b, b.Session, ch, b.Container(
		TextDisplay("Antinuke Triggered"),
		Sep(),
		Section(
			"Actor: <@"+actorID+">",
			"Action: "+action+" (case filed)",
			"Score: "+itoa(int64(score)),
			"Reason: "+reason,
			"Timestamp: "+time.Now().UTC().Format(time.RFC3339),
		),
	))
}

func moduleSummary(events []protection.Event) string {
	counts := make(map[string]int, len(events))
	order := make([]string, 0, len(events))
	for _, e := range events {
		label := protection.ModuleLabels[e.Module]
		if label == "" {
			label = "module " + strconv.Itoa(e.Module)
		}
		if counts[label] == 0 {
			order = append(order, label)
		}
		counts[label]++
	}
	parts := make([]string, 0, len(order))
	for _, label := range order {
		parts = append(parts, label+" x"+strconv.Itoa(counts[label]))
	}
	return strings.Join(parts, ", ")
}

func whitelistSet(csv string) map[string]bool {
	out := make(map[string]bool)
	for _, id := range strings.Split(csv, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = true
		}
	}
	return out
}

func snowflakeGreater(a, b string) bool {
	na, nerr := strconv.ParseUint(a, 10, 64)
	nb, _ := strconv.ParseUint(b, 10, 64)
	if nerr != nil {
		return false
	}
	return na > nb
}


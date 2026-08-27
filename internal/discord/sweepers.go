package discord

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/automation"
	"vilicus/internal/sched"
	"vilicus/internal/store"
)


const (
	reminderBatch  = 25
	tempRoleBatch  = 25
	tempBanBatch   = 25
	giveawayBatch  = 10
	reminderPeriod = 5 * time.Second
	tempRolePeriod = 30 * time.Second
	tempBanPeriod  = 30 * time.Second
	giveawayPeriod = 15 * time.Second

	automationPeriod = 15 * time.Second
	antinukeSweep    = 10 * time.Second
)

func (b *Bot) startSweeper(ctx context.Context) {
	go sched.Loop(ctx, "reminders", reminderPeriod, b.flushReminders)
	go sched.Loop(ctx, "temproles", tempRolePeriod, b.sweepTempRoles)
	go sched.Loop(ctx, "tempbans", tempBanPeriod, b.sweepTempBans)
	go sched.Loop(ctx, "giveaways", giveawayPeriod, func(ctx context.Context) { b.sweepGiveaways(ctx) })
	go sched.Loop(ctx, "automation", automationPeriod, b.sweepAutomationIntervals)
	go sched.Loop(ctx, "antinuke", antinukeSweep, b.sweepAntinuke)
}

func (b *Bot) sweepAutomationIntervals(ctx context.Context) {
	due, err := b.Store.DueIntervalRules(ctx, time.Now())
	if err != nil {
		slog.Warn("automation interval sweep failed", "err", err)
		return
	}
	for _, rule := range due {
		if err := b.Store.TouchAutomationRuleRun(ctx, rule.ID); err != nil {
			continue
		}
		b.executeAutomation(ctx, b.Session, automation.Compile(rule),
			automationEvent{Kind: "interval", GuildID: rule.GuildID, GuildName: b.guildName(rule.GuildID)})
	}
}

func (b *Bot) flushReminders(ctx context.Context) {
	due, err := b.Store.DueReminders(ctx, time.Now(), reminderBatch)
	if err != nil {
		slog.Warn("reminder sweep failed", "err", err)
		return
	}
	for _, r := range due {
		container := b.Container(
			TextDisplay("Reminder"),
			Sep(),
			Section(
				fmt.Sprintf("<@%s>", r.UserID),
				truncate(r.Body, 1500),
				"Set: <t:"+itoa(r.CreatedAt.Unix())+":R>",
			),
		)
		sendSoft(b, b.Session, r.ChannelID, container)
		if err := b.Store.MarkReminderDelivered(ctx, r.ID); err != nil {
			slog.Warn("reminder mark delivered failed", "id", r.ID, "err", err)
		}
	}
}

func (b *Bot) sweepTempRoles(ctx context.Context) {
	due, err := b.Store.DueTempRoles(ctx, time.Now(), tempRoleBatch)
	if err != nil {
		slog.Warn("temprole sweep failed", "err", err)
		return
	}
	for _, t := range due {
		if err := b.Session.GuildMemberRoleRemove(t.GuildID, t.UserID, t.RoleID); err != nil {
			slog.Warn("temprole removal failed", "guild_id", t.GuildID, "user_id", t.UserID, "err", err)
		}
		if err := b.Store.MarkTempRoleRemoved(ctx, t.ID); err != nil {
			slog.Warn("temprole mark removed failed", "id", t.ID, "err", err)
		}
	}
}

func (b *Bot) sweepTempBans(ctx context.Context) {
	due, err := b.Store.DueTempBans(ctx, time.Now(), tempBanBatch)
	if err != nil {
		slog.Warn("tempban sweep failed", "err", err)
		return
	}
	for _, t := range due {
		if err := b.Session.GuildBanDelete(t.GuildID, t.UserID); err != nil {
			slog.Warn("tempban lift failed", "guild_id", t.GuildID, "user_id", t.UserID, "err", err)
		} else {
			slog.Info("tempban lifted", "guild_id", t.GuildID, "user_id", t.UserID, "case_no", t.CaseNo)
		}
		if err := b.Store.MarkTempBanUnbanned(ctx, t.ID); err != nil {
			slog.Warn("tempban mark unbanned failed", "id", t.ID, "err", err)
		}
	}
}

func (b *Bot) sweepGiveaways(ctx context.Context) {
	due, err := b.Store.DueGiveaways(ctx, time.Now(), giveawayBatch)
	if err != nil {
		slog.Warn("giveaway sweep failed", "err", err)
		return
	}
	for _, g := range due {
		if !b.Store.MarkGiveawayDrawn(ctx, g.ID) {
			continue
		}
		entries, err := b.Store.ListGiveawayEntries(ctx, g.ID)
		if err != nil {
			slog.Warn("giveaway entries read failed", "id", g.ID, "err", err)
			continue
		}
		winners := pickWinners(entries, g.Winners)
		if len(winners) > 0 {
			_ = b.Store.SetGiveawayWinners(ctx, g.ID, winners)
		}
		b.announceGiveawayResult(b.Session, g, winners, len(entries))
	}
}

func pickWinners(entries []string, n int) []string {
	if len(entries) == 0 || n <= 0 {
		return nil
	}
	rand.Shuffle(len(entries), func(a, b int) { entries[a], entries[b] = entries[b], entries[a] })
	if n > len(entries) {
		n = len(entries)
	}
	return entries[:n]
}

func (b *Bot) announceGiveawayResult(s *discordgo.Session, g store.Giveaway, winners []string, entrants int) {
	var body string
	switch {
	case len(winners) == 0:
		body = "The giveaway ended with no entries."
	case len(winners) == 1:
		body = "Winner: <@" + winners[0] + ">"
	default:
		mentions := make([]string, 0, len(winners))
		for _, w := range winners {
			mentions = append(mentions, "<@"+w+">")
		}
		body = "Winners: " + strings.Join(mentions, ", ")
	}

	container := b.Container(
		TextDisplay("Giveaway Ended"),
		Sep(),
		Section(
			"Prize: "+truncate(g.Prize, 200),
			body,
			itoa(int64(entrants))+" total entries.",
			"Hosted by: <@"+g.HostedBy+">",
		),
	)
	sendSoft(b, s, g.ChannelID, container)

	if g.MessageID != "" {
		edit := &discordgo.MessageEdit{
			ID:      g.MessageID,
			Channel: g.ChannelID,
			Components: &[]discordgo.MessageComponent{b.Container(
				TextDisplay("Giveaway Ended"),
				Sep(),
				Section(
					"Prize: "+truncate(g.Prize, 200),
					body,
				),
			)},
		}
		if _, err := s.ChannelMessageEditComplex(edit); err != nil {
			slog.Warn("giveaway panel edit failed", "id", g.ID, "err", err)
		}
	}
}


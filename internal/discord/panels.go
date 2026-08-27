package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
	"vilicus/internal/store"
)


func (b *Bot) PostRolePanel(ctx context.Context, s *discordgo.Session, gid, channelID, title, createdBy string, roles []string) (*components.Container, error) {
	guild, err := s.Guild(gid)
	if err != nil || guild == nil {
		return nil, fmt.Errorf("could not load guild")
	}

	byID := make(map[string]*discordgo.Role, len(guild.Roles))
	for _, r := range guild.Roles {
		byID[r.ID] = r
	}

	var rows []discordgo.MessageComponent
	var row []discordgo.MessageComponent
	bindings := make([]store.RoleBinding, 0, len(roles))
	for _, rid := range roles {
		label := "@" + rid
		if r := byID[rid]; r != nil {
			label = r.Name
		}
		payload, _ := json.Marshal(rbPayload{G: gid, R: rid})
		row = append(row, discordgo.Button{
			Label:    truncate(label, 80),
			Style:    discordgo.PrimaryButton,
			CustomID: components.BuildCustomID("rb", "role", payload, time.Time{}),
		})
		if len(row) == 5 {
			rows = append(rows, discordgo.ActionsRow{Components: row})
			row = nil
		}
		bindings = append(bindings, store.RoleBinding{GuildID: gid, RoleID: rid, Label: label})
	}
	if len(row) > 0 {
		rows = append(rows, discordgo.ActionsRow{Components: row})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no roles given")
	}

	panel := b.Container(
		TextDisplay(title),
		Sep(),
		TextDisplay("Click a button to toggle that role. One click on / one click off."),
	)
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Flags:      components.FlagComponentsV2,
		Components: append([]discordgo.MessageComponent{panel}, rows...),
	})
	if err != nil {
		return nil, err
	}
	if err := b.Store.AddRoleBindings(context.Background(), gid, msg.ID, createdBy, bindings); err != nil {
		slog.Warn("role panel bindings write failed; deleting panel", "err", err)
		_ = s.ChannelMessageDelete(channelID, msg.ID)
		return nil, fmt.Errorf("failed to store panel bindings")
	}

	lines := make([]string, 0, len(roles))
	for _, rid := range roles {
		lines = append(lines, "<@&"+rid+"> (`"+rid+"`)")
	}
	return b.Container(
		TextDisplay("Role Panel Created"),
		Sep(),
		Section(
			"Channel: <#"+channelID+">",
			strings.Join(lines, "\n"),
			"Buttons stay live across restarts.",
		),
	), nil
}

func (b *Bot) DeleteRolePanel(s *discordgo.Session, gid, channelID, messageID string) (int64, error) {
	n, err := b.Store.DeleteRoleBindings(context.Background(), gid, messageID)
	if err != nil {
		return 0, err
	}
	_ = s.ChannelMessageDelete(channelID, messageID)
	return n, nil
}

func (b *Bot) StartGiveaway(ctx context.Context, s *discordgo.Session, gid, channelID, prize, hostedBy string, winners int, d time.Duration) (*components.Container, error) {
	endsAt := time.Now().UTC().Add(d)
	g, err := b.Store.CreateGiveaway(ctx, gid, channelID, prize, winners, endsAt, hostedBy)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(gwPayload{G: gid, ID: g.ID})
	btn := discordgo.Button{
		Label:    "Enter Giveaway",
		Style:    discordgo.PrimaryButton,
		CustomID: components.BuildCustomID("gw", "join", payload, endsAt.Add(10*time.Minute)),
	}

	panel := b.Container(
		TextDisplay("Giveaway"),
		Sep(),
		Section(
			"Prize: "+truncate(prize, 200),
			fmt.Sprintf("Winners: %d", winners),
			fmt.Sprintf("Ends: <t:%d:R>", endsAt.Unix()),
			fmt.Sprintf("Hosted by: <@%s>", hostedBy),
		),
		Sep(),
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{btn}},
	)
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Flags:      components.FlagComponentsV2,
		Components: []discordgo.MessageComponent{panel},
	})
	if err != nil {
		return nil, err
	}
	if err := b.Store.SetGiveawayMessage(ctx, g.ID, msg.ID); err != nil {
		slog.Warn("giveaway message link failed", "id", g.ID, "err", err)
	}

	jump := msgLink(gid, channelID, msg.ID)
	return b.Container(
		TextDisplay("Giveaway Started"),
		Sep(),
		Section(
			"Prize: "+truncate(prize, 200),
			fmt.Sprintf("Ends: <t:%d:R> (%s)", endsAt.Unix(), commands.FormatDuration(d)),
			fmt.Sprintf("Winners: %d", winners),
			"[Jump to entry panel]("+jump+")",
		),
	), nil
}

func (b *Bot) RerollGiveaway(ctx context.Context, s *discordgo.Session, gid, messageID, actorID string, extraWinners int) (*components.Container, error) {
	g, err := b.Store.GetGiveawayByMessage(ctx, gid, messageID)
	if err != nil {
		return nil, fmt.Errorf("no giveaway found for that message id")
	}
	if !g.Drawn {
		return nil, fmt.Errorf("that giveaway has not ended yet")
	}

	entries, err := b.Store.ListGiveawayEntries(ctx, g.ID)
	if err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("that giveaway has no entries")
	}
	prev := make(map[string]bool, len(g.WinnerIDs))
	for _, w := range g.WinnerIDs {
		prev[w] = true
	}
	pool := make([]string, 0, len(entries))
	for _, e := range entries {
		if !prev[e] {
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("every entrant already won")
	}

	n := extraWinners
	if n < 1 {
		n = 1
	}
	winners := pickWinners(pool, n)
	all := append(append([]string{}, g.WinnerIDs...), winners...)
	_ = b.Store.SetGiveawayWinners(ctx, g.ID, all)

	mentions := make([]string, 0, len(winners))
	for _, w := range winners {
		mentions = append(mentions, "<@"+w+">")
	}
	sendSoft(b, s, g.ChannelID, b.Container(
		TextDisplay("Giveaway Reroll"),
		Sep(),
		Section(
			"Prize: "+truncate(g.Prize, 200),
			"New winner: "+strings.Join(mentions, ", "),
			"Rerolled by: <@"+actorID+">",
		),
	))

	return b.Container(
		TextDisplay("Reroll Complete"),
		Sep(),
		Section(
			"New winner: "+strings.Join(mentions, ", "),
			fmt.Sprintf("%d previous winners excluded.", len(prev)),
		),
	), nil
}


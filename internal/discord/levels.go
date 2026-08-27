package discord

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/bwmarrin/discordgo"
)


const (
	xpGateTTL     = time.Minute
	xpGateMaxKeys = 20000
)

var xpAmounts = [...]int64{15, 18, 20, 22, 25}

type xpGateEntry struct{ at time.Time }

func (b *Bot) registerLevelHandlers() {
	b.Session.AddHandler(b.onMessageCreateXP)
}

func (b *Bot) onMessageCreateXP(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" || m.Author == nil || m.Author.Bot || len(m.Content) == 0 {
		return
	}
	key := m.GuildID + ":" + m.Author.ID

	now := time.Now()
	b.xpMu.Lock()
	if e, ok := b.xpGate[key]; ok && now.Sub(e.at) < xpGateTTL {
		b.xpMu.Unlock()
		return
	}
	b.xpGate[key] = xpGateEntry{at: now}
	if len(b.xpGate) > xpGateMaxKeys {
		for k, e := range b.xpGate {
			if now.Sub(e.at) >= xpGateTTL {
				delete(b.xpGate, k)
			}
		}
	}
	b.xpMu.Unlock()

	b.safeEvent("xp", func(ctx context.Context) {
		delta := xpAmounts[rand.Intn(len(xpAmounts))]
		newXP, newLvl, up, err := b.Store.AwardXP(ctx, m.GuildID, m.Author.ID, delta)
		if err != nil {
			return
		}
		if up {
			slog.Info("level up", "guild_id", m.GuildID, "user_id", m.Author.ID,
				"level", newLvl, "xp", newXP)
		}
	})
}


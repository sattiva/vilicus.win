package discord

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
)


const (
	maxActivePolls   = 200
	maxPollOptions   = 10
	pollExpiryGrace  = 5 * time.Minute
	pollButtonPerRow = 5
)

type Poll struct {
	ID        string
	GuildID   string
	ChannelID string
	MessageID string
	Question  string
	Options   []string
	Votes     map[string]int
	EndsAt    time.Time
}

type pollStore struct {
	mu    sync.Mutex
	polls map[string]*Poll
}

func newPollStore() *pollStore {
	return &pollStore{polls: make(map[string]*Poll)}
}

func (ps *pollStore) create(p *Poll) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if len(ps.polls) >= maxActivePolls {
		var oldestID string
		var oldest time.Time
		first := true
		for id, existing := range ps.polls {
			if first || existing.EndsAt.Before(oldest) {
				oldestID, oldest, first = id, existing.EndsAt, false
			}
		}
		delete(ps.polls, oldestID)
	}
	ps.polls[p.ID] = p
}

func (ps *pollStore) get(id string) (*Poll, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, ok := ps.polls[id]
	return p, ok
}

func (ps *pollStore) delete(id string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.polls, id)
}

func newPollID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("p%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func (b *Bot) StartPoll(s *discordgo.Session, gid, channelID, question string, options []string, duration time.Duration) (string, error) {
	p := &Poll{
		ID:        newPollID(),
		GuildID:   gid,
		ChannelID: channelID,
		Question:  question,
		Options:   options,
		Votes:     make(map[string]int),
		EndsAt:    time.Now().UTC().Add(duration),
	}

	container := b.renderPoll(p)
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Flags:      components.FlagComponentsV2,
		Components: []discordgo.MessageComponent{container},
	})
	if err != nil {
		return "", err
	}
	p.MessageID = msg.ID

	b.polls.create(p)
	slog.Info("poll started", "poll_id", p.ID, "channel_id", channelID, "ends_at", p.EndsAt.Format(time.RFC3339))
	return msg.ID, nil
}

func (b *Bot) renderPoll(p *Poll) *components.Container {
	counts := make([]int, len(p.Options))
	for _, choice := range p.Votes {
		if choice >= 0 && choice < len(counts) {
			counts[choice]++
		}
	}

	lines := []discordgo.MessageComponent{
		components.TextDisplay{Content: "Poll"},
		components.Separator{Divider: true, Spacing: 1},
		components.TextDisplay{Content: truncateLine(p.Question, 500)},
		components.TextDisplay{Content: fmt.Sprintf("%d votes - closes <t:%d:R>", len(p.Votes), p.EndsAt.Unix())},
		components.Separator{Divider: true, Spacing: 1},
	}
	for idx, opt := range p.Options {
		lines = append(lines, components.TextDisplay{
			Content: fmt.Sprintf("**%d.** %s - %d", idx+1, truncateLine(opt, 120), counts[idx]),
		})
	}

	expiry := p.EndsAt.Add(pollExpiryGrace)
	buttons := make([]discordgo.MessageComponent, 0, len(p.Options))
	for idx := range p.Options {
		payload, _ := json.Marshal(pollPayload{ID: p.ID, Option: idx})
		buttons = append(buttons, discordgo.Button{
			Label:    fmt.Sprintf("%d", idx+1),
			Style:    discordgo.PrimaryButton,
			CustomID: components.BuildCustomID("poll", "vote", payload, expiry),
		})
	}

	children := []discordgo.MessageComponent{b.Container(lines...)}
	for start := 0; start < len(buttons); start += pollButtonPerRow {
		end := start + pollButtonPerRow
		if end > len(buttons) {
			end = len(buttons)
		}
		children = append(children, discordgo.ActionsRow{Components: buttons[start:end]})
	}
	return b.Container(children...)
}

func (b *Bot) editPollMessage(p *Poll) {
	container := b.renderPoll(p)
	_, err := b.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         p.MessageID,
		Channel:    p.ChannelID,
		Components: &[]discordgo.MessageComponent{container},
	})
	if err != nil {
		slog.Warn("poll message edit failed", "poll_id", p.ID, "err", err)
	}
}

type pollPayload struct {
	ID     string `json:"id"`
	Option int    `json:"o"`
}

func decodePollPayload(raw []byte) (string, int, bool) {
	var p pollPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.ID == "" {
		return "", 0, false
	}
	return p.ID, p.Option, true
}

func (b *Bot) handlePollVote(b2 *Bot, s *discordgo.Session, i *discordgo.InteractionCreate, ref *components.ComponentRef) {
	if ref.Action != "vote" {
		AckEphemeral(s, i, "Unknown poll control.")
		return
	}
	pollID, optIdx, ok := decodePollPayload([]byte(ref.Payload))
	if !ok {
		AckEphemeral(s, i, "This poll control is invalid.")
		return
	}

	b.polls.mu.Lock()
	p, exists := b.polls.polls[pollID]
	if !exists {
		b.polls.mu.Unlock()
		AckEphemeral(s, i, "This poll is no longer active.")
		return
	}
	if optIdx < 0 || optIdx >= len(p.Options) {
		b.polls.mu.Unlock()
		AckEphemeral(s, i, "That option does not exist.")
		return
	}
	now := time.Now().UTC()
	if now.After(p.EndsAt) {
		id := p.ID
		b.polls.mu.Unlock()
		b.polls.delete(id)
		AckEphemeral(s, i, "This poll has ended.")
		return
	}
	uid := interactionUserID(i)
	prev, hadPrev := p.Votes[uid]
	p.Votes[uid] = optIdx
	b.polls.mu.Unlock()

	b.editPollMessage(p)

	msg := fmt.Sprintf("Vote recorded: %d. %s", optIdx+1, p.Options[optIdx])
	if hadPrev && prev != optIdx {
		msg = fmt.Sprintf("Vote changed to: %d. %s", optIdx+1, p.Options[optIdx])
	}
	AckEphemeral(s, i, msg)
}

func truncateLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}


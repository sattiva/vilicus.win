package discord

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands/moderation"
)

func (b *Bot) registerComponentHandlers() {
	b.RegisterComponents("poll", b.handlePollVote)
	b.RegisterComponents("cases", b.handleCasesPage)
	b.RegisterComponents("rb", b.handleRoleBindingToggle)
	b.RegisterComponents("gw", b.handleGiveawayEnter)
}


type rbPayload struct {
	G string `json:"g"`
	R string `json:"r"`
}

func (b *Bot) handleRoleBindingToggle(b2 *Bot, s *discordgo.Session, i *discordgo.InteractionCreate, ref *components.ComponentRef) {
	if ref.Action != "role" || i.GuildID == "" {
		AckEphemeral(s, i, "This control is not available here.")
		return
	}
	var p rbPayload
	if err := json.Unmarshal([]byte(ref.Payload), &p); err != nil || p.R == "" {
		AckEphemeral(s, i, "This control is invalid.")
		return
	}
	if p.G != "" && p.G != i.GuildID {
		AckEphemeral(s, i, "This control belongs to a different server.")
		return
	}

	bindings, err := b.Store.ListRoleBindings(context.Background(), i.GuildID, i.Message.ID)
	if err != nil || len(bindings) == 0 {
		AckEphemeral(s, i, "This panel no longer exists.")
		return
	}
	listed := false
	for _, bd := range bindings {
		if bd.RoleID == p.R {
			listed = true
			break
		}
	}
	if !listed {
		AckEphemeral(s, i, "That role is not on this panel.")
		return
	}

	uid := interactionUserID(i)
	has := false
	if mem, _ := s.State.Member(i.GuildID, uid); mem != nil {
		for _, rid := range mem.Roles {
			if rid == p.R {
				has = true
				break
			}
		}
	}

	var msg string
	if has {
		if err := s.GuildMemberRoleRemove(i.GuildID, uid, p.R); err != nil {
			slog.Warn("role panel remove failed", "err", err)
			AckEphemeral(s, i, "Could not remove that role. Do I have Manage Roles and a higher position?")
			return
		}
		msg = "Removed <@&" + p.R + ">."
	} else {
		if err := s.GuildMemberRoleAdd(i.GuildID, uid, p.R); err != nil {
			slog.Warn("role panel add failed", "err", err)
			AckEphemeral(s, i, "Could not assign that role. Do I have Manage Roles and a higher position?")
			return
		}
		msg = "Gave you <@&" + p.R + ">."
	}
	AckEphemeral(s, i, msg)
}


type gwPayload struct {
	G  string `json:"g"`
	ID int64  `json:"id"`
}

func (b *Bot) handleGiveawayEnter(b2 *Bot, s *discordgo.Session, i *discordgo.InteractionCreate, ref *components.ComponentRef) {
	if ref.Action != "join" || i.GuildID == "" {
		AckEphemeral(s, i, "This control is not available here.")
		return
	}
	var p gwPayload
	if err := json.Unmarshal([]byte(ref.Payload), &p); err != nil || p.ID == 0 {
		AckEphemeral(s, i, "This control is invalid.")
		return
	}

	g, err := b.Store.GetGiveaway(context.Background(), p.ID)
	if err != nil {
		AckEphemeral(s, i, "That giveaway could not be found.")
		return
	}
	if g.GuildID != i.GuildID {
		AckEphemeral(s, i, "This control belongs to a different server.")
		return
	}
	if g.Drawn || time.Now().After(g.EndsAt) {
		AckEphemeral(s, i, "This giveaway has ended.")
		return
	}

	fresh, err := b.Store.AddGiveawayEntry(context.Background(), p.ID, interactionUserID(i))
	if err != nil {
		AckEphemeral(s, i, "Entry failed; try again in a moment.")
		return
	}
	if !fresh {
		AckEphemeral(s, i, "You are already entered. One entry per person.")
		return
	}
	AckEphemeral(s, i, "Entered. Good luck.")
}

func (b *Bot) handleCasesPage(b2 *Bot, s *discordgo.Session, i *discordgo.InteractionCreate, ref *components.ComponentRef) {
	if ref.Action != "page" {
		AckEphemeral(s, i, "Unknown history control.")
		return
	}
	gid, targetID, filter, page, ok := moderation.DecodeCasesPayload([]byte(ref.Payload))
	if !ok {
		AckEphemeral(s, i, "This history control is invalid.")
		return
	}
	if i.GuildID == "" || i.GuildID != gid {
		AckEphemeral(s, i, "This control belongs to a different server.")
		return
	}

	if !memberHasPerm(i, discordgo.PermissionManageMessages) {
		AckEphemeral(s, i, "You need the Manage Messages permission to browse case history.")
		return
	}

	container := moderation.RenderHistoryPage(b, gid, targetID, page, filter)
	if _, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         i.Message.ID,
		Channel:    i.ChannelID,
		Components: &[]discordgo.MessageComponent{container},
	}); err != nil {
		slog.Warn("history page edit failed", "err", err)
		AckEphemeral(s, i, "Failed to load that page.")
		return
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
}

func memberHasPerm(i *discordgo.InteractionCreate, perm int64) bool {
	if i.Member == nil {
		return false
	}
	if i.Member.Permissions&int64(discordgo.PermissionAdministrator) != 0 {
		return true
	}
	return i.Member.Permissions&perm != 0
}


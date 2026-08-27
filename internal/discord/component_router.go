package discord

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/logging"
)


type ComponentHandler func(b *Bot, s *discordgo.Session, i *discordgo.InteractionCreate, ref *components.ComponentRef)

type ModalHandler func(b *Bot, s *discordgo.Session, i *discordgo.InteractionCreate, ref *components.ComponentRef)

type componentRouter struct {
	mu       sync.RWMutex
	handlers map[string]ComponentHandler
	modals   map[string]ModalHandler
}

func newComponentRouter() *componentRouter {
	return &componentRouter{
		handlers: make(map[string]ComponentHandler),
		modals:   make(map[string]ModalHandler),
	}
}

func (r *componentRouter) register(ns string, h ComponentHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[strings.ToLower(ns)] = h
}

func (r *componentRouter) registerModal(ns string, h ModalHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modals[strings.ToLower(ns)] = h
}

func (b *Bot) RegisterComponents(ns string, h ComponentHandler) {
	b.compRouter.register(ns, h)
}

func (b *Bot) RegisterModals(ns string, h ModalHandler) {
	b.compRouter.registerModal(ns, h)
}

func (b *Bot) handleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	ref, err := components.ParseCustomID(data.CustomID)
	if err != nil {
		ackEphemeral(s, i, "This control is invalid or outdated.")
		return
	}
	if !ref.Expiry.IsZero() && time.Now().After(ref.Expiry) {
		ackEphemeral(s, i, "This control has expired.")
		return
	}

	b.compRouter.mu.RLock()
	h, ok := b.compRouter.handlers[ref.NS]
	b.compRouter.mu.RUnlock()

	if !ok {
		ackEphemeral(s, i, "This control is no longer active.")
		return
	}

	reqID := logging.NewID()
	slog.Info("component interaction", "ns", ref.NS, "action", ref.Action,
		"guild_id", i.GuildID, "user_id", interactionUserID(i), "req_id", reqID)

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("component handler panic", "ns", ref.NS, "err", rec, "req_id", reqID)
				ackEphemeral(s, i, "Something went wrong handling that control.")
			}
		}()
		h(b, s, i, ref)
	}()
}

func (b *Bot) handleModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	ref, err := components.ParseCustomID(data.CustomID)
	if err != nil {
		ackEphemeral(s, i, "This form is invalid or outdated.")
		return
	}

	b.compRouter.mu.RLock()
	h, ok := b.compRouter.modals[ref.NS]
	b.compRouter.mu.RUnlock()

	if !ok {
		ackEphemeral(s, i, "This form is no longer active.")
		return
	}

	reqID := logging.NewID()
	slog.Info("modal submit", "ns", ref.NS, "guild_id", i.GuildID, "user_id", interactionUserID(i), "req_id", reqID)

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("modal handler panic", "ns", ref.NS, "err", rec, "req_id", reqID)
				ackEphemeral(s, i, "Something went wrong handling that form.")
			}
		}()
		h(b, s, i, ref)
	}()
}

func ackEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func AckEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	ackEphemeral(s, i, msg)
}

func RespondContainer(s *discordgo.Session, i *discordgo.InteractionCreate, c *components.Container) error {
	resp, err := components.NewResponse(c)
	if err != nil {
		return err
	}
	return s.InteractionRespond(i.Interaction, resp)
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}


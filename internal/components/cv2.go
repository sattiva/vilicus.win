package components

import (
	"encoding/json"
	"errors"

	"github.com/bwmarrin/discordgo"
)

const (
	ComponentTypeActionRow         discordgo.ComponentType = 1
	ComponentTypeButton            discordgo.ComponentType = 2
	ComponentTypeSelectMenu        discordgo.ComponentType = 3
	ComponentTypeTextInput         discordgo.ComponentType = 4
	ComponentTypeUserSelect        discordgo.ComponentType = 5
	ComponentTypeRoleSelect        discordgo.ComponentType = 6
	ComponentTypeMentionableSelect discordgo.ComponentType = 7
	ComponentTypeChannelSelect     discordgo.ComponentType = 8
	ComponentTypeSection           discordgo.ComponentType = 9
	ComponentTypeTextDisplay       discordgo.ComponentType = 10
	ComponentTypeThumbnail         discordgo.ComponentType = 11
	ComponentTypeMediaGallery      discordgo.ComponentType = 12
	ComponentTypeFile              discordgo.ComponentType = 13
	ComponentTypeSeparator         discordgo.ComponentType = 14
	ComponentTypeContainer         discordgo.ComponentType = 15

	DefaultGreyAccent = 0x2B2D31
	FlagComponentsV2  = 1 << 15
)

type Container struct {
	AccentColor *int                         `json:"accent_color,omitempty"`
	Spoiler     bool                         `json:"spoiler,omitempty"`
	Components  []discordgo.MessageComponent `json:"components"`
}

func (c Container) Type() discordgo.ComponentType {
	return ComponentTypeContainer
}

func (c Container) MarshalJSON() ([]byte, error) {
	type Alias Container
	return json.Marshal(&struct {
		Type discordgo.ComponentType `json:"type"`
		Alias
	}{
		Type:  ComponentTypeContainer,
		Alias: (Alias)(c),
	})
}

type TextDisplay struct {
	Content string `json:"content"`
}

func (t TextDisplay) Type() discordgo.ComponentType {
	return ComponentTypeTextDisplay
}

func (t TextDisplay) MarshalJSON() ([]byte, error) {
	type Alias TextDisplay
	return json.Marshal(&struct {
		Type discordgo.ComponentType `json:"type"`
		Alias
	}{
		Type:  ComponentTypeTextDisplay,
		Alias: (Alias)(t),
	})
}

type Section struct {
	Accessory  discordgo.MessageComponent   `json:"accessory,omitempty"`
	Components []discordgo.MessageComponent `json:"components"`
}

func (s Section) Type() discordgo.ComponentType {
	return ComponentTypeSection
}

func (s Section) MarshalJSON() ([]byte, error) {
	type Alias Section
	return json.Marshal(&struct {
		Type discordgo.ComponentType `json:"type"`
		Alias
	}{
		Type:  ComponentTypeSection,
		Alias: (Alias)(s),
	})
}

type Separator struct {
	Divider bool `json:"divider,omitempty"`
	Spacing int  `json:"spacing,omitempty"`
}

func (s Separator) Type() discordgo.ComponentType {
	return ComponentTypeSeparator
}

func (s Separator) MarshalJSON() ([]byte, error) {
	type Alias Separator
	return json.Marshal(&struct {
		Type discordgo.ComponentType `json:"type"`
		Alias
	}{
		Type:  ComponentTypeSeparator,
		Alias: (Alias)(s),
	})
}

type MediaGalleryItem struct {
	Media       MediaItem `json:"media"`
	Description string    `json:"description,omitempty"`
	Spoiler     bool      `json:"spoiler,omitempty"`
}

type MediaItem struct {
	URL string `json:"url"`
}

type MediaGallery struct {
	Items []MediaGalleryItem `json:"items"`
}

func (m MediaGallery) Type() discordgo.ComponentType {
	return ComponentTypeMediaGallery
}

func (m MediaGallery) MarshalJSON() ([]byte, error) {
	type Alias MediaGallery
	return json.Marshal(&struct {
		Type discordgo.ComponentType `json:"type"`
		Alias
	}{
		Type:  ComponentTypeMediaGallery,
		Alias: (Alias)(m),
	})
}

func NewBaseContainer(children ...discordgo.MessageComponent) *Container {
	col := DefaultGreyAccent
	return &Container{
		AccentColor: &col,
		Components:  children,
	}
}

func NewCustomContainer(accentColor int, footer string, children ...discordgo.MessageComponent) *Container {
	items := make([]discordgo.MessageComponent, 0, len(children)+2)
	items = append(items, children...)

	if footer != "" {
		items = append(items, Separator{Divider: true, Spacing: 1})
		items = append(items, TextDisplay{Content: footer})
	}

	return &Container{
		AccentColor: &accentColor,
		Components:  items,
	}
}

func Validate(c *Container) error {
	if c == nil {
		return errors.New("nil container")
	}
	if len(c.Components) == 0 {
		return errors.New("empty container")
	}
	if len(c.Components) > 40 {
		return errors.New("components limit exceeded (max 40)")
	}
	return nil
}

func NewResponse(c *Container) (*discordgo.InteractionResponse, error) {
	if err := Validate(c); err != nil {
		return nil, err
	}
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:      FlagComponentsV2,
			Components: []discordgo.MessageComponent{c},
		},
	}, nil
}

func NewResponseData(c *Container) (*discordgo.InteractionResponseData, error) {
	if err := Validate(c); err != nil {
		return nil, err
	}
	return &discordgo.InteractionResponseData{
		Flags:      FlagComponentsV2,
		Components: []discordgo.MessageComponent{c},
	}, nil
}

func NewWebhookEdit(c *Container) (*discordgo.WebhookEdit, error) {
	if err := Validate(c); err != nil {
		return nil, err
	}
	return &discordgo.WebhookEdit{
		Components: &[]discordgo.MessageComponent{c},
	}, nil
}


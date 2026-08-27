package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
)

type HierarchyEngine struct{}

func GetHighestRolePosition(g *discordgo.Guild, m *discordgo.Member) int {
	if m == nil || g == nil {
		return 0
	}
	if g.OwnerID == m.User.ID {
		return 999999
	}
	top := 0
	roleMap := make(map[string]*discordgo.Role, len(g.Roles))
	for _, r := range g.Roles {
		roleMap[r.ID] = r
	}
	for _, rid := range m.Roles {
		if r, ok := roleMap[rid]; ok {
			if r.Position > top {
				top = r.Position
			}
		}
	}
	return top
}

func CanModerate(g *discordgo.Guild, caller *discordgo.Member, target *discordgo.Member) (bool, string) {
	if caller == nil || target == nil || g == nil {
		return false, "Invalid context"
	}
	if caller.User.ID == target.User.ID {
		return false, "Cannot target yourself"
	}
	if target.User.ID == g.OwnerID {
		return false, "Cannot target guild owner"
	}
	if caller.User.ID == g.OwnerID {
		return true, ""
	}

	callerPos := GetHighestRolePosition(g, caller)
	targetPos := GetHighestRolePosition(g, target)

	if callerPos <= targetPos {
		return false, "Target has equal or higher role hierarchy"
	}
	return true, ""
}

func CanBotModerate(g *discordgo.Guild, botMember *discordgo.Member, target *discordgo.Member) (bool, string) {
	if botMember == nil || target == nil || g == nil {
		return false, "Invalid context"
	}
	if target.User.ID == g.OwnerID {
		return false, "Bot cannot target guild owner"
	}

	botPos := GetHighestRolePosition(g, botMember)
	targetPos := GetHighestRolePosition(g, target)

	if botPos <= targetPos {
		return false, "Bot role is lower or equal to target role hierarchy"
	}
	return true, ""
}

func CanManageRole(g *discordgo.Guild, caller *discordgo.Member, botMember *discordgo.Member, role *discordgo.Role) (bool, string) {
	if g == nil || caller == nil || botMember == nil || role == nil {
		return false, "Invalid context"
	}
	if role.Managed {
		return false, "Cannot manage integration/managed role"
	}
	if role.ID == g.ID {
		return false, "Cannot manage @everyone role"
	}

	botPos := GetHighestRolePosition(g, botMember)
	if role.Position >= botPos {
		return false, "Target role is higher or equal to bot highest role"
	}

	if caller.User.ID != g.OwnerID {
		callerPos := GetHighestRolePosition(g, caller)
		if role.Position >= callerPos {
			return false, "Target role is higher or equal to your highest role"
		}
	}

	return true, ""
}

func DispatchAudit(ctx context.Context, b BotInterface, s *discordgo.Session, gid, modID, targetID, action, reason, extra string) {
	_ = b.GetStore().LogAudit(ctx, gid, modID, targetID, action, reason, extra)

	gcfg, err := b.GetStore().GetGuildConfig(ctx, gid)
	if err != nil || gcfg.LogChannelID == "" {
		return
	}

	container := b.Container(
		components.TextDisplay{Content: fmt.Sprintf("Audit Log: %s", action)},
		components.Separator{Divider: true, Spacing: 1},
		components.Section{
			Components: []discordgo.MessageComponent{
				components.TextDisplay{Content: fmt.Sprintf("Moderator: <@%s> (%s)", modID, modID)},
				components.TextDisplay{Content: fmt.Sprintf("Target: <@%s> (%s)", targetID, targetID)},
				components.TextDisplay{Content: fmt.Sprintf("Reason: %s", reason)},
				components.TextDisplay{Content: fmt.Sprintf("Details: %s", extra)},
				components.TextDisplay{Content: fmt.Sprintf("Timestamp: %s", time.Now().UTC().Format(time.RFC3339))},
			},
		},
	)

	_, _ = s.ChannelMessageSendComplex(gcfg.LogChannelID, &discordgo.MessageSend{
		Flags:      components.FlagComponentsV2,
		Components: []discordgo.MessageComponent{container},
	})
}


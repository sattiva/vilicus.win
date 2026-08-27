package protection

import "time"


const (
	ModuleGuildUpdate   = 1
	ModuleChannelCreate = 10
	ModuleChannelDelete = 12
	ModuleKick          = 20
	ModuleMemberPrune   = 21
	ModuleBanAdd        = 22
	ModuleBotAdd        = 28
	ModuleRoleCreate    = 30
	ModuleRoleUpdate    = 31
	ModuleRoleDelete    = 32
	ModuleWebhookCreate = 50
	ModuleBulkDelete    = 73
)

var ModuleWeights = map[int]int{
	ModuleGuildUpdate:   30,
	ModuleChannelCreate: 15,
	ModuleChannelDelete: 25,
	ModuleKick:          20,
	ModuleMemberPrune:   40,
	ModuleBanAdd:        35,
	ModuleBotAdd:        25,
	ModuleRoleCreate:    10,
	ModuleRoleUpdate:    20,
	ModuleRoleDelete:    30,
	ModuleWebhookCreate: 25,
	ModuleBulkDelete:    15,
}

var ModuleLabels = map[int]string{
	ModuleGuildUpdate:   "guild update",
	ModuleChannelCreate: "channel create",
	ModuleChannelDelete: "channel delete",
	ModuleKick:          "kick",
	ModuleMemberPrune:   "member prune",
	ModuleBanAdd:        "ban",
	ModuleBotAdd:        "bot add",
	ModuleRoleCreate:    "role create",
	ModuleRoleUpdate:    "role update",
	ModuleRoleDelete:    "role delete",
	ModuleWebhookCreate: "webhook create",
	ModuleBulkDelete:    "bulk message delete",
}

type Event struct {
	At     time.Time
	Weight int
	Module int
	Target string
}

func Score(events []Event, now time.Time, window time.Duration) (int, []Event) {
	cut := now.Add(-window)
	kept := events[:0]
	score := 0
	for _, e := range events {
		if e.At.After(cut) {
			kept = append(kept, e)
			score += e.Weight
		}
	}
	return score, kept
}

const (
	PunishTimeout = "timeout"
	PunishKick    = "kick"
	PunishBan     = "ban"
)

func NormalizePunish(s string) string {
	switch s {
	case PunishTimeout, PunishKick:
		return s
	default:
		return PunishBan
	}
}

func ValidPunish(s string) bool {
	return s == PunishTimeout || s == PunishKick || s == PunishBan
}


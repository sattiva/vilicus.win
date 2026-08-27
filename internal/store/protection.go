package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"vilicus/internal/protection"
)


type ProtectionConfig struct {
	GuildID         string `json:"guild_id"`
	AntispamEnabled bool   `json:"antispam_enabled"`
	AntispamMsgs    int    `json:"antispam_msgs"`
	AntispamWindow  int    `json:"antispam_window_seconds"`
	AntilinkMode    string `json:"antilink_mode"`
	FilterWords     string `json:"filter_words"`
	HoneypotChannel string `json:"honeypot_channel_id"`
	HoneypotAction  string `json:"honeypot_action"`
}

var ErrProtectionUnconfigured = errors.New("protection not configured")

func scanProtection(row interface{ Scan(...any) error }) (*ProtectionConfig, error) {
	var c ProtectionConfig
	var enabled int
	err := row.Scan(&c.GuildID, &enabled, &c.AntispamMsgs, &c.AntispamWindow, &c.AntilinkMode, &c.FilterWords, &c.HoneypotChannel, &c.HoneypotAction)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProtectionUnconfigured
	}
	if err != nil {
		return nil, err
	}
	c.AntispamEnabled = enabled == 1
	return &c, nil
}

func (s *Store) GetProtectionConfig(ctx context.Context, gid string) (*ProtectionConfig, error) {
	return scanProtection(s.db.QueryRowContext(ctx,
		`SELECT guild_id, antispam_enabled, antispam_msgs, antispam_window_seconds, antilink_mode, filter_words, honeypot_channel_id, honeypot_action
		 FROM protection_config WHERE guild_id = ?`, gid))
}

func (s *Store) SaveProtectionConfig(ctx context.Context, c *ProtectionConfig) error {
	if c.AntispamMsgs < 3 {
		c.AntispamMsgs = 3
	}
	if c.AntispamMsgs > 30 {
		c.AntispamMsgs = 30
	}
	if c.AntispamWindow < 2 {
		c.AntispamWindow = 2
	}
	if c.AntispamWindow > 30 {
		c.AntispamWindow = 30
	}
	switch c.AntilinkMode {
	case "mods", "on":
	default:
		c.AntilinkMode = "off"
	}
	c.HoneypotChannel = strings.TrimSpace(c.HoneypotChannel)
	c.HoneypotAction = protection.NormalizePunish(c.HoneypotAction)

	words := strings.Split(c.FilterWords, ",")
	clean := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" {
			clean = append(clean, w)
		}
	}
	c.FilterWords = strings.Join(clean, ",")

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO protection_config
			(guild_id, antispam_enabled, antispam_msgs, antispam_window_seconds, antilink_mode, filter_words, honeypot_channel_id, honeypot_action)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (guild_id) DO UPDATE SET
			antispam_enabled = excluded.antispam_enabled,
			antispam_msgs = excluded.antispam_msgs,
			antispam_window_seconds = excluded.antispam_window_seconds,
			antilink_mode = excluded.antilink_mode,
			filter_words = excluded.filter_words,
			honeypot_channel_id = excluded.honeypot_channel_id,
			honeypot_action = excluded.honeypot_action,
			updated_at = CURRENT_TIMESTAMP`,
		c.GuildID, boolInt(c.AntispamEnabled), c.AntispamMsgs, c.AntispamWindow, c.AntilinkMode, c.FilterWords, c.HoneypotChannel, c.HoneypotAction)
	return err
}


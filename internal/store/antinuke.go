package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"vilicus/internal/protection"
)


type AntinukeConfig struct {
	GuildID        string `json:"guild_id"`
	Enabled        bool   `json:"enabled"`
	Punish         string `json:"punish"`
	Threshold      int    `json:"threshold"`
	WindowSeconds  int    `json:"window_seconds"`
	Whitelist      string `json:"whitelist"`
	AlertChannelID string `json:"alert_channel_id"`
}

var ErrAntinukeUnconfigured = errors.New("antinuke not configured")

func scanAntinuke(row interface{ Scan(...any) error }) (*AntinukeConfig, error) {
	var c AntinukeConfig
	var enabled int
	err := row.Scan(&c.GuildID, &enabled, &c.Punish, &c.Threshold, &c.WindowSeconds, &c.Whitelist, &c.AlertChannelID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAntinukeUnconfigured
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1
	return &c, nil
}

func (s *Store) GetAntinukeConfig(ctx context.Context, gid string) (*AntinukeConfig, error) {
	return scanAntinuke(s.db.QueryRowContext(ctx,
		`SELECT guild_id, enabled, punish, threshold, window_seconds, whitelist, alert_channel_id
		 FROM antinuke_config WHERE guild_id = ?`, gid))
}

func (s *Store) EnabledAntinukeGuilds(ctx context.Context) ([]*AntinukeConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id, enabled, punish, threshold, window_seconds, whitelist, alert_channel_id
		 FROM antinuke_config WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AntinukeConfig
	for rows.Next() {
		c, err := scanAntinuke(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SaveAntinukeConfig(ctx context.Context, c *AntinukeConfig) error {
	switch c.Punish {
	case protection.PunishTimeout, protection.PunishKick:
	default:
		c.Punish = protection.PunishBan
	}
	if c.Threshold < 20 {
		c.Threshold = 20
	}
	if c.Threshold > 1000 {
		c.Threshold = 1000
	}
	if c.WindowSeconds < 10 {
		c.WindowSeconds = 10
	}
	if c.WindowSeconds > 300 {
		c.WindowSeconds = 300
	}

	fields := strings.Split(c.Whitelist, ",")
	seen := make(map[string]bool, len(fields))
	clean := make([]string, 0, len(fields))
	for _, id := range fields {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	c.Whitelist = strings.Join(clean, ",")

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO antinuke_config
			(guild_id, enabled, punish, threshold, window_seconds, whitelist, alert_channel_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (guild_id) DO UPDATE SET
			enabled = excluded.enabled,
			punish = excluded.punish,
			threshold = excluded.threshold,
			window_seconds = excluded.window_seconds,
			whitelist = excluded.whitelist,
			alert_channel_id = excluded.alert_channel_id,
			updated_at = CURRENT_TIMESTAMP`,
		c.GuildID, boolInt(c.Enabled), c.Punish, c.Threshold, c.WindowSeconds, c.Whitelist, c.AlertChannelID)
	return err
}


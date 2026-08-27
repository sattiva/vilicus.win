package store

import (
	"context"
	"database/sql"
	"errors"
)


type StarboardConfig struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	Threshold int    `json:"threshold"`
	Enabled   bool   `json:"enabled"`
}

type StarboardPost struct {
	ID         int64  `json:"id"`
	GuildID    string `json:"guild_id"`
	SourceID   string `json:"source_msg_id"`
	BoardMsgID string `json:"board_msg_id"`
	Stars      int    `json:"stars"`
}

var ErrStarboardDisabled = errors.New("starboard not configured")

func scanStarboardConfig(row interface{ Scan(...any) error }) (*StarboardConfig, error) {
	var c StarboardConfig
	var enabled int
	err := row.Scan(&c.GuildID, &c.ChannelID, &c.Threshold, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrStarboardDisabled
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1
	return &c, nil
}

func (s *Store) GetStarboardConfig(ctx context.Context, gid string) (*StarboardConfig, error) {
	return scanStarboardConfig(s.db.QueryRowContext(ctx,
		`SELECT guild_id, channel_id, threshold, enabled FROM starboard_config WHERE guild_id = ?`, gid))
}

func (s *Store) SaveStarboardConfig(ctx context.Context, c *StarboardConfig) error {
	if c.Threshold < 1 {
		c.Threshold = 1
	}
	if c.Threshold > 25 {
		c.Threshold = 25
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO starboard_config (guild_id, channel_id, threshold, enabled)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (guild_id) DO UPDATE SET channel_id = excluded.channel_id,
			threshold = excluded.threshold, enabled = excluded.enabled`,
		c.GuildID, c.ChannelID, c.Threshold, boolInt(c.Enabled))
	return err
}

func (s *Store) AddStar(ctx context.Context, gid, sourceMsgID string) (int, string, error) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO starboard_posts (guild_id, source_msg_id, stars) VALUES (?, ?, 1)
		 ON CONFLICT (guild_id, source_msg_id) DO UPDATE SET stars = stars + 1`, gid, sourceMsgID)
	if err != nil {
		return 0, "", err
	}
	var stars int
	var board sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT stars, board_msg_id FROM starboard_posts WHERE guild_id = ? AND source_msg_id = ?`,
		gid, sourceMsgID).Scan(&stars, &board)
	if err != nil {
		return 0, "", err
	}
	return stars, board.String, nil
}

func (s *Store) RemoveStar(ctx context.Context, gid, sourceMsgID string) (int, string, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE starboard_posts SET stars = MAX(stars - 1, 0) WHERE guild_id = ? AND source_msg_id = ?`,
		gid, sourceMsgID)
	if err != nil {
		return 0, "", err
	}
	var stars int
	var board sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT stars, board_msg_id FROM starboard_posts WHERE guild_id = ? AND source_msg_id = ?`,
		gid, sourceMsgID).Scan(&stars, &board)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	return stars, board.String, nil
}

func (s *Store) SetStarboardBoardMessage(ctx context.Context, gid, sourceMsgID, boardMsgID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE starboard_posts SET board_msg_id = ? WHERE guild_id = ? AND source_msg_id = ?`,
		boardMsgID, gid, sourceMsgID)
	return err
}

func (s *Store) ListStarboardPosts(ctx context.Context, gid string, limit int) ([]StarboardPost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, guild_id, source_msg_id, board_msg_id, stars FROM starboard_posts
		 WHERE guild_id = ? AND stars > 0 ORDER BY stars DESC LIMIT ?`, gid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StarboardPost
	for rows.Next() {
		var p StarboardPost
		if err := rows.Scan(&p.ID, &p.GuildID, &p.SourceID, &p.BoardMsgID, &p.Stars); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}


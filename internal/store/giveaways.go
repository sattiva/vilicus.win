package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)


type Giveaway struct {
	ID        int64     `json:"id"`
	GuildID   string    `json:"guild_id"`
	ChannelID string    `json:"channel_id"`
	MessageID string    `json:"message_id"`
	Prize     string    `json:"prize"`
	Winners   int       `json:"winners"`
	WinnerIDs []string  `json:"winner_ids,omitempty"`
	EndsAt    time.Time `json:"ends_at"`
	Drawn     bool      `json:"drawn"`
	HostedBy  string    `json:"hosted_by"`
	CreatedAt time.Time `json:"created_at"`
}

type GiveawayEntry struct {
	ID        int64     `json:"id"`
	GiveawayI int64     `json:"giveaway_id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

var ErrGiveawayNotFound = errors.New("giveaway not found")

func scanGiveaway(row interface{ Scan(...any) error }) (*Giveaway, error) {
	var g Giveaway
	var drawn int
	var winnerIDs string
	err := row.Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.MessageID, &g.Prize,
		&g.Winners, &winnerIDs, &g.EndsAt, &drawn, &g.HostedBy, &g.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGiveawayNotFound
	}
	if err != nil {
		return nil, err
	}
	g.Drawn = drawn == 1
	if winnerIDs != "" {
		g.WinnerIDs = strings.Split(winnerIDs, ",")
	}
	return &g, nil
}

const giveawayCols = `id, guild_id, channel_id, message_id, prize, winners, winner_ids, ends_at, drawn, hosted_by, created_at`

func (s *Store) CreateGiveaway(ctx context.Context, gid, channelID, prize string, winners int, endsAt time.Time, hostedBy string) (*Giveaway, error) {
	if winners < 1 {
		winners = 1
	}
	if winners > 20 {
		winners = 20
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO giveaways (guild_id, channel_id, prize, winners, ends_at, hosted_by)
		 VALUES (?, ?, ?, ?, ?, ?)`, gid, channelID, prize, winners, endsAt.UTC(), hostedBy)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetGiveaway(ctx, id)
}

func (s *Store) GetGiveaway(ctx context.Context, id int64) (*Giveaway, error) {
	return scanGiveaway(s.db.QueryRowContext(ctx,
		`SELECT `+giveawayCols+` FROM giveaways WHERE id = ?`, id))
}

func (s *Store) GetGiveawayByMessage(ctx context.Context, gid, messageID string) (*Giveaway, error) {
	return scanGiveaway(s.db.QueryRowContext(ctx,
		`SELECT `+giveawayCols+` FROM giveaways WHERE guild_id = ? AND message_id = ?`, gid, messageID))
}

func (s *Store) SetGiveawayMessage(ctx context.Context, id int64, messageID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE giveaways SET message_id = ? WHERE id = ?`, messageID, id)
	return err
}

func (s *Store) DueGiveaways(ctx context.Context, now time.Time, limit int) ([]Giveaway, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+giveawayCols+` FROM giveaways WHERE drawn = 0 AND ends_at <= ?
		 ORDER BY ends_at ASC LIMIT ?`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Giveaway
	for rows.Next() {
		var g Giveaway
		var drawn int
		var winnerIDs string
		if err := rows.Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.MessageID, &g.Prize,
			&g.Winners, &winnerIDs, &g.EndsAt, &drawn, &g.HostedBy, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.Drawn = drawn == 1
		if winnerIDs != "" {
			g.WinnerIDs = strings.Split(winnerIDs, ",")
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) ListGiveaways(ctx context.Context, gid string) ([]Giveaway, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+giveawayCols+` FROM giveaways WHERE guild_id = ? ORDER BY created_at DESC`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Giveaway
	for rows.Next() {
		g, err := scanGiveaway(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (s *Store) SetGiveawayWinners(ctx context.Context, id int64, winnerIDs []string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE giveaways SET winner_ids = ? WHERE id = ?`, strings.Join(winnerIDs, ","), id)
	return err
}

func (s *Store) MarkGiveawayDrawn(ctx context.Context, id int64) bool {
	res, err := s.db.ExecContext(ctx,
		`UPDATE giveaways SET drawn = 1 WHERE id = ? AND drawn = 0`, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func (s *Store) AddGiveawayEntry(ctx context.Context, giveawayID int64, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO giveaway_entries (giveaway_id, user_id) VALUES (?, ?)`,
		giveawayID, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *Store) ListGiveawayEntries(ctx context.Context, giveawayID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM giveaway_entries WHERE giveaway_id = ? ORDER BY id ASC`, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}


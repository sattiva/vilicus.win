package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)


const xpAwardCooldown = time.Minute

func XPLevelFor(xp int64) int64 {
	var lvl int64
	var need int64 = 100
	for xp >= need {
		xp -= need
		lvl++
		need = 5*lvl*lvl + 50*lvl + 100
	}
	return lvl
}

func XPToNext(xp int64) int64 {
	var lvl int64
	var need int64 = 100
	for xp >= need {
		xp -= need
		lvl++
		need = 5*lvl*lvl + 50*lvl + 100
	}
	return need - xp
}

type XPRow struct {
	GuildID string    `json:"guild_id"`
	UserID  string    `json:"user_id"`
	XP      int64     `json:"xp"`
	Level   int64     `json:"level"`
	LastAt  time.Time `json:"-"`
}

var ErrXPCooldown = errors.New("xp award on cooldown")

func (s *Store) AwardXP(ctx context.Context, gid, uid string, delta int64) (int64, int64, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT xp, last_award FROM user_xp WHERE guild_id = ? AND user_id = ?`, gid, uid)
	var cur int64
	var last sql.NullTime
	err := row.Scan(&cur, &last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) && time.Since(last.Time) < xpAwardCooldown {
		return 0, 0, false, ErrXPCooldown
	}

	newXP := cur + delta
	newLvl := XPLevelFor(newXP)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_xp (guild_id, user_id, xp, level, last_award)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (guild_id, user_id) DO UPDATE SET
			xp = excluded.xp, level = excluded.level, last_award = excluded.last_award`,
		gid, uid, newXP, newLvl, time.Now().UTC())
	if err != nil {
		return 0, 0, false, err
	}
	return newXP, newLvl, newLvl > XPLevelFor(cur), nil
}

func (s *Store) GetUserXP(ctx context.Context, gid, uid string) (*XPRow, error) {
	r := &XPRow{}
	err := s.db.QueryRowContext(ctx,
		`SELECT guild_id, user_id, xp, level, last_award FROM user_xp WHERE guild_id = ? AND user_id = ?`,
		gid, uid).Scan(&r.GuildID, &r.UserID, &r.XP, &r.Level, &r.LastAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) Leaderboard(ctx context.Context, gid string, limit int) ([]XPRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id, user_id, xp, level, last_award FROM user_xp
		 WHERE guild_id = ? ORDER BY xp DESC LIMIT ?`, gid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []XPRow
	for rows.Next() {
		var r XPRow
		if err := rows.Scan(&r.GuildID, &r.UserID, &r.XP, &r.Level, &r.LastAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}


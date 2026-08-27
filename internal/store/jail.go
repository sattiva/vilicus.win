package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)


var ErrJailBackupNotFound = errors.New("store: no jail backup for that user")

type JailBackup struct {
	GuildID   string    `json:"guild_id"`
	UserID    string    `json:"user_id"`
	Roles     []string  `json:"roles"`
	JailedBy  string    `json:"jailed_by"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) SaveJailBackup(ctx context.Context, gid, userID, jailedBy, reason string, roles []string) error {
	clean := make([]string, 0, len(roles))
	for _, r := range roles {
		if r = strings.TrimSpace(r); r != "" {
			clean = append(clean, r)
		}
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO jail_backups (guild_id, user_id, roles, jailed_by, reason, created_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(guild_id, user_id) DO UPDATE SET
    roles=excluded.roles,
    jailed_by=excluded.jailed_by,
    reason=excluded.reason,
    created_at=CURRENT_TIMESTAMP`,
		gid, userID, strings.Join(clean, ","), jailedBy, reason)
	return err
}

func (s *Store) GetJailBackup(ctx context.Context, gid, userID string) (*JailBackup, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT guild_id, user_id, roles, jailed_by, reason, created_at FROM jail_backups WHERE guild_id = ? AND user_id = ?`,
		gid, userID)
	var b JailBackup
	var joined string
	if err := row.Scan(&b.GuildID, &b.UserID, &joined, &b.JailedBy, &b.Reason, &b.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrJailBackupNotFound
		}
		return nil, err
	}
	if joined != "" {
		b.Roles = strings.Split(joined, ",")
	}
	return &b, nil
}

func (s *Store) ListJailBackups(ctx context.Context, gid string) ([]JailBackup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id, user_id, roles, jailed_by, reason, created_at FROM jail_backups WHERE guild_id = ? ORDER BY created_at DESC`,
		gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []JailBackup
	for rows.Next() {
		var b JailBackup
		var joined string
		if err := rows.Scan(&b.GuildID, &b.UserID, &joined, &b.JailedBy, &b.Reason, &b.CreatedAt); err != nil {
			return nil, err
		}
		if joined != "" {
			b.Roles = strings.Split(joined, ",")
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) DeleteJailBackup(ctx context.Context, gid, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jail_backups WHERE guild_id = ? AND user_id = ?`, gid, userID)
	return err
}


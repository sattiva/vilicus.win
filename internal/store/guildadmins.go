package store

import (
	"context"
	"time"
)

const (
	RoleSuperadmin = "superadmin"
	RoleAdmin      = "admin"
	RoleViewer     = "viewer"
)

func NormalizeAdminRole(role string) string {
	switch role {
	case RoleSuperadmin:
		return RoleSuperadmin
	case RoleViewer:
		return RoleViewer
	default:
		return RoleAdmin
	}
}

func ValidAdminRole(role string) bool {
	return role == RoleSuperadmin || role == RoleAdmin || role == RoleViewer
}

type GuildAdmin struct {
	GuildID       string    `json:"guild_id"`
	DiscordUserID string    `json:"discord_user_id"`
	GrantedBy     string    `json:"granted_by"`
	GrantedAt     time.Time `json:"granted_at"`
}


func (s *Store) AddGuildAdmin(ctx context.Context, gid, uid, grantedBy string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO guild_admins (guild_id, discord_user_id, granted_by)
VALUES (?, ?, ?)
ON CONFLICT(guild_id, discord_user_id) DO UPDATE SET granted_by = excluded.granted_by`,
		gid, uid, grantedBy)
	return err
}

func (s *Store) RemoveGuildAdmin(ctx context.Context, gid, uid string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM guild_admins WHERE guild_id = ? AND discord_user_id = ?`, gid, uid)
	return err
}

func (s *Store) IsGuildAdmin(ctx context.Context, gid, uid string) bool {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM guild_admins WHERE guild_id = ? AND discord_user_id = ?`, gid, uid).Scan(&one)
	return err == nil && one == 1
}

func (s *Store) ListGuildAdmins(ctx context.Context, gid string) ([]GuildAdmin, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id, discord_user_id, granted_by, granted_at FROM guild_admins WHERE guild_id = ? ORDER BY granted_at DESC`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GuildAdmin
	for rows.Next() {
		var g GuildAdmin
		if err := rows.Scan(&g.GuildID, &g.DiscordUserID, &g.GrantedBy, &g.GrantedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) ListGuildAdminGuilds(ctx context.Context, uid string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id FROM guild_admins WHERE discord_user_id = ? ORDER BY guild_id ASC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var gid string
		if err := rows.Scan(&gid); err != nil {
			return nil, err
		}
		out = append(out, gid)
	}
	return out, rows.Err()
}

func (s *Store) ListAllGuildAdmins(ctx context.Context) ([]GuildAdmin, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id, discord_user_id, granted_by, granted_at FROM guild_admins ORDER BY granted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GuildAdmin
	for rows.Next() {
		var g GuildAdmin
		if err := rows.Scan(&g.GuildID, &g.DiscordUserID, &g.GrantedBy, &g.GrantedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) UpdateAdminRole(ctx context.Context, uid, role string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admins SET role = ? WHERE discord_user_id = ?`, NormalizeAdminRole(role), uid)
	return err
}


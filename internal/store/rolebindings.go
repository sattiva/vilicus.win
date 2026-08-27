package store

import (
	"context"
)


type RoleBinding struct {
	ID        int64  `json:"id"`
	GuildID   string `json:"guild_id"`
	MessageID string `json:"message_id"`
	RoleID    string `json:"role_id"`
	Label     string `json:"label"`
}

func (s *Store) AddRoleBindings(ctx context.Context, gid, messageID, createdBy string, bindings []RoleBinding) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, b := range bindings {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO role_bindings (guild_id, message_id, role_id, label, created_by)
			 VALUES (?, ?, ?, ?, ?)`, gid, messageID, b.RoleID, b.Label, createdBy); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListRoleBindings(ctx context.Context, gid, messageID string) ([]RoleBinding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, guild_id, message_id, role_id, label FROM role_bindings
		 WHERE guild_id = ? AND message_id = ? ORDER BY id ASC`, gid, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoleBinding
	for rows.Next() {
		var b RoleBinding
		if err := rows.Scan(&b.ID, &b.GuildID, &b.MessageID, &b.RoleID, &b.Label); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRoleBindings(ctx context.Context, gid, messageID string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM role_bindings WHERE guild_id = ? AND message_id = ?`, gid, messageID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}


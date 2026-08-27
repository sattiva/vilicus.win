package store

import (
	"context"
	"errors"
	"strings"
	"time"
)


type Reminder struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	GuildID   string    `json:"guild_id"`
	ChannelID string    `json:"channel_id"`
	Body      string    `json:"body"`
	DueAt     time.Time `json:"due_at"`
	Delivered bool      `json:"delivered"`
	CreatedAt time.Time `json:"created_at"`
}

type TempRole struct {
	ID        int64     `json:"id"`
	GuildID   string    `json:"guild_id"`
	UserID    string    `json:"user_id"`
	RoleID    string    `json:"role_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Removed   bool      `json:"removed"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateReminder(ctx context.Context, userID, gid, channelID, body string, dueAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reminders (user_id, guild_id, channel_id, body, due_at) VALUES (?, ?, ?, ?, ?)`,
		userID, gid, channelID, body, dueAt.UTC())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DueReminders(ctx context.Context, now time.Time, limit int) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, guild_id, channel_id, body, due_at, delivered, created_at FROM reminders WHERE delivered = 0 AND due_at <= ? ORDER BY due_at ASC LIMIT ?`,
		now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Reminder
	for rows.Next() {
		var r Reminder
		if err := rows.Scan(&r.ID, &r.UserID, &r.GuildID, &r.ChannelID, &r.Body, &r.DueAt, &r.Delivered, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) MarkReminderDelivered(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET delivered = 1 WHERE id = ?`, id)
	return err
}

func (s *Store) AddTempRole(ctx context.Context, gid, userID, roleID string, expiresAt time.Time, createdBy string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO temp_roles (guild_id, user_id, role_id, expires_at, created_by) VALUES (?, ?, ?, ?, ?)`,
		gid, userID, roleID, expiresAt.UTC(), createdBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DueTempRoles(ctx context.Context, now time.Time, limit int) ([]TempRole, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, guild_id, user_id, role_id, expires_at, removed, created_by, created_at FROM temp_roles WHERE removed = 0 AND expires_at <= ? ORDER BY expires_at ASC LIMIT ?`,
		now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []TempRole
	for rows.Next() {
		var t TempRole
		if err := rows.Scan(&t.ID, &t.GuildID, &t.UserID, &t.RoleID, &t.ExpiresAt, &t.Removed, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) MarkTempRoleRemoved(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE temp_roles SET removed = 1 WHERE id = ?`, id)
	return err
}

type TempBan struct {
	ID        int64     `json:"id"`
	GuildID   string    `json:"guild_id"`
	UserID    string    `json:"user_id"`
	Reason    string    `json:"reason"`
	ExpiresAt time.Time `json:"expires_at"`
	Unbanned  bool      `json:"unbanned"`
	CreatedBy string    `json:"created_by"`
	CaseNo    int64     `json:"case_no"`
	CreatedAt time.Time `json:"created_at"`
}

var ErrActiveTempBan = errors.New("user already has an active tempban")

func (s *Store) CreateTempBan(ctx context.Context, gid, userID, reason string, expiresAt time.Time, createdBy string, caseNo int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO temp_bans (guild_id, user_id, reason, expires_at, created_by, case_no) VALUES (?, ?, ?, ?, ?, ?)`,
		gid, userID, reason, expiresAt.UTC(), createdBy, caseNo)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrActiveTempBan
	}
	return err
}

func (s *Store) DueTempBans(ctx context.Context, now time.Time, limit int) ([]TempBan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, guild_id, user_id, reason, expires_at, unbanned, created_by, case_no, created_at FROM temp_bans WHERE unbanned = 0 AND expires_at <= ? ORDER BY expires_at ASC LIMIT ?`,
		now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []TempBan
	for rows.Next() {
		var t TempBan
		var unbanned int
		if err := rows.Scan(&t.ID, &t.GuildID, &t.UserID, &t.Reason, &t.ExpiresAt, &unbanned, &t.CreatedBy, &t.CaseNo, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Unbanned = unbanned == 1
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) MarkTempBanUnbanned(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE temp_bans SET unbanned = 1 WHERE id = ?`, id)
	return err
}

type DashAuditEntry struct {
	ID        int64     `json:"id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	IP        string    `json:"ip"`
	ReqID     string    `json:"req_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) LogDashAudit(ctx context.Context, actorID, action, detail, ip, reqID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dashboard_audit_log (actor_id, action, detail, ip, req_id) VALUES (?, ?, ?, ?, ?)`,
		actorID, action, detail, ip, reqID)
	return err
}

func (s *Store) ListDashAudit(ctx context.Context, limit int) ([]DashAuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, actor_id, action, detail, ip, req_id, created_at FROM dashboard_audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []DashAuditEntry
	for rows.Next() {
		var e DashAuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.Detail, &e.IP, &e.ReqID, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}


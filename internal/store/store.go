package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type GuildConfig struct {
	GuildID          string    `json:"guild_id"`
	Prefix           string    `json:"prefix"`
	LogChannelID     string    `json:"log_channel_id"`
	WelcomeChannelID string    `json:"welcome_channel_id"`
	AutoRoleID       string    `json:"auto_role_id"`
	JailRoleID       string    `json:"jail_role_id"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type UserConfig struct {
	UserID    string    `json:"user_id"`
	Prefix    string    `json:"prefix"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Admin struct {
	DiscordUserID string    `json:"discord_user_id"`
	Username      string    `json:"username"`
	Role          string    `json:"role"`
	AddedAt       time.Time `json:"added_at"`
}

type Session struct {
	ID            string    `json:"id"`
	DiscordUserID string    `json:"discord_user_id"`
	Username      string    `json:"username"`
	Avatar        string    `json:"avatar"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`

	Epoch int `json:"csrf_epoch"`
}

type CommandLog struct {
	ID          int64  `json:"id"`
	CommandName string `json:"command_name"`
	GuildID     string `json:"guild_id"`
	UserID      string `json:"user_id"`
	Status      string `json:"status"`
	ExecutionMS int64  `json:"execution_ms"`
	AckMS     int64     `json:"ack_ms,omitempty"`
	SendMS    int64     `json:"send_ms,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Spans struct {
	AckMS  int64
	SendMS int64
}

var NoSpans = Spans{AckMS: -1, SendMS: -1}

type AuditLogEntry struct {
	ID          int64     `json:"id"`
	GuildID     string    `json:"guild_id"`
	ModeratorID string    `json:"moderator_id"`
	TargetID    string    `json:"target_id"`
	Action      string    `json:"action"`
	Reason      string    `json:"reason"`
	Extra       string    `json:"extra"`
	CreatedAt   time.Time `json:"created_at"`
}

type BotSettings struct {
	BotName      string `json:"bot_name"`
	AccentColor  string `json:"accent_color"`
	FooterText   string `json:"footer_text"`
	ActivityType string `json:"activity_type"`
	ActivityName string `json:"activity_name"`
}

const (
	cachePosTTL   = 5 * time.Minute
	cacheNegTTL   = 30 * time.Second
	cfgCacheCap   = 10000
	usrCacheCap   = 50000
	logQueueCap   = 1024
	writerFlushMS = 250
	writerBatch   = 64
	maxOpenConns  = 5
)

type cacheItem struct {
	val any
	exp time.Time
}

func (it cacheItem) expired(now time.Time) bool {
	return now.After(it.exp)
}

type Store struct {
	db        *sql.DB
	cacheLock sync.RWMutex
	cfgCache  map[string]cacheItem
	usrCache  map[string]cacheItem
	setCache  map[string]string

	stopOnce sync.Once
	stopCh   chan struct{}

	logQ       chan CommandLog
	droppedCmd atomic.Int64

	stmtGetGuild      *sql.Stmt
	stmtUpsertGuild   *sql.Stmt
	stmtGetUser       *sql.Stmt
	stmtUpsertUser    *sql.Stmt
	stmtDeleteUser    *sql.Stmt
	stmtGetSession    *sql.Stmt
	stmtCreateSession *sql.Stmt
	stmtDeleteSession *sql.Stmt
	stmtGetAdmin      *sql.Stmt
	stmtListAdmins    *sql.Stmt
	stmtAddAdmin      *sql.Stmt
	stmtDeleteAdmin   *sql.Stmt
	stmtPruneLogs     *sql.Stmt
	stmtPruneSessions *sql.Stmt
	stmtGetSetting    *sql.Stmt
	stmtSetSetting    *sql.Stmt
	stmtLogAudit      *sql.Stmt
	stmtGetAuditLogs  *sql.Stmt
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=cache_size(-8000)&_pragma=temp_store(MEMORY)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	s := &Store{
		db:       db,
		cfgCache: make(map[string]cacheItem),
		usrCache: make(map[string]cacheItem),
		setCache: make(map[string]string),
		stopCh:   make(chan struct{}),
		logQ:     make(chan CommandLog, logQueueCap),
	}

	if err := s.Migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := s.prepare(); err != nil {
		return nil, err
	}

	if err := s.initDefaultSettings(); err != nil {
		return nil, err
	}

	s.startBackground()
	return s, nil
}

func (s *Store) startBackground() {
	go s.writerLoop()
	go s.janitorLoop()
}

func (s *Store) writerLoop() {
	tick := time.NewTicker(writerFlushMS * time.Millisecond)
	defer tick.Stop()

	buf := make([]CommandLog, 0, writerBatch)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		s.insertCommandLogs(buf)
		buf = buf[:0]
	}

	for {
		select {
		case <-s.stopCh:
			for {
				select {
				case cl := <-s.logQ:
					buf = append(buf, cl)
					if len(buf) >= writerBatch {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case cl := <-s.logQ:
			buf = append(buf, cl)
			if len(buf) >= writerBatch {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

func (s *Store) insertCommandLogs(batch []CommandLog) {
	tx, err := s.db.Begin()
	if err != nil {
		s.droppedCmd.Add(int64(len(batch)))
		return
	}
	stmt, err := tx.Prepare(`INSERT INTO command_usage_log (command_name, guild_id, user_id, status, execution_ms, ack_ms, send_ms, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		s.droppedCmd.Add(int64(len(batch)))
		return
	}
	msOrNull := func(ms int64) any {
		if ms < 0 {
			return nil
		}
		return ms
	}
	for _, cl := range batch {
		if _, err := stmt.Exec(cl.CommandName, cl.GuildID, cl.UserID, cl.Status, cl.ExecutionMS,
			msOrNull(cl.AckMS), msOrNull(cl.SendMS), cl.CreatedAt.UTC()); err != nil {
			s.droppedCmd.Add(1)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		s.droppedCmd.Add(int64(len(batch)))
	}
}

func (s *Store) janitorLoop() {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-tick.C:
			now := time.Now()
			s.cacheLock.Lock()
			for k, it := range s.cfgCache {
				if it.expired(now) {
					delete(s.cfgCache, k)
				}
			}
			for k, it := range s.usrCache {
				if it.expired(now) {
					delete(s.usrCache, k)
				}
			}
			s.cacheLock.Unlock()
		}
	}
}

func (s *Store) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *Store) MaxConns() int { return maxOpenConns }

func (s *Store) DroppedCommands() int64 { return s.droppedCmd.Load() }

func (s *Store) QueueDepth() int { return len(s.logQ) }

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) SizeBytes(ctx context.Context) (int64, error) {
	var pages, pageSize int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); err != nil {
		return 0, err
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return 0, err
	}
	return pages * pageSize, nil
}

func (s *Store) Close() error {
	s.Stop()
	for _, st := range []*sql.Stmt{
		s.stmtGetGuild, s.stmtUpsertGuild, s.stmtGetUser, s.stmtUpsertUser,
		s.stmtDeleteUser, s.stmtGetSession, s.stmtCreateSession, s.stmtDeleteSession,
		s.stmtGetAdmin, s.stmtListAdmins, s.stmtAddAdmin, s.stmtDeleteAdmin,
		s.stmtPruneLogs, s.stmtPruneSessions, s.stmtGetSetting, s.stmtSetSetting,
		s.stmtLogAudit, s.stmtGetAuditLogs,
	} {
		if st != nil {
			_ = st.Close()
		}
	}
	return s.db.Close()
}

func (s *Store) prepare() error {
	var err error

	s.stmtGetGuild, err = s.db.Prepare(`SELECT guild_id, prefix, log_channel_id, welcome_channel_id, auto_role_id, jail_role_id, updated_at FROM guild_config WHERE guild_id = ?`)
	if err != nil {
		return err
	}

	s.stmtUpsertGuild, err = s.db.Prepare(`
INSERT INTO guild_config (guild_id, prefix, log_channel_id, welcome_channel_id, auto_role_id, jail_role_id, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(guild_id) DO UPDATE SET
    prefix=excluded.prefix,
    log_channel_id=excluded.log_channel_id,
    welcome_channel_id=excluded.welcome_channel_id,
    auto_role_id=excluded.auto_role_id,
    jail_role_id=excluded.jail_role_id,
    updated_at=CURRENT_TIMESTAMP
`)
	if err != nil {
		return err
	}

	s.stmtGetUser, err = s.db.Prepare(`SELECT user_id, prefix, updated_at FROM user_config WHERE user_id = ?`)
	if err != nil {
		return err
	}

	s.stmtUpsertUser, err = s.db.Prepare(`
INSERT INTO user_config (user_id, prefix, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_id) DO UPDATE SET
    prefix=excluded.prefix,
    updated_at=CURRENT_TIMESTAMP
`)
	if err != nil {
		return err
	}

	s.stmtDeleteUser, err = s.db.Prepare(`DELETE FROM user_config WHERE user_id = ?`)
	if err != nil {
		return err
	}

	s.stmtGetSession, err = s.db.Prepare(`SELECT id, discord_user_id, username, avatar, expires_at, created_at, csrf_epoch FROM dashboard_sessions WHERE id = ? AND expires_at > CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}

	s.stmtCreateSession, err = s.db.Prepare(`INSERT INTO dashboard_sessions (id, discord_user_id, username, avatar, expires_at, created_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`)
	if err != nil {
		return err
	}

	s.stmtDeleteSession, err = s.db.Prepare(`DELETE FROM dashboard_sessions WHERE id = ?`)
	if err != nil {
		return err
	}

	s.stmtGetAdmin, err = s.db.Prepare(`SELECT discord_user_id, username, role, added_at FROM admins WHERE discord_user_id = ?`)
	if err != nil {
		return err
	}

	s.stmtListAdmins, err = s.db.Prepare(`SELECT discord_user_id, username, role, added_at FROM admins ORDER BY added_at ASC`)
	if err != nil {
		return err
	}

	s.stmtAddAdmin, err = s.db.Prepare(`INSERT INTO admins (discord_user_id, username, role, added_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP) ON CONFLICT(discord_user_id) DO UPDATE SET username=excluded.username, role=excluded.role`)
	if err != nil {
		return err
	}

	s.stmtDeleteAdmin, err = s.db.Prepare(`DELETE FROM admins WHERE discord_user_id = ?`)
	if err != nil {
		return err
	}

	s.stmtPruneLogs, err = s.db.Prepare(`DELETE FROM command_usage_log WHERE created_at < datetime('now', ?)`)
	if err != nil {
		return err
	}

	s.stmtPruneSessions, err = s.db.Prepare(`DELETE FROM dashboard_sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}

	s.stmtGetSetting, err = s.db.Prepare(`SELECT val FROM bot_settings WHERE key = ?`)
	if err != nil {
		return err
	}

	s.stmtSetSetting, err = s.db.Prepare(`INSERT INTO bot_settings (key, val) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET val=excluded.val`)
	if err != nil {
		return err
	}

	s.stmtLogAudit, err = s.db.Prepare(`INSERT INTO moderation_audit_log (guild_id, moderator_id, target_id, action, reason, extra, created_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`)
	if err != nil {
		return err
	}

	s.stmtGetAuditLogs, err = s.db.Prepare(`SELECT id, guild_id, moderator_id, target_id, action, reason, extra, created_at FROM moderation_audit_log WHERE guild_id = ? ORDER BY id DESC LIMIT ?`)
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) initDefaultSettings() error {
	defaults := map[string]string{
		"bot_name":      "Vilicus",
		"accent_color":  "0x2B2D31",
		"footer_text":   "Vilicus Framework",
		"activity_type": "Playing",
		"activity_name": ".help | Vilicus",
	}

	for k, v := range defaults {
		var cur string
		err := s.stmtGetSetting.QueryRow(k).Scan(&cur)
		if err == sql.ErrNoRows {
			_, _ = s.stmtSetSetting.Exec(k, v)
			s.setCache[k] = v
		} else if err == nil {
			s.setCache[k] = cur
		}
	}
	return nil
}

func (s *Store) GetSetting(k, def string) string {
	s.cacheLock.RLock()
	if v, ok := s.setCache[k]; ok {
		s.cacheLock.RUnlock()
		return v
	}
	s.cacheLock.RUnlock()

	var val string
	err := s.stmtGetSetting.QueryRow(k).Scan(&val)
	if err != nil {
		return def
	}

	s.cacheLock.Lock()
	s.setCache[k] = val
	s.cacheLock.Unlock()
	return val
}

func (s *Store) SetSetting(k, v string) error {
	_, err := s.stmtSetSetting.Exec(k, v)
	if err != nil {
		return err
	}
	s.cacheLock.Lock()
	s.setCache[k] = v
	s.cacheLock.Unlock()
	return nil
}

func (s *Store) GetBotSettings() *BotSettings {
	return &BotSettings{
		BotName:      s.GetSetting("bot_name", "Vilicus"),
		AccentColor:  s.GetSetting("accent_color", "0x2B2D31"),
		FooterText:   s.GetSetting("footer_text", "Vilicus Framework"),
		ActivityType: s.GetSetting("activity_type", "Playing"),
		ActivityName: s.GetSetting("activity_name", ".help | Vilicus"),
	}
}

func (s *Store) ParseAccentColor() int {
	raw := s.GetSetting("accent_color", "0x2B2D31")
	raw = strings.TrimPrefix(raw, "#")
	raw = strings.TrimPrefix(raw, "0x")
	val, err := strconv.ParseInt(raw, 16, 64)
	if err != nil {
		return 0x2B2D31
	}
	return int(val)
}

func cloneGuildConfig(c *GuildConfig) *GuildConfig {
	cc := *c
	return &cc
}

func cloneUserConfig(u *UserConfig) *UserConfig {
	uu := *u
	return &uu
}

func evictLocked(m map[string]cacheItem, cap int) {
	if len(m) < cap {
		return
	}
	now := time.Now()
	for k, it := range m {
		if it.expired(now) {
			delete(m, k)
		}
	}
	for k := range m {
		if len(m) < cap {
			return
		}
		delete(m, k)
	}
}

func (s *Store) GetGuildConfig(ctx context.Context, gid string) (*GuildConfig, error) {
	now := time.Now()
	s.cacheLock.RLock()
	if it, ok := s.cfgCache[gid]; ok && !it.expired(now) {
		s.cacheLock.RUnlock()
		return cloneGuildConfig(it.val.(*GuildConfig)), nil
	}
	s.cacheLock.RUnlock()

	c := &GuildConfig{GuildID: gid, Prefix: "."}
	row := s.stmtGetGuild.QueryRowContext(ctx, gid)
	err := row.Scan(&c.GuildID, &c.Prefix, &c.LogChannelID, &c.WelcomeChannelID, &c.AutoRoleID, &c.JailRoleID, &c.UpdatedAt)

	ttl := cachePosTTL
	if err == sql.ErrNoRows {
		ttl = cacheNegTTL
	} else if err != nil {
		return nil, err
	}

	s.cacheLock.Lock()
	evictLocked(s.cfgCache, cfgCacheCap)
	s.cfgCache[gid] = cacheItem{val: c, exp: now.Add(ttl)}
	s.cacheLock.Unlock()

	return cloneGuildConfig(c), nil
}

func (s *Store) SaveGuildConfig(ctx context.Context, c *GuildConfig) error {
	if c.Prefix == "" {
		c.Prefix = "."
	}
	_, err := s.stmtUpsertGuild.ExecContext(ctx, c.GuildID, c.Prefix, c.LogChannelID, c.WelcomeChannelID, c.AutoRoleID, c.JailRoleID)
	if err != nil {
		return err
	}

	s.cacheLock.Lock()
	s.cfgCache[c.GuildID] = cacheItem{val: cloneGuildConfig(c), exp: time.Now().Add(cachePosTTL)}
	s.cacheLock.Unlock()
	return nil
}

func (s *Store) GetUserConfig(ctx context.Context, uid string) (*UserConfig, error) {
	now := time.Now()
	s.cacheLock.RLock()
	if it, ok := s.usrCache[uid]; ok && !it.expired(now) {
		s.cacheLock.RUnlock()
		return cloneUserConfig(it.val.(*UserConfig)), nil
	}
	s.cacheLock.RUnlock()

	u := &UserConfig{UserID: uid, Prefix: ""}
	row := s.stmtGetUser.QueryRowContext(ctx, uid)
	err := row.Scan(&u.UserID, &u.Prefix, &u.UpdatedAt)

	ttl := cachePosTTL
	if err == sql.ErrNoRows {
		ttl = cacheNegTTL
	} else if err != nil {
		return nil, err
	}

	s.cacheLock.Lock()
	evictLocked(s.usrCache, usrCacheCap)
	s.usrCache[uid] = cacheItem{val: u, exp: now.Add(ttl)}
	s.cacheLock.Unlock()

	return cloneUserConfig(u), nil
}

func (s *Store) SaveUserConfig(ctx context.Context, u *UserConfig) error {
	_, err := s.stmtUpsertUser.ExecContext(ctx, u.UserID, u.Prefix)
	if err != nil {
		return err
	}

	s.cacheLock.Lock()
	s.usrCache[u.UserID] = cacheItem{val: cloneUserConfig(u), exp: time.Now().Add(cachePosTTL)}
	s.cacheLock.Unlock()
	return nil
}

func (s *Store) DeleteUserConfig(ctx context.Context, uid string) error {
	_, err := s.stmtDeleteUser.ExecContext(ctx, uid)
	if err != nil {
		return err
	}

	s.cacheLock.Lock()
	delete(s.usrCache, uid)
	s.cacheLock.Unlock()
	return nil
}

func (s *Store) ResolvePrefix(ctx context.Context, gid, uid string) string {
	if uid != "" {
		if u, err := s.GetUserConfig(ctx, uid); err == nil && u.Prefix != "" {
			return u.Prefix
		}
	}
	if gid != "" {
		if g, err := s.GetGuildConfig(ctx, gid); err == nil && g.Prefix != "" {
			return g.Prefix
		}
	}
	return "."
}

func (s *Store) LogAudit(ctx context.Context, gid, modID, targetID, action, reason, extra string) error {
	_, err := s.stmtLogAudit.ExecContext(ctx, gid, modID, targetID, action, reason, extra)
	return err
}

func (s *Store) GetAuditLogs(ctx context.Context, gid string, limit int) ([]AuditLogEntry, error) {
	rows, err := s.stmtGetAuditLogs.QueryContext(ctx, gid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		if err := rows.Scan(&e.ID, &e.GuildID, &e.ModeratorID, &e.TargetID, &e.Action, &e.Reason, &e.Extra, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (s *Store) ListGuildConfigs(ctx context.Context) ([]GuildConfig, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT guild_id, prefix, log_channel_id, welcome_channel_id, auto_role_id, updated_at FROM guild_config ORDER BY guild_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []GuildConfig
	for rows.Next() {
		var c GuildConfig
		if err := rows.Scan(&c.GuildID, &c.Prefix, &c.LogChannelID, &c.WelcomeChannelID, &c.AutoRoleID, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) LogCommand(ctx context.Context, cmd, gid, uid, status string, ms int64, spans ...Spans) error {
	cl := CommandLog{
		CommandName: cmd,
		GuildID:     gid,
		UserID:      uid,
		Status:      status,
		ExecutionMS: ms,
		AckMS:       -1,
		SendMS:      -1,
		CreatedAt:   time.Now().UTC(),
	}
	if len(spans) > 0 {
		cl.AckMS = spans[0].AckMS
		cl.SendMS = spans[0].SendMS
	}
	select {
	case s.logQ <- cl:
		return nil
	default:
		s.droppedCmd.Add(1)
		return nil
	}
}

func (s *Store) GetRecentLogs(ctx context.Context, limit int) ([]CommandLog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, command_name, guild_id, user_id, status, execution_ms, created_at FROM command_usage_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []CommandLog
	for rows.Next() {
		var l CommandLog
		if err := rows.Scan(&l.ID, &l.CommandName, &l.GuildID, &l.UserID, &l.Status, &l.ExecutionMS, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *Store) CountLogs(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM command_usage_log`).Scan(&count)
	return count, err
}

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.stmtCreateSession.ExecContext(ctx, sess.ID, sess.DiscordUserID, sess.Username, sess.Avatar, sess.ExpiresAt)
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	sess := &Session{}
	row := s.stmtGetSession.QueryRowContext(ctx, id)
	err := row.Scan(&sess.ID, &sess.DiscordUserID, &sess.Username, &sess.Avatar, &sess.ExpiresAt, &sess.CreatedAt, &sess.Epoch)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.stmtDeleteSession.ExecContext(ctx, id)
	return err
}

func (s *Store) DeleteAllSessions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM dashboard_sessions`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) BumpAllSessionEpochs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE dashboard_sessions SET csrf_epoch = csrf_epoch + 1`)
	return err
}

func (s *Store) IsAdmin(ctx context.Context, uid string, seedAdmins []string) bool {
	for _, a := range seedAdmins {
		if a == uid {
			return true
		}
	}
	adm, err := s.GetAdmin(ctx, uid)
	return err == nil && adm != nil
}

func (s *Store) GetAdmin(ctx context.Context, uid string) (*Admin, error) {
	adm := &Admin{}
	row := s.stmtGetAdmin.QueryRowContext(ctx, uid)
	err := row.Scan(&adm.DiscordUserID, &adm.Username, &adm.Role, &adm.AddedAt)
	if err != nil {
		return nil, err
	}
	return adm, nil
}

func (s *Store) ListAdmins(ctx context.Context) ([]Admin, error) {
	rows, err := s.stmtListAdmins.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Admin
	for rows.Next() {
		var a Admin
		if err := rows.Scan(&a.DiscordUserID, &a.Username, &a.Role, &a.AddedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) AddAdmin(ctx context.Context, uid, name, role string) error {
	_, err := s.stmtAddAdmin.ExecContext(ctx, uid, name, role)
	return err
}

func (s *Store) DeleteAdmin(ctx context.Context, uid string) error {
	if _, err := s.stmtDeleteAdmin.ExecContext(ctx, uid); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM guild_admins WHERE discord_user_id = ?`, uid)
	return err
}

func (s *Store) Prune(retentionDays, auditDays int) error {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	if auditDays <= 0 {
		auditDays = 180
	}
	mod := fmt.Sprintf("-%d days", retentionDays)
	auditMod := fmt.Sprintf("-%d days", auditDays)

	if _, err := s.stmtPruneLogs.Exec(mod); err != nil {
		return err
	}
	if _, err := s.stmtPruneSessions.Exec(); err != nil {
		return err
	}
	colds := []struct{ q, mod string }{
		{`DELETE FROM moderation_audit_log WHERE created_at < datetime('now', ?)`, auditMod},
		{`DELETE FROM dashboard_audit_log WHERE created_at < datetime('now', ?)`, auditMod},
		{`DELETE FROM reminders WHERE delivered = 1 AND due_at < datetime('now', ?)`, mod},
		{`DELETE FROM temp_roles WHERE removed = 1 AND expires_at < datetime('now', ?)`, auditMod},
	}
	for _, c := range colds {
		if _, err := s.db.Exec(c.q, c.mod); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Checkpoint() error {
	_, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

func (s *Store) Analyze() error {
	_, err := s.db.Exec(`ANALYZE`)
	return err
}

func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}


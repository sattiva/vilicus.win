package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
)


type migration struct {
	version int
	name    string
	up      string
}

var migrations = []migration{
	{1, "baseline", baselineSchema},
	{2, "cases_audit_reminders_temproles", v2Schema},
	{3, "temp_bans", v3Schema},
	{4, "engagement_starboard_bindings_protection_xp_giveaways", v4Schema},
	{5, "automation_rules", v5Schema},
	{6, "antinuke_honeypot", v6Schema},
	{7, "jail", v7Schema},
	{8, "authz_guild_admins", v8Schema},
	{9, "cases_fts_csrf_epoch", v9Schema},
	{10, "cmd_log_latency_spans", v10Schema},
}

const baselineSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS guild_config (
    guild_id TEXT PRIMARY KEY,
    prefix TEXT NOT NULL DEFAULT '.',
    log_channel_id TEXT NOT NULL DEFAULT '',
    welcome_channel_id TEXT NOT NULL DEFAULT '',
    auto_role_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_config (
    user_id TEXT PRIMARY KEY,
    prefix TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS admins (
    discord_user_id TEXT PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'admin',
    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS dashboard_sessions (
    id TEXT PRIMARY KEY,
    discord_user_id TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS command_usage_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command_name TEXT NOT NULL,
    guild_id TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'success',
    execution_ms INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bot_settings (
    key TEXT PRIMARY KEY,
    val TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS moderation_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    moderator_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    action TEXT NOT NULL,
    reason TEXT NOT NULL,
    extra TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cmd_log_guild_time ON command_usage_log(guild_id, created_at);
CREATE INDEX IF NOT EXISTS idx_cmd_log_time ON command_usage_log(created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON dashboard_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_mod_audit_guild_time ON moderation_audit_log(guild_id, created_at);
`

const v2Schema = `
CREATE TABLE IF NOT EXISTS mod_cases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    case_no INTEGER NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('warn','mute','kick','ban','tempban','jail','unjail','note','unban','timeout')),
    moderator_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMP,
    active INTEGER NOT NULL DEFAULT 1,
    actor_kind TEXT NOT NULL DEFAULT 'discord',
    req_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (guild_id, case_no)
);
CREATE INDEX IF NOT EXISTS idx_cases_target ON mod_cases(guild_id, target_id, created_at);
CREATE INDEX IF NOT EXISTS idx_cases_expiry ON mod_cases(guild_id, expires_at) WHERE active = 1;

CREATE TABLE IF NOT EXISTS case_notes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    case_id INTEGER NOT NULL REFERENCES mod_cases(id) ON DELETE CASCADE,
    author_id TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_case_notes_case ON case_notes(case_id, created_at);

CREATE TABLE IF NOT EXISTS dashboard_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    req_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dal_time ON dashboard_audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_dal_actor ON dashboard_audit_log(actor_id, created_at);

CREATE TABLE IF NOT EXISTS reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    body TEXT NOT NULL,
    due_at TIMESTAMP NOT NULL,
    delivered INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_reminders_due ON reminders(due_at) WHERE delivered = 0;

CREATE TABLE IF NOT EXISTS temp_roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    removed INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_temp_roles_due ON temp_roles(expires_at) WHERE removed = 0;
`

const v3Schema = `
CREATE TABLE IF NOT EXISTS temp_bans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMP NOT NULL,
    unbanned INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL DEFAULT '',
    case_no INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_temp_bans_due ON temp_bans(expires_at) WHERE unbanned = 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_temp_bans_active ON temp_bans(guild_id, user_id) WHERE unbanned = 0;
`

const v4Schema = `
CREATE TABLE IF NOT EXISTS starboard_config (
    guild_id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL DEFAULT '',
    threshold INTEGER NOT NULL DEFAULT 3,
    enabled INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS starboard_posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    source_msg_id TEXT NOT NULL,
    board_msg_id TEXT NOT NULL DEFAULT '',
    stars INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (guild_id, source_msg_id)
);

CREATE TABLE IF NOT EXISTS role_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (guild_id, message_id, role_id)
);
CREATE INDEX IF NOT EXISTS idx_role_bindings_msg ON role_bindings(guild_id, message_id);

CREATE TABLE IF NOT EXISTS protection_config (
    guild_id TEXT PRIMARY KEY,
    antispam_enabled INTEGER NOT NULL DEFAULT 0,
    antispam_msgs INTEGER NOT NULL DEFAULT 6,
    antispam_window_seconds INTEGER NOT NULL DEFAULT 5,
    antilink_mode TEXT NOT NULL DEFAULT 'off',
    filter_words TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_xp (
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    xp INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 0,
    last_award TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
    PRIMARY KEY (guild_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_user_xp_rank ON user_xp(guild_id, xp DESC);

CREATE TABLE IF NOT EXISTS giveaways (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    message_id TEXT NOT NULL DEFAULT '',
    prize TEXT NOT NULL,
    winners INTEGER NOT NULL DEFAULT 1,
    winner_ids TEXT NOT NULL DEFAULT '',
    ends_at TIMESTAMP NOT NULL,
    drawn INTEGER NOT NULL DEFAULT 0,
    hosted_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_giveaways_due ON giveaways(ends_at) WHERE drawn = 0;

CREATE TABLE IF NOT EXISTS giveaway_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    giveaway_id INTEGER NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (giveaway_id, user_id)
);
`

const v5Schema = `
-- Automation engine (plan.md section 5.3 P2, 02 doc v2): typed rows instead of JSON
-- blobs so every column is queryable and validated at write time.
CREATE TABLE IF NOT EXISTS automation_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    name TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    trigger TEXT NOT NULL,
    trigger_arg TEXT NOT NULL DEFAULT '',
    channels TEXT NOT NULL DEFAULT '',
    actors TEXT NOT NULL DEFAULT 'any',
    min_account_age INTEGER NOT NULL DEFAULT 0,
    require_roles TEXT NOT NULL DEFAULT '',
    forbid_roles TEXT NOT NULL DEFAULT '',
    phrases TEXT NOT NULL DEFAULT '',
    pattern TEXT NOT NULL DEFAULT '',
    links INTEGER NOT NULL DEFAULT 0,
    min_mentions INTEGER NOT NULL DEFAULT 0,
    cooldown_seconds INTEGER NOT NULL DEFAULT 0,
    counter_limit INTEGER NOT NULL DEFAULT 0,
    counter_window INTEGER NOT NULL DEFAULT 0,
    actions TEXT NOT NULL DEFAULT '',
    template TEXT NOT NULL DEFAULT '',
    last_run TIMESTAMP,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (guild_id, name)
);
CREATE INDEX IF NOT EXISTS idx_automation_guild ON automation_rules(guild_id);
CREATE INDEX IF NOT EXISTS idx_automation_interval_due
    ON automation_rules(last_run) WHERE enabled = 1 AND trigger = 'interval';
`

const v6Schema = `
-- Protection depth (plan.md section 5.3 P3): honeypot trap channel rides on the
-- existing per-guild protection row; antinuke gets its own typed table since
-- only the audit-log sweeper reads it, not the message pipeline.
ALTER TABLE protection_config ADD COLUMN honeypot_channel_id TEXT NOT NULL DEFAULT '';
ALTER TABLE protection_config ADD COLUMN honeypot_action TEXT NOT NULL DEFAULT 'ban';

CREATE TABLE IF NOT EXISTS antinuke_config (
    guild_id TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL DEFAULT 0,
    punish TEXT NOT NULL DEFAULT 'ban',
    threshold INTEGER NOT NULL DEFAULT 100,
    window_seconds INTEGER NOT NULL DEFAULT 60,
    whitelist TEXT NOT NULL DEFAULT '',
    alert_channel_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const v7Schema = `
-- Jail/unjail (deferred backlog): a configured holding role on guild_config
-- plus a per-user role snapshot so unjail restores exactly what jail removed.
ALTER TABLE guild_config ADD COLUMN jail_role_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS jail_backups (
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    roles TEXT NOT NULL DEFAULT '',
    jailed_by TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (guild_id, user_id)
);
`

const v8Schema = `
CREATE TABLE IF NOT EXISTS guild_admins (
    guild_id TEXT NOT NULL,
    discord_user_id TEXT NOT NULL,
    granted_by TEXT NOT NULL DEFAULT '',
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (guild_id, discord_user_id)
);
CREATE INDEX IF NOT EXISTS idx_guild_admins_user ON guild_admins(discord_user_id);
`

const v9Schema = `
ALTER TABLE dashboard_sessions ADD COLUMN csrf_epoch INTEGER NOT NULL DEFAULT 0;

CREATE VIRTUAL TABLE IF NOT EXISTS cases_fts USING fts5(
    body, reason, guild_id UNINDEXED, src UNINDEXED, rid UNINDEXED,
    tokenize='porter'
);

CREATE TRIGGER IF NOT EXISTS trg_cases_fts_ai AFTER INSERT ON mod_cases BEGIN
    INSERT INTO cases_fts(body, reason, guild_id, src, rid)
    VALUES ('', NEW.reason, NEW.guild_id, 'case', NEW.id);
END;
CREATE TRIGGER IF NOT EXISTS trg_cases_fts_au AFTER UPDATE OF reason ON mod_cases BEGIN
    DELETE FROM cases_fts WHERE src = 'case' AND rid = NEW.id;
    INSERT INTO cases_fts(body, reason, guild_id, src, rid)
    VALUES ('', NEW.reason, NEW.guild_id, 'case', NEW.id);
END;
CREATE TRIGGER IF NOT EXISTS trg_cases_fts_ad AFTER DELETE ON mod_cases BEGIN
    DELETE FROM cases_fts WHERE src = 'case' AND rid = OLD.id;
END;

CREATE TRIGGER IF NOT EXISTS trg_notes_fts_ai AFTER INSERT ON case_notes BEGIN
    INSERT INTO cases_fts(body, reason, guild_id, src, rid)
    VALUES (NEW.body, '', '', 'note', NEW.id);
END;
CREATE TRIGGER IF NOT EXISTS trg_notes_fts_au AFTER UPDATE OF body ON case_notes BEGIN
    DELETE FROM cases_fts WHERE src = 'note' AND rid = NEW.id;
    INSERT INTO cases_fts(body, reason, guild_id, src, rid)
    VALUES (NEW.body, '', '', 'note', NEW.id);
END;
CREATE TRIGGER IF NOT EXISTS trg_notes_fts_ad AFTER DELETE ON case_notes BEGIN
    DELETE FROM cases_fts WHERE src = 'note' AND rid = OLD.id;
END;
`

const v10Schema = `
ALTER TABLE command_usage_log ADD COLUMN ack_ms INTEGER;
ALTER TABLE command_usage_log ADD COLUMN send_ms INTEGER;
`

func checksum(s string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(sum[:])
}

func (s *Store) Migrate() error {
	if err := s.ensureChecksumColumn(); err != nil {
		return fmt.Errorf("ensure checksum column: %w", err)
	}

	applied := make(map[int]string)
	rows, err := s.db.Query(`SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		var cs sql.NullString
		if err := rows.Scan(&v, &cs); err != nil {
			rows.Close()
			return err
		}
		applied[v] = cs.String
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range migrations {
		if _, ok := applied[m.version]; ok {
			if want := checksum(m.up); applied[m.version] != "" && applied[m.version] != want {
				return fmt.Errorf("migration %d (%s) checksum mismatch: applied %s want %s",
					m.version, m.name, applied[m.version], shortHash(want))
			}
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.up); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
			m.version, m.name, checksum(m.up)); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	var maxVer int
	for _, m := range migrations {
		if m.version > maxVer {
			maxVer = m.version
		}
	}
	_, err = s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, maxVer))
	return err
}

func (s *Store) ensureChecksumColumn() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
	    version INTEGER PRIMARY KEY,
	    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE schema_migrations ADD COLUMN name TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			if !strings.Contains(err.Error(), "already exists") {
				return err
			}
		}
	}
	if _, err := s.db.Exec(`ALTER TABLE schema_migrations ADD COLUMN checksum TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			if !strings.Contains(err.Error(), "already exists") {
				return err
			}
		}
	}
	return nil
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}


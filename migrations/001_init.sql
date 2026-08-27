CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS guild_config (
    guild_id TEXT PRIMARY KEY,
    prefix TEXT NOT NULL DEFAULT '/',
    log_channel_id TEXT NOT NULL DEFAULT '',
    welcome_channel_id TEXT NOT NULL DEFAULT '',
    auto_role_id TEXT NOT NULL DEFAULT '',
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

CREATE INDEX IF NOT EXISTS idx_cmd_log_guild_time ON command_usage_log(guild_id, created_at);
CREATE INDEX IF NOT EXISTS idx_cmd_log_time ON command_usage_log(created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON dashboard_sessions(expires_at);

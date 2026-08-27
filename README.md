# Vilicus

Vilicus is a high-performance Discord bot and integrated administration dashboard written in Go. Engineered specifically to operate comfortably within a 512MB VPS budget (typically consuming only ~35MB - 65MB RSS at runtime) with zero CGO dependencies.

It provides a clean, production-ready starter base for developers building modern Go Discord applications  -  shipping complete with turnkey OAuth2 web management, database migrations, structured logging, and Components V2 message rendering out of the box.

## Design Constraints
- **Discord Components V2 only:** Every user-facing message (both slash and prefix) is built using top-level `Container` payloads with the `IsComponentsV2` (`1 << 15`) message flag.
- **Single neutral grey accent:** Defaults to `0x2B2D31`, dynamically customizable via SQLite and the admin control panel.
- **Zero emoji:** Strictly zero emoji across all bot responses, command outputs, select menus, logs, and dashboard templates.
- **Pure Go database:** Built on `modernc.org/sqlite` with WAL mode and compiled zero-CGO footprint.

## Requirements
- Go 1.26+
- Discord Bot Application with:
  - Gateway Intents: `Guilds`, `GuildMessages`, `MessageContent`, `GuildMembers`
  - OAuth2 Scopes: `identify`, `bot`, `applications.commands`
  - Redirect URI configured for dashboard authentication

## Environment Variables

| Variable | Required | Default | Description |
| :--- | :--- | :--- | :--- |
| `DISCORD_BOT_TOKEN` | Yes | - | Bot authentication token |
| `DISCORD_APP_ID` | Optional | - | Application ID for command registration |
| `DISCORD_GUILD_ID` | Optional | - | Guild ID for instant development command sync |
| `DISCORD_OAUTH_CLIENT_ID` | Yes | - | OAuth2 client ID for dashboard login |
| `DISCORD_OAUTH_CLIENT_SECRET` | Yes | - | OAuth2 client secret |
| `DISCORD_OAUTH_REDIRECT_URL` | No | `http://localhost:8080/auth/callback` | OAuth2 callback URL |
| `SESSION_SECRET` | Yes | `vilicus-secret-key-...` | 32-byte secret key for cookie signing and CSRF tokens |
| `ADMIN_USER_IDS` | No | - | Comma-separated list of Discord user IDs granted root admin access |
| `HTTP_PORT` | No | `8080` | Web dashboard listen port |
| `DB_PATH` | No | `data/vilicus.db` | Path to SQLite database file |
| `LOG_PATH` | No | `data/vilicus.log` | Path to rotating log file |
| `LOG_LEVEL` | No | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | No | `text` | Log format (`text` for development, `json` for production) |
| `LOG_RETENTION_DAYS` | No | `30` | Days of command execution logs to retain |
| `RETENTION_AUDIT_DAYS` | No | `180` | Days of moderation/dashboard audit rows to retain |
| `PPROF_ENABLED` | No | `false` | Enable `net/http/pprof` profiling server on port 6060 |
| `COOKIE_SECURE` | No | `true` | Set the `Secure` flag on auth cookies; disable only for plain-HTTP localhost |
| `TRUSTED_PROXIES` | No | - | Comma-separated IPs/CIDRs of reverse proxies; `X-Forwarded-For` is honored only from these peers |

## Command Catalog

All commands support dual execution via **Slash Commands** (`/`) and **Prefix Messages** (`.` default).

### General
| Command | Aliases | Description |
| :--- | :--- | :--- |
| `.ping` / `/ping` | `latency`, `p` | Gateway roundtrip latency |
| `.help` / `/help` | `commands`, `cmds`, `h` | Interactive categorized command reference with in-memory caching |
| `.prefix` / `/prefix` | `setprefix`, `pfx` | Inspect or configure server and personal custom prefixes |
| `.userinfo` / `/userinfo` | `whois`, `ui`, `user` | Account details, guild joined date, and assigned roles |
| `.serverinfo` / `/serverinfo` | `guildinfo`, `si` | Member counts, role counts, and channel statistics |
| `.avatar` / `/avatar` | `av`, `pfp` | Direct link to user avatar asset |
| `.banner` / `/banner` | `ubanner`, `userbanner` | Direct link to user profile banner asset |
| `.about` / `/about` | - | Runtime stats: uptime, memory, goroutines, command count |
| `.choose a, b, ...` / `/choose` | `pick`, `decide` | Pick one of 2-25 comma-separated options at random |
| `.roll [NdM]` / `/roll` | `dice`, `d` | Dice roll in NdM notation (default 1d20; limits 25 dice / 1000 sides) |
| `.poll <question> <options> [duration]` / `/poll` | `vote` | Live vote with buttons; one vote per user, changeable until close (max 10 choices, 7 days) |
| `.remind <1h30m> <text>` / `/remind` | `remindme`, `reminder` | Persisted reminder delivered back to the same channel when due |
| `.snipe [channel]` / `/snipe` | `s` | Show the most recently deleted message in a channel (5-minute window) |
| `.rank [user]` / `/rank` | `level`, `xp` | XP card with level progress bar (MEE6-style curve) |
| `.leaderboard` / `/leaderboard` | `lb`, `top` | Top 10 members by XP in this server |
| `.rolepanel create\|delete` / `/rolepanel` | `rpanel` | Post self-serve role button panels (up to 10 roles); bindings survive restarts |
| `.giveaway <dur> [winners] <prize>` / `/giveaway` | `gstart` | Start an enter-button giveaway; the scheduler draws winners automatically (max 30d, 20 winners) |
| `.greroll <message_id> [winners]` / `/greroll` | `grerolls` | Redraw a finished giveaway; previous winners are excluded from the pool |
| `.emoji jumbo\|steal\|list\|delete` / `/emoji` | - | Custom emoji tools: full-size view, copy from another server (Manage Guild Expressions), roster, removal |
| `.membercount` / `/membercount` | `mc` | Membership snapshot with boost tier and server creation date |
| `.bots` / `/bots` | - | List bot members from the cached roster |
| `.channelinfo [channel]` / `/channelinfo` | `ci`, `chaninfo` | Channel type, topic, category, slowmode, and creation details |
| `.roles` / `/roles` | `roletree` | Role tree sorted by position with colors and member counts |
| `.invites` / `/invites` | - | Active invite codes with inviter, usage, and target (`ManageGuild`) |

### Utility
Pure-stdlib helpers  -  every command here answers on the fast path.
| Command | Aliases | Description |
| :--- | :--- | :--- |
| `.uuid [4\|7]` / `/uuid` | - | RFC 9562 UUIDs: real v7 millisecond-epoch IDs or v4 random (batch of 5) |
| `.hash <sha256\|sha512> <text>` / `/hash` | - | SHA-2 hashes in hex; legacy md5/sha1 deliberately unsupported |
| `.encode <scheme> <text>` / `/encode` | - | Encode as base64, base64url, base32, or hex |
| `.decode <scheme> <text>` / `/decode` | - | Decode any of the four schemes with strict validation |
| `.gen <kind> [len]` / `/gen` | `generate` | crypto/rand password/token/hex/base64 up to 1024 chars, rejection-sampled (no modulo bias) |
| `.entropy <text>` / `/entropy` | - | Shannon entropy in bits plus charset ceiling and a strength verdict |

### Moderation (Audited & Role Hierarchy Protected)
Every ban, kick, timeout, warn, tempban, and temprole writes a **case row** (`mod_cases`) with a per-guild sequential case number, shown in the response title and audit entry.
| Command | Aliases | Permission | Description |
| :--- | :--- | :--- | :--- |
| `.role add\|remove` / `/role` | `r` | `ManageRoles` | Grant or revoke roles with strict hierarchy checks |
| `.kick` / `/kick` | `k` | `KickMembers` | Kick member with hierarchy verification + case record |
| `.ban` / `/ban` | `b` | `BanMembers` | Ban user with hierarchy verification + case record |
| `.tempban <user> <dur> [reason]` / `/tempban` | `tban` | `BanMembers` | Immediate ban with scheduled auto-unban ("12h", "3d", max 365d); sweeper lifts it; one live tempban per user |
| `.timeout` / `/timeout` | `mute`, `to` | `ModerateMembers` | Apply or clear member timeouts in minutes + case record |
| `.warn` / `/warn` | `w` | `ModerateMembers` | Warn with required reason, best-effort DM notice, case record |
| `.history <user>` / `/history` | `cases`, `hist` | `ManageMessages` | Paginated case history browser (Prev/Next buttons, 15-min window) |
| `.case <number>` / `/case` | - | `ManageMessages` | Full case lookup: status, type, participants, duration, notes timeline |
| `.casenote <number> <text>` / `/casenote` | `note` | `ModerateMembers` | Append a staff note to any case |
| `.reason <number> <text>` / `/reason` | `setreason` | `ModerateMembers` | Update a case's reason; audited |
| `.modstats [user]` / `/modstats` | `mystats` | `ModerateMembers` | Per-moderator action tally by case type for this server |
| `.temprole <user> <role> <dur>` / `/temprole` | `trole`, `tr` | `ManageRoles` | Grant a role that auto-expires ("1h30m", "3d"; max 365d); sweeper removes it |
| `.jail <user> [reason]` / `/jail` | - | `ManageRoles` | Swap the member's roles for the configured jail role (snapshot saved for restore); requires jail_role in config; double-jail refused |
| `.unjail <user>` / `/unjail` | - | `ManageRoles` | Release a jailed member: removes the jail role and restores the saved role set (partial failures stay retryable) |
| `.purge` / `/purge` | `clear`, `clean` | `ManageMessages` | Bulk delete 1 to 100 recent messages |
| `.starboard set\|off\|show` / `/starboard` | - | `ManageGuild` | Configure the starboard board channel and star threshold (1-25, default 3) |
| `.protect antispam\|antilink\|filter\|show` / `/protect` | `protection` | `ManageGuild` | Burst antispam timeouts, invite/link blocking (`off`\|`mods`\|`on`), word filter; mods exempt |
| `.protect honeypot [#channel\|off] [action]` | - | `ManageGuild` | Trap channel: any non-mod post earns an instant timeout/kick/ban and a case |
| `.protect antinuke on\|off\|punish\|threshold\|alert\|whitelist ...` | - | `ManageGuild` | Audit-log threat watcher: sliding-window scoring per admin, punish ladder timeout(1h)/kick/ban, owner/bot/whitelist exempt |
| `.automate add\|list\|show\|enable\|disable\|delete\|test` / `/automate` | - | `ManageGuild` | Automation rules: trigger -> conditions -> actions with placeholders; `test` dry-runs a rule against sample text without firing anything |

### Music (requires a configured Lavalink v4 node)
Set `LAVALINK_HOST` (+ optional `LAVALINK_PORT`/`LAVALINK_PASSWORD`/`LAVALINK_SECURE`) to enable; with no host the whole subsystem stays dormant. Control gate: moderators or anyone in the player's voice channel.

| Command | Aliases | Description |
| :--- | :--- | :--- |
| `.play <url\|search> [top]` / `/play` | - | Play a URL or search text; `top` queues next instead of last |
| `.pause` / `/resume` / `/skip` / `/stop` | - | Transport controls; stop clears the queue and stays in voice |
| `.queue [page]` / `/np` | - | Queue pages / now-playing card |
| `.seek <m:ss>` / `.volume <0-150>` | - | Position and volume control |
| `.loop off\|track\|queue` / `.shuffle` / `.clear` | - | Queue modes |

The web dashboard mirrors cases at **/cases**: per-guild index with counts, full-text search over case reasons and note bodies (`q=`), paginated table with target-ID filter, case detail with notes timeline, plus CSRF-protected note/deactivate actions  -  every mutation lands in `dashboard_audit_log`. Beyond cases: **/analytics** (command volume by hour, latency histogram, runtime metrics), **/maintenance** (superadmin backup-now / prune-now / vacuum-now over nightly verified backups), per-guild config **export/import** JSON bundles on **/guilds**, and a staged confirm flow for destructive console actions (ban/kick/unban resolve the target first and require a single-use token).

### Configuration
| Command | Aliases | Permission | Description |
| :--- | :--- | :--- | :--- |
| `.config get\|set` / `/config` | `cfg`, `settings` | `Administrator` | Inspect or update guild prefix, logging channel, and auto-roles |

### Event Logging
When a log channel is configured (`/config set`), the bot posts CV2 cards for: member joins/leaves, message deletions (with cached pre-edit content), message edits, and bans/unbans.

### Engagement & Automation
- **Starboard:** * reactions track per-message star counts in SQLite; crossing the threshold posts a live-updating card to the board channel, and dropping back under it removes the post. Automated filter/antispam actions file real cases attributed to the bot so they show up in history alongside staff actions.
- **Role panels:** Button-driven self-serve roles. The customID payload is only a hint  -  every click re-validates against the stored binding row, so forged or stale buttons can never grant an unlisted role.
- **Levels-lite:** Passive XP on message activity (15-25 XP per user per minute, MEE6-style quadratic curve); writes are cooldown-gated at the store layer with an in-memory fast gate keeping the hot path off SQLite.
- **Giveaways:** Enter-button panels persisted in SQLite with one-entry-per-user fairness; the sweeper claims due draws via compare-and-set before any Discord calls, announces winners, disarms the panel, and rerolls exclude previous winners.
- **Automation rules:** Typed SQL rows (trigger -> conditions -> actions). Triggers: message create, member join/leave/ban/unban, role add/remove (synthesized by diffing member roles across updates), and scheduler-driven intervals. Conditions cover channel include/ignore lists, bot/human actors, account age, required/forbidden roles, phrases, regex, link presence, mention counts, plus per-user cooldowns and N-per-window counters sharing one key layout so writer and reader can never diverge. Actions: delete, timeout, ban, kick, role grant/revoke, DM, reply, channel post, audit-log card, stop  -  automated moderation files real cases attributed to the bot. `/automate test` evaluates a rule against sample text and reports the first failing check without consuming cooldowns.

---

## Prefix Management
- **Default server prefix:** `.`
- **View prefix:** `.prefix`
- **Set server prefix (Admin):** `.prefix set <prefix>`
- **Set personal custom prefix:** `.prefix self <prefix>` (works in any server)
- **Reset personal prefix:** `.prefix reset`

---

## Role Hierarchy & Moderation Engine
All moderation actions run through the [HierarchyEngine](file:///c:/Users/sativa/Documents/projects/me-srcs/misc/vilicus.win/internal/discord/commands/hierarchy.go):
1. **Target Position Check:** Prevents moderating members with higher or equal role positions.
2. **Bot Position Check:** Verifies bot has higher role position than target member and target role.
3. **Owner Immunity:** Protects guild owner from moderation commands.
4. **Automated Audit Logging:** Every moderation event is recorded in SQLite `moderation_audit_log` and dispatches an audit Container to the server's configured `log_channel_id`.

---

## Project Structure
```
 cmd/
    bot/                  Main entrypoint, server lifecycle, signal handling
 internal/
    components/           Components V2 types, custom containers, validation,
                            customID grammar (<ns>:<action>:<payload>:<expiry>)
    automation/           Pure automation engine half: rule compile/match,
                            spec validation, action parsing, template expansion
    config/               Environment loader and defaults
    discord/              Gateway session, prefix/slash routers, interaction handlers
       component_router.go  Namespaced button/modal dispatch with expiry + panic isolation
       events.go        Member/message/ban event dispatchers (panic-recovered)
       logroutes.go     Log-channel card routing for all event types
       poll.go          In-memory poll state, live vote handling
       starboard.go     Star ledger + live board card lifecycle
       protection.go    Antispam burst detector, antilink modes, word filter
       levels.go        XP gate feeding the store-side cooldown writer
       automation.go    Automation engine stateful half: rule cache,
                          cooldowns/counters, dispatchers, action executor
       panels.go        Role panel posting + giveaway start/reroll flows
       sweepers.go      Reminders, temp roles/bans, giveaway draws, automation intervals
       cooldowns.go     Per-user token buckets (fast 5/10s, danger 2/10s)
       commands/
           types.go      Unified Command interface & BotInterface
           hierarchy.go  Role hierarchy verification & audit dispatcher
           util.go       Snowflake/mention/duration argument parsers
           general/      ping, help, prefix, userinfo, serverinfo, avatar, banner,
                           about, choose, roll, snipe, poll, remind, rank, leaderboard,
                           rolepanel, giveaway, emoji, membercount, bots, channelinfo,
                           roles, invites, uuid, hash, encode/decode, gen, entropy
           moderation/   role, kick, ban, timeout, purge, warn, history, temprole,
                           tempban, case tools, modstats, starboard, protect, automate
           config/       config get/set
    logging/              Slog initialization, rotation, correlation ID middleware
    sched/                Jittered interval loop for background sweeps
    store/                SQLite layer, checksummed migrations, async batched telemetry
                            writer, TTL caches, cases/reminders/temp-roles, dash audit
    web/                  Chi router, OAuth2 auth, handlers, security middleware
 migrations/               SQL migration scripts (baseline + v2 schema)
 web/
    static/               Vendored assets (htmx)  -  no third-party script origins
    templates/            Glassmorphism UI templates (fog canvas, no emoji)
 docs/spitari-commands/    Reference notes on ported command designs
 Dockerfile                Multi-stage minimal container build
 docker-compose.yml        Compose configuration
 Makefile                  Build automation targets
 vilicus.service           Systemd unit file
```

## Security Architecture
- **Security Headers:** Strict CSP (`script-src 'self'`  -  htmx is vendored locally), plus HSTS (when `COOKIE_SECURE=true`), COOP, `Permissions-Policy`, `frame-ancestors 'none'`, `base-uri`, and `form-action`.
- **Trusted-Proxy IP Resolution:** `X-Forwarded-For` is honored only when the direct peer matches `TRUSTED_PROXIES`; otherwise the raw remote address is used.
- **Token Bucket Rate Limiting:** Global IP rate limiter (30 req/s sustained, burst 60) and strict auth limiter (max 5 attempts/10s). Idle buckets are evicted every 10 minutes so unique-source floods cannot grow the map unbounded.
- **CSRF Protection:** Timing-safe HMAC-SHA256 token validation on all state-changing endpoints.
- **OAuth State:** Constant-time comparison and single-use burn on callback; cookie cleared on logout.
- **Cookie Flags:** `HttpOnly`, `SameSite=Lax`, and `Secure` (config-gated) on session and state cookies.
- **Console Hardening:** Snowflake validation on all IDs, required reason + confirmation checkbox for destructive actions, bot-hierarchy gate mirroring the command-side check.
- **Dashboard Audit Trail:** Every dashboard mutation (logins, guild/settings/admin changes, console executions, rejected attempts) lands in `dashboard_audit_log`.
- **Health Endpoints:** `/healthz` (liveness) and `/readyz` (2-second SQLite ping) for process supervisors.

## Runtime Architecture Notes
- **Fast-path slash responses:** Commands that need no network I/O answer their interaction in a single REST round trip; everything else uses defer+edit. Either way the command executes exactly once.
- **Async telemetry:** Command usage logs go through a non-blocking channel into a batched writer (250ms / 64-row flushes); overflow is counted and dropped, never blocking the hot path.
- **Checksummed migrations:** Applied transactionally per step with SHA-256 content checks mirrored into `PRAGMA user_version`.
- **Bounded caches:** Guild/user config caches carry TTLs (5 min positive / 30 s negative), size caps (10k / 50k entries), clone-on-return semantics, and a janitor sweep.
- **Scheduled enforcement:** Jittered sweeper loops lift expired temporary sanctions  -  temp roles every 30s, temp bans every 30s (batches of 25, marked consumed even if the Discord call fails so they never re-fire), giveaway draws every 15s claimed via compare-and-set before any Discord calls, and automation interval rules every 15s with `last_run` stamped before firing so slow sends can't double-fire.
- **Protection-lite:** Per-guild filter/antilink/antispam config is cached for 15s so the per-message hot path stays off SQLite; burst windows and action cooldowns live in memory only and reset on restart.
- **Maintenance:** Six-hourly housekeeping prunes expired rows (command logs, audits, reminders, temp roles), runs a WAL checkpoint, and refreshes query statistics  -  no full-database VACUUM stalls.

## Resource Profile & Tuning
- Baseline memory: ~15MB RSS.
- Zero CGO runtime footprint.
- All hot-path interaction responses acknowledge within sub-100ms via deferred channel message responses.

## License
MIT

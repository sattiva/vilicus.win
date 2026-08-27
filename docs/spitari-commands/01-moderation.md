# 01  -  Moderation & Protection (Chronicle -> Vilicus)

Chronicle's mod surface = `internal/commands/moderation/*` + protection engines in
`internal/manager/config.go` + audit plumbing in `internal/moderation/{audit,log,resolve}.go`.
Shared plumbing first, then per command.

## Shared plumbing

- **Case system**: every action gets an ID from the "AuditLogs" bbolt bucket `NextSequence()`;
  the case number is embedded in the action string and re-parsed by
  `(?i)\(Case\s*#?(\d+)[^)]*\)` when logging.
- **Audit-log confirmation** (`ProcAudit`, audit.go): sleep 800ms -> `GuildAuditLog(limit 5)` ->
  match entry.TargetID to the acted-on user, skip if actor == bot or entry older than 5s
  (snowflake decode: `id >> 22 + 1420070400000`) -> log as `"<Action> (Manual)"`. This is how
  Chronicle catches *manual* bans/kicks done by other staff.
- **Modlog embed** (`LogAction`): grey `0x808080`, author "Modlog Entry" with resolved
  footer icon, Duration field split out of the reason string, DM notice via `DMUserAction`
  past-tense map (`ban/hardban/softban->banned`, `jail->jailed`, `untimeout->untimeouted`, ...).
- **ResolveMember** (`resolve.go`): `<@!id>` -> snowflake regex (17-21 digits) -> API fetch ->
  GuildMembersSearch exact->substring -> state scan -> full `GuildMembers(1000)` scan on
  username/nick/globalname. ResolveChannel analogous.

**Vilicus v2 shared**: fold all of this into the existing HierarchyEngine  -  add case IDs
(autoincrement table), manual-action detection (compare audit log vs bot-issued actions),
DM notices, and member/channel resolution into one package. All outputs become CV2 containers.

---

### `.warn <user> [reason]` / `.unwarn <user> <case#>`
Warn appends a case to the guild's warn bucket, DMs past tense, posts modlog embed; threshold
escalation is left to owners via owner tasks. Unwarn deletes by case number.
**v2**: `mod_cases` SQLite table (guild, target, actor, type, reason, duration, created);
`.warn` walks hierarchy engine; auto-escalation rule column instead of external automation.

### `.jail <user> [reason]` / `.unjail <user>`
Swaps all member roles for the configured jail role, storing the stripped role IDs in the
jail bucket so unjail restores exactly. Creates the jail role if missing.
**v2**: same mechanic, but store prior roles in a dedicated table and run through hierarchy
engine + audit; refuse if bot's top role <= any stripped role.

### `.lockdown` / `.unlock`
Lockdown sets `SendMessages=false` overwrite for @everyone in every text channel (skips ones
already denied); unlock reverses using the stored snapshot from lockdown time.
**v2**: snapshot overwrites in DB before mutating; single CV2 status container with progress;
dashboard button mirror.

### `.stripstaff <user>`
Removes every role with permissions above a floor (Kick/Ban/Manage*/Administrator bits) from a
member  -  emergency de-escalation of a compromised staff account.
**v2**: keep, but compute the strip list from role permission bits and log each removed role.

### `.history <user>` (+ `history_*` paging components)
Paginated case list for a member: reads all cases for gid+uid, sorts desc, renders pages,
`history_<gid>_<uid>_<page>` buttons flip pages (anyone may browse).
**v2**: SQL `LIMIT/OFFSET` + component pagination helper (Vilicus has none yet  -  build one).

### `.modstats` / `.modsearch <query>` / `.notes <member>` (+ notes subcommands)
Aggregations over cases: counts per moderator (modstats), free-text grep over reasons
(modsearch), freeform staff notes CRUD on a member (notes add/remove/list).
**v2**: three read-only queries over `mod_cases` + a `member_notes` table; search uses FTS5
(Vilicus already ships modernc sqlite which supports it).

### `.modlog channel <#chan>`
Sets the modlog destination in guild config cache + DB.
**v2**: subsumed by expanding Vilicus `.config set` (add `log_channel_id` variants: modlog,
message-log, join-log...).

### `.log add|remove|color|ignore ...` (logger.go)
Per-event audit logging router: events = messages/members/roles/channels/invites/emojis/voice/
server/all; maps event->channel, optional per-event hex color override, ignore list of users/
channels. Handlers live behind gateway event dispatchers writing formatted embeds.
**v2**: Vilicus killer feature  -  implement as `log_routes(guild, event, channel_id, color)`
table + `log_ignores`; render logs as CV2 containers with the accent color per event. This is
the single highest-value port in this file.

### `.nuke [#channel]`
Clones the channel (type/name/topic/position/overwrites) then deletes the original, keeping
continuity of the conversation location while wiping history. Confirm not required  -  perms only.
**v2**: require confirm component (like backup flow), audit-log it, and preserve webhooks by
re-attaching.

### `.clear/.purge <amount> [member|search]`
Bulk delete with optional author filter or substring filter: fetches messages, filters in
memory, chunks to <=100 per bulk-delete call, respects 14-day API limit implicitly.
**v2**: port filters onto Vilicus purge; report per-filter counts in the result container.

### `.rmute <user> [duration]`
Role-based mute (for members where Discord timeouts are undesired): adds mute role, schedules
removal via the TempRoles loop.
**v2**: implement as sugar over Vilicus temprole port (below).

### `.temprole <user> <role> <duration>` (10s expiry loop)
Assigns role with expiry timestamp stored in tempRoles bucket; background loop every 10s
removes expired assignments.
**v2**: `temp_assignments(guild,user,role,expires_at)` + Vilicus-side 30s sweeper; also cover
"temporary role on join" configs.

### `.stickyrole add/remove <user> <role>`
Roles re-applied automatically when the member rejoins (stored per guild+user).
**v2**: trivial table + hook into Vilicus's GuildMemberAdd handler (doesn't exist yet  -  add
member-add dispatcher while porting autorole).

### `.slowmode <seconds> [#channel]`
Sets RateLimitPerUser on a channel (0 clears). Accepts `10s/2m` style input.
**v2**: direct port; add dashboard toggle.

### `.thread lock [thread] [reason]` (+7 help pages: lock/unlock/archive/unarchive/slowmode/add/delete)
Thread management wrapper: resolves thread by mention/id/current channel, applies
`ChannelEdit` archived/locked flags or deletes.
**v2**: fold into a Vilicus `/thread` command group; low effort.

### `.unbanall`
Server-owner-only: iterates all bans (`GuildBans` paged) and unbans everyone. Confirmation via
typing the exact command again.
**v2**: keep owner-only + confirm component + audit row listing count.

### `.clearinvites`
Deletes every invite the bot can (`GuildInvites` -> DeleteInvite each), skipping ones it lacks
permission for.
**v2**: direct port; report kept/deleted counts.

### `.drag <@m1> [@m2...] <#voice>`
Moves each mentioned member's voice connection to the target channel; requires mover's
MoveMembers and checks per-target connect perms.
**v2**: direct port with hierarchy check against each target.

### `.newmembers [count]`
Lists most recently joined members (account age + join order) from state.
**v2**: trivial read of Vilicus state; useful alongside raid response.

### `.recentban <count> [reason]`
Mass-bans the N most recent joins  -  raid hammer. Owner/admin gated.
**v2**: port but require a confirm component and cap at 25/target selection by account age.

### `.talk <#channel> <@role>`
Toggles SendMessages overwrite for a role in a channel (reads current state, flips it).
**v2**: direct port; show resulting permission state in the container.

### `.revokefiles <on|off> [#channel]`
Toggles AttachFiles+EmbedLinks deny for @everyone in a channel.
**v2**: merge with `.talk` into a single `/channel-lock` group (send/files/embeds/embed-links
toggles)  -  cleaner than two commands.

### `.topic <text>`
Sets current channel topic; empty arg shows current.
**v2**: direct port.

### `.naughty [#channel]`
Marks channel NSFW for 30 seconds then reverts (goroutine sleep).
**v2**: skip unless wanted; if ported, make duration an argument.

### `.perms <command>` / `.restrict <command>` (restrict.go, 6 help pages)
Per-command permission overrides: restrict toggles a command fully off per guild; perms
subcommands bind required role/perms to a trigger. Enforced at dispatch.
**v2**: Vilicus-wide feature worth having: `command_overrides(guild, trigger, disabled,
min_perms, allowed_roles)` checked inside the unified executor before Execute.

### `.quarantine <user> [reason]` / `.release <user>` / `.quarantined`
Strips all roles, applies quarantine role, stores original roles keyed by user; release
restores; quarantined lists active isolations. Backed by honeypot/antiraid auto-quarantine too.
**v2**: same storage pattern as jail; unify jail/quarantine into one "isolation" subsystem with
two labels.

### `.settings config`
Dumps the guild settings cache (prefix, modlog, protection states) as an overview embed.
**v2**: becomes Vilicus `.config get` expanded + dashboard parity page.

### `.roles` / `.roleinfo` / `.role add|remove`
`roles` lists guild roles w/ counts; `roleinfo` dumps one role (perms decoded to names);
role add/remove is the near-dupe of Vilicus's.
**v2**: port roles/roleinfo as read-only info commands (Vilicus has neither).

### Protection configuration commands
These are thin CRUD UIs over the manager's cached configs (see README section conventions):

#### `.antispam` family (enable/disable/v2beta/consecutive/similarity/limit/action/timeout/bypass/whitelist)
Configures burst limit window, monologue streak length, Levenshtein similarity %, action
ladder, bypass perm toggle, whitelist entries. V1 keeps timestamps per `gid:uid`; V2Beta adds
pattern-burst (threshold max(Limit/2+1,3)), streak default 6, duplicate/similar detection
(`1  dist/maxLen >= pct` over last 3 of 10 msgs). Bypass = ManageGuild|Admin + whitelist.
**v2**: implement engine in Vilicus as middleware on MessageCreate with a
`antispam_config` row per guild; expose same knobs via slash options + dashboard form.
Improvement: make similarity window/streak configurable and persist offender stats.

#### `.antilink` family (action/timeout/bypass/invitesonly/allowed/blocked/whitelist)
Modes: invites-only -> blocked-domains contains-match -> allowlist (empty allowlist blocks ALL
links). Four compiled regexes (full URL / www / bare-domain ~70 TLDs / invite).
**v2**: port with the same mode precedence; precompile regexes at config-save; add
"punish nickname/channel-name links" scope.

#### `.filter` family (add/remove/list/settings/whitelist)
Blocked-word engine with three match passes: direct, homoglyph-normalized (full map:
Cyrillic , fullwidth, accented, leetspeak @/4/3/1/!/0/5/$/7/2, ->ss...), and noise-tolerant
scan skipping non-alphanumerics (defeats `b.a.d.w.o.r.d`). AllowedWords escape hatch.
Regex cache invalidated on save.
**v2**: port all three passes verbatim (they're the best part); store words in SQL; add
per-word action severity.

#### `.antinuke` (15 help pages: on/off/punish/whitelist/thresholds per module)
Owner-only toggle + per-module thresholds across 14 audit modules (channel delete/create,
role delete/create/update, ban/kick/webhook/member prune, guild update, etc.), threat scoring
per actor, punishment ladder, whitelisted actors.
**v2**: biggest engineering item in the doc  -  needs Vilicus to gain an audit-log watcher
goroutine per guild. Port the module/threshold model; score = weighted recent actions;
actions timeout/kick/ban + optional quarantine.

#### `.antiraid`, `.honeypot`
Antiraid: join-burst detection (N joins/M seconds) -> auto actions + alert channel.
Honeypot: designated trap channel; anyone posting there gets banned/quarantined.
**v2**: both straightforward given a member-add dispatcher; share the antinuke punish ladder.

### `.blacklist add/remove/list` (owner)
Global blacklist: targets users/bots with scopes (all-commands, full-bot, category, specific
trigger), optional expiry duration + reason; enforced early in dispatch.
**v2**: adopt concept globally: `blacklists(subject_type, subject_id, scope, expires_at,
reason)` checked in Vilicus's router; dashboard-managed.

### `.servers` (owner)
Lists all guilds the bot is in (names, IDs, member counts) with paging components
(`ownersrv_*`, incl. modal-driven page jump); leave-guild action.
**v2**: exists better as a dashboard page (Vilicus already lists guilds); add leave button
there instead of a chat command.

### `.owner` dashboard (2817 lines; `owner*` components)
Subcommands: dash (live stats container), logs (paged bot log viewer + modal jump), eval
(modal code evaluation  -  arbitrary Go? actually formats/exec via reflection helpers), palantir
(SQLite browser w/ query box), servers, buckets (bbolt explorer), panic/shred (wipes all bot
DBs after confirm), backup `<channel>` (uploads bots.db copy), netstat (established TCP conns
of process), aiexpansion/osint per-user grants, fisch gate config, task automation engine.
**v2**: port selectively  -  task engine (02-utility-ai.md), palantir-style log browser belongs
on the dashboard; do NOT port eval/panic/netstat into a shared-hosting bot (they're host-level
footguns). Keep shred equivalent as "wipe guild data" scoped to one guild.

### `.owner task` automation presets & engine
See 02-utility-ai.md (documented with the automation engine).

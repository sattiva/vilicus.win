# Spitari-Watchdog ("Chronicle") Command Reference & Vilicus Parity Plan

This directory documents **every command Chronicle has that Vilicus does not**, how each one
actually works internally (dispatch, gates, storage, APIs, outputs), and a concrete
**"Vilicus v2" redesign** for porting it  -  upgraded where Chronicle's version is weak.

Source of truth: full read of `C:\Users\sativa\Documents\projects\me-srcs\misc\spitari-watchdog`
(all of `internal/commands`, `internal/plugins`, `internal/manager`, `internal/automation`,
`internal/ai`, `internal/lavalink`, plus support packages). Every mechanism below is taken from
that code, not guessed; where a detail is internal to a file only skimmed (economy payout
tables, some fun-API wrappers) the entry says so instead of inventing numbers.

## Files

| File | Coverage |
|---|---|
| [01-moderation.md](01-moderation.md) | Mod actions, cases, protection config, logging, owner/blacklist/servers |
| [02-utility-ai.md](02-utility-ai.md) | AI suite, backup system, reminders/scheduling, snipes, boards, levels, giveaways, embeds, utility core |
| [03-network-tools.md](03-network-tools.md) | DNS/IP/Minecraft/crypto/JWT/OSINT/port tools, web scrape/crawl/search/download, ticker |
| [04-general-guild.md](04-general-guild.md) | Welcome/goodbye, boost roles, autorole/reaction/button roles, birthdays, Fisch/Roblox, vouches, vanity, info family |
| [05-fun.md](05-fun.md) | Media lookups, social probes, text toys, games, joke/security-novelty commands |
| [06-roleplay.md](06-roleplay.md) | The ~68 generated action-GIF commands (one shared engine) |
| [07-music.md](07-music.md) | Full Lavalink player surface + filter presets |
| [08-plugins.md](08-plugins.md) | Economy (39 triggers), captcha, custom commands, Lua scripting, moon, coinflip |

## Overlap  -  what Vilicus already covers

Trigger-for-trigger, only these exist in both bots:

| Trigger | Chronicle version | Vilicus version | Verdict |
|---|---|---|---|
| `ping` | latency embed | CV2 container, same idea | keep Vilicus |
| `help` | paginated component browser (`help_*`) + per-cmd `cmdhelp_*` pages | categorized cached reference | keep Vilicus, steal Chronicle's per-command help pages |
| `prefix` | view/set guild + personal prefix | same feature set | keep Vilicus |
| `ban` / `kick` / `timeout` | duration suffixes, case system, DM notice, audit-log confirmation | hierarchy-engine versions, minutes-only timeout | keep Vilicus core, port case IDs + duration parsing + DM notices |
| `clear` (~ `purge`) | amount + member/search filters | 1-100 bulk delete | port member/search filtering |
| `role` add/remove | named-perm checks, role arg resolution | hierarchy-checked | near-parity |
| `config` ~ `.settings config` | bigger settings surface | get/set prefix/log/autorole | expand Vilicus settings instead of adding new cmd |

Everything else below is missing from Vilicus: **438 unique triggers** in Chronicle
(`grep -rhoE 'Trigger:\s+"[^"]+"' internal/commands internal/plugins | sort -u`), minus the
3 exact overlaps above -> **~435 commands to consider**. Of those, 68 are generated roleplay
variants sharing one engine, and 39 are economy subcommands sharing one storage layer  -  so the
real engineering surface is ~330 distinct mechanisms.

## Chronicle conventions every command shares

Understanding these five things explains 80% of any entry:

1. **Dispatch**: personal prefix -> guild prefix -> mention; longest multiword trigger first;
   then TOS gate -> blacklist scopes -> 8 cmds/5s rate limit. Commands are prefix-only
   (no slash registration anywhere).
2. **CommandContext helpers**: `ctx.Reply` (plain text), `ctx.Respond(embed)` via
   `config.Build` which stamps brand color/footer/avatar, `ctx.ReplyAndGet/EditReply`
   (edit-in-place for "thinking..." flows), `ctx.EditOrReplyLarge(text, fallbackFilename)`
   (auto-attaches oversized output as a .txt file), `ctx.SendHelp("name")`.
3. **Storage**: bbolt `bots.db`, ~100 buckets, AES-256-GCM at rest under `CHRONICLE_CRYPT_KEY`.
   High-volume message logs go to SQLite Palantir + FTS5 archive instead.
4. **Permissions**: ad-hoc per command  -  usually `UserChannelPermissions(author, chan)` against
   `PermissionAdministrator` / `ManageGuild` / bits, or `isBotOwner()` (env `CHRONICLE_OWNER_IDS`).
5. **Output style**: classic embeds, `[+]`/`[!]`/`[*]` prefixes, `<t:...:R>` timestamps,
   footer "sativa.cfd".

## Global "Vilicus v2" upgrades applied to every port

Each per-command entry lists specifics; these apply everywhere and are not repeated:

- **Components V2 everywhere**  -  grey `0x2B2D31` accent Container, TextDisplay/Section/
  MediaGallery/Separator, zero emoji. Replaces Chronicle's embed+emoji output wholesale.
- **Dual slash + prefix** for free via Vilicus's unified Command interface, plus autocomplete
  on ID/name arguments.
- **Hierarchy engine + audit** on anything destructive (Chronicle mostly checks bare perms and
  rarely writes an audit row outside modlog).
- **SQLite store schema** replacing bbolt blobs  -  typed tables, WAL, prepared statements,
  dashboard-readable. Every config command gets a web-panel mirror.
- **Per-user token buckets** from Vilicus's web limiter reused as command cooldowns.
- **Fixes of Chronicle bugs found during the read** are called out inline, e.g.: backup
  overwrite-remap by *role name* (breaks if renamed), passwordless backups keyed by the bot
  token itself, fake UUIDv1/v7, dead `message_delete`/`message_update` automation triggers,
  daily-message counter key mismatch, duplicate MCServer registry entry.

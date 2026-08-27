# 02  -  Utility & AI (Chronicle -> Vilicus)

## AI suite (`internal/ai` + `internal/commands/utility/{ask,myai,asklogs}.go`)

### `.ask <prompt>` (aliases `ai`, `gpt`; flags `--tts`, `--voice=Name`)
Full pipeline, in order:
1. Flag parse (`--tts/--voice` pulled out of args).
2. Gates: `AIEnabled` global -> provider list non-empty -> OwnerOnly check -> token balance
   (skipped for owners when `AIOwnerBypass`) -> per-user RL (`checkRL`: cooldown / busy /
   heavy-usage queue with an artificial `time.Sleep` delay + notice).
3. Attachment ingestion: each attachment <=2MB downloaded; `.zip/.jar` exploded via
   archive/zip (files <=500KB appended); text extensions whitelisted else NUL-byte sniff;
   referenced (replied-to) message attachments included; all appended to the prompt.
4. Memory: last 5 conversations from the AIConvo store replayed as user/assistant pairs.
5. Provider pick: first configured or `DefaultAIProvider`; MaxTokens 1200 (4000 owners).
6. `ai.Generate` tool loop (<=5 iterations, iteration-4 strips tools, fallback chain,
   429/5xx retry x3 backoff 1s->2s) over **14 tools**: web_search (DDG-lite), wikipedia_search,
   web_scrape, web_crawl (depth<=2/pages<=20), get_weather (wttr.in), dns_lookup, ip_lookup,
   calculate (safe arithmetic parser), screenshot (microlink+fallback), ping (TCP:80),
   whois (port 43 iana), hash_text (md5/sha256), uuid v4, discord_context (any user incl.
   bot-owner flag). OpenAI-path models get a DSML fallback parser that lifts
   `<DSMLtool_calls>` XML blocks out of plain content into real tool calls.
7. Billing: est tokens = len/4 both ways, peak-hour window multiplier, tier multipliers
   (free 2x / basic 1x / premium 0.5x); cost line appended to output.
8. Post: "Expansion Sandbox" (if enabled per-user or owner) zips any
   ``` ```lang FILE:name\n...``` blocks into codebase_sandbox.zip and uploads; `--voice`
   renders TTS (ElevenLabs if key set & voice=="mommy", else StreamElements, else Google
   Translate) as mp3; oversized text auto-attached as ai_response.txt.
System prompt = prompts.json content with `${currentDate}/${userRecognition}/
${channelContext}/${searchInstructions}` substituted. NOTE: the shipped default prompt is an
"unfiltered" jailbreak persona  -  replace entirely in any port.

**v2**: Vilicus port should keep steps 1-8 minus billing tiers unless wanted: single
provider config row (type/key/model/base-url), same 14 tools extracted into a reusable
package, CV2 output with collapsible tool-trace section (tool name + ms), file-attachment
ingestion, and conversation memory keyed per user+channel with a `.forget` reset. Add proper
streaming-free UX: edit-in-place like Chronicle's thinking message.

### `.myai [apikey|aiprompt|provider|model|reset|status]` (alias `mai`) / `.myask <prompt>` (alias `maiask`)
Per-user BYO key: stores UserAICfg{APIKey, Type(one of 17 provider strings), Model, SysPrompt}
in DB; status shows masked key (`first4****last4`). myask builds an ephemeral AIProvider
`user:<uid>` and runs the same tool loop at 4000 tokens with no billing; custom prompt
overrides the global one.
**v2**: port as-is conceptually; keys belong encrypted-at-rest (Vilicus has no crypto layer  - 
either add AES-GCM with a master key env or accept plaintext with a documented warning).
Add `.myai test` that pings the provider with 1 token.

### `.aibalance [@user]` / `.redeem <key>` / `.aihistory [n]` (aliases `aih`,`convos`,`aiquestions`)
Balance+tier view (owner may query others); redeem consumes single-use key rows adding tokens
then deletes the key; history lists last convos (bubble-sorted by timestamp  -  O(n), fine at
their scale) and renders full Q/A on index arg, attaching as file past 1900 chars.
**v2**: if the token economy is wanted at all: SQL tables + atomic UPDATE...RETURNING for
redemption (Chronicle's read-add-write is race-prone). Otherwise skip; history alone is worth
porting as `.aihistory`.

### `.asklogs <query>`
FTS5 MATCH query over the Palantir message archive; top hits formatted with author/channel/
timestamp, optionally summarized by the LLM ("RAG-ish"). Requires the Palantir subsystem.
**Vilicus equivalent**: only meaningful if Vilicus gains message logging (see log-routes in
01). Then: `message_archive` FTS table + this command becomes a straight SQL wrapper  -  no LLM
required for the base feature.

## Backup system (`backup.go` 590L + `backup_crypto.go`)

### `.backup create|load|list|delete|info|export|import` (alias `backups`)
- **create**: fetches guild/channels/roles; snapshots every unmanaged role (name/color/hoist/
  mentionable/perms/position) and channel (name/type/topic/bitrate/userLimit/parent/position/
  nsfw/slowmode + overwrites with role names resolved); rotation keeps newest 3 per guild;
  ID = `bk-<8 hex>`.
- **load**: admin-only confirm buttons (`backup_confirm:<id>`/`cancel`). Confirm handler
  (goroutine): deletes ALL channels except the invocation one, deletes all non-managed roles,
  recreates @everyone perms then roles in snapshot order building name->newID map, creates
  categories then children remapping parent IDs and overwrites **by role name**, posts a
  restore summary into the first recreated text channel, deletes the invocation channel.
  BUG: name-based overwrite remap breaks for renamed/duplicate role names; position ordering
  relies on API acceptance.
- **export**: JSON-marshals the backup, AES-256-GCM encrypts with SHA256(pass) where pass =
  explicit password arg, ELSE **the bot's own token**, ELSE constant fallback; sends
  `<id>.chronicle`. Security note: passwordless exports are decryptable by anyone holding the
  token, and the token-derived ciphertext leaks nothing directly but ties exports to the bot.
- **import**: downloads attachment (or replied attachment), decrypts (same secret ladder),
  validates name+channels, saves under new ID re-keyed to current guild/time.
**v2**: port the whole flow with three fixes: (1) store overwrite targets by stable ID *and*
name with ID-preferred remap; (2) KDF  -  scrypt/argon2 over user password, never bot-token
keys; (3) chunked restore with progress edits + abort button. Snapshot schema maps cleanly to
two SQLite tables (`backup_meta`, `backup_channels` JSON).

## Automation engine (`internal/automation/engine.go` 909L + owner_tasks.go)

### `.owner task ...` (presets + JSON)
Owner-scoped automation tasks stored as JSON blobs: triggers `member_role_add/remove,
member_join/leave/ban/unban/nick_change, message_create, reaction_add, voice_join/leave,
interval`; condition matchers guild wildcard/channel include-ignore/bots-humans/account-age/
target-lacks-has-roles(RequireAll)/contains-phrases/regex/has-invites-links/mention-count/
message-count counters/per-user cooldown; actions ban(days clamp->1)/kick/softban/unban/
timeout/untimeout/add-remove-toggle-role/set-reset-nick/dm/channel-msg/reply/delete/purge/
log-audit/webhook-post(async)/stop_propagation. Placeholder expansion covers ~18 tokens
({user},{guild},{timestamp:<t:f>},...). Interval loop ticks 15s, tasks default 60s period,
per-task LastRunAt. Presets: `role_ban/role_kick/role_timeout/role_grant...`.
BUGS found: no dispatchers wired for `message_delete`/`message_update` triggers (dead config);
DailyMessages counter reads a different bucket-key layout than the writer writes (always 0).
**v2**: port engine design wholesale but fix both bugs and drive it from typed SQL rows;
expose a builder UI (component wizard) instead of raw JSON for common cases; keep JSON import
for power users. This is Vilicus's future "automod rules" feature  -  highest-value item in the
whole doc together with log-routes.

## Time-based utilities

### `.remind <time> <text>` / `.reminders`
Parses relative durations (+ absolute dates), stores reminder rows, 2s loop fires due items ->
DM/channel mention. Reminders lists pending w/ cancel indices.
**v2**: direct port onto a `reminders` table + Vilicus ticker; add slash autocomplete of
pending reminders for cancel.

### `.schedule <time> <#channel> <text>`
Same loop; posts a message to a channel at time X instead of DM.
**v2**: merge with remind as one scheduler (target=user vs channel).

## Per-user/guild memory utilities

### `.afk [reason]` / `.lastpings`
AFK sets flag+timestamp; any mention of the user gets an inline notice with duration; auto-
clears on their next message. lastpings buffers recent mentions of you (who/when/where) with
`lastpings_*` component paging.
**v2**: two tiny tables + hook in Vilicus's MessageCreate path; zero-emoji notices.

### `.snipe` / `.editsnipe` / `.reactionsnipe` / `.clearsnipe` (`snipe_*/esnipe_*/rsnipe_*`)
In-memory ring per channel capturing last deleted message (author/content/attachments/time),
last edits (before->after pairs), last reaction removals. Commands display latest with paging
components; clearsnipe wipes channel state (mods).
**v2**: port to SQLite with retention window (e.g. 12h) instead of process memory so restarts
don't lose state; gate viewing behind ManageMessages like most bots do.

### `.highlight add/remove/list` (6 help pages)
Keyword watchlist per user: MessageCreate scans (case-insensitive contains), DMs the watcher
with jump link; cooldown to avoid spam.
**v2**: FTS or simple LIKE scan is fine at Vilicus scale; add regex option and channel scope.

### `.tag <name> [content]` (4 pages: recall/create/delete/list)
Guild-scoped snippets bucket; bare `.tag x` recalls (content supports `{user}` style tokens?  - 
plain content only), create/delete manage.
**v2**: fold into Vilicus alongside custom commands (08) as one "snippets" feature with tags +
invokes unified.

### `.alias add/remove/list` (6 pages)
Personal or guild alias: maps shortcut->existing command string; dispatch-time rewrite (the
router expands aliases before trigger match).
**v2**: implement in Vilicus router as pure prefix rewrite; store per (guild,user).

### `.invoke <trigger> <template...>` / `remove` / `list`
Guild custom dynamic command: template is replayed as if typed (can chain other commands);
Manage Guild gated.
**v2**: same as tag above  -  one snippets system covering tag (static) + invoke (command
template) + customcmd (rich actions).

### `.autoreact add/remove/list` / `.autoresponder add/remove/list`
Word-triggered emoji reactions and canned replies on message match; guild buckets.
**v2**: subsume into the automation engine as presets (trigger=message_create,
conditions=contains_phrases, actions=react/reply) instead of separate subsystems.

## Boards & engagement

### `.starboard channel/threshold/...` (120L)
Reaction * count threshold -> reposts message (jump link, content, image) to board channel;
dedupe by original ID updates the post.
**v2**: port with configurable emoji (not hard-coded star), CV2 quote container.

### `.clownboard` (426L, 15 help pages)
Same mechanic for clown emoji with leaderboard subcommands (most-clowned users), self-nominations,
cooldowns. Tone: mockery  -  optional to carry over.
**v2**: generalize starboard to N boards (emoji, threshold, channel)  -  one table kills both.

### `.levels [member]` / `.setxp` / `.setlevel` / `.removexp` (22 help pages incl. rank card, rewards, no-xp channels)
XP on message (per-message cooldown), level formula stored per user; rank rendering; rewards
roles at levels; ignore-channels; leaderboards. Admin cmds force-set values.
**v2**: port engine (table `xp(guild,user,xp,level,last_msg)`), skip image rank-card initially
(Vilicus zero-image aesthetic -> CV2 progress bar made of Section/Separator components).

### `.giveaways ...` (22 help pages, 5s draw loop)
Start (duration/prize/winners) -> embed + `giveaway_join_<id>` button entries stored; loop
draws at end, picks N distinct entrants, announces; reroll subcommand.
**v2**: port with SQL entries table (dedupe by user), component join button, dashboard mirror.

## Channel & voice

### `.voicemaster setup` + dynamic hub (835L, `vm_*` components)
Creates category + "Join to Create" hub; joining spawns a temp VC owned by the joiner with a
control panel (lock/unlock, limit, name, claim, kick/permit via select menus) rendered as
`vm_*` components; owner transfer on leave.
**v2**: port  -  needs voice-state dispatcher in Vilicus (add while porting). Panel as CV2
container edited in place.

### `.channel create|delete|rename|clone|nsfw|slowmode|topic ...` (10 pages) / `.hide <chan>` / `.unhide`
Channel CRUD wrappers perm-gated; hide/unhide toggles @everyone ViewChannel.
**v2**: one `/channel` group with subcommands; hide/unhide become `view` toggle there.

### `.moveall <from> <to>`
Moves all voice members of one channel to another (loops VoiceChannelMove).
**v2**: direct port.

### `.imgonly add/remove/list` (3 pages)
Image-and-link-only mode per channel: MessageCreate deletes plain-text-only messages.
**v2**: middleware-style channel mode table checked in the message path alongside filter.

## Misc utility core

### `.embed create/edit/send/code` family + `.createembed` / `.editembed` / `.embedcode` (10 pages)
Named embed templates stored as JSON, edited by name, sent to channels; embedcode prints the
JSON for copy-paste. createembed/editembed are interactive modal-driven builders
(`utility_expansion.go`).
**v2**: Vilicus-native decision needed: CV2 doesn't use classic embeds  -  port as *template
messages*: store full CV2 payload JSON, `.msgtemplate create/send/code`, builder modal writes
TextDisplay fields. More useful than porting embeds verbatim.

### `.names [member]` / `.gnames` / `.clearnames`
Username/nickname change history logged by an event handler; per-member and per-guild views;
clearnames wipes (mods).
**v2**: requires member-update dispatcher; table `name_history(guild,user,kind,value,at)`.

### `.invites` / `.inviteinfo <code>` / `.clearinvites`(01) 
Lists active invites (code/creator/uses/expiry); inviteinfo resolves any code without joining
(invite API) showing guild/channel/age.
**v2**: direct ports; inviteinfo is genuinely useful for mod triage.

### `.topcommands`
Leaderboard from the command-stats counters (every execution increments usage).
**v2**: add usage counting inside Vilicus executor (one INSERT)  -  enables this + dashboard chart.

### `.uptime`
Process uptime + go runtime stats.
**v2**: exists implicitly on dashboard; trivial chat port.

### `.firstmessage`, `.inrole <role>` (`inrole_*` paging), `.messages [user]`, `.whoisweb`
firstmessage fetches channel opener; inrole paginated member list of a role; messages counts
messages-per-user from counters; whoisweb opens Discord profile web link for an ID.
**v2**: all trivial reads; port inrole pagination helper (shared with history).

### `.stickymessage add/remove/list` (4 pages)
Per-channel sticky: after each user message, bot re-posts its sticky (deletes previous).
Implementation keeps last sticky ID in memory/db and schedules repost.
**v2**: port; throttle reposts (only if last message isn't already the sticky).

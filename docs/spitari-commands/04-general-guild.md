# 04  -  General & Guild Features (Chronicle -> Vilicus)

## Greeting / membership lifecycle

### `.welcome add/remove/test/list` (5 pages, JSON template)
Stores per-guild welcome channel + message; message is a Go text/template or JSON embed spec;
placeholders member/guild/server-count; fired on GuildMemberAdd; `.welcome test` renders for
the caller.
### `.goodbye ...` (5 pages)
Identical subsystem on GuildMemberRemove.
**v2 (both)**: one `greeting_config(guild, kind, channel_id, template)` table + Vilicus
member-add/remove dispatcher (to be added). Template engine: keep Go templates but expose the
same placeholder set as automation ({user},{guild},{member_count}...). Render as CV2 container
with optional banner image slot.

### `.autorole` (5 pages)
Role(s) auto-granted on join with optional delay seconds and bot/human scope; stored per
guild; join dispatcher applies.
**v2**: `auto_roles` table + same dispatcher; dashboard form.

### `.boosts add/remove/list <channel> <message>` 
Custom boost announcement messages per channel (GuildMemberBoost events are inferred from
member-update premium_since transitions).
**v2**: needs member-update watcher (same one as name-history); low effort after that.

### `.boosterrole <color> <name>` + rename/color/icon subcommands (18 pages)
Booster-exclusive custom role: verifies booster status, creates role positioned above base,
tracks ownership so color/name/icon edits and boost-loss removal work.
**v2**: port  -  genuinely engaging feature; table `booster_roles(guild,user,role_id)`;
requires role-position management care in hierarchy engine.

### `.boosters`
List of current boosters w/ duration (premium_since).
**v2**: trivial state read.

## Reaction & button roles

### `.reactionrole` (7 pages)
Classic react-to-get-role panels: message link + emojirole map stored; reaction_add/remove
dispatcher grants/revokes; dedupe/one-of-many modes.
### `.buttonrole create/editpanel/list` (9 pages)
Same via button components: panel built from parsed title/desc/footer/color/image args,
buttons added dynamically (`br_*` custom IDs), editpanel re-renders panel message.
**v2 (both)**: implement once as "role picker" supporting both reaction and button modes
(one table `role_bindings(guild,message_id,kind,key,role_id,mode exclusive)`); Vilicus has no
reaction dispatcher yet  -  add alongside.

### `.react <msglink> <emoji...>`
Bot reacts to any message with listed emojis (mods use it to seed reactions).
### `.reaction add/remove/list` (4 pages)
Word-triggered auto-reactions config.
### `.previousreact ...`
Auto-reacts to the message *before* the command user's message (chain games).
### `.noselfreact ...`
Config that auto-removes a user's reaction on their own message.
**v2**: react = trivial API call. The three configs fold into the automod/automation engine
(message_create middleware) rather than standalone systems.

## Time & scheduling fun

### `.birthday set/show/list` / `.timezone set [@user] [tz]` / `.hall`
Birthday loop (30s tick) checks day==today across stored dates, posts to configured channel
with age; timezone stores IANA zone per user used by birthday/time displays; hall = birthday
"hall of fame" listing.
**v2**: two tables (`user_meta(user,tz,birthday)`) + daily ticker; CV2 birthday card.

### `.bumpreminder set/channel`
Detects Disboard bump messages (author/embed signature) and reminds after 2h cooldown.
**v2**: niche but cheap: pattern matcher in message path + scheduled reminder reuse.

### `.dailyquestion set/channel` / `.dailyquote set/channel`
Daily rotating question (bundled list) or quote posted by 30s-loop date check; owners can
supply custom lists.
**v2**: merge into one "scheduled posts" feature: `scheduled_posts(guild,channel,cron-ish,
source(static|list|api))`.

## Game-integration monitors

### `.fisch [event_query]` (+ owner gating via `.owner fisch`)
Live browser of Fisch (Roblox game) servers from a community API: player counts, active
events (craters/cosmic relics/weather), rendered grey CV2-style containers, instant join
links; quotas/cooldowns configurable.
### `.fischmonitor add/remove/filter <#channel>` (11 pages)
Persistent poller per channel: creates webhook, posts server/event updates every 30s,
filterable event list, auto-cleans webhook on remove.
**v2**: game-specific; port only if wanted. Architecture worth noting: webhook-per-channel +
poll loop + filter table is the generic "feed monitor" pattern  -  build that generically
(`feeds(guild,channel,url,interval,filters)`) and Fisch becomes a preset.

### `.roblox <username>` (alias `rblx`, 26 help pages incl. sub-profile views)
Roblox user lookup: username->ID (users API), profile details (display name, created, badges,
presence where public), thumbnail render, friends/groups counts. 4s cooldown.
**v2**: portable keyless (Roblox public APIs); CV2 profile card.

## Reputation

### `.vouch <user> [comment]` / `.vouches [@user] [page]` (`vouch:*` paging)
Reputation vouchers: vouch appends entry (from,to,comment,at) preventing self-vouch/dupes;
vouches paginates with buttons; plugin layer adds cfg bucket (alt_check, alt_require,
acct_age_days) gating who may vouch/receive.
### `.vouchcheck <user>`
Aggregates vouches + staff notes into a reputation audit summary.
**v2**: single `vouches` table + anti-abuse gates (account age, guild tenure)  -  cleaner than
Chronicle's split general/plugin implementation. vouchcheck becomes an aggregate query.

## Vanity & tags

### `.vanity set/check` (4 pages)
Watches members' custom status for a string; matching users get a role (vanity reward);
checker lists current matches.
### `.servertag set/remove/check`
Same mechanic matching tag text in display names.
**v2**: both = member-update watcher + simple match rule table; unify as "name/status
watchers".

## Sticker/emoji/info family

### `.sticker add/send/info` (6 pages)
Steals/sends stickers by URL upload or ID info dump.
### `.emoji <emoji>` (9 pages incl. steal/add/delete/list)
Jumbo view + management (steal from other guilds via CDN download -> GuildEmojiCreate).
### `.steal <emoji> [name]` (`steal_*` component confirm)
Fun-package emoji thief with confirm button + name choice.
**v2**: one `/emoji` group: jumbo, steal (needs ManageEmojis perm + hierarchy), list, delete.
Image fetch pure stdlib.

### `.membercount`, `.bots`, `.channelinfo <chan>` (7 pages)
Counts humans/bots/boost tier; bot list w/ intents-ish badges; channel metadata dump
(type/topic/parent/perms summary).
### `.serveravatar/.serverbanner [user]` (guild-specific avatar/banner)
### `.guildicon/.guildbanner/.splash [guild_id]`
Cross-guild asset fetches by ID (works for any shared guild).
### `.whois <user>` (~ vilicus userinfo), `.pfp`, `.banner`
Per-user global assets  -  Vilicus has avatar/banner already; whois ~ userinfo.
**v2**: port membercount/bots/channelinfo + cross-guild asset trio; skip dupes.

### `.firstmessage`, `.inrole <role>`, `.messages [user]`, `.whoisweb`
Covered in 02 (utility core).

### `.yo`
Novelty ping variant (custom response text).
**v2**: skip or fold into Vilicus's ping easter egg.

### `.help`, `.ping`, `.prefix`, `.settings`
Overlaps  -  see README table.

## Owner/misc general

### `.fisch` gate config, `.owner *`  -  see 01-moderation.md owner section.

### `.verify` (captcha trigger surface)
Entry point for captcha flow  -  see 08-plugins.md captcha section.

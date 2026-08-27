# 07  -  Music (Chronicle -> Vilicus)

Backend: `internal/lavalink/lavalink.go` (1019L) + `player.go`. Vilicus has no music stack,
so this file is both command reference and the port blueprint.

## Engine facts (needed to understand the commands)

- **Nodes**: 14 hardcoded public Lavalink v4 nodes w/ credentials in-source; WS
  `/v4/websockets` handshake (Authorization/User-Id/Client-Name "Chronicle"). Preferred node
  = list[0]; a monitor goroutine re-probes `/version` (10s interval, 1s timeout) **only while
  no active players**; failure -> `cycleNode` to next, players re-attached.
- **LoadTracks**: `ytsearch:` prefix for queries; retries <= max(3, min(nodes,6)) cycling
  nodes on failure.
- **Voice**: raw op:4 state update + voice-server packet forwarding to node when both
  state+server known.
- **Local hosting** (optional): auto-downloads Lavalink.jar (GitHub release; build pins
  4.0.8), ensures Java (PATH -> bundled JRE -> Temurin 21 from adoptium API per OS/arch),
  writes application.yml (youtube plugin 1.18.0, MUSIC/WEB/MWEB/TVHTML5EMBEDDED/ANDROID_VR
  clients, all filters), JVM `-Xmx256m -XX:+UseSerialGC -Xverify:none`.
- **Player state per guild** (player.go): queue slice, loop mode off/track/queue, volume
  0-150, shuffle flag (shuffles after current), AddNext, RemovePosition, MovePosition;
  now-playing embed green 0x00ff00.

**Vilicus v2 engine**: self-host-first (own node via Docker compose  -  Vilicus already ships
docker-compose), single configured node, no public-credential list. Same player struct ported
as-is. Voice intents must be added to Vilicus's gateway config.

## Commands (music.go)

### `.play <url_or_query>`
Joins author's VC (or errors), resolves via LoadTracks (URL direct / ytsearch query),
appends queue (or AddNext with `playtop`-style arg), starts if idle, posts AnnounceStart.
Spotify/Apple URLs get metadata-resolved then re-searched on YouTube.
**v2**: same flow; CV2 now-playing card with Section (title/author/duration + requestor) and
MediaGallery thumbnail.

### `.pause` / `.resume`
PATCH player paused true/false.
### `.skip`
Advances to next queue entry (or stops if empty); vote-skip absent  -  requester/perm only.
### `.stop`
Clears queue + stops + leaves VC.
### `.queue [page]`
Paginated queue view (10/page): current + upcoming with durations and requestors.
### `.np`
Now-playing card w/ live-ish position bar (static snapshot, not updating).
### `.seek <ts|seconds>` / `.fastforward <offset>` / `.rewind <offset>`
Absolute or relative position PATCH (timestamp `1:30` parsing shared).
### `.volume <0-150>`
Clamped volume PATCH; persists per guild until changed.
### `.loop <off|track|queue>`
Sets loop mode; track repeats current, queue replays list.
### `.shuffle`
Toggles shuffle-after-current behavior.
### `.clear`
Empties queue (keeps current playing).
### `.preset <name>` / `.preset active`
Applies one of **11 filter presets** via Lavalink filters PATCH:
vaporwave(rate .85, lowPass), nightcore(1.25/1.25), chipmunk(1.10/1.50), boost(EQ lows+mids),
piano(EQ tamed highs), metal(EQ scooped mids), soft(EQ gentle), vibrato(tremolo depth .5
freq 10), 8d(rotation Hz .2), karaoke(karaoke filter band). `active` shows current filter
chain. `.preset off` resets all filters.
**v2**: presets port verbatim  -  they're just filter JSON templates; expose as autocomplete
choices. Add per-guild default volume + DJ-role gating table (Chronicle gates by perms
bits only).

## Port order recommendation

1. Lavalink client (connect/load/voice/player PATCH)  -  the hard 20%.
2. play/pause/resume/skip/stop/queue/np  -  the 80% usage.
3. seek/ff/rewind/volume/loop/shuffle/clear.
4. Presets + local-node autostart (reuse Chronicle's ensureJava/application.yml logic).

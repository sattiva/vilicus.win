# 05  -  Fun, Media & Novelty (Chronicle -> Vilicus)

Most of these are thin wrappers over public APIs or local transforms. Entries are compact;
mechanics = what the handler actually does. Group headers mark shared engines.

## Text transforms (local, zero network)

- `.ascii <text>`  -  figlet-style block font via bundled font maps; output in code block.
  **v2**: portable pure Go.
- `.owoify/.uwu <text>`  -  regex r/l->w + face insertion tables. `.freaky <text>`  -  adds
  tongue/drool emoji per word rule. **v2**: trivial ports (note vilicus zero-emoji constraint
  -> keep faces out or make opt-in).
- `.piglatin <text>`  -  consonant-cluster rotation rules. **v2**: trivial.
- `.charinfo <text>`  -  per-rune unicode codepoint + name lookup (unicode package).
- `.timediff <id1> <id2>`  -  snowflake decode math (`>>22+epoch`), delta humanized.
- `.randomhex` / `.color <hex>`  -  random color + contrast/HSL details; color shows swatch
  thumbnail. **v2**: swatch becomes a 1x1 data-URI? No  -  CV2 accent-color trick: render
  container with that accent.

## Media lookups (API-backed)

- `.anime <q>` / `.character <q>`  -  MyAnimeList search (Jikan-style endpoints), top result
  card w/ score/synopsis trim.
- `.book <q>`  -  OpenLibrary search API.
- `.tvshow <q>`  -  TVmaze singlesearch.
- `.twitch <user>`  -  helix-less scrape/API for live status + avatar.
- `.youtube <q>`  -  search scrape/API first video link + title.
- `.game <q>`  -  RAWG/cheap API game info.
- `.define <word>`  -  dictionaryapi.dev; `.urban <word>`  -  Urban Dictionary API (0th entry,
  definition/example truncated).
- `.lyrics <song>`  -  LRCLIB synced/plain lyrics fetch, long lyrics attached as .txt.
- `.findsong <song>` / `.find-id <artist>`  -  Genius keyless scrape (public token trick) for
  song metadata/artist ID.
- `.osu <user> [mode]`  -  osu! v1 API (keyed) profile stats card (rank/pp/modes).
**v2 group**: all direct ports behind one `media.go` with shared HTTP client + 5s timeouts +
per-command cooldowns. CV2 result cards, no emoji.

## Social existence probes

- `.github <user|owner/repo>`  -  api.github.com user or repo card (stars/langs).
- `.cashapp <$cashtag>`  -  probe cash.app/$x status code -> exists/not.
- `.tiktok/.twitter/.spotify <user>`  -  profile URL builders + light existence checks.
- `.paypal <username>`  -  paypal.me probe.
- `.telegram <user>` (read fully)  -  GET t.me/<u>, og:title/description/image meta extraction,
  HTML-entity unescape, embed w/ thumbnail; 404 -> not found.
**v2**: one `probes.go` with uniform "exists / link / avatar" triad responses.

## Entertainment & randomness

- `.8ball <question>`  -  seeded answer list.
- `.fortune`  -  fortune list. `.fact`  -  useless-facts API. `.kanye`  -  kanye.rest.
- `.compliment [@user]`  -  compliment API mention-wrapped.
- `.cat` / `.dog`  -  image APIs (thedogapi/cataas).
- `.choose <a|b|...>`  -  split on separators, rand pick.
- `.rps <move>`  -  beats-map vs crypto/rand move.
- `.wouldyourather`  -  WYR API or bundled list.
- `.fyp`  -  random TikTok share link from scraper pool.
- `.quickpoll <q>`  -  reacts  on the invoking message. `.poll <duration> <q>`  -  timed poll:
  posts, sleeps goroutine, counts reactions at expiry, announces winner.
  **v2**: use native poll objects? Discord polls exist now; otherwise component-button poll
  with tally table  -  better than reaction counting.
- `.rate <type> [@user]`  -  deterministic hash(user+type)%101 so results are stable per user;
  bar rendered in text. `.ship <u1> [u2]`  -  same seeded % + compatibility tier text.
- `.penis [user]`  -  seeded length joke bar. `.jukesiq`  -  seeded IQ + progress ring image
  (freetype-drawn). `.sun`  -  ASCII sun.
- `.blacktea` (55L+, `blacktea_*` components)  -  multiplayer word game: start prompt, join
  button, players submit words via modal rounds, scoring loop.
  **v2**: port concept as a generic "word game" session engine if wanted; low priority.
- `.chess [play|...]` (7 pages)  -  full chess vs Stockfish: notnil/chess board state, engine via
  external stockfish binary/API, board images or unicode board, move input parsing, levels
  0-20. NOTE merry.txt flags unsanitized FEN interpolation into a JS renderer (injection risk
  in their impl). **v2**: only port with strict input validation; heavy feature.

## Webhook pranks

- `.fakemessage <user> <text>` / `.impersonate <@user> [text] [media_url]`
Creates webhook in channel named after target with target avatar, posts the message (or media)
then deletes webhook. Impersonate allows arbitrary media attachment.
**v2**: works only where bot has ManageWebhooks; port with explicit perms check + audit log
entry (abuse-prone  -  Chronicle has none).

## Image generation

- `.quote [text|reply]`  -  renders quote card PNG locally (golang/freetype + x/image already
  deps): author pfp circle-cropped, wrapped text, watermark.
- `.billing <cashapp/paypal> <amount> <user>`  -  fake payment-success screenshot generator
  (drawn image) "for trolling skids". **v2**: fabricated receipts are exactly the kind of
  content Vilicus shouldn't ship  -  recommend skipping despite novelty.
- `.jumbo <emoji>`  -  custom emoji CDN URL or Twemoji fallback posted large.
- `.makemp3 <video_url>`  -  rehosts video attachment URL as audio-content-type link so Discord
  player plays it.
- `.hummusmap` / `.mommy setup ...`  -  freetype-rendered joke images / channel-configured joke
  persona panel (`mommy_*` config stored per guild).
**v2**: quote+jumbo portable; image-gen jokes optional; makemp3 is a URL trick  -  fine.

## Conflict-stats & regional commands

- `.gaza` / `.casualties`  -  daily casualty stats aggregator (API/wiki scrape), formatted
  fields + source links.
- `.lebanon`  -  same for Lebanon. `.bombs <palestine|lebanon|iran>`  -  conflict stats variant.
- `.border [region]`  -  Wikipedia current-events feed filtered to border/geopolitics items.
- `.tatreez` / `.hummusfact`  -  Wikipedia API fact pulls on cultural topics.
- `.hummus [loc|recipe]` / `.hummuschef`  -  location-based joke tracker (maps API distance
  sort) / generated platter card.
**v2**: political-stat trackers are legitimate info commands if sources stay cited; port the
pattern (fetch->format->source footer) not necessarily the topics.

## Security-novelty bundle (mostly owner-gated in Chronicle)

- `.token <token>` (owner)  -  decodes bot/user token structure (base64 user-ID segment),
  validates against Discord API, prints profile. Credential-handling command  -  **v2: skip**
  (normalizes pasting secrets into chat).
- `.skidcheck <user>`  -  heuristic threat score (account age, default-avatar, spam history
  from Palantir counts) -> verdict tiers.
- `.audit <user>`  -  last 5 archived messages by user (Palantir query).
- `.attackmap`  -  static link list to live threat maps.
- `.paste <text>` / `.paste load <id>`  -  AES-GCM encrypted one-time paste rows in DB: load
  decrypts and deletes row.
- `.temp`  -  host CPU temps via gopsutil-ish probing (host-dependent).
- `.entropy`  -  see 03. `.whcheck [channel]`  -  lists webhooks, flags orphaned/exposed ones.
- `.ocrfilter <image>`  -  OCR pass then invite/token/blacklist regex scan of extracted text.
- `.autobackup [status|now]`  -  background bots.db snapshot state trigger.
- `.fuckmicrosoft` (`fuckms_*` paging)  -  curated Windows vulnerability/exploit-technique list
  w/ pagination. Novelty content.
**v2**: worth porting: whcheck (real defensive value), ocrfilter (pairs with filter engine +
OCR), paste (nice utility), audit (needs archive). Skip: token, billing-style fabricators,
fuckmicrosoft content.

## Misc fun leftovers

- `.weed`, `.blunt`, `.juul`, `.yart`  -  joke image/response commands.
- `.airkiss`... roleplay set  -  see 06-roleplay.md.
- `.coinflip` (plugin)  -  see 08-plugins.md.
- `.moon`, `.mooncycle` (plugin)  -  see 08-plugins.md.

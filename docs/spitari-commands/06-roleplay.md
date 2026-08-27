# 06  -  Roleplay Action Commands (68 triggers, one engine)

`internal/commands/fun/roleplay.go` generates the entire family at init from a table of
`rpConfig` structs, appended to the registry as `fun.RoleplayCommands`. Every command works
identically:

## Shared mechanism (exact)

1. **Config per trigger** (`rpConfig`): trigger name, category keys for 5 GIF sources
   (`OtakuCat`, `GifukaiCat`, `TaomaCat`, `NekoBotCat`, `NekosCat`, `WaifuCat`), canned
   `SelfTexts` (no target) and `TargetTexts` (with target), `IsNSFW` flag.
2. **Fetch**: parallel-ish attempt order over the source APIs (nekobot.xyz
   `/api/image?type=...`, otaku/gifukai/taoma/nekos/waifu.pics style JSON `{url}` fields).
   Successful URL sets are cached per category in a mutex-guarded `map[string]*rpCache`
   so repeat commands often serve from cache.
3. **Fallback ladder**: if all sources fail -> hardcoded Giphy media URLs:
   generic list for most actions + `specificRPFallbacks` for hug/kiss/slap/pat/cuddle/cry/
   smile (+ explicit NSFW entries).
4. **Render**: grey embed  -  action verb line: no-arg -> random `SelfTexts` ("X is being shy");
   with `<@target>` -> random `TargetTexts` ("X hugs Y"), GIF as image, footer brand.
5. **NSFW flag** gates which channels/sources are acceptable.

## Full member list (68)

airkiss, angrystare, bark, bite, bleh, brofist, celebrate, cheers, clap, confused, cool,
drool, evillaugh, facepalm, grabbreast*, grabwaist, handhold, happy, headbang, laugh, lick,
love, mad, meow, nervous, nom, nuzzle, nyah, peek, pinch, poke, pout, punch, sad, scared,
shout, shrug, shy, sigh, sip, sleep, slowclap, smack, smug, sneeze, sorry, stare, surprised,
sweat, thumbsup, tickle, tired, wave, wink, woah, yawn, yay, yes
(+ the separately-defined hug, kiss, slap, pat, cuddle, cry, smile, fuck*, blowjob*, anal*,
punch/poke variants that share the fallback tables)

\* NSFW-flagged members exist in the same engine.

## Vilicus v2

One data-driven subsystem instead of 68 commands:

- **Single slash command** `/action name:<verb> target:@user` with autocomplete over the
  verb table + prefix shortcuts generated from the same table at startup.
- Verb table lives in a Go map or SQLite table: `{verb, self_lines[], target_lines[], tags[]}`
   -  adding an action = one row, zero code.
- GIF sourcing: pick ONE reliable provider + Giphy fallback; cache URLs with TTL; drop the
  five-source lottery.
- CV2 render: Section with target mention + MediaGallery item; grey accent; zero emoji.
- Content policy hook: NSFW-tagged verbs resolve only in age-gated channels (check channel
  NSFW flag)  -  Chronicle's flag exists but enforcement is loose.

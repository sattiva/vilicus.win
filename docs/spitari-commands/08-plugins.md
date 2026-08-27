# 08  -  Plugins (Chronicle -> Vilicus)

Plugin contract (`internal/plugins/plugins.go`): `Plugin { Name; Init(db, mgr); Commands() []*manager.Command }`,
registered in root main.go before bots start. Eight plugins ship.

## Economy (`internal/plugins/economy/`  -  plugin.go, gambling.go 1086L, shop.go 889L, stock.go 679L)

Storage: buckets `bktEcoAccts` (per-user account: wallet/bank/xp/level/inventory/assets),
`bktEcoCfg` (per-guild symbol/name/toggles), `bktEcoShop` (custom items). Component handlers:
`bj:*` (blackjack buttons), `hl:*` (highlow), `shop:*` (paging). All cooldowns are
timestamp-in-account checks.

### Wallet & transfers
- `.balance [@user]`  -  wallet/bank/net-worth + level/XP view.
- `.deposit <amt|all>` / `.withdraw <amt|all>`  -  walletbank moves.
- `.pay <@user> <amt>`  -  direct transfer (self-pay and overdraft blocked).
**v2**: `eco_accounts(guild,user,wallet,bank,...)` table; all money moves in one transactional
helper (Chronicle does read-modify-write without tx  -  race-prone under concurrency).

### Income loops
- `.daily`  -  24h claim w/ streak bonus multiplier.
- `.work`  -  random job payout (30m cd). `.beg`  -  tiny payout or nothing (30s cd).
- `.crime`  -  high-risk payout/fine (1h cd). `.rob <@user>`  -  steal % of target wallet on
  success, fine on fail (2h cd; target shielded if bank-heavy).
- `.fish` / `.hunt` / `.mine`  -  loot-table rolls (items->inventory or coins) 30m cds.
**v2**: one `cooldowns(guild,user,kind,last)` table + a single roll engine with named loot
tables; exact payout numbers live in Chronicle's gambling.go (skimmed  -  not reproduced here).

### Gambling
- `.slots <bet>`  -  3-reel symbol match payouts. `.cf <h|t> <bet>`  -  2x coinflip.
- `.dice <1-6> <bet>`  -  5x exact guess. `.roulette <bet> <red|black|green|0-36>`  -  up to 35x.
- `.highlow <bet>`  -  higher/lower chain vs next number (`hl:*` buttons).
- `.blackjack <bet>`  -  full BJ vs dealer with Hit/Stand/Double buttons (`bj:*`), blackjack
  3:2, dealer stands per house rule.
- `.scratch`  -  scratchcard instant reveal. `.race [n]` (`hr`)  -  horse race betting on lanes.
**v2**: same games are fine to port; centralize RNG + bet escrow (withdraw bet first, pay out
after) so a crash can't mint coins. Exact multipliers = design decision for Vilicus.

### Shop & items
- `.shop [page]` (`shop:*`)  -  lists custom items. `.buy <id> [qty]`, `.sell <id> [qty]`
  (50% custom value / fixed default), `.inventory [@user]`, `.use <item_id>` consumables.
- `.shopadd <id> <price> <name> [desc] [--role @r]` / `.shopremove <id>` /
  `.shopedit <id> <price|desc|stock> <v>`  -  admin CRUD; role-linked items grant the role on
  purchase.
**v2**: `eco_items(guild,id,name,price,stock,role_id,desc)` + purchase tx that decrements
stock, debits wallet, grants role atomically.

### Wealth meta
- `.richest [page]`  -  net-worth leaderboard. `.networth [@user]`  -  breakdown incl.
  inventory+stocks+assets.
**v2**: SQL aggregate views.

### Stocks / lottery / waifus / assets
- `.stock [list]`  -  live prices watchlist; buy/sell shares flows (portfolio tracked in
  account); prices from a quote API at trade time.
- `.lottery buy [qty]`  -  100-coin tickets, pooled pot, periodic draw loop picks winner.
- `.waifu roll` (`.wroll`)  -  gacha roll rarity tables; `.wlist` collection; claims/dupe rules.
- `.buyasset <id> [qty]` / `.sellasset ...` / `.business`  -  real-estate/business purchases with
  passive income accrual checked on interactions.
**v2**: stocks need price caching (don't hit API per trade); lottery as scheduled job;
assets' passive income should accrue via ticker not lazily-on-read (Chronicle's lazy model
drifts).

### Admin
- `.eco set/add/remove/reset/resetall/symbol/name/toggle`  -  account surgery + guild branding.
**v2**: keep, perms Administrator + audit rows.

## Captcha plugin (`internal/plugins/captcha/`)
`.verify` entry -> DM challenge: steambap/captcha 150x64 PNG; select menu of 5 shuffled
options (4 decoys); `captcha_start:*`/`captcha_select:*` components track attempts; wrong
picks decrement attempts -> FailureAction ban/kick; Verified/Unverified role swap on pass;
expiry countdown `<t:R>` re-issues. Join handler auto-challenges new members when enabled.
**v2**: port flow (solid design) with CV2 presentation; store pending verifications in SQL
with expiry sweeper.

## Custom commands plugin (`internal/plugins/customcommands/`)
`.customcmd/cc/customcmds create/delete/edit/list/info`: actions list (send_message,
send_embed, delay, add_role, remove_role, quarantine, dm) composed per trigger; requirement
gates (named perm bits, role); bypass owner-only toggle; list skips native-collision triggers.
`parseParams` handles quoted values + \" \n unescape.
**v2**: fold into Vilicus snippets/automation layer (02)  -  actions map 1:1 onto automation
action types already specified there.

## Lua scripting plugin (`internal/plugins/lua_plugin/`)
Scans `plugins/lua/*.lua`; seeds example.lua; scripts call
`chronicle.register_command{trigger=...,aliases=...,name=...,description=...,category=...,execute=...}`;
per-execution fresh gopher-lua state exposing CommandContext userdata (reply, send_text,
guild/channel/author IDs+tag, args); `.reloadlua` hot-swaps.
**v2**: dynamic scripting = arbitrary code execution by design. If wanted in Vilicus: isolate
(gopher-lua with no io/os modules, instruction-count limits, timeout kill) and gate loading
to the dashboard file manager, admins only.

## Moon plugin
`.moon` / `.mooncycle`  -  ASCII moon-phase render from synodic math (2551443s epoch delta);
phase name lookup.
**v2**: pure-math novelty; portable as-is.

## Fun plugin
`.coinflip`  -  heads/tails.
**v2**: trivial.

## Vouch plugin
Config bucket for the general vouch system (alt_check, alt_require, acct_age_days) + help
pages. Core logic documented in 04-general-guild.md.
**v2**: merge into vouch port as config columns.

## link plugin  -  NOT PORTED
C2 beacon under `--agent` (encrypted POST beacon every 5s to hardcoded controller, exec/
put/download/startup-persistence commands). This is remote-control infrastructure, not a bot
feature; Vilicus has no equivalent and should never gain one. Documented here only to close
out the inventory.

## fischrelay / aitools  -  NOT COMMANDS
Separate binaries (user-token webhook relay; HTTP :4000 tool microservice). No Discord
commands; out of parity scope but noted for completeness.

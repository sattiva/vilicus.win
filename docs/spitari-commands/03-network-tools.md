# 03  -  Network, Web & Crypto Tools (Chronicle -> Vilicus)

Chronicle's "systems-security engineering utility" layer. All HTTP calls go through
`internal/spoofer` (7 desktop UAs, optional SOCKS pool from `CHRONICLE_PROXY_POOL` or bundled
socks4/5 lists) and `internal/bootstrap/resolver.go` forces a Go resolver with 2s dial +
1.1.1.1/8.8.8.8 fallback. The `internal/ai/tools` package (706L) is the reusable core; the
`aitools/` microservice (:4000) mirrors the same endpoints over HTTP for external callers.

## DNS / IP / hosts

### `.dig <domain>` / `.dnslookup <domain>`
A/AAAA/MX/TXT (+CNAME) via net.Resolver; formatted field list.
**v2**: port with the same resolver hardening; CV2 code-block output.

### `.ip <addr>` / `.randomip`
IP lookup against ip-api.com fields (country/ISP/ASN/proxy flags); randomip generates a valid
random IPv4 (avoids 0/10/127/169.254/224+/reserved ranges).
**v2**: trivial ports; cache lookups 5min.

### `.pinghost <host>` 
TCP :80 dial + RTT measure (not ICMP).
**v2**: port; label it honestly as TCP ping.

### `.portscan <host>` (owner-only)
Sequential dials of a common-port list (~20 ports), timeout per dial, open/closed table.
**v2**: only worth porting if Vilicus keeps an owner persona; add concurrency + rate cap.

### `.cfresolve <domain>`
Scans ~15 common subdomains (mail, cpanel, direct, vpn...) resolving each; any A record that
isn't a Cloudflare range = probable origin leak.
**v2**: port as-is (defensive recon use); add CIDR check against published CF ranges.

### `.mcserver <ip> [port]` / `.mcuser <username>`
Minecraft: SLP status query (or mcsrvstat.us API) -> MOTD/players/version/icon; user -> Mojang
API UUID + skin/head renders.
**v2**: niche; port only on demand, API-backed.

## Cryptography (`lotsofcryptography.go`, 463L)

### `.uuid [v1|v4|v7]`
BUG: all three are crypto-rand bytes with version bits painted on  -  v1 has no timestamp/MAC,
v7 no ms-epoch prefix.
**v2**: port with real v7 (48-bit ms epoch + rand) and v4; drop v1.

### `.hash <md5|sha1|sha256|sha512> <text>`
Straight stdlib hashes, hex out.
**v2**: keep sha256/sha512; drop md5/sha1 or mark legacy.

### `.encode|.decode <base64|hex> <text>`
StdEncoding base64 / hex round-trip.
**v2**: add urlsafe + base32.

### `.gen <password|token|hex|base64> [len]`
crypto/rand generation, charset modulo (tiny bias), caps 1024.
**v2**: use rejection sampling for unbiased passwords; default 32.

### `.encrypt <text> <key>` / `.decrypt <text> <key>` (aliases `aes`,`aesdecrypt`)
AES-256-GCM; key <32B gets SHA256-derived; output base64(nonce+ct).
**v2**: fine pattern; enforce key >=16 chars, print key-derivation notice.

### `.entropy <password>`
Shannon entropy over char frequency -> bits + strength verdict.
**v2**: port (pure math, zero deps).

### `.jwt <token>`
Splits header/payload/signature, base64url-decodes both JSONs, checks exp/nbf vs now, flags
`alg:none` / HS256-with-public-key style audit warnings.
**v2**: port verbatim concept; pure stdlib.

### `.hashcrack <hash>` (alias `hashid`)
Identifies algorithm by length/charset ($2y$ bcrypt etc.) then dictionary lookup against a
bundled small wordlist (plus optional API).
**v2**: identification half is legit to port (defensive); skip cracking lookup in Vilicus.

### `.ciphercheck <host[:port]>`
TLS handshake dump: protocol version, cipher suite, cert CN/SANs/expiry, grade verdict.
**v2**: port with crypto/tls InsecureSkipVerify single-shot dial.

## OSINT / recon bundle (mostly `sativa_extras.go`; several owner-gated)

### `.osint <ip/domain/user>` (`osint_vector:*` components)
Pipelines multiple probes (ip-api, DNS, whois, breach-style APIs where keyed) into a paged
component report; access restricted to owners + `.owner osint enable` grants.
**v2**: skip unless there's a real use; if ported, each probe becomes a Vilicus tool func
shared with the AI tool set rather than one mega-command.

### `.shodan <ip>` / `.shodanhost <query>` / `.censys <ip>`
Keyless InternetDB (shodan internet-db) / keyed Shodan host query / Censys search  -  ports,
services, vuln banners, SSL info.
**v2**: InternetDB half is keyless & clean -> portable; keyed halves need env keys.

### `.subdomain <domain>` / `.subdomainbrute <domain>`
Passive: crt.sh certificate-transparency query; brute: concurrent DNS dials of wordlist.
**v2**: crt.sh half portable (JSON API); brute half fine with 20-goroutine cap.

### `.subtake <domain>`
Checks CNAMEs of common subdomains against dangling-pointing fingerprints (aws/azure/gh-pages...).
**v2**: niche defensive tool; portable as fingerprint table + CNAME walk.

### `.reverseip <ip>` / `.wayback <url>` / `.asninfo <ip>` / `.dnsdump <domain>` / `.dnspropagate <domain>`
HackerTarget/ViewDNS-style reverse lookups; Wayback CDX API latest snapshots; Team Cymru
whois -h whois.cymru.com ASN mapping; CT/logs dump; multi-resolver consistency check.
**v2**: all thin HTTP/TCP wrappers  -  port opportunistically behind one `/recon` group with
shared rate limiting.

### `.headers <url>` / `.certinfo <domain>` / `.s3check <bucket>`
Raw GET header dump (redirect chain shown); TLS cert details (see ciphercheck); S3 bucket
open-list probe (GET /?list-type=2&max-keys=5).
**v2**: headers+certinfo portable; s3check is a classic defensive audit  -  fine to include.

### `.maclookup <mac>` / `.binwalk <file>` / `.exif <image>` / `.strings <file>` / `.disasm <file>`
MAC OUI vendor (api.macvendors); signature scan of uploaded binary (embedded magic table);
EXIF tag extraction (x/image dep already in Chronicle); printable-string extraction; capstone
disasm preview.
**v2**: exif (x/image) and strings portable cheaply; binwalk-lite (magic table) fun but low
value; disasm needs a disasm dep  -  skip.

### `.leakcheck <email>` / `.doxsearch <username>` / `.botnetcheck <ip>` / `.attackmap`
Breach-API wrapper; multi-site username existence sweep (github/reddit/twitter probes);
Feodo/CINSArmy blocklist checks; links page to live cyberattack maps.
**v2**: privacy-adjacent; recommend porting only blocklist checks (.botnetcheck equivalent)
which are defensive. doxsearch/leakcheck conflict with Vilicus's cleaner positioning.

## Web content tools

### `.get <url> [ua]`
Raw fetch printing status + headers + body (truncated, attached as file when huge), optional
spoofed UA.
**v2**: port with size cap + content-type guard (never render remote HTML).

### `.scrape <url>`
Title/description/meta/text-content extraction (tools.Scraper: regex meta tags + text
collapse + link harvest <=N).
**v2**: port using golang.org/x/net/html instead of regexes (cleaner, handles malformed HTML).

### `.crawl <url> [depth] [max_pages]`
BFS from seed: same-origin only, depth<=2 pages<=20 enforced, per-page title/desc/links.
**v2**: port with robots.txt respect added.

### `.search <query>` / `.duckduckgo <query>`
DDG lite POST scrape parsed by result-snippet/result-link regexes (internal/search pkg),
spoofer headers, 6s timeout; export subcommand dumps results as file.
**v2**: DDG lite is fragile  -  port with html parser + fallback to Bing? Keep single source,
add caching.

### `.download <spotify-link>`
Resolves Spotify track metadata (no auth embed parse), finds YouTube match, streams via
yt-dlp-less direct Lavalink/3rd-party -> attaches MP3.
**v2**: legally fraught + brittle; recommend NOT porting into Vilicus proper.

### `.ticker <symbol>` (160L)
Crypto 24h chart from public exchange API (CoinGecko/Binance klines), ASCII sparkline or
image chart, price/24h% fields.
**v2**: portable (CoinGecko keyless); sparkline as unicode blocks fits zero-image rule.

### `.convert <amount> <from> <to>`
Fiat/crypto conversion via exchange rate API.
**v2**: trivial port.

### `.math <expression>`
Safe arithmetic parser (same engine as AI `calculate` tool: shunting-yard, +*/^ parens).
**v2**: port the parser  -  reusable for dashboard expressions too.

### `.l <luau-file/code>` (430L, embedded Lune engine)
Drops bundled Lune binaries per-OS into data dir, writes script to temp, executes, captures
stdout/stderr  -  used as Luau decompiler/source-dumper helper.
**v2**: embedding a script runtime executor = RCE surface; recommend not porting. If Luau
tooling needed, sandbox via separate process + resource limits like Chronicle's own build
does, or skip.

### `.codebase [path/repo]`
Walks directory (git-aware ignores), bundles text files <=size into one context file/zip for
LLM consumption.
**v2**: neat dev-tool; portable pure-Go; attach as zip.

### `.luac <code>`
gopher-lua parse/compile syntax check, no execution.
**v2**: safe subset  -  portable without shipping Lua runtime? No: needs the parser, which is
the same dep. Low priority.

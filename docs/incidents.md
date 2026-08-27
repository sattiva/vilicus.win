# Incident Response Runbook

Assumes a standard deployment: single binary
under systemd (`vilicus.service`) or docker-compose, SQLite at `DB_PATH`,
nightly verified backups in `BACKUP_DIR` (7 daily + 4 weekly rotation),
`.env` holds all secrets. Every dashboard action is in `dashboard_audit_log`
with actor + IP + req_id; chat moderation is in `moderation_audit_log`.

Correlation: structured log lines carry `req_id`; grep it across web and bot
logs to reconstruct one request's path.

---

## 1. Suspected bot-token leak

Symptoms: unknown guilds appearing, unexpected command usage, Discord dev-portal
"bot joined new servers" emails, gateway sessions you didn't start.

1. **Regenerate first, investigate second.** Dev portal -> Bot -> Reset Token.
   The old token dies on regeneration  -  no coordinated rotation needed.
2. Update `DISCORD_BOT_TOKEN` in `.env`, restart the service:
   `systemctl restart vilicus` (or `docker compose up -d`).
3. Kick anything the attacker added while the old token was live: leave unknown
   guilds via the dashboard console or dev portal, delete unknown webhook /
   integrations they created (Server Settings -> Integrations).
4. Check how far the blast radius goes: `dashboard_audit_log` for foreign
   actors (see incident 2)  -  token leaks and panel compromises often travel
   together.
5. If the leak came from this host (world-readable `.env`, committed file,
   log line printing secrets), fix that hole before considering it closed.

## 2. Dashboard compromise

Symptoms: unknown admin row, audit entries you didn't make, unfamiliar IPs in
the audit tail on `/analytics`, config changes nobody claimed.

1. **Kill every session now:** superadmin -> Admins -> "Log out all devices"
   (or directly: stop service, `sqlite3 $DB_PATH 'DELETE FROM
   dashboard_sessions'`, start). Outstanding CSRF tokens die with the epoch
   bump on the next privilege change; deleting sessions kills them outright.
2. Remove unknown admins and guild-admin grants from the Admins page. Note
   their IDs before deleting  -  you'll want them for step 4.
3. Rotate `SESSION_SECRET`: set the new value as `SESSION_SECRET` and move the
   current one to `SESSION_SECRET_OLD` for a 24h grace window if you need
   zero-downtime rotation; otherwise just replace it and let everyone re-login.
4. Review `SELECT * FROM dashboard_audit_log WHERE actor_id IN (<unknown ids>)
   ORDER BY created_at` for everything they touched: settings saves, config
   imports, console actions. Undo what matters  -  console bans/kicks also filed
   cases, so `/cases` shows the full list with case numbers to reverse.
5. If any console destructive action fired under the intruder: check
   `moderation_audit_log` for `[web:<name>]`-tagged reasons and lift wrongly
   applied sanctions (`.unban`, unjail).
6. Only after the panel is clean: consider whether the entry point was a
   compromised operator account (Discord-side)  -  password reset there beats
   more server hardening.

## 3. Database corruption / disk failure

Symptoms: `readyz` returning 503 ("db unavailable"), `integrity_check`
failures in logs, SQLite errors (`database disk image is malformed`),
process crash-looping.

1. Stop the service. Do not keep writing to a suspect file.
2. Assess: `sqlite3 $DB_PATH 'PRAGMA integrity_check'`  -  `ok` means you can
   restart instead of restoring; anything else continues below.
3. Restore the latest **verified** backup (every backup passed
   `integrity_check` at creation time; pick newest):
   ```
   mv $BACKUP_DIR/vilicus-<newest>.db $DB_PATH
   chown vilicus:vilicus $DB_PATH   # perms 0600
   systemctl start vilicus
   ```
4. Verify recovery: `/healthz` 200, `/readyz` 200, spot-check recent cases and
   guild configs against what people remember. Lost window = since the last
   night's backup; say so in the write-up.
5. If WAL files (`-wal`, `-shm`) exist next to the restored file from the dead
   instance, delete them  -  they belong to the corrupt database, not the backup.
6. Root-cause the corruption before trusting the box again: disk full, OOM
   killer mid-write, filesystem without barriers, failing drive.

## 4. Runaway feature / upstream storm

Symptoms: goroutine count climbing on `/analytics`, RSS slope upward, REST
429 counter rising fast, rate-limit refusals spiking, Discord API errors in
bulk.

1. Identify the feature from the metrics table + recent deploys. Music without
   a healthy node, an automation rule with a self-triggering loop, or a
   log-route pointed at itself are the usual suspects.
2. Fastest lever is always the process: `systemctl stop vilicus`. A broken
   automation rule stays broken across restarts  -  disable the specific rule
   from the community page (or `.automate disable <name>`) before starting up.
3. Feature env flags (music: unset `LAVALINK_HOST`) degrade cleanly by design;
   use them when the problem is a subsystem rather than bad data.
4. Restart, watch RSS/goroutines/queue-depth for 15 min. Queue depth should
   stay near zero; dropped-command counts must not increase.

## 5. Post-incident write-up template

Keep these short; the point is pattern memory, not paperwork.

```
Date:          YYYY-MM-DD HH:MM-HH:MM (UTC)
Severity:      minor | major | critical
Detected by:   (alert / user report / noticed)
Timeline:      req_id-correlated bullet list, first anomaly -> containment
Blast radius:  data lost / actions taken by attacker / users affected
Root cause:    technical, one paragraph
Fixes:         shipped now / tracked as issues
Residual risk: what remains exposed and why it's accepted
```

Store write-ups beside this doc (`docs/incidents/YYYY-MM-DD-<slug>.md`).

---

## Prevention checklist (quarterly)

- [ ] Restore drill: restore latest backup to a scratch dir, boot against it,
      `/readyz` green (plan section 3.6)
- [ ] `.env` perms still 0600, not world-readable, not in git history
- [ ] Backup dir contains expected daily+weekly files and each passes
      `PRAGMA integrity_check`
- [ ] Admin list matches reality; departed operators removed
- [ ] `dashboard_audit_log` skim for IP addresses you don't recognize
- [ ] govulncheck clean on current toolchain (CI enforces; check it's green)

# Deploying ADX

One Linux host (D-21: WokeWorks' Hostinger server), Caddy in front, the Go server and its River workers in one process, and the web build served as static files. Everything here is in `deploy/` as examples; copy and fill them on the server. Nothing in this runbook has been executed against the production host yet (see `docs/build/verification.md`).

## Layout on the host

```
/srv/adx/server/releases/<stamp>/adx     built binaries, one dir per release
/srv/adx/server/current -> releases/…    what systemd runs
/srv/adx/web/releases/<stamp>/           built web app
/srv/adx/web/current -> releases/…       what Caddy serves
/var/lib/adx/data/adx.db                 the database (ADX_DB_PATH)
/var/lib/adx/data/assets/                stored files (ADX_ASSETS_DIR)
/etc/adx/adx.env                         environment, root:adx 0640
/etc/adx/connections.key                 32 random bytes, adx:adx 0600
/var/backups/adx/<stamp>/                backups (see backup-restore.md)
```

## First install

1. Create the `adx` user and the directories above. The data directories must exist before the first start: production refuses to create them, so an unmounted volume fails loudly instead of starting empty.
2. Create the key once: `head -c 32 /dev/urandom > /etc/adx/connections.key && chmod 600 /etc/adx/connections.key`. Back it up somewhere other than the database backups.
3. Copy `deploy/adx.env.example` to `/etc/adx/adx.env` and fill it. Leave `ADX_ENV` unset. Dev mode has no sign-in and only listens on localhost.
4. Install `deploy/adx.service.example` as `/etc/systemd/system/adx.service`, plus the backup service and timer. `systemctl enable adx adx-backup.timer`.
5. Install `deploy/Caddyfile.example` with the real domain. Caddy obtains the certificate itself.
6. Run `deploy/release.sh /srv/adx` from a checkout of the commit to deploy.

## Each release

`deploy/release.sh` builds into a fresh release directory, swaps the server symlink, restarts, and polls `GET /api/ready` for up to 30 seconds. Only then does it swap the web symlink. If readiness never passes it points the server symlink back at the previous release and restarts, so a failed build or a server that can't open its storage never replaces the last working deployment.

## Health and readiness

- `GET /api/health` is liveness: the process answers.
- `GET /api/ready` runs the checks (database reachable, migrations applied, job worker running) and answers 200 or 503 with each check's name and pass/fail. It never includes paths, errors or secrets; the cause is in the server log. The web app's footer shows the same result.

## Configuration groups

| Group | Variables | Notes |
|---|---|---|
| Identity | `ADX_AUTHENTIK_ISSUER`, `ADX_AUTHENTIK_API_TOKEN`, `ADX_AUTHENTIK_GROUP_ID`, `ADX_AUTHENTIK_EMAIL_STAGE_ID`, `ADX_BOOTSTRAP_ADMIN`, `ADX_PUBLIC_URL`, `ADX_OIDC_CLIENT_ID`, `ADX_OIDC_CLIENT_SECRET`, `ADX_AUTHENTIK_GROUP_NAME` | all required outside dev; discovery against the issuer must succeed at startup |
| Writes | `ADX_LIVE_WRITES` | off by default; on, the Google Ads and Meta write adapters are registered and the outbox dispatches |
| Storage | `ADX_DB_PATH`, `ADX_ASSETS_DIR`, `ADX_CONNECTION_KEY_FILE` | directories must exist in production |
| Provider | `ADX_GHL_CREDENTIALS_FILE` (optional), `ADX_GOOGLE_OAUTH_CLIENT_ID`, `ADX_GOOGLE_OAUTH_CLIENT_SECRET`, `ADX_GOOGLE_ADS_LOGIN_CUSTOMER_ID`, `ADX_META_APP_ID`, `ADX_META_APP_SECRET`, `ADX_ADS_REFRESH_TRAILING_DAYS` | a platform without its pair of settings can't be connected and says so |
| Reports | `ADX_SMTP_HOST`, `ADX_SMTP_PORT`, `ADX_SMTP_USERNAME`, `ADX_SMTP_PASSWORD`, `ADX_SMTP_FROM` | optional; without a host reports are generated, not delivered |
| Model | `ADX_MODEL_PROVIDER`, `ADX_MODEL_API_KEY`, `ADX_MODEL_NAME`, `ADX_MODEL_EFFORT`, `ADX_MODEL_BASE_URL`, `ADX_MODEL_TIMEOUT_SECONDS`, `ADX_ASSISTANT_DAILY_TOKEN_BUDGET`, `ADX_MODEL_INPUT_PRICE_MICROS`, `ADX_MODEL_OUTPUT_PRICE_MICROS` | optional; without a key Ask ADX says it is not configured |
| Deployment | `ADX_ADDR`, `ADX_ENV` | Caddy owns the public domain |

Later phases add their own groups to this table and to `deploy/adx.env.example`.

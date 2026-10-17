# Verification log

Commands run and what they showed, per task. "Local" means fake providers and temporary SQLite. "Live" means a real provider account and is recorded separately; nothing in this file claims live verification unless a row says so.

## Toolchain (2026-10-17)

| Tool | Version |
|---|---|
| go | go1.27.2 darwin/arm64 |
| node | v22.23.1 |
| pnpm | 10.33.4 |
| sqlc | v1.31.1 |
| golangci-lint | 2.14.0 |

## B0.1 Baseline

Baseline before any change (commit `23b8843`):

```
cd server && go test ./...        # all packages ok, no failures
cd server && go vet ./... && golangci-lint run   # 0 issues
cd web && pnpm check && pnpm typecheck && pnpm test   # 19 files clean, 4 tests pass
```

Migration rehearsal: `server/store/store_test.go` seeds projects, users and a GoHighLevel connection at migration 00003, applies every later migration, and asserts the project, the user and the credential/account rows (including the encrypted secret bytes) survive. This test keeps guarding every migration added in later tasks.

Live: none.

## B0.2 Deployment, persistence and recovery

Local:

```
cd server && go test ./store/ ./core/ops/ ./api/ ./cmd/adx/   # ok
make check && make test                                        # 0 issues, all pass
```

- `TestProductionNeedsExistingStorage`: production refuses a missing data directory and does not create it; dev creates it.
- `TestBackupRestoresRecordsAndCredentialsWithTheKey`: seals a GoHighLevel private token with a fake provider, backs up, restores into a new directory, migrates, lists the project, and proves `Check` succeeds with the right key and fails with a wrong one. The backup file does not contain the token.
- `TestReadyNeedsNoSessionAndFailsWith503`: `/api/ready` answers without a session, flips to 503 when the jobs check fails, and the body carries no path.
- Web: `pnpm check`, `pnpm typecheck` clean; the footer shows Checking / Server ready / Server not ready: &lt;checks&gt; / Server unreachable. Not browser-tested yet (no dev server run in this task).

Live: `deploy/release.sh`, the systemd units and the Caddyfile were written, not executed. No host access in this session.

## B0.3 Real session identity and workspace scope

Local:

```
cd server && go test ./core/identity/ ./providers/authentik/ ./api/ ./cmd/adx/ ./core/users/   # ok
make check && make test
```

- `core/identity`: scoped session with membership role and workspace; cookie value never stored; callback single-use, expired, forged, provider-refused, outside-group and unknown-subject all refused; role change applies on next request; disable ends the session; 7-day idle and 30-day absolute expiry; sign-out; open-redirect `return_to` values collapse to `/`; provider failure starts no session.
- `providers/authentik` OIDC against an in-process issuer (discovery, JWKS, token, userinfo): authorize URL has state, nonce, S256 challenge and the groups scope and never the verifier; the exchange sends code, verifier and redirect URI; groups come from the ID token or userinfo; wrong nonce, wrong signing key and a bad code are refused; unreachable issuer fails startup.
- `api`: outside dev, no cookie, forged role/user headers and an invented cookie are all 401 and the dead cookie is cleared; a viewer's session stays viewer whatever header is sent; disabling the user makes the next request 401; logout ends only that session and a cross-site POST can't log out; dev admin exists only in dev and only without a session.
- `cmd/adx`: production requires issuer, public URL, client ID and secret, https public URL, and reachable discovery; dev starts without them.
- Web: `pnpm check`, `pnpm typecheck` clean. Header shows checking / Sign in / Dev admin (no sign-in) / name · role · workspace with Sign out.

Live: not verified. Needs the `adx-auth` provider's client ID and secret, its redirect URI set to `ADX_PUBLIC_URL/api/auth/callback`, and the groups scope mapping. Owner of the authentik instance: WokeWorks (D-24).

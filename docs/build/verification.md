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

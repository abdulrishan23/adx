# Build decisions

Decisions, engineering assumptions and unresolved business rules made while running `ADX_AUTONOMOUS_BUILD.md`. Numbered `BD-nn` so they don't collide with the `D-nn` decisions in `docs/architecture.md`. When one of these changes an architecture decision, the architecture doc gets a new `D-nn` entry too and the old one is marked replaced, never deleted.

Business policy is never invented here. Where a rule is missing it is listed under "Unresolved business rules" with who supplies it, and the code ships the rule as typed configuration in an unconfigured state.

## Decisions

### BD-01. Build ledger lives in `docs/build/`
2026-10-17. `progress.json` (task states and evidence), `decisions.md` (this file), `verification.md` (commands and results, fake-provider versus live) and `inventory.md` (baseline). Updated after each task and committed with it.

### BD-02. Commit per task, locally, one-line Conventional Commits
2026-10-17. The checkout has no remote. Each task that passes `make check` and `make test` is committed on `main` so progress is diffable. No force-push or reset.

### BD-03. The spec's "existing lead ingestion" and mock layer do not exist here
2026-10-17. The checkout is the pristine baseline. Lead import, analytics screens and mock selection are built new in phase 1 rather than extended. Recorded so nobody looks for code that was never there.

### BD-04. Toolchain installs
2026-10-17. pnpm 10.33.4, sqlc 1.31.1 and golangci-lint 2.14.0 were installed user-locally (see `inventory.md`). Nothing else was installed system-wide. air is optional and was skipped.

## Engineering assumptions

Defaults chosen so the platform can be built without the owner's policy. Each one can be overridden by configuration and none activates an external effect.

- EA-01. External mutation policy defaults to disabled for every provider and action until an admin configures it (spec section 2).
- EA-02. Assistant tools default to read-only.
- EA-03. Missing identity denies; missing workspace or account scope denies; missing business evidence is null or "needs review", never 0 or a guess.
- EA-04. An unresolved customer match is never auto-merged.

## Unresolved business rules

Supplied by Abdul (Reportage CEO, owner of this build) or by whoever he names. Dependent behaviour stays disabled or unconfigured until then.

| Rule | Needed by | State |
|---|---|---|
| Final GoHighLevel status-to-stage mapping | B1.2, B1.3 | unconfigured |
| MQL/SQL criteria | B4.2 | unconfigured |
| Attribution-conflict resolution (Google and Meta both claim a lead) | B1.3 | unconfigured; claims kept separate |
| Source precedence for price/inventory facts | B2.1 | unconfigured; conflicts need review |
| Approved budgets and write privileges per account | B0.4, B3.4 | unconfigured; writes disabled |
| Urgent anomaly thresholds | B1.4 | unconfigured; checks don't fire |
| Calling, recording and contact policy | B4.4, B4.5 | unconfigured; live calling disabled |
| Retention periods | B0.5, B6.3 | unconfigured; nothing deleted automatically |
| Reporting recipients | B1.4 | unconfigured; nothing delivered |
| Exact Excel media-plan template and marketing skill file | B2.3 | missing; exact-template adapter blocked |

### BD-05. Production never creates its storage directories
2026-10-17. `checkStorage` in `cmd/adx` requires the directories of `ADX_DB_PATH` and `ADX_ASSETS_DIR` to exist outside dev. Dev creates them.

- **Why.** A missing directory on a server means the volume isn't mounted. Creating it would start a fresh, empty ADX on a disk that disappears at the next reboot, and nobody would notice until the data was gone.
- **Cost.** First install has one more manual step, listed in `docs/runbooks/deploy.md`.

### BD-06. Readiness reports check names, never causes
2026-10-17. `GET /api/ready` answers 200 or 503 with `{ready, checks[{name, ok}]}`. The cause of a failure goes to the server log. The route needs no session because the proxy and the deploy script call it.

### BD-07. Backups hold sealed credentials, the key travels separately
2026-10-17. `adx backup DIR` copies the database (SQLite `VACUUM INTO`) and the assets tree. The encryption key is never copied. The restore runbook rehearses in an isolated directory first. This is also the stance for every later secret: a copy of the data alone must be useless.

### BD-08. Web sign-in is OpenID Connect against authentik, sessions live in SQLite
2026-10-17. Implements D-24 for the web. `core/identity` owns sign-in attempts (hashed state, PKCE verifier, hashed nonce, 10-minute life, single use) and sessions (hashed random ID, 7 days idle, 30 days absolute, looked up on every request). `providers/authentik/oidc.go` does discovery, the code exchange with PKCE and client secret, RS256 verification and the nonce check, and reads `groups` from the ID token or userinfo. The `groups` claim must contain the configured group name (`ADX_AUTHENTIK_GROUP_NAME`, default `adx-users`) or `users.SignIn` refuses.

- **Why.** D-24 already chose authentik and database-backed sessions. A trusted-proxy header boundary was the alternative; it was rejected because the API and the web build are deployed separately and a header can be set by anyone who reaches the Go port.
- **Cost.** The authentik `adx-auth` provider needs the groups scope mapping enabled and the exact redirect URI `ADX_PUBLIC_URL/api/auth/callback`. Live sign-in is unverified until the provider's client ID and secret exist (see verification.md).
- **Not done.** MCP sign-in (ADX as an OAuth server for Claude, D-24) is not part of B0.3 and stays pending.

### BD-09. Actor carries a workspace; phase 0 has one
2026-10-17. `core.Actor` gains `WorkspaceID`, filled from the session's membership, never from the client. Migration 00006 creates workspace 1 "Reportage" (`core.DefaultWorkspace`) and a membership per existing user. While there is one workspace, `memberships.role` and `enabled` mirror `users.role` and `users.status`; `core/users` writes both in one transaction and ends the person's sessions when disabling them. New domains from B0.4 on scope their rows by `workspace_id` and refuse an actor whose workspace doesn't match. Existing tables (`projects`, `accounts`, `credentials`) stay unscoped until B6.1 migrates them.

- **Cost.** Two columns say the same thing until B6.1 drops the mirror. Recorded so the duplication is removed, not extended.

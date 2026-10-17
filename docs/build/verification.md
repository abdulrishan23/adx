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

## B0.4 Policy and provider capability contracts

Local:

```
cd server && go test ./core/policies/ ./core/connections/ ./api/   # ok
make check && make test
```

- `core/policies`: no document denies even a verified token; account not enabled, write capability unverified, account not connected each deny with their reason; marketer vs admin modes (needs_approval vs self-approved allowed); off, viewer, unknown action denied; missing caps deny raises; kill switch per account and per workspace, cleared again; admin-only Put with validation and incrementing versions; another workspace sees its own absent policy.
- `core/connections`: `ListAccounts` shows connected → stale after 25 h → needs_permission after a rejected read (keeping the last verified time) → failed on a raw error (message replaced by a safe one) → connected after a good check; one real account throughout, never a fixture.
- `api`: GET /api/policies for a marketer shows unconfigured; PUT refused for a marketer (403), invalid action 400, admin 201 with version 1; kill switch PUT 200; GET /api/accounts without a connection service returns an honest empty list.
- Web: `/connections` and `/settings/policies` pages with loading, empty, error and unconfigured states; typecheck and biome clean. Not browser-tested in this task.

Live: none. No Google or Meta account exists yet; the policy document and caps are the owner's inputs (decisions.md, unresolved rules).

## B0.5 Evidence, runs and durable operation foundation

Local:

```
cd server && go test ./core/runs/ ./core/evidence/ ./jobs/ ./api/   # ok
make check && make test
```

- `core/runs`: same occurrence reused while queued or running; failed run requeued as attempt 2 under the same public id; succeeded refused with partial or unknown completeness; failure needs a message; two workers can't lease one run; two runs can't hold one account; expired lease → interrupted with a reason; the dead worker's later Progress/Finish refused; the account frees for the other run; runs invisible from another workspace.
- `core/evidence`: identical re-record ×3 keeps one row; changed payload creates version 2 and supersedes version 1; lookup lists current versions only, scoped by workspace; invalid class/completeness/kind refused; viewer can read, not write.
- `jobs` (real River on temp SQLite): a started run is worked exactly once, records three evidence rows as the system actor in the run's workspace, finishes complete; executor `core.Error` becomes the run's safe error; a raw error is replaced by a generic message; a kind with no executor fails visibly instead of hanging.
- Web: `/runs` page and `RunStatus` component (state, incomplete-read marker, observed-through freshness, safe error, attempt). Typecheck and biome clean.

Live: n/a (no provider work exists yet to run).

## B0.6 Reusable creative and voice integration boundaries

Local:

```
cd server && go test -race ./core/assets/ ./providers/filestore/ ./core/voice/   # ok
```

- `core/assets`: review transitions (no draft → approved shortcut, approved terminal), an acceptance covers exactly one ref and never grants delivery, the four formats' dimensions.
- `providers/filestore`: identical bytes → same key, different bytes → different key, the stored file is not rewritten, a tampered file is refused on open with a checksum error, traversal keys refused, empty and untyped puts refused, missing directory refused.
- `core/voice` with fakes and the race detector: greeting frames reach the caller in order, caller audio reaches the engine, an interruption clears the far side once, the end sends `end-of-call` and waits for the echo, transcript returned; caller hangup ends cleanly; codec mismatch is an error, not noise; engine start failure fails the call; maximum duration ends a never-ending engine; an unsigned status callback is refused.
- Inventories: `docs/build/reuse-creative.md`, `docs/build/reuse-voice.md`.

Live: none. No telecom route or speech credential is configured, by design.

## B1.2 GHL mapping, identity evidence and funnel configuration

Local:

```
cd server && go test ./core/leads/ ./providers/gohighlevel/ ./api/   # ok
make check && make test
```

- `core/leads`: no mapping → status not located, stage null, nothing marked missing; mapping → status, stage, case-insensitive project match, source; same contact without the status field keeps "Follow Up" and records `statusMissingSince`; an unmapped status is kept raw, listed under `unmapped`, and a new mapping version re-stages it; two contacts sharing a phone stay two leads and the shared phone is counted; a two-page full import then a failed page-2 scan removes nothing and records the cursor; the retry resumes at `cursor-1`, completes, removes the vanished contact; identical content re-recorded is one evidence row; incremental import passes `since` and removes nothing; click id → high, UTM → low, repeated attribution records not duplicated; validation and role checks; unconfigured CRM fails the run with a conflict, not a panic.
- `providers/gohighlevel` contact search against a fake server: POST /contacts/search with Bearer token, Version 2021-07-28, `locationId`, `pageLimit` 100, `sort` [dateAdded, id], no `page`; the next cursor is the last contact's `searchAfter` verbatim; a short page is the last page; lowercase names, multi-select and numeric custom fields, both attribution objects decoded; incremental adds the `dateUpdated` range filter and sort; 429 → "limit" conflict, 401 → "rejected" conflict, provider body never surfaces.
- `api`: list, coverage and funnel readable by a viewer; funnel PUT 403 for a marketer, 201 for an admin, 400 for an unknown stage; import 403 for a viewer, 404 for an unconnected account.
- Web: `/leads` (coverage, filter by stage, raw status next to stage, "field missing on last scan", bucket-not-a-source marker, import button for marketers) and `/settings/funnel` (field locations, observed statuses with counts, stage per status, unmapped banner, save as new version). Typecheck and biome clean. Not browser-tested.

Live: not verified. Needs the ADX Test location's Private Integration Token (docs/gohighlevel-development.md) and the owner's answer on which field holds the status. Three API details are marked unconfirmed in BD-20 and will be checked on the first live import.

## B1.1 Google and Meta real read adapters

Local:

```
cd server && go test ./core/reporting/ ./core/connections/ ./providers/googleads/ ./providers/meta/ ./jobs/   # ok
make check && make test
```

- `core/reporting` with a fake paginated provider: two campaign pages and two metric pages; page 2 of metrics fails once → run failed, page 1 kept, cursor recorded, freshness marks partial with the error; retry reads only page 2 (campaign pages hit once each, metric page 2 twice: one failure, one read); a ghost campaign's row counted and skipped; re-read replaces revised rows without duplicates; a range including Dubai's today ends partial, observed through yesterday; Google and Meta rows keep separate claim columns and filter by platform; role, date and account validation; unconfigured platform fails the run with a conflict.
- `providers/googleads` against a fake API: searchStream path, Bearer header, `login-customer-id` from the dashed manager id, no developer token, both batches decoded, micros → fils with rounding, int64 strings, null conversions stay null; hierarchy walks `listAccessibleCustomers` then `customer_client` and skips a forbidden customer; quota error carries `retryDelay` 42s and hides the request id; auth, authorization and 5xx mapped.
- `providers/meta` against a fake Graph: insights params (campaign level, `time_increment=1`, `time_range`), token in the header not the URL, string numbers, spend 5339.5 → 533950 fils and 0.01 → 1, only the `lead` action counted, `paging.next` followed and a foreign-host cursor refused, budgets as minor-unit strings with daily/lifetime period and nil for ad-set budgets; 190 → reconnect, 17 → throttled with the header's 12 minutes, 200 → permissions, 100/1487534 → too large, transient → unavailable.
- OAuth adapters: Google consent URL has offline access, S256 and the adwords scope; exchange sends the verifier and redirect URI; refresh token required; `invalid_grant` → reconnect. Meta exchanges the short token for a long-lived one, reads the subject from `/me`, refresh re-exchanges, 400 → reconnect.
- `core/connections`: a sign-in reaching no accounts saves nothing; the manager is not saved as a spend account; currency, time zone and parent saved; tokens sealed; `ProviderToken` refreshes within an hour of expiry once and stores the new expiry; viewer refused; replayed callback refused.
- Web: connect buttons for admins, per-account freshness line ("never read" is a state), "Refresh last 30 days" for marketers. Typecheck and biome clean. Not browser-tested.

Live: not verified. Needs a Google Cloud OAuth client on a project with Google Ads API access and the Head of Marketing & AI's Google sign-in; a Meta Business app with the connecting person given a role (D-22). Neither exists on this machine. Same-account/date reconciliation against the platforms' own UI is therefore blocked.

## B1.3 Deterministic metrics and comparison engine

Local:

```
cd server && go test ./core/reporting/   # ok
make check && make test
```

- `TestAnalyticsNullsRatiosAndComparison`: a lead at 02:00 Dubai (22:00 UTC the day before) counts on the Dubai day; Follow Up and Done-Deal are qualified; Google conversions (6) and Meta leads (30) stay in separate fields in rows and summary; cost per lead 200000 ÷ 2; revenue and ROAS null (not undefined); cost per qualified lead undefined with zero qualified; summary spend equals the rows' sum; coverage counts eligible, matched, unclassified and with-status; previous period computed as the two days before; Google +100% relative, Meta's relative change undefined with no base and absolute not available; leads from zero are undefined relative with a real absolute; `comparison_incomplete` warning when the comparison period has gaps.
- `TestAnalyticsGroupingFiltersAndMissingDays`: project, day and platform groupings reconcile with the summary; platform filter keeps Meta out; a day with no rows is listed as missing for the account; empty period gives empty rows and null summary spend; role and parameter validation; year-over-year range.
- `TestNoFunnelMeansNoQualifiedFigures`: without a funnel mapping qualified leads, deals and cost per qualified lead are null, and cost per lead is undefined with spend and no leads.
- Web: `/analytics` with period, grouping, platform and comparison controls; freshness, coverage, dependencies-missing and warnings shown; "Not available", "No leads" and "no base" are distinct states. Typecheck and biome clean. Not browser-tested.

Live: not applicable until B1.1's live gate opens; the engine runs on whatever rows the adapters import.

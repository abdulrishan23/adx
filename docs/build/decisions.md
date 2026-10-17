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

### BD-10. Policy is a versioned document; unconfigured denies every write
2026-10-17. `core/policies` stores the permission checklist, caps and per-account write enablement as one JSON document per workspace, with a new version on every save. `Evaluate(actor, request)` returns allowed, needs_approval or denied with a stable reason code and the policy version. Denial reasons in order: no workspace, role below marketer, unknown action, no policy document, kill switch (workspace or account), account not connected, account not enabled for writes, write capability not verified for the account, action unconfigured or off, cap unconfigured. D-13's table is exposed as `SuggestedDefaults` for an admin to apply deliberately; caps and write accounts stay empty in it, so applying it alone still denies.

- **Why.** The spec forbids inventing business policy and says a token existing never implies permission to write. Keeping the document versioned lets a later proposal name the version it was decided under.
- **Cost.** Until Abdul (or the CMO) sets caps and enables accounts, no write path can run, including in a demo. That is intended.

### BD-11. Capability state per account, health derived from it
2026-10-17. `account_capabilities` records each probe (`read.leads` for GoHighLevel so far) as verified, failed or needs_permission with a safe message. The accounts screen derives health: failed if any capability failed, needs_permission if any needs permission, stale if the last verification is older than 24 hours (engineering default), else connected. Health is never stored and no fixture account is ever shown.

### BD-12. Exported JSON types get unique names across core packages
2026-10-17. `gentypes` flattens every package into one TypeScript module, so `policies.Action` and `audit.Action` would merge into one wrong union. Types that cross the wire are named uniquely (`WriteAction`, `PolicyDocument`, `PolicyState`). Internal types can keep short names.

### BD-13. Runs are named by occurrence, leased by workers, never completed by a partial read
2026-10-17. `core/runs` keys each piece of durable work by `(workspace, kind, occurrence_key)`. Starting the same occurrence returns the existing run while it is queued, running or finished; a failed or interrupted run is requeued as a new attempt under the same reference. Workers lease the run and, when it names a provider account, the account, for 5 minutes between renewals. Every write by a worker is conditional on still holding the lease. `Finish` refuses `succeeded` unless completeness is `complete`. Recovery (startup and every minute) marks runs with expired leases `interrupted` and retries them up to 3 attempts, then leaves them for a person.

- **Why.** The spec requires stable occurrence keys, account leases and interrupted-run recovery, and that a partial read can't be called complete. Conditional writes make a dead worker's late writes harmless.
- **Cost.** A run kind must call `Progress` at least every 5 minutes or lose its lease. Long provider reads page, so that fits.

### BD-14. Evidence is versioned, never edited
2026-10-14. `core/evidence` keys an observation by `(workspace, source, kind, source_ref)`. Re-recording identical content is a no-op; different content inserts version n+1 and marks the old row superseded. Class is observed, estimated or inferred and completeness complete or partial, both required. Retention is unconfigured (`retain_until` null) and nothing is deleted.

### BD-15. River inserts the run's job in the run's transaction
2026-10-17. `jobs.Queue` implements `runs.Enqueuer` with River's `InsertTx` and a unique constraint on `run_id`, so a committed run always has exactly one live job and a rolled-back run has none. The job worker finds the run's executor by kind; unknown kinds fail the run visibly. Executors live outside `jobs` (phase 1 registers imports and refreshes in `cmd/adx`).

### BD-16. Creative behaviour is ported, not retained as a service
2026-10-17. The creative-studio app (`docs/build/reuse-creative.md`) is not run as a service beside ADX. Its layout document (`DesignDoc` v1), format table, copy blocks, house-rule checks and EN/AR handling are ported as `core/assets` types and, later, a React editor on the same document. Storage is `core/assets.Store`, implemented by `providers/filestore` (content-addressed by SHA-256 under `ADX_ASSETS_DIR`, written once, verified on read). Identity is ADX's session. Review is `assets.ReviewState` (draft → rendered → in_review → approved/revise); approved is terminal and a changed document is a new version with no approval. `ContentAcceptance.GrantsDelivery()` is always false: delivery authorization lives in the execution domain (B3.4).

- **Why.** The source has no immutability, no versions, anonymous approvals and Supabase identity, all of which the spec requires ADX to own. Its document model and rules are worth keeping.
- **Cost.** The canvas editor has to be rebuilt in ADX's React app (B1.6). The Python generator's contrast-aware placement is not ported.

### BD-17. Voice is three interfaces and a bridge, with fakes only
2026-10-17. `core/voice` declares `Telephony` (place/hang up/parse signed callbacks), `Transport` (one call's media frames, clear, mark) and `Speech` (conversation sessions). `voice.Bridge` ports the reference source's orchestration: codec check, queue engine audio in order, clear the far side on interruption, send an end mark and wait for its echo before closing, start timeout and maximum duration. No education rule, prompt or outcome enum is ported. `voicetest` fakes run the whole bridge with no credentials. Twilio and ElevenLabs adapters are not written in phase 0; the UAE telecom route is unchosen and unauthorized (blocker for B4.4). Ambience mixing and µ-law framing are deferred to B4.5.

### BD-18. D-27 recorded in docs/architecture.md
2026-10-17. The widening of scope the spec asks for is recorded as D-27 with the earlier decisions it changes marked, per the spec's instruction not to delete decision history. Live use of calling and messaging needs Reportage's written policy; the build does not.

### BD-19. Leads are provider contacts; the funnel is a versioned mapping that ships unconfigured
2026-10-17. `core/leads` keys a lead by `(account, GoHighLevel contact id)`. Nothing merges on phone or email: shared numbers are counted and shown, and person resolution is B4.1's. Every contact observation is evidence (`gohighlevel/contact`, versioned on content). The status, project and source are read through `FunnelDocument`, a versioned per-workspace mapping: where the status lives (custom field, contact field or tag prefix, because which CRM column holds it is open question #34), raw status → stage (catalog fixed in code: open, attempted, engaged, meeting, offer, won, disqualified, excluded, dead), blank status → stage, raw source → ADX source. Saving a new version re-stages every lead. With no mapping, status isn't located and stage stays null; with a mapping but an unmapped status, the raw value is kept and listed for the admin.

- **Why.** Reference.md says admins edit the status-to-stage table and ADX flags unmapped statuses; Hisham's table is a proposal, not a configured rule, so it is not seeded.
- **Status missing.** When a scan returns a contact without the configured status field, the last known status stays and `status_missing_since` records it (spec B1.2). Only a complete full scan marks unseen contacts removed; a failed or partial scan removes nothing, and a retry resumes from the recorded `searchAfter` cursor.
- **Campaign match.** Attribution records are graded by reference.md's match levels: a click id (gclid, gbraid, wbraid, fbclid) is high, a platform campaign id medium, a hand-typed UTM campaign low. The campaign reference stays text until B1.3 has campaign rows to resolve it against.

### BD-20. GoHighLevel import uses POST /contacts/search with searchAfter
2026-10-17. From HighLevel's current docs (`scratchpad` research, 2026-10-17): `GET /contacts/` is deprecated and orders only by `dateAdded`; `POST /contacts/search` (API version 2021-07-28, Private Integration Token accepted) pages with the opaque `searchAfter` array each contact carries, which is the only way past 10,000 records. Full imports sort by `dateAdded, id`; incremental imports filter `dateUpdated` with a `range` and sort by it. Page size 100 (the API allows 500) to stay inside the 100-requests-per-10-seconds burst limit. Search results carry `firstNameLowerCase`/`lastNameLowerCase` rather than cased names, `customFields[].value` of any JSON type, and two attribution objects (`attributionSource`, `lastAttributionSource`) rather than an array. Webhooks need a Marketplace OAuth app, not a PIT, so contact deletes are detected only by full scans until an OAuth app exists.

- **Unconfirmed on the docs.** The `searchAfter` element shape (treated as opaque JSON), the `range` value shape for date filters (ISO-8601 strings assumed), and 429 behaviour (no `Retry-After` documented; the run fails safely and is retried). Live verification against the ADX Test location will confirm or correct these.

### BD-21. Google Ads and Meta are read through separate adapters written from current docs
2026-10-17. `providers/googleads` targets REST v26 with `searchStream` (unpaged JSON array of batches, int64 as strings, micros ÷ 1e6 to minor units, no developer token since the 2026-09-09 sunset, `login-customer-id` only through a manager). `providers/meta` targets Graph v26.0 (`/act_{id}/insights` at campaign level with `time_increment=1`; numbers are strings; `spend` is decimal major units and budgets are minor-unit strings, both normalized to minor units; `paging.next` followed only on the API host; the token travels in the Authorization header, never the URL). Each maps provider errors to safe `core.Error` messages with the provider's text and trace ids kept in the cause. Google's `conversions` and Meta's `lead` action are stored in different columns (`conversions`, `platform_leads`) and never summed. The doc-sourced facts are in `docs/build/api-notes/`; unconfirmed items (Meta lead node fields, Meta `/me/adaccounts` reference page, Google REST response cap) are listed there.

### BD-22. Ad platforms connect by OAuth, one credential per person, accounts discovered from the token
2026-10-17. `connections.StartAds/FinishAds` reuse the GoHighLevel attempt table (state, browser binding, 10 minutes) plus a PKCE verifier (migration 00011). After the exchange the signed-in person's stable id becomes the credential subject (D-26: one Google or Meta sign-in reaches many accounts). Accounts are discovered through the reader before anything is saved; a sign-in that reaches no accounts is refused; managers and businesses are parents, not accounts. Each discovered account gets `read.reporting` verified. `ProviderToken` refreshes a credential within five minutes of expiry (Google refresh token; Meta re-exchanges the long-lived token) and reseals it. Google's OAuth client and Meta's app are configuration (`ADX_GOOGLE_OAUTH_*`, `ADX_META_APP_*`); without them the connect buttons answer "not configured".

### BD-23. Refresh runs re-read a trailing window and record freshness per account
2026-10-17. `reporting.Execute` reads campaigns, then daily metrics page by page, then conversion actions, with progress after every page so a retry resumes at the cursor. Rows are replaced on re-read because both platforms revise recent days (Google: late conversions and invalid-traffic removals; Meta: up to 28 days). A range that includes the account's today is finished as `partial`, observed through yesterday. `account_freshness` holds last success, observed-through day, partial flag and last safe error; the connections screen shows it. A daily periodic job refreshes every connected ad account for the last `ADX_ADS_REFRESH_TRAILING_DAYS` (default 30). Metric rows for campaigns the campaign read did not return are counted (`unknownCampaignRows`) and skipped, never invented.

### BD-24. Metrics are deterministic, versioned and null-safe
2026-10-17. `reporting.Analytics` computes every figure in Go from stored rows under `FormulaVersion` v1: spend (minor units, null when currencies mix), impressions, clicks, Google conversions and Meta `lead` actions in separate fields that are never summed, GoHighLevel leads by the Dubai day they arrived, qualified leads and Done-Deals from the funnel stage (null without a funnel), cost per lead and per qualified lead as spend ÷ count (undefined on a zero count, null without spend), revenue and ROAS null until Done-Deal values arrive as a field (#35). Comparisons (previous period of the same length, same dates a year earlier, or custom) give absolute and relative change per metric; a zero or missing base makes the relative change undefined, never infinite. The summary is built from the same buckets as the rows so they reconcile by construction. Days with no rows for an account in scope are listed as missing, and the comparison period's gaps raise `comparison_incomplete`. Accounts outside Asia/Dubai are shown by calendar date with an `account_timezone_differs` warning because the platforms give no hourly data to re-cut a day.

### BD-25. Leads reach campaigns through their attribution's campaign reference; campaigns reach projects through their names
2026-10-17. A lead counts for a campaign when its first attribution's `matched_campaign_ref` equals the campaign's platform id or its name (case-insensitive). Click ids alone can't name a campaign without a click-to-campaign lookup, so they prove the platform but not the campaign until B3.3's measurement work. A campaign's project is read from its name: the project's words as whole words, a leading "The" optional (Reportage writes "Wadi" and "The Wadi"). A campaign naming two projects or none stays unassigned and is listed by `POST /api/analytics/assign-projects`, which also runs at the end of every refresh.

### BD-26. Reports are occurrences with frozen results; delivery needs recipients and a transport
2026-10-17. `core/reports` keys each report by schedule and period start (or by period and parameters for ad hoc requests), so a schedule tick that runs twice reuses the stored report and never sends it again. The stored `result` is the `ReportingResult` snapshot with its formula version and freshness, shown again exactly. Daily reports cover yesterday in Dubai, weekly the last Monday to Sunday, monthly the previous month, quarterly the previous quarter. Delivery is `unconfigured` with no recipients (the owner's input) or no SMTP host; `pending` → `sent`/`failed` is a conditional update so two deliverers can't both send. The email body is plain text rendered from the deterministic figures; "not available" and "no leads" are written out. A periodic job runs due schedules every six hours; the occurrence keys make extra ticks no-ops.

### BD-27. Anomaly checks ship with no thresholds and never act
2026-10-17. `core/alerts` keeps a versioned per-workspace configuration for three checks: cost spike (percent above the campaign's average over the days it ran in a baseline window, with a minimum spend and a minimum of three baseline days so a campaign that ran once has no baseline), zero leads (spend above a minimum on a day whose figures are complete with no lead in GoHighLevel), stale ingestion (no complete figures newer than a maximum age). With no configuration no check runs; with a null threshold that check skips. A day whose figures are not complete raises nothing but a skip note and, if configured, a stale-ingestion alert: no figures is not zero leads. The same check firing for the same subject while its alert is open adds an occurrence, and the same day is never counted twice. Every alert message ends "No change was made": alerts observe; proposals (B3.4) act.

### BD-28. The assistant reads through an allowlist, and every number is checked against tool results
2026-10-17. `core/assistant` owns the conversation loop: the model proposes tool calls, deterministic code runs only the four allowlisted read tools (`get_analytics`, `get_lead_coverage`, `list_alerts`, `get_connections`) with the asking person's actor, validates each input against the tool's schema before running it, and feeds results back as data. A tool name outside the allowlist, whoever or whatever asked for it (the person, the model, or text inside a tool result), is refused with a message saying the assistant cannot change budgets, campaigns, leads or settings; there is no write tool and no path to the policy evaluator or proposals. The final answer is scanned for numbers; any number that appears in no tool result (allowing thousands separators, minor-unit rounding, years and dates) is flagged `ungrounded_numbers`, and an answer with numbers but no tool call is flagged `no_tool_evidence`. The screen shows these as unverified. Each turn stores the model, tokens, cost (null unless prices are configured), evidence refs and warnings. Model failures become an `unavailable` turn with a safe message and a `retryable` flag; refusals become `refused`; neither is replayed to the model as history. Limits: 6 tool rounds per question, 4000 output tokens per call, a daily token budget per workspace (default 2M).

### BD-29. Claude through the official Go SDK is the one production adapter
2026-10-17. `providers/anthropic` implements `assistant.Model` with the Anthropic Go SDK (Messages API, tools as `tool_use`/`tool_result` blocks, thinking left at the model's adaptive default, `output_config.effort` from configuration). Default model `claude-opus-5-5`, effort `medium`, 120 s timeout, two retries. API errors map to retryable (429, 5xx, 529, timeouts, network) or not (401/403: key; 400/404: configuration) with safe messages; the provider's text and ids stay in the cause. The key, model name, base URL, effort, timeout, budget and prices are `ADX_MODEL_*` settings; none exists on this machine, so live calls are unverified. Server-side refusal fallbacks (`fallbacks`) are not enabled: a refusal is recorded as a refused turn, which is the conservative choice for a read-only assistant; this is a configuration to revisit with the owner.

### BD-30. Creative pilot: SVG template renderer, review bound to output checksum, text-only editor
2026-10-17. `core/creative` ports the creative source's layout rules as `assets.Layout` (logo top centre under the 9:16 keep-clear zone, text block above the bottom zone, Arabic right-aligned and right-to-left with Arabic fonts) and its house-rule checks as `assets.CheckDocument`; errors block rendering. `providers/svgrender` renders a document to a deterministic SVG with embedded images, labelled `template`; the SVG is the immutable output whose checksum review binds to. Submitting is putting the version on the review board; approval records the output checksum and produces a `ContentAcceptance` that never grants delivery. Editing a draft is in place; editing anything rendered, in review, approved or sent back creates the next draft version with no approval. Export streams the stored bytes after verifying the checksum. Files are served with a sandboxing content security policy and `nosniff` because SVG can carry script. The web editor changes text and vertical position; the canvas drag editor and pixel export are B3.1.

- **Why.** The pilot needs the journey end to end with the invariants the spec names (approval invalidated by edits, board ≠ delivery, no fixtures). An SVG renderer is deterministic, testable and needs no font files on the server; the browser rasterizes it.
- **Cost.** Visual fidelity of long Arabic copy is unverified until someone looks at it in a browser (recorded in `pilot-gate.md`). Rasterized exports for ad platforms wait for B3.1.

### BD-31. Brand facts: classified by provenance, conflicts block approval, scope is the brand
2026-10-17. `core/brands` owns brands (ours or competitor), the project-to-brand assignment (a nullable `brand_id` on `projects`, existing ids kept), offers as immutable versions with effective dates, and facts as single assertions with effective dates. A fact's class says where it came from: `needs_review` (a person entered it), `inferred` (ADX derived it from a named evidence row; only the job actor may write one), `approved` (an admin signed it off), `retired` (history, with the reason). Only approved facts become claims. Two current facts for the same key and scope with different values and overlapping dates open a conflict; a fact in an open conflict cannot be approved, and resolving a conflict retires the loser without approving the winner. A project fact beside a brand-wide fact is not a conflict; the project's wins for that project. Offers are approved as "true as written" and the approval never carries to a new version; revising an offer lists the creatives built on older versions as affected and changes none of them. A creative takes an offer only from its project's brand, and a render uploaded for a project only on that project. The brand kit route moved to `/api/projects/{id}/brand-kit` to free `/api/brands` for brands.

- **Why.** The spec's gate (contradictory sources need review; a changed offer names what it affects; another brand's evidence never reaches a draft) is about provenance and scope, so both are explicit columns, not conventions. Source precedence is an unresolved business rule, so nothing resolves a conflict but an admin.
- **Cost.** Every fact is a row and every claim lookup is a query per key; fine at Reportage's scale. Facts have no numeric comparison (a "from" price of 1,200,000 and 1,200,001 conflict), which is the point.

### BD-32. Research: every retrieval is an item, the network boundary is one package, estimates stay labelled
2026-10-17. `core/research` holds briefs (a question about a brand or project) made of items: pages to read, competitor pages, keyword seeds, our own account audit. A run works each pending item once and records either findings with an evidence row or a safe failure on the item; a source that didn't answer produces no finding. Findings carry a scope (`account` for our own figures, `public` for anything off the web) and the evidence class (`observed`, `estimated`); the two scopes are listed apart and never summed. Keyword Planner volumes and bids are `estimated` and say so in their payload. A competitor typed by name resolves only on an exact match with a competitor brand; a partial match or none leaves the item in `needs_identity` with the candidates listed, nothing is fetched for it, and no brand is created from a typed name. `providers/webfetch` is the only code that fetches a URL someone typed: http/https on the standard ports, no credentials in the URL, the host resolved and every answer checked against loopback, private, link-local, multicast, carrier-NAT, documentation and IPv4-mapped ranges, the connection pinned to the approved address, redirects re-checked, bodies capped and reduced to text. Nothing in a page is followed or interpreted: links, meta refreshes and instructions are text in an evidence row. A refusal is final; an unavailable source is retried on the next run. Briefs refuse an address the fetcher would refuse so the person sees it at once.

- **Why.** The spec's gates are about not inventing (statistics, identities) and not being steered (private network, injected text). Making every retrieval a row with a state makes gaps visible instead of filled, and keeping fetching in one package makes the boundary auditable.
- **Cost.** Competitor ad observation has no API route for the UAE (Meta's Ad Library API returns only political ads outside the EU), so competitor research is pages a person names. Keyword ideas need a Google Ads account and ride on its OAuth; the request shape is unverified live (api-notes). The web extractor is regex-based, good enough for text, not a DOM.

### BD-33. Plans: immutable validated versions, acceptance without spend, exact template gated
2026-10-17. `core/plans` stores a plan's versions as immutable JSON documents typed in Go: allocations by channel from a fixed catalog (`Channels`, each marked supported or manual), asset, page and measurement requirements, test hypotheses, observation windows, evidence references and assumptions classed assumption, unavailable or cited. Saving validates in code and stores the result with the version; the state is `ready` with no blocking issue, `needs_input` when a required input is missing, `invalid` when what is there contradicts itself (allocations not totalling the budget, an unknown channel, a reference to evidence that doesn't exist, a dangling citation, a project of another brand). An admin accepts a `ready` version; acceptance is terminal for that version, `GrantsSpend` is always false and `ExecutionAuthorization` is always null in this domain. Any edit is the next version with no acceptance. Export and import go through `TemplateAdapter`s: JSON (whole document, lossless) and CSV (allocation table with the plan header) always; the exact Excel adapter is absent, `Templates()` says why and what unblocks it, and `format=xlsx` is a 409 with that reason, never a 500 or a generic file dressed as the template.

- **Why.** The spec's gates for this task are arithmetic that can't drift, acceptance that isn't permission, and an exact-template capability that is reported blocked rather than faked.
- **Cost.** The CSV form drops requirements and assumptions, which is stated in its test and in the UI's wording. The channel catalog is ADX's, not Reportage's agency's; unsupported channels can be planned and are flagged manual.

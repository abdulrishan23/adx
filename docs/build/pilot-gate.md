# Phase 1 pilot gate (B1.6)

Taken on 2026-10-17 at the end of phase 1. The local journey is implemented and tested; the connected pilot needs inputs that don't exist on this machine. Nothing here claims verification it didn't do.

## The journey, locally

| Step | State | Evidence |
|---|---|---|
| Sign in with a verified session, scoped to the Reportage workspace | implemented locally | `core/identity`, `api/auth_test.go` |
| Connect GoHighLevel (private token) and import leads | implemented locally | `core/leads`, `providers/gohighlevel/contacts_test.go` |
| Connect Google Ads and Meta by OAuth, discover accounts, refresh campaigns and daily figures | implemented locally | `core/reporting/ingest_test.go`, `providers/googleads`, `providers/meta`, `core/connections` ads test |
| Deterministic analytics with coverage, freshness and comparisons | implemented locally | `core/reporting/metrics_test.go` |
| Scheduled reports and anomaly alerts, unconfigured by default | implemented locally | `core/reports`, `core/alerts` |
| Ask ADX, read-only, grounded | implemented locally | `core/assistant`, `providers/anthropic` |
| Upload a render, lay out copy in 9:16 / 4:5 / 1:1 / 1.91:1 in English or Arabic, render, review board, approve, export | implemented locally | `core/creative`, `core/assets/layout_test.go`, `providers/svgrender` |

## What the connected pilot still needs

| Input | Who | Blocks |
|---|---|---|
| authentik `adx-auth` provider client id/secret, redirect URI, groups scope mapping | WokeWorks (D-24) | live sign-in |
| GoHighLevel private integration token for the ADX Test location, saved with `ADX_CONNECTION_KEY_FILE` | Reportage GHL admin / Abdul | live lead import; confirms three unconfirmed API details (BD-20) |
| Which GHL field holds the lead status, and the status-to-stage table | Abdul | stage classification, qualified leads, deals |
| Google Cloud OAuth client on a project with Google Ads API access; the Head of Marketing & AI's sign-in | Reportage / WokeWorks | live Google reads, same-day reconciliation against the Google Ads UI |
| Meta Business app id/secret; the connecting person given a role on the app (D-22) | WokeWorks / Reportage | live Meta reads and reconciliation |
| Hosting approval in writing naming the server location (D-21, #35) | Reportage | moving any real lead data |
| SMTP host and a shared sender (D-24) and report recipients | WokeWorks / Abdul | report delivery |
| Anomaly thresholds | Abdul / CMO | alerts firing |
| Model API key and data-use approval | Abdul | live Ask ADX answers |

## Not done in phase 1

- No browser session was run against the dev server in this build; the web pages pass typecheck and lint but were not inspected visually. In particular, long Arabic copy in the four formats has not been inspected visually (acceptance item of B1.6): the layout and the RTL rendering are covered by unit tests on the document and the SVG, which is not the same thing.
- The canvas drag editor from the creative source is not ported; the pilot editor changes text and vertical position only (B3.1).
- Pixel export (PNG/JPG) is not implemented; the output is SVG, rasterized by the browser when viewed. A server-side rasterizer or a browser export is B3.1.
- Live calling, messaging and any ad-account write are not part of phase 1 by design.

# Phase 2 gate (B2.1–B2.5)

Taken on 2026-10-17. The journey the spec names, "brand evidence/conflict → research → typed plan → validation → correction → immutable accepted version", runs locally end to end in tests:

| Step | Where it is proven |
|---|---|
| Two price sources disagree, neither becomes a claim until an admin resolves, approval then makes the claim | `core/brands/brands_test.go` |
| A research brief reads pages and keyword estimates, refuses private addresses, leaves an ambiguous competitor unresolved | `core/research/research_test.go` |
| A typed plan whose allocations must total the budget; a model-drafted version pinned to the frame and cleaned of uncited figures | `core/plans/plans_test.go`, `core/plans/generate_test.go` |
| Validation blocks, a correction becomes the next version, an admin accepts a ready version; acceptance grants no spend | `core/plans/plans_test.go` |
| An accepted version is immutable; the diff names it as superseded in content | `core/plans/generate_test.go` (TestDiffAndRevisionImpact) |
| Monthly comparison with hypotheses that cite evidence and stay hypotheses; reviews carry forward | `core/research/schedules_test.go` |

Externally blocked, as the spec allows: the exact Excel template adapter (no file), live Keyword Planner and web fetches (no Google OAuth client), live plan drafting (no model API key). Not done: browser inspection of the five new pages (`/brands`, `/research`, `/plans` and the creative and offer additions).

# Phase 3 gate (B3.1–B3.6)

Taken on 2026-10-17. The spec's journey, "accepted plan → exact reviewed assets/page → test measurement → authorized proposal → budget reservation → fake provider deployment → simulated timeout → reconcile without duplicate creation", runs locally in tests:

| Step | Where it is proven |
|---|---|
| Approved creative bound to a plan version, claims checked against approved facts, render as a durable run | `core/creative/production_test.go` |
| Published, tested landing page with a scoped form; rollback names the old version | `core/pages/pages_test.go`, `api/pages_test.go` |
| One test submission → one non-counting test event; readiness joins plan, assets, pages, accounts and checks | `core/measurement/measurement_test.go` |
| Policy-checked proposal, approval bound to the target version, atomic budget reservation, outbox intent | `core/proposals/proposals_test.go` |
| Fake-provider dispatch, timeout after a real create reconciled by marker with no duplicate, partial stays partial | `core/execution/execution_test.go` |
| Restart mid-send recovered as uncertain, kill switch holds, compensation never claims spend back, delivery from impressions | `core/execution/recovery_test.go` |

The live provider gate is separate and was not run: no account is enabled for writes, `ADX_LIVE_WRITES` is unset, no test account exists on this machine and no written approval was given. The Google Ads and Meta write adapters are tested against fakes of the documented responses only.

## Phase 4 gate: lead intelligence, voice and CRM feedback

Taken on 2026-10-17. Every phase 4 task is implemented locally with its pass conditions as tests; nothing phase 4 adds reaches a customer, a CRM field or a phone without configuration and authorization that do not exist on this machine.

| Capability | Where it is proven | Live state |
|---|---|---|
| People and enquiries resolved conservatively, candidates for shared phones, reversible links and merges, class-kept enrichment | `core/crm/crm_test.go` | enrichment provider: none |
| Signals scored once per occurrence under versioned rules, unknown identity excluded, corrections with history | `core/signals/signals_test.go` | GoHighLevel webhook: no secret configured |
| Sales brief with a rule-based next step, field-owned CRM outbox (echo vs conflict vs ownership), drafts from approved facts, suppression at every send | `core/outreach/outreach_test.go`, `providers/gohighlevel/fields_test.go` | messaging adapter: none; field writer: off (`ADX_LIVE_WRITES`) |
| Calls gated on an authorized route, contactability checks, atomic reservation, unknown results reconciled, signed callbacks | `core/calls/calls_test.go`, `providers/twilio/twilio_test.go` | route: unchosen; `ADX_TELECOM_ROUTE` unset |
| Conversation bridge with interruption clearing, transcript evidence, trusted-only application, corrections, recording access and retention | `core/qualification/qualification_test.go`, `core/voice/audio_test.go`, `providers/elevenlabs/elevenlabs_test.go`, `providers/twilio/stream_test.go` | speech engine: unset |
| Operator speech as a previewed read-only transcript; evaluation against labels; predictive claims gated | `core/assistant/assistant_test.go`, `core/evaluation/evaluation_test.go` | no labelled calls |

What live use still needs, in the owner's hands: Reportage's written reversal of "not open to automated calls" and a chosen, authorized UAE telecom route; a messaging policy and provider; `ADX_WEBHOOK_SECRET` with a GoHighLevel webhook; `ADX_LIVE_WRITES=1` with `ghl.lead_fields.set` allowed for the location; ElevenLabs credentials with a `ulaw_8000` agent.

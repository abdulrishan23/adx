# ADX capability matrix

Written for B6.4 on 2026-10-17 from `docs/build/progress.json`, `docs/build/verification.md` and the code. Two columns matter: what is proven locally (real core, temporary SQLite databases, fake providers) and what live use still needs. Nothing below is production verified: no provider account, model key, telecom route or messaging policy has been supplied to this build. `implemented_local` means the task's pass conditions are tests that pass in `make test`; the blocker column names the exact input that would move it to live.

## Summary

- Tasks: 38; implemented locally: 38; pending: 0; production verified: 0.
- Every phase gate's integrated journey exists as a test: phase 0 to 5 in each domain's `_test.go` and the connected journeys in `server/journey/journey_test.go` and `server/cmd/adx/backup_test.go` (restore and recovery).
- The live blockers are listed per task below and summarised in "What live use needs".

## Phase 0: Foundations

| Task | Capability | Local state | Live blocker |
|---|---|---|---|
| B0.1 | Baseline, migration map and build ledger | implemented_local | creative source CODE_SNAPSHOT.md at the spec path: n/a: equivalent app found at ~/Desktop/creative-studio and inventoried |
| B0.2 | Deployment, persistence and recovery | implemented_local | live deployment: server access and Reportage's written approval of the hosting location |
| B0.3 | Real session identity and workspace scope | implemented_local | live sign-in: adx-auth provider client ID/secret, redirect URI registration and groups scope mapping from the authentik admin (WokeWorks) |
| B0.4 | Policy and provider capability contracts | implemented_local | any external write: owner sets caps (daily spend increase %, changes per day) and enables write accounts in the policy document |
| B0.5 | Evidence, runs and durable operation foundation | implemented_local | none needed |
| B0.6 | Reusable creative and voice integration boundaries | implemented_local | live calling: owner chooses and authorizes a UAE telecom route and supplies the calling/recording/contact policy; Reportage's written reversal of 'no automated calls'; AI image generation: owner chooses a provider; otherwise template layout only, labelled as such |

## Phase 1: Marketing core

| Task | Capability | Local state | Live blocker |
|---|---|---|---|
| B1.1 | Google and Meta real read adapters | implemented_local | live Google Ads reads: a Google Cloud project with Google Ads API access (Basic or Standard) and an OAuth client; ADX_GOOGLE_OAUTH_CLIENT_ID/SECRET; the Head of Marketing & AI signs in once (D-20); manager id if access goes through the WokeWorks MCC; live Meta reads: a Meta Business app with ADX_META_APP_ID/SECRET; the connecting person given a role on the app (D-22 Standard Access) |
| B1.2 | GHL mapping, identity evidence and funnel configuration | implemented_local | live lead import: Private Integration Token for the ADX Test location saved through /api/connections/crm/connect with ADX_CONNECTION_KEY_FILE set; stage classification: owner (Abdul) saves the funnel mapping: which GHL field holds the status, and the status-to-stage table (Hisham's 2026-10-02 table is a proposal) |
| B1.3 | Deterministic metrics and comparison engine | implemented_local | revenue and ROAS: Akarati sends Done-Deal value and date into GoHighLevel as real fields (#35) |
| B1.4 | Reports, schedules and anomaly observations | implemented_local | report delivery: owner names recipients per schedule; SMTP host and a shared sender address (D-24); anomaly checks firing: owner sets thresholds: cost spike percent and minimum spend, zero-lead minimum spend, stale-ingestion maximum age |
| B1.5 | Read-only grounded assistant | implemented_local | live assistant answers: ADX_MODEL_API_KEY (an Anthropic API key with budget) and the owner's data-use approval for sending aggregated figures to the model (D-25 open point) |
| B1.6 | Pilot creative integration and end-to-end validation | implemented_local | pilot inputs listed in docs/build/pilot-gate.md: the connected pilot |

## Phase 2: Brand, research and plans

| Task | Capability | Local state | Live blocker |
|---|---|---|---|
| B2.1 | Brand, offer and approved-fact knowledge | implemented_local | source precedence for price/inventory facts: nothing in code; conflicts are resolved by an admin by design until Abdul names a precedence |
| B2.2 | Sourced market, competitor, SEO and customer research | implemented_local | Google Cloud OAuth client with Google Ads API access: live keyword ideas |
| B2.3 | Structured plan schema and template adapter | implemented_local | Reportage's Excel media-plan template (.xlsx) and the marketing skill file: the exact-template adapter and its round-trip test |
| B2.4 | Plan generation, validation and revision impact | implemented_local | model API key and data-use approval: live plan drafting |
| B2.5 | Recurring research and plan evaluation | implemented_local | none needed |

## Phase 3: Assets, pages, measurement and controlled execution

| Task | Capability | Local state | Live blocker |
|---|---|---|---|
| B3.1 | Production asset/version and approval lifecycle | implemented_local | none needed |
| B3.2 | Landing page lifecycle and form path | implemented_local | which host each page family lives on and deploy access: an external publishing adapter |
| B3.3 | Measurement and launch-readiness verification | implemented_local | a connected Google Ads or Meta account: the account gate in readiness and live conversion-action checks |
| B3.4 | Proposal authority, budget reservation and outbox | implemented_local | approved budgets and write privileges per account (unresolved rule): any real proposal passing policy |
| B3.5 | Google/Meta controlled deployment and reconciliation | implemented_local | an account enabled for writes in policy, a test account, ADX_LIVE_WRITES=1 and written approval: the live write gate |
| B3.6 | Pause, compensation and launch validation | implemented_local | test account enabled for writes, ADX_LIVE_WRITES=1, written approval: the live launch validation journey |

## Phase 4: Lead intelligence, voice and CRM feedback

| Task | Capability | Local state | Live blocker |
|---|---|---|---|
| B4.1 | Person/enquiry resolution and enrichment | implemented_local | an enrichment provider with a contract, cost and permitted attributes: live enrichment runs |
| B4.2 | Observable signals and scoring | implemented_local | ADX_WEBHOOK_SECRET and a GoHighLevel webhook action pointing at /webhooks/ghl: live stage and message signals |
| B4.3 | Nurturing, objections and CRM feedback | implemented_local | Reportage's messaging policy (channels, hours, consent) and a chosen messaging provider: sending approved drafts; ADX_LIVE_WRITES=1, ghl.lead_fields.set allowed and write.lead_fields verified for the location: CRM field dispatch |
| B4.4 | UAE telecom integration and call lifecycle | implemented_local | a chosen UAE telecom route authorized in writing, and Reportage's written reversal of 'not open to automated calls': live calling; ADX_TELECOM_ROUTE=twilio with account SID and auth token: the reference adapter |
| B4.5 | Conversational qualification and corrected outcomes | implemented_local | ElevenLabs API key and an agent configured for ulaw_8000; the B4.4 route: live qualification calls |
| B4.6 | User voice interface and lead-loop evaluation | implemented_local | none needed |

## Phase 5: Native AI CRM and coordinated agents

| Task | Capability | Local state | Live blocker |
|---|---|---|---|
| B5.1 | Native CRM ownership and migration | implemented_local | none needed |
| B5.2 | Skill, agent and capability registry | implemented_local | none needed |
| B5.3 | Durable workflow graph and task orchestration | implemented_local | none needed |
| B5.4 | Supervisor, conflict arbitration and constrained autonomy | implemented_local | none needed |
| B5.5 | Learning records and organizational memory | implemented_local | none needed |

## Phase 6: Multi-client, channels, reliability and handoff

| Task | Capability | Local state | Live blocker |
|---|---|---|---|
| B6.1 | Full client isolation and industry configuration | implemented_local | none needed |
| B6.2 | Selected channel extensions and native capability review | implemented_local | messaging policy (who may send what, on which channel, with what consent) and ADX_WHATSAPP_*/ADX_SMTP_* with ADX_LIVE_WRITES=1: a live send through the new adapters |
| B6.3 | Reliability, cost and model release evaluation | implemented_local | backup retention period from Reportage: setting ADX_BACKUP_RETENTION_DAYS above 0 |
| B6.4 | Final seven-phase capability verification and handoff | implemented_local | real business users and a connected environment: the operator-effort review and production verification |

## Connectors

`core/connections.Catalog` is the source of truth and `/api/connections/capabilities` shows it with what the server has wired. In short:

| Connector | Reads (local) | Writes (local, under policy and `ADX_LIVE_WRITES`) | Never |
|---|---|---|---|
| GoHighLevel | contacts, custom fields, pipeline stages; webhook events with `ADX_WEBHOOK_SECRET` | ADX-owned custom fields through the outbox | conversations, pipeline writes |
| Google Ads | accounts, campaigns, daily figures, conversion actions, keyword ideas | paused campaign, budget raise/lower, pause | audiences, conversions, exclusions, publishing |
| Meta | accounts, campaigns, daily figures | paused campaign, budget raise/lower, pause | lead forms, pixel, organic publishing |
| WhatsApp Cloud | nothing (no inbound) | approved draft as plain text in the 24-hour window | templates |
| Email (SMTP) | nothing | approved draft; staff reports and links | mailbox reading |
| Telephony (Twilio reference) | status callbacks | qualification call on an authorized route | SMS |
| ElevenLabs | agent audio | conversational agent over the call stream | |
| Anthropic | | plans, research, drafts, readings, assistant, workflow steps, behind one metered daily cap | |

## What live use needs

| Input | Who | Unblocks |
|---|---|---|
| Hosting approval naming the server location, server access | Reportage, WokeWorks | deployment |
| authentik `adx-auth` client id/secret, redirect URI, groups scope | WokeWorks | sign-in with two memberships, workspace switching in the browser |
| GoHighLevel private integration token for the test location; the status field and stage table | Reportage GHL admin, Abdul | lead import, stage classification, CRM field dispatch |
| Google Cloud OAuth client with Google Ads API access; Meta Business app id/secret | Reportage, WokeWorks | live reads, reconciliation, keyword ideas, conversion-action checks |
| Caps and write privileges per account; a test account; written approval; `ADX_LIVE_WRITES=1` | Abdul, CMO | any external write, the launch journey |
| Model API key and data-use approval | Abdul | assistant answers, plan drafting, call readings, workflow runs, release evaluation |
| SMTP host and shared sender; report recipients; anomaly thresholds | WokeWorks, Abdul | report delivery, alerts, email outreach |
| Messaging policy (channels, hours, consent) and WhatsApp Cloud credentials | Reportage | sending approved drafts |
| A UAE telecom route authorized in writing and the written reversal of "not open to automated calls"; Twilio or another adapter's credentials; ElevenLabs key and agent | Reportage | qualification calls |
| Reportage's Excel media-plan template | Reportage | the exact-template adapter |
| Backup retention period | Reportage | `ADX_BACKUP_RETENTION_DAYS` above 0 |
| Source precedence for price and inventory facts; Done-Deal value in GoHighLevel | Abdul, Akarati | automatic fact resolution; revenue and ROAS |

Each of these is also recorded on its task in `docs/build/progress.json` with the state it would move to.

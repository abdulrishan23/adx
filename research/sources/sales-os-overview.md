# Sales OS: Feature Overview

Sales OS (repo `WokeWorks/dead-leads-stafford`) is a sales desk for working a company's old and cold CRM leads. It pulls leads from Zoho, finds each lead's LinkedIn profile, works out which products the lead qualifies for, scores how ready they are to buy from what they do, and gives reps a written brief, a ranked worklist, WhatsApp and email outreach, phone calls by a person or an AI caller, and an AI assistant. The first client is Stafford Global, a UK education consultancy that enrols working professionals into online UK university degrees, mostly MBAs. Most product choices lean toward that case. A lead is a person who once enquired about a degree and went cold, and "eligibility" means which courses they could get into.

It started as a dead-lead re-engagement tool. That is why the app title ("Dead Leads Stafford"), the README and many code comments still say "dead leads". It has since grown into a general sales desk. The app is served under the `/sales-os` base path.

This document lists every feature in the code, what it is for, and roughly how it works. It is written from the working tree at commit `caa0d25` (2026-09-24) on branch `feat/outreach-return-tokens`. That branch is three commits ahead of `main` (`095ef05`, 2026-09-05). The three commits add WhatsApp link tracking, the `outreach_return` signal and one tracked WhatsApp template. Everything else described here is on `main`. The production environment file is not visible. "Built" below means the code is merged and runs when its credentials are present. "Off by default" means a setting has to be switched on first.

## Stack

| Layer | Choice |
|---|---|
| App | Next.js 15 (App Router) and React 19 in TypeScript. One app holds the pages, the API routes and the background loops |
| Database | SQLite through better-sqlite3. `data/app.db` holds all operational data, including auth. `data/courses.db` is the course catalog, shipped in git and copied into `app.db` on first boot. The schema is created and migrated in code by `lib/db.ts` on first connection. There is no migration tool |
| Background jobs | In-process `setInterval` loops started from `instrumentation.ts`. Enrichment drain every 30 seconds, failed-event recovery every hour, the sweep (wakes every 10 minutes and runs at four set times in the Dubai day), recording retention once a day. No queue and no worker process |
| Custom server | `server.ts` wraps Next with a plain Node HTTP server. It is needed for the voice WebSocket bridge, the live call monitor and the inbound email listener. It runs under `pnpm dev:voice` and `pnpm start:voice` |
| AI | Vercel AI SDK against an OpenAI-compatible gateway. OpenCode Zen (default chat model `glm-5.1`) or OpenAI, switchable from an admin page. There are two tiers, a chat model for batch work and an agent model for the assistant |
| Auth | better-auth with email and password and its admin plugin. Sign-up is disabled, so admins create accounts |
| Messaging and voice | Twilio for WhatsApp Business and for phone calls. ElevenLabs Conversational AI for the AI caller. Deepgram (default) or ElevenLabs Scribe for call transcription. A Google Workspace mailbox over SMTP and IMAP for email |
| Data vendors | Zoho CRM, read only. ContactOut and Reverse Contact to find LinkedIn URLs from an email. Apify actors for LinkedIn name search and profile scraping. Clay is still wired in but switched off |
| Frontend | Tailwind 3, Radix, motion and GSAP, a WebGL "liquid glass" visual layer, React Flow for the automation canvas, Tiptap for the email editor |
| Hosting | PM2 on a VPS. `deploy.sh` connects over SSH, pulls, builds and restarts |
| Tests | 49 Vitest files over pure logic (scoring helpers, time maths, TwiML, tracking tokens, parsing). `scripts/verify-*.ts` run integration checks by hand. No end-to-end tests |

The README is mostly accurate. `voice.md` is the pre-build design document for voice and says nothing is built yet. Voice is in fact built, with some differences listed under "Where the repo's own docs differ". Many code comments point at design documents (`PLAN.md`, `docs/voice-*.md`, `docs/mail/*`) that are not in the repository. The `docs/` folder is gitignored, and only four files from it exist locally.

## Core concepts

### Leads and the book

A lead is one person, keyed by email (unique). Leads that came from Zoho also carry `zoho_id`, which is the stable join key because an email can change. A lead row holds 18 CRM columns copied from Zoho or a CSV, the raw Zoho record as JSON, the scraped LinkedIn profile, the eligibility result, the AI brief, the scores and segment, the CRM-style pipeline stage, the outreach status, the assigned rep, and suppression flags.

"The book" is the set of leads a person works. A rep's book is the leads assigned to them. A manager's book is every lead. Suppressed leads are hidden from every worklist but stay in the database with their history.

At Stafford a "dead lead" is a Zoho lead with status `Dead` whose dead reason is not one of five excluded reasons (job seeker, high school student, inadequate English, joined a competitor, other). Only those leads enter through the pull. Leads already in the app always sync.

### Events, the identity bridge and signals

An event is a raw thing that happened to a lead, stored in the `events` table. Sources are website tracking through Google Tag Manager (`gtm`), Zoho (`zoho`), WhatsApp through Twilio (`twilio`), email (`mail`, and `unipile` on old rows), calls (`voice`) and Clay (`clay`).

Website events arrive anonymous. Each carries a browser cookie id (`wokeworks_uid`) and a session id. The identity bridge binds a cookie to a lead when a form submit carries an email that matches a lead. On the branch it also binds when the visitor lands from a tracked link we sent. Binding copies the cookie's earlier anonymous history onto the lead and runs it through detection, so patterns that were invisible while anonymous can fire. A cookie binds to one lead only, and the first binding wins.

A signal is a conclusion drawn from events, from the clock, or from a logged conversation. "Compared three courses in one visit" is a signal. Each signal type has one detector that subscribes to the event types that can satisfy it, and an occurrence key (a session, a day, an episode, an event id) so each real instance fires once. A database unique index backs this up. Each signal type also has an ordered action chain: recompute the score, regenerate the brief, draft outreach, suppress the lead, overlay new career facts, re-classify eligibility, or re-scrape the profile. A failed action is logged and the chain carries on.

### Scores, segments and tiers

There are three numbers. The arithmetic is deterministic and config-driven, and the model never sets a number. One input, the eligibility fit label, does come from the model.

Readiness (0 to 100, baseline 10) says how ready a lead is to act now. Each live signal adds base points from `lib/scoring/scoring-config.json`. Points decay by a per-type half-life, repeats of the same type count less, and signals are grouped into ten themes. Each theme has a ceiling and a diminishing-returns curve, so piling up one kind of intent stops paying. Negative signals sit in their own theme and subtract.

Eligibility strength (0 to 1) is the fit of the lead's best recommended course: strong 1.0, moderate 0.65, weak 0.35, and 0 when nothing fits or the lead is disqualified. The fit label comes from the AI eligibility classifier.

The combined score is readiness times eligibility strength, rounded. It is the ranking key. Because it multiplies, a lead with no eligible course always scores 0, however active they are.

The segment is the combined score cut into four bands: HOT at 55 or more, WARM at 30, NURTURE at 12, DEAD below that. The UI calls segments "tiers" and shows a fifth state, "Unscored", for leads never scored. A tier meter of five dots shows heat on cards and rows.

`score_history` is an append-only log of meaningful score changes, each with a frozen breakdown. Pure decay drift is not logged unless it crosses a segment boundary.

### Pipeline stage and outreach status

The pipeline stage is the CRM-style funnel position. It has 26 values copied from Stafford's Zoho `Lead_Status` picklist, from "New" through "Applied", "Conditional Offer" and "Paid". The lead page groups them as Prospecting, Nurturing, Application and Post-offer. Stage changes are recorded locally and never written back to Zoho.

The outreach status is where the conversation stands: `uncontacted`, `intro_sent`, `replied` or `window_expired`. The last one means the lead's WhatsApp 24-hour reply window has closed.

### Batches and enrichment jobs

A batch is any ingestion operation: a CSV upload, a Zoho sync tick, a manual Zoho pull, or a backfill. An enrichment run is also stored as a batch so it gets the same detail view. `batch_leads` records permanently which leads each batch touched.

An enrichment job is one attempt to find and scrape one lead's LinkedIn profile. It walks stages from `queued` to `done`, with branches for a name search, human review, and failure. The lead carries a copy of its latest job's stage for fast filtering.

### Interactions, tasks, notes and history

An interaction is a logged contact with a lead, stored in `verifications`. It has a channel (call, WhatsApp, email, in person), a free-text note, and an outcome from seven values: interested and needs info, requested a callback, not interested right now, affordability concern, enrolled elsewhere, explicitly refused, confirmed eligibility. Each outcome has consequences. It may create a follow-up task, schedule a re-entry date, raise a positive or negative signal, or suppress the lead. Calls produce the same outcomes through their AI summary.

Tasks are follow-ups with an assignee and a due date. Notes are free text with an author. `lead_history` is the audit log for stage changes, assignments, logged interactions and restores.

### Suppression and re-entry

A suppressed lead is out of the running on every channel. Two signals suppress: `opted_out` and `joined_competitor`. Both come from Zoho (an opt-out flag, an "Unsubscribed" status, a competitor dead reason) or from a rep's logged outcome. Every send path, the bot caller and auto-drafting check suppression at the moment of action.

The re-entry queue holds a date on which a lead should be looked at again. The eligibility classifier sets one when a lead would qualify within 24 months of normal career progress. A logged "not interested" or "affordability concern" sets one six months out. On the due date a daily job re-runs eligibility, score and brief, and creates a "Revisit" task for the assigned rep.

### Proposals and actors

A proposal (`pending_actions`) is something the system or the assistant wants to do that needs a human click: send a WhatsApp, send an email, spend on enrichment, or settle a LinkedIn review. It stores the exact tool input, so approving replays exactly what the rep saw. It lands in the queue of the lead's assigned rep and expires after 24 hours.

Every write goes through an action function in `lib/actions/`, and each action checks permissions itself. An actor is a user, an agent (the assistant acting for a user, with that user's role and never more), or the system (cron and pipelines). Agent writes are audited as `agent:<email>`.

### Threads

The word means two things in the code. An assistant thread is one conversation with the AI assistant. It is stored server-side, private to the person who started it even from managers, and deleted after 7 idle days unless pinned. An outreach thread is a lead's message history across WhatsApp and email. `inbox_reads` records when each rep last opened it, which drives unread badges.

### Roles

There are three roles: executive (a rep), manager and admin. Each includes the one below. Any signed-in user without a role counts as a rep. A rep reaches only leads assigned to them. A manager reaches every lead, assigns leads and launches AI calls. An admin also sees ingestion (Inflow) and the settings pages.

### The sweep

The sweep is the wall clock. It runs at four times in the Dubai day, by default 07:30, 11:30, 15:00 and 18:30, and the first run of the day also runs the daily jobs. The jobs run in this order: Zoho incremental sync, the enrichment reaper, WhatsApp window expiry, went quiet (daily), repeated ignores (daily), the re-entry queue (daily), pruning old assistant threads (daily), and a score recompute so decay lands on quiet leads. WhatsApp window expiry also runs on every 10-minute wake.

### Where AI is used

The scores, signals, ranking and action centre order are deterministic code. A model is called in these places:

- Eligibility classification. It picks courses from the catalog, with fit, entry track and fee tier. Fees are then filled from the catalog by code.
- The rep brief, regenerated after scoring and on most signals.
- Auto-drafted outreach on strong signals.
- Message suggestions in the outreach inbox (first message, 24 and 48 hour follow-ups, reply, objection analysis).
- Ranking WhatsApp templates for a lead, with a rule-based fallback.
- Call summaries, which give a summary, an outcome, key points, the objection, whether a human was asked for, a confidence level, and how firm a refusal was.
- The AI caller, which is an ElevenLabs agent with its own model.
- The assistant, using the agent model and tools.
- Speech to text for call recordings (Deepgram or ElevenLabs) and for dictation into the assistant (Deepgram).

## Feature summary

| Feature | How it runs | Uses AI | Status |
|---|---|---|---|
| Zoho sync, pull and backfill | Every sweep tick, plus manual pull and Bulk Read backfill | No | Built, needs Zoho credentials. Read only |
| CSV upload | Admin upload | No | Built |
| LinkedIn enrichment | 30-second drain loop, vendor webhooks, reaper on the sweep | No | Built. Provider waterfall on, Clay off |
| Eligibility classification | After enrichment, on career signals, on demand, on re-entry | Yes | Built |
| Readiness score and segments | On every signal and every sweep tick | No | Built |
| Website signals | GTM webhook into the dispatcher | No | Built, needs the GTM container on the client site |
| Message and CRM signals | Twilio, IMAP and Zoho events into the dispatcher | No | Built |
| Career signals | Clay webhook | No | Built, no live source |
| Clock signals (went quiet, repeated ignores) | Daily sweep | No | Built |
| Rep brief | Background job after scoring and on most signals | Yes | Built |
| Auto-drafted outreach | Last step of a signal's chain, into the approval queue | Yes | Built |
| WhatsApp | Twilio WABA, 24-hour window, approved templates | Template ranking | Built, needs Twilio |
| Email | SMTP send, IMAP listener, open and click tracking | No | Built, off unless `MAIL_ENABLED=true` |
| WhatsApp link tracking and `outreach_return` | Short links and landing markers | No | Built on the branch, not on `main` |
| Outreach inbox | Page, polls for new messages | Suggestions | Built |
| Human calls | Browser softphone over Twilio, transcript and summary after | Summary | Built, needs voice numbers and Twilio voice keys |
| AI caller (bot calls) | Manager launches one lead at a time, ElevenLabs over a Twilio media stream | Yes | Built, needs the custom server, voice numbers and an ElevenLabs agent |
| Interactions and outcomes | Rep logs, or call summary | Calls only | Built |
| Tasks, notes, history | Rep and system actions | No | Built |
| Assignment | Round robin on ingest, manager reassign | No | Built, auto-assign on by default |
| Suppression and restore | Signals, manager restore | No | Built |
| Re-entry queue | Daily sweep | Re-classifies | Built |
| Action centre | One union query, banded | No | Built |
| Dashboard | Server-rendered headline, widgets, assistant | Assistant only | Built |
| Assistant (copilot) | Agent model with 33 tools | Yes | Built |
| Approval queue | Proposals, approve replays the tool | No | Built |
| Automation view and dials | Admin page over the live registries | No | Built |
| Admin settings | Seven admin pages | Eligibility test runs | Built |
| Inflow | Four admin pages | No | Built. `/inflow-next` is an unlinked preview |
| Demo console | Admin page, non-production only | Optional | Not product |

## Features

### Zoho sync, pull and backfill

Zoho is the source of leads. The connection uses a Zoho self-client refresh token and can point at production or the sandbox (`ZOHO_ENV`). There are three doors in.

The incremental sync runs as the first job of every sweep tick and from a "Pull now" button. It asks Zoho for leads modified since a cursor, reaching back two minutes because Zoho's search index lags. Leads already in the app always update. New leads only enter if they pass the dead-lead gate. A run that brings in or changes leads is recorded as a batch. The last status and error of every run are kept so the Inflow page can say a sync failed.

The full pull fetches every lead that currently passes the gate. It suits sandbox and pilot volumes. The backfill uses Zoho Bulk Read for production scale, one page per run, and creates no enrichment jobs on purpose, so a large import never spends money by itself.

The upsert matches by `zoho_id`, then by email, so a CSV lead is adopted rather than duplicated. On update it refreshes only the CRM columns and the raw record. It never touches the LinkedIn data, scores or brief. It compares the old and new record and emits events for changes it sees: `crm.opted_out`, `crm.joined_competitor` and `crm.status_changed`. Those feed the CRM signals. An opt-out lifted in Zoho un-suppresses the lead directly. A new lead gets its pipeline stage from its Zoho status, a round-robin rep if auto-assign is on, and an enrichment job if auto-enrich is on (off by default).

Nothing is written back to Zoho. Stage changes, notes, interactions and assignments stay in Sales OS. Config for Zoho's push notifications exists (`ZOHO_NOTIFY_URL`, `ZOHO_WEBHOOK_TOKEN`), but no route receives them, so changes arrive on the sweep's four ticks a day or a manual pull.

### CSV upload

An admin can upload Zoho CSV exports. Rows are matched by email and split into batches of 50. The importer reads a fixed set of about 20 column labels and ignores the rest. A re-upload refreshes CRM columns only and leaves the pipeline stage, enrichment, scores and brief alone. Enrichment on import is a checkbox, on by default, and only applies to leads never enriched. After an import, any anonymous browsing cookies whose form email now matches a lead are bound and dispatched.

### LinkedIn enrichment

Enrichment turns an email into a LinkedIn profile, because Stafford's eligibility depends on job history, seniority and education. Every attempt is one row in `enrichment_jobs`. A drain loop moves jobs every 30 seconds, vendor webhooks advance them, and a reaper on the sweep closes jobs that went silent.

1. Queued jobs go to a provider waterfall: ContactOut first, then Reverse Contact. Both look up a LinkedIn URL from the email and bill only on a find. Up to 15 leads are looked up per tick. A quota or auth error leaves the job queued and stops calling that provider for the rest of the tick.
2. A miss goes to a fallback finder. It runs an Apify name search by name and country, scores the candidates against the CRM data, and then asks ContactOut for each candidate's emails. A candidate whose emails include the lead's own email is accepted. One whose emails differ is dropped. A single airtight match is accepted, ambiguous lists wait for a human, and nothing found ends the job.
3. Jobs with a URL are batched 20 at a time into an Apify profile scrape (about $4 per 1,000 profiles per the code). The Apify webhook writes the profile. A machine-chosen profile whose name or country contradicts the CRM is sent to human review instead of being scored.
4. A finished profile triggers eligibility, then the score, then the brief. A job that ends without a profile still runs that chain on CRM data alone.

Admins can pause the drain, which stops spending but keeps queued jobs and still accepts webhooks for work already paid for. Clay can be switched back on with an `app_config` flag and no deploy. A rep can paste a LinkedIn URL by hand, which skips the lookup. Reps can also re-enrich their own leads from the leads list. LinkedIn reviews are settled by admins in the action centre.

`docs/vendor-dossier.md` records the vendor research behind this. On a hand-picked 100-lead test set, ContactOut found 50 URLs and Reverse Contact 58, against Clay's historical 27.7% hit rate. The leads are mostly personal Gmail addresses.

### Eligibility classification

The classifier decides which Stafford courses a lead could join and how well each fits. It sends the model the whole course catalog (79 courses from 8 UK universities, with entry tracks, fees by region, modules and intakes), the lead's CRM fields, a seniority tier derived from the job title by keyword, and the raw LinkedIn history. The model returns up to four course recommendations with fit, entry track (standard or non-standard), fee tier and relevant modules. It can also mark the lead disqualified, suggest a redirect when the course they enquired about is a poor fit, give a framing note based on why they dropped off, and flag that they will qualify within a set number of months. Code fills fee amounts from the catalog, so a fee is never a model guess.

The steering rules sit in an admin-editable config with draft, publish and one-step rollback: routing rules by profile, a fee tier map by geography, a policy for qualifications already held, disqualifiers (C-suite, owners, the retired), plays for each dead reason, and worked examples. The admin page can preview the prompt and test a draft against sample leads before publishing.

### Readiness score and segments

See "Scores, segments and tiers" above. The score is recomputed on every signal, after eligibility, and on every sweep tick for leads that still hold a signal younger than its decay horizon. Leads past every horizon are skipped. The lead page shows the gauge, the three themes that moved the score most, each signal's live contribution, and the score history.

### Signals

The detector catalogue as built:

| Signal | Comes from | Fires when | Base points | Runs after |
|---|---|---|---|---|
| High-intent action | Website form submit | Any enquiry, apply or callback form, once per session | 35 | Score, brief, draft |
| Dormant returned | Any genuine activity | The lead acts after 30 or more days of silence | 18 | Score, brief, draft |
| Came back via outreach (branch) | Click redirect or landing marker | The lead lands on our site from a link we sent, once per day | 22 | Score, brief, draft |
| Comparison shopping | Course page views | 3 or more different courses in one visit | 20 | Score, brief, draft |
| Fees interest | Fee and pricing events | Fee views in 2 or more visits less than 7 days apart | 22 | Score, brief, draft |
| Financing focus | Payment plan and pricing button events | 2 or more in one visit | 20 | Score, brief, draft |
| Deep course engagement | Dwell, scroll, modules, assessment events | 3 or more in one visit | 15 | Score, brief, draft |
| Stuck on eligibility | Entry requirements page | 2 or more visits less than 14 days apart, with no form submit | 15 | Score, brief, draft |
| Site visit | Course, fee and eligibility page views | First such page in a visit | 2 | Score |
| WhatsApp reply | Twilio inbound | Once per day | 25 | Score, brief, draft |
| Email reply | IMAP inbound | Once per day | 25 | Score, brief, draft |
| Email click | Click redirect | Once per day, link preview fetchers ignored | 12 | Score, brief |
| WhatsApp read, email open | Read receipt, open pixel | Once per day | 4 | Score |
| Job change, promotion | Clay webhook | Each event | 15 | Overlay, re-classify, score, brief, re-scrape, draft |
| Contacted, not negative | Zoho status change | Status moves to Contacted, Warm, Hot, Follow Up, Contact in Future, Pre-Qualified or CEG-Interested, once per day | 15 | Score, brief |
| Contacted by bot, by human | A call or a logged interaction with a positive outcome | Per call or log | 15 bot, 32 human | Score, brief |
| Cooled off by bot, by human | Not interested or affordability outcome | Per call or log, scaled by how firm the refusal was | -8 bot, -20 human | Score, brief |
| Eligibility confirmed | Rep logs confirmed eligibility | Per log | 10 | Score, brief |
| Opted out, joined competitor | Zoho change or rep outcome | Per event | -35 | Suppress, score |
| Went quiet | Daily clock | An active lead goes 30 days without activity | -10 | Score |
| Repeated ignores | Daily clock | 3 touches since the last positive contact, the latest at least 3 days old | -8 | Score |

The thresholds, event lists and sweep times are admin-editable dials (see Automation view). Opens and read receipts never count as "activity" for dormancy, because mail prefetchers and previews fake them. Three older signal types (company growth, certification, financial unblocking) keep their rows but no longer score.

The website events expected from GTM are `view_course`, `view_fee`, `view_pricing`, `view_payment_plan`, `pricing_cta_click`, `view_eligibility`, `view_modules`, `view_assessment`, `course_time_30s`, `course_scroll_75` and `form_submit`. A code comment says PostHog page views will join later. That is not built.

Career signals are built but have no live source. The vendor dossier records that no one was ever enrolled in Clay's monitoring tables and the signals callback was never wired. `docs/signals-handoff.md` is an open brief to replace Clay with an in-house monitor.

### Rep brief

The brief is the one piece of rep-facing prose on a lead. The model reads the lead's identity, enquiry, career history, eligibility result, scores and their breakdown, up to 500 signals, up to 1,000 site events and selected Zoho fields (rep notes, the free-text dead reason, competitor, demographics). It reasons in a hidden scratchpad, then writes a headline, the situation, one recommended action, the course to lead with and why, two or three talking points, and one risk to watch. It is told to narrate the scores and never change them. A failed generation keeps the previous brief. The voice agent, the WhatsApp template ranker and auto-drafting all reuse the stored brief.

### Auto-drafted outreach

When a strong signal fires, the last step of its chain writes a message for the rep to approve. It runs on high-intent action, dormant returned, came back via outreach, comparison shopping, fees interest, financing focus, deep course engagement, stuck on eligibility, WhatsApp reply, email reply, job change and promotion. It skips unassigned and suppressed leads. It does not draft again within 24 hours of an earlier draft for the same lead, whatever happened to that draft. A reply is the exception. It expires the standing draft and writes a fresh one. The channel is WhatsApp when the lead's 24-hour window is open and they have a mobile, otherwise email. The draft lands in the approval queue of the lead's rep, with a one-line reason. Nothing is sent without a click.

### WhatsApp

WhatsApp runs through a Twilio WhatsApp Business number. Inside 24 hours of the lead's last inbound message a rep can send free text. Outside it, only a Meta-approved template can be sent, and the send function enforces this for every caller. 23 templates are defined in code, all named `stafford_*`, and all mapped to Twilio ContentSids. They cover first contact, re-engagement, course pivots, programme details, call scheduling and reminders, eligibility checks, payment options, documents, application, offers, enrolment and referral. A model ranks the templates for a lead using its brief, stage and tier, and falls back to fixed rules when the call fails.

Inbound messages are matched to a lead by phone digits and raise `whatsapp_replied`. A reply to an unassigned lead puts it back with its last owner, or round-robins it. Delivery and read receipts update message status, and the first read raises `whatsapp_read`. Templates are managed from an admin page, but edits to body, triggers and priority are reset by the seed list on every restart. Only the active switch and the ContentSid survive.

### Email

Email is off unless `MAIL_ENABLED=true`. When on, the app sends as one mailbox over SMTP and listens to it over IMAP. Auth is an app password or a Google service account with domain-wide delegation. Outbound email gets an open pixel and rewritten links, both carrying signed tokens. The Reply-To is plus-addressed with the lead id and a signature, so a reply can be matched to its lead with certainty. Matching falls back to thread headers, then the sender address.

The listener uses IMAP IDLE for speed and a UID watermark with a periodic reconcile for completeness. It only runs inside `server.ts`. Inbound replies raise `email_replied`. Admins can change the sender name, plus-addressing, open and click tracking, the listener switch and the poll interval without a deploy. The mailbox and secrets stay in env. Self-hosted mail replaced Unipile in late August 2026, and old rows still carry the `unipile` source.

### Link tracking (branch)

On the branch, links in WhatsApp messages are rewritten to short links (`/r/<code>`) that record a click and redirect. An approved template's URL button carries a fresh short code at send time. Links to our own site (by default `staffordglobal.org`) also carry a signed landing marker. When the page reports that marker through GTM, the visitor's cookie is bound to the lead without any form, and `outreach_return` fires. Email click tracking itself is on `main`.

### Outreach inbox

The `/outreach` page is the rep's messaging workspace in three columns. The left column lists the leads in the rep's book with their last message, with search and filters. Unread replies drive the badge in the nav. The middle column is the conversation with Call, WhatsApp and Email tabs. The WhatsApp tab shows how long the 24-hour window has left and offers templates outside it. The email tab has a rich text editor. Both can ask the model for a suggestion: a first message, a 24-hour or 48-hour follow-up, a reply to the last message, or an objection reading of the conversation. The right column shows why this lead (the brief), talking points, quick angles to drop into a message (payment plans, ROI framing, scholarships, flexible start, part-time option, different angle, soft re-engage), notes, and the outreach status control. A send needs the lead to be assigned and contactable.

### Human calls

A rep calls from the browser with a softphone built on the Twilio Voice SDK. The call goes out from a pool of voice numbers. The lead hears a recording notice before being connected. The call is recorded in two channels. When the recording is ready it is transcribed (Deepgram Nova-3 by default, ElevenLabs Scribe selectable from the admin page) and summarised by the model. The summary gives an outcome, key points, the objection, whether the lead asked for a human, a confidence level, and how firm any refusal was. A high-confidence outcome is applied at once through the same chain as a logged interaction. A low-confidence one is held for a rep to confirm or correct. No answer, voicemail and wrong number are recorded and do not score.

### AI caller (bot calls)

A manager can have the AI agent call a lead, one lead at a time, from the call menu on the lead page. Before dialling, the app checks that the lead is contactable, that the agent is under its concurrency cap (default 15), that no bot call is already running for that lead, that it is within the allowed calling hours in the lead's own timezone (default 07:00 to 21:00, worked out from the lead's country), and that the lead has had fewer than 3 bot calls in total.

Twilio places the call in its Ireland region and streams the audio to the app's WebSocket bridge, which relays it to an ElevenLabs agent session. The agent's prompt is built from admin-editable config (greeting, identity, rules, guardrails, workflow, examples, allowed topics, handoff triggers) plus a lead block made from the stored brief and the facts of the recommended course. The agent is a qualifier and scheduler. It discloses that it is automated, never states a fee, never judges eligibility, and hands anything on the handoff list to a human callback. Office background noise can be mixed in. Managers can watch a live call's transcript and, on request, its audio. A rep can force-end a call that never reached the lead. The finished transcript goes through the same summary and outcome path as a human call.

Recordings from both kinds of call are copied to S3-compatible storage when configured. The setup notes use Cloudflare R2. Twilio's copy is deleted after a retention period (default 90 days), and only after the archive copy is verified. Transcripts and summaries are kept.

The design document's routing rules (which leads get a bot and which a human), batch campaign launches and the inbound call safety net are not built. The example env file says the voice numbers were on hold pending purchase. Whether production has them is not visible.

### Interactions and outcomes

The "Log interaction" drawer on the lead page records channel, note and outcome. It can also flag corrections to the profile (title, company, company size, years of experience) and re-score from them. The consequences are fixed in `lib/interactions.ts`:

| Outcome | Effect |
|---|---|
| Interested, needs info | Task "Send programme details" due in 2 days, positive contact signal |
| Requested callback | Task "Call back" due tomorrow, positive contact signal |
| Not interested right now | Re-entry in 6 months, negative signal |
| Affordability concern | Re-entry in 6 months, negative signal |
| Enrolled elsewhere | Joined-competitor signal, which suppresses the lead |
| Explicitly refused | Opted-out signal, which suppresses the lead |
| Confirmed eligibility | Eligibility-confirmed signal |

A human contact scores higher than a bot contact, and a firm refusal costs more than a hesitant one.

### Tasks, notes and history

Tasks have a title, an assignee and an optional due date. They come from reps, from interaction outcomes, from the re-entry queue and from the assistant. Only the creator, the assignee or a manager can delete one. Notes can be edited or deleted only by their author or a manager. The lead page shows notes and tasks together, plus the history log.

### Assignment

New leads from a CSV or a Zoho pull are assigned round robin to reps, meaning every user without a manager or admin role, in email order. The rotation cursor is the last assignment in the history log. Auto-assign is on by default and an admin can turn it off. Managers assign or reassign one lead or many at once. A rep cannot reassign their own lead.

### Suppression and restore

Suppressed leads show a banner on the lead page and disappear from lists, feeds and drafting. A manager can restore one with a required reason, which is logged. Restore is refused while Zoho still has the lead opted out. Restoring is not available to the assistant.

### Action centre

The action centre is one ranked list built from five tables at request time. It holds, in rank order: overdue tasks, replies nobody has answered (an inbound message newer than the owner's last view), proposals waiting for approval, tasks due today, LinkedIn reviews (admins only), enrichment that gave up in the last 14 days (admins only), signals from the last 14 days, later tasks, and unassigned leads (managers only). A rep sees their own work and a manager the whole book. The order is fixed code so reps can trust it.

The `/actions` page groups the list into four bands by what the rep has to do: "Waiting on your decision" (proposals, reviews, stuck jobs), "Nobody has answered them" (replies), "Yours to chase" (tasks) and "Worth knowing" (signals, unassigned leads). Selecting an item opens a decision pane with the lead's facts and a preview. From the pane a rep can send or discard a draft, confirm or reject a LinkedIn candidate, retry a stuck job, mark a task done, take an unassigned lead, or call.

### Dashboard

The home page is the same for every role. It shows a greeting, a headline with the count of overdue tasks (the rep's own, or the team's for a manager), three widgets for the action centre bands that link into `/actions`, an ask bar that opens the assistant with role-specific suggested prompts, and a notification panel of replies and drafts. Pin and snooze in that panel are local to the browser. Holding the space bar dictates into the ask bar.

### Assistant (copilot)

The assistant is a chat on the dashboard and in a side panel on every page. It runs the agent model in a tool loop capped at 12 steps and 60 seconds. The actor is built from the session on the server, so a tool call can never choose whose authority it runs under. Tools are filtered by role before the model sees them.

| Group | Tools | What happens when called |
|---|---|---|
| Read | search leads, get lead (by slices), why this score, lead timeline, conversation thread, my work queue, course lookup, fetch a web page, web search | Runs at once |
| Manager and admin read | book stats, pipeline overview, ingestion health, list batches, get batch, list users | Runs at once |
| Write | set stage, assign lead, add, edit and delete notes, create, update and delete tasks, log interaction, set outreach status, mark thread read, set LinkedIn URL | Pauses for an approval click in the chat |
| Gated | send WhatsApp, send email, request enrichment, resolve LinkedIn review, retry enrichment, abandon enrichment | Writes a proposal to the approval queue |

That is 33 tools. Web search is registered only when SearXNG, Tavily, Exa or Brave is configured. Each tool result renders as a card in the chat. The system prompt tells the model to look up course facts with course lookup and never from memory or the web, to say what it is about to do before a pausing tool, and not to restate what a card already shows. Threads are saved on the server and can be pinned, archived and renamed.

### Approval queue

Proposals from auto-drafting and from gated tools wait in `pending_actions`. Approving re-checks the approver's own access, then replays the stored tool input through the same action the preview used. Rejecting records it. Proposals expire after 24 hours. They show in the action centre and the dashboard panel.

### Automation view

`/admin/automation` is a canvas that draws the running system from its registries rather than from a hand-made diagram. It shows every signal with its triggers, its dedup rule, its firing condition with current values, and its action chain, with model steps and approval steps marked. It also shows the clock jobs and two flow diagrams for ingestion and enrichment, overlaid with live counts. Admins edit the dials here (silence days, touch counts, per-visit thresholds, the event lists for fees, financing and depth, episode gaps, and sweep times) and the auto-assign and auto-enrich switches. A changed dial applies going forward only.

### Admin settings

All admin pages sit under `/admin` as tabs and are reached from the avatar menu.

| Page | What it controls |
|---|---|
| Eligibility | The classifier steering config, with draft, publish, rollback, prompt preview and test runs |
| The AI (models) | Provider (OpenCode Zen or OpenAI), chat model and agent model, from the provider's live model list. Shows whether a key is present without showing it |
| Templates | WhatsApp templates and a matrix of which template each stage and tier would get under the fallback rules |
| Voice | The agent's prompt config with draft and publish, transcription provider, background noise and volumes, recording retention, voice numbers, call statistics and concurrency |
| Mail | Sender name, plus-addressing, tracking, listener and poll interval, plus mailbox status |
| Users | Create users, set roles, set passwords |
| Automation | See above |

### Inflow

Inflow is the admin ingestion area with four tabs. Sources shows the Zoho connection state (last check, last success or failure, next automatic pull), the two rules for arriving leads (auto-assign and auto-enrich), and the CSV upload and Zoho pull actions. Queue lists leads never sent for enrichment, filtered by batch, dead reason, country and course level, and launches enrichment runs on all of them or the first N. Jobs lists every enrichment job by stage, including jobs stuck for 4 hours or more. History lists every batch and run with a funnel drawer.

`/inflow-next` is a preview of a redesign (Pipeline, Sources, History) served beside the current pages. It is admin-gated, read only, and not linked from the nav. Its links to `/inflow/batches/<id>` point at a route that no longer exists.

### Demo console

`/demo` is backstage tooling for live demos and testing. It is admin only and does not exist when `NODE_ENV` is production. Both the middleware and each of its 16 API routes check this. It can create a lead, pull one from Zoho by email, bind a chosen browser cookie to a lead, inject single events, play scripted scenarios (each with a near-miss twin that should not fire), replay raw webhook payloads through the real routes, shift a lead's history back in time, run a sweep tick on demand, and clear a lead back to baseline. The cookie bridge exists because, per the route's own comment, the live enquiry form cannot fire the form event that binds a visitor. It is not part of the product.

## Data model

### Tables

Tables are created in `lib/db.ts`, with the voice tables in `lib/voice/db.ts`. "Stafford" marks content that is specific to education or to Stafford's CRM.

| Table | What it holds | Generic or Stafford |
|---|---|---|
| `leads` | One row per person. See the column split below | Mixed |
| `courses` | The course catalog: programme, university, degree type, study mode, fees, duration, plus a JSON blob with entry tracks, fee tiers by region, modules and intakes | Stafford |
| `batches` | Ingestion operations and enrichment runs, with source, status, trigger, error and Zoho cursor movement | Generic |
| `batch_leads` | Which leads each batch inserted or updated | Generic |
| `enrichment_jobs` | One LinkedIn enrichment attempt per row, with stage timestamps, candidates and human review | Generic mechanism, LinkedIn-centric |
| `events` | Raw events from every source, anonymous or attributed | Generic |
| `lead_uids` | Which browser cookies belong to which lead | Generic |
| `signals` | Fired signals with payload, occurrence key, and score before and after | Generic table, Stafford-flavoured types |
| `score_history` | Meaningful score changes with a frozen breakdown | Generic |
| `reentry_queue` | Dates to look at a lead again, with a reason | Generic |
| `verifications` | Logged interactions: channel, note, outcome, corrections | Generic (the name is historical) |
| `notes` | Notes on a lead | Generic |
| `tasks` | Tasks on a lead with assignee and due date | Generic |
| `lead_history` | Audit log of stage, assignment, interaction and restore changes | Generic |
| `outreach_messages` | Every WhatsApp and email message in and out, with status and provider ids | Generic |
| `outreach_links` | WhatsApp short-link codes and their destinations (branch) | Generic |
| `inbox_reads` | When each rep last opened each lead's conversation | Generic |
| `waba_templates` | WhatsApp templates with variables, tier and stage triggers, priority and ContentSid | Generic table, Stafford content |
| `pending_actions` | Proposals waiting for a human: tool, exact payload, owner, status, expiry | Generic |
| `chat_threads`, `chat_messages` | Assistant conversations, per user | Generic |
| `app_config` | Key-value settings: eligibility and voice configs (draft, published, previous), detection dials, LLM choice, feature switches, sweep watermarks | Generic store, some Stafford content |
| `zoho_sync_state` | Zoho cursor and last sync outcome | Zoho-specific |
| `mail_sync_state` | IMAP UID watermark per mailbox | Generic |
| `voice_calls` | One row per call: mode, direction, numbers, status, recording, summary, outcome, review status, archive state | Generic |
| `voice_transcript_segments` | Transcript turns per call, by speaker | Generic |
| `voice_numbers` | The outbound caller-ID pool | Generic |
| `user`, `session`, `account`, `verification` | better-auth tables. `user.role` holds the role. Note that `verification` here is unrelated to `verifications` above | Generic |

The `leads` columns split as follows.

| Kind | Columns |
|---|---|
| Generic identity and CRM | `email`, `full_name`, `mobile`, `country`, `nationality`, `company`, `job_title`, `lead_source`, `lead_owner`, `crm_status`, `crm_created_by`, `zoho_id`, `zoho_data`, `opt_out`, `crm_modified_time` |
| Generic sales state | `segment`, `combined_score`, `readiness_score`, `pipeline_stage` (generic column, Stafford values), `assigned_to`, `outreach_status`, `suppressed`, `suppressed_reason`, `last_activity_at`, `last_waba_inbound_at`, `wokeworks_uid`, `batch_id`, `lead_brief` |
| Enrichment | `enriched_data`, `apify_raw_data`, `enrichment_stage` (generic mechanism, LinkedIn data) |
| Stafford and education | `course`, `course_level`, `university`, `highest_qualification`, `work_exp_total`, `work_exp_managerial`, `work_exp_teaching`, `programme_eligibility`, `eligibility_strength`, and `dead_reason` (generic idea, Stafford picklist values) |

### Key types in `lib/types.ts`

| Type | What it is | Generic or Stafford |
|---|---|---|
| `Segment` | HOT, WARM, NURTURE, DEAD | Generic |
| `SignalType` | All signal names, including retired ones | Mixed. Fees, financing, course engagement, comparison, stuck on eligibility, eligibility confirmed and the career pair are education-shaped |
| `EventSource` | gtm, zoho, twilio, mail, unipile, voice, clay | Generic |
| `Theme` | The ten readiness themes | Mostly generic. `eligibility` and `career` are education-shaped |
| `BatchSource`, `BatchStatus` | Ingestion operation kinds and states | Generic |
| `JobStage`, `EnrichmentJob`, `FinderCandidate` | The enrichment pipeline | LinkedIn-centric |
| `InteractionChannel`, `InteractionOutcome`, `OutcomeTone`, `CoolOffIntensity` | Logged contact vocabulary | Mostly generic. `confirmed_eligibility` and `affordability_concern` are education-shaped |
| `Lead`, `LeadCrmFields` | The lead row and its 18 CRM columns | Mixed, as above |
| `Batch`, `Verification`, `Signal`, `ScoreHistory`, `LeadEvent`, `LeadUid`, `ReentryQueue`, `Note`, `Task`, `LeadHistory` | Row shapes of the generic tables | Generic |
| `GtmWebhookPayload` | A website event: type, cookie, session, optional email, page, params such as `course_id` | Generic shape, Stafford params |
| `NormalizedProfile`, `EnrichedLead` | The scraped LinkedIn profile and the merged input to eligibility | Career data for an education buyer |
| `CsvLead` | The Zoho CSV export columns | Stafford |
| `ProgrammeRecommendation`, `ProgrammeEligibility` | Course picks with fit, entry track, fee tier and modules, plus disqualification, redirect, dead-reason strategy and re-entry | Stafford |
| `LeadBrief` | Headline, situation, recommended action, primary course, talking points, risk | Generic except `primary_course` |
| `PipelineStage` | The 26 stage values and labels | Stafford |
| `UserRole` | executive, manager, admin | Generic |
| `OutreachStatus`, `OutreachChannel`, `OutreachDirection`, `OutreachMsgStatus`, `OutreachMessage` | Messaging vocabulary | Generic |

## Pages and navigation

The top nav shows Dashboard, Actions, Leads and Outreach to everyone, and Inflow to admins. Admin settings are reached from the avatar menu. Every page except `/login` and the short-link redirect needs a session.

| Route | Who | What it shows |
|---|---|---|
| `/login` | Anyone | Email and password sign-in |
| `/` | All roles | Dashboard: headline, three action widgets, assistant ask bar, replies and drafts panel |
| `/actions` | All roles | The action centre board. Reps see their own work, managers add unassigned leads, admins add LinkedIn reviews and stuck enrichment |
| `/leads` | All roles | Lead list with tier tabs (All, Hot, Warm, Nurture, Dead), search, filters (enrichment, CRM stage, assignee for managers), a "My leads" or "All leads" toggle for managers, contact-state counts (waiting on you, never contacted, untouched hot, gone quiet, no phone number, unassigned), bulk actions (re-enrich, regenerate brief, set stage, assign for managers) and keyboard navigation |
| `/leads/[id]` | All roles | The lead page. A header with tier and score, an action bar (log interaction, an outreach dock for WhatsApp and email, a call menu with bot call for managers and human call), and five tabs. Overview has the LinkedIn field, the score gauge, the re-engagement summary, and the brief with the recommended course. History has the activity stream, signal timeline, score history and record changes. Notes has notes and tasks. Interactions has logged contacts and calls. CRM Data has the raw Zoho record and the stage and assignee controls |
| `/outreach` | All roles | The three-column outreach inbox |
| `/inflow`, `/inflow/queue`, `/inflow/jobs`, `/inflow/history` | Admin | Ingestion: Sources, Queue, Jobs, History |
| `/inflow-next`, `/inflow-next/sources`, `/inflow-next/history` | Admin | Unlinked preview of an Inflow redesign. Not part of the current navigation |
| `/admin/eligibility`, `/admin/models`, `/admin/waba`, `/admin/voice`, `/admin/mail`, `/admin/users`, `/admin/automation` | Admin | Settings, see Admin settings |
| `/waba` | Any signed-in user | Redirects to `/admin/waba`, which is admin only |
| `/demo` | Admin, non-production only | The demo console. Not product |
| `/r/[code]` | Public | WhatsApp short-link redirect (branch) |

The API routes under `app/api` serve these features:

| Route group | Feature |
|---|---|
| `/api/auth` | better-auth |
| `/api/leads/*`, `/api/notes`, `/api/tasks` | Lead list, stage, assignment and bulk assignment, notes, tasks, log interaction (`verify`), re-classify, re-enrich, set LinkedIn URL, restore, history, the auto-assign switch |
| `/api/outreach/*` | Inbox lead list, messages, send, AI suggestions, outreach status, read markers, unread count |
| `/api/waba/*` | Templates, template ranking, template send |
| `/api/voice/*` | Softphone token, outbound TwiML and recording notice, bot call start and its TwiML, call list, hang up, outcome correction, recording playback, re-transcribe |
| `/api/webhooks/*` | GTM site events, Clay career signals and Clay URL callbacks, Apify profile results, Twilio WhatsApp inbound and status, Twilio call status and recording |
| `/api/mail/t/o`, `/api/mail/t/c` | Email open pixel and click redirect |
| `/api/chat`, `/api/threads`, `/api/pending`, `/api/tools/preview`, `/api/transcribe` | Assistant, its threads, the approval queue, tool previews, dictation |
| `/api/upload`, `/api/zoho/*`, `/api/enrichment/*`, `/api/batches` | Inflow: CSV upload, Zoho sync, pull and backfill, enrichment queue, jobs, pause and auto-enrich, batch detail |
| `/api/admin/*`, `/api/users` | Admin settings and the user roster |
| `/api/demo/*` | Demo console |

## Stafford-specific vs generic

### What would need to change for a real estate developer

- The course catalog becomes a project and unit inventory. Eligibility classification is built around courses, entry tracks, fee tiers by region and qualifications already held. A property version would need a different fit model, for example budget against unit price, buyer intent and payment plan fit, with its own steering config. This matters more than it looks, because the combined score multiplies readiness by this fit. Without a fit layer every lead scores 0.
- The Stafford columns on `leads` (course, course level, university, qualification, three work-experience fields) give way to property fields such as project of interest, unit type, budget and purpose. The CSV mapper, the Zoho field mapping (`lib/zoho/map-lead.ts`, `lib/zoho/crm-context.ts`) and the brief's CRM context all read Stafford's custom Zoho fields and need remapping to the developer's Zoho layout.
- The pipeline stages, the status-to-stage map, the "contacted" and "attempted" status lists, the dead-lead gate and the excluded dead reasons are all copied from Stafford's Zoho picklists.
- The website signals assume course pages. Fees interest, financing focus and deep engagement already take their event lists from admin dials, so they could be pointed at price list, payment plan and floor plan events. Comparison shopping counts distinct `course_id` values and stuck on eligibility watches one page type; both are written for courses. The GTM container on the developer's site would need to emit the matching events.
- LinkedIn enrichment and the career signals exist because degree eligibility depends on job history. For property buyers they may matter less, and the enrichment spend may not pay back.
- Interaction outcomes "confirmed eligibility" and "affordability concern" and the matching signals are education-shaped.
- Every prompt is written for Stafford: eligibility, brief, outreach voice, assistant, WhatsApp template ranking and the voice agent. The 23 WhatsApp templates are Stafford text and would need new Meta approvals. The voice agent would need a new vertical file in `config/voice-agents/` and a new ElevenLabs agent, which the code was designed for.
- `OUTREACH_SITE_HOSTS`, the course-page link targets and the sign-in branding name Stafford.

### What transfers as is

- The event, detector, signal and action-chain engine, with occurrence keys, recovery and the identity bridge.
- Readiness scoring with themes, decay and dampening, segments and score history.
- The sweep and its clock jobs, already timed to the Dubai working day.
- Zoho incremental sync, pull, backfill and the upsert differ, as mechanisms.
- Batches and the Inflow pages.
- Assignment, tasks, notes, history, interactions and their outcome chain, suppression, restore and re-entry.
- The action centre, the dashboard and the approval queue.
- The assistant, its role-bounded actor model, the tool registry and the tool cards (the course lookup tool aside).
- WhatsApp sending with window enforcement and templates, email over SMTP and IMAP with tracking, link tracking, and the outreach inbox.
- Voice calling as a whole, including the softphone, the bot caller, quiet hours, attempt caps, transcription, summaries into outcomes, live monitoring, and recording archive and retention.
- Admin pages for models, users, mail, voice and automation dials.

The app is single-tenant. There is no organization or workspace concept, and every user works one shared book.

## Known limits

- Zoho is read only. Stages, notes, interactions and assignments are not written back. A profile correction to years of experience is overwritten by the next sync. Zoho push notifications are configured in env but no route receives them.
- The lead page loads any lead by id for any signed-in user. It does not run the per-lead access check that the API routes and assistant tools use, so a rep can read a lead outside their book by URL. This is inferred from the page code and was not tested.
- The GTM webhook accepts every request when `GTM_WEBHOOK_SECRET` is unset, and that variable is not in `.env.example`. Twilio webhooks are guarded by a shared secret in the URL rather than Twilio's signature check.
- Career signals have no live source, as described under Signals.
- Email is off unless `MAIL_ENABLED=true`. The inbound email listener, the bot-call bridge and the live call monitor run only under `server.ts`. The committed PM2 config (`ecosystem.config.js`) runs `pnpm start`, which is plain `next start`. Which one production actually runs is not visible.
- Voice numbers were on hold pending purchase according to the example env file. Bot calls are launched by hand, one lead at a time. Routing between bot and human, batch launches and inbound calls are not built.
- WhatsApp template edits made in the admin page are reset to the code's seed list on every restart, apart from the active switch and the ContentSid.
- `leads.outreach_status` is stale for many leads according to a code comment, so the leads page counts contact state from messages instead.
- The automation view's enrichment diagram still draws Clay as the lookup step, while the code uses the ContactOut and Reverse Contact waterfall by default.
- `/inflow-next` links to a batch page that no longer exists.
- All background work runs inside the one Node process. A restart drops in-flight background jobs; the recovery loop and the reaper pick most of them up later. Scaling past one process would need a real queue.
- `deploy.sh` holds the server's SSH password in plain text.
- The repository references design documents that are not committed, so the history of most decisions lives only in code comments.

## Where the repo's own docs differ from the code

- `voice.md` says nothing is built. The human softphone, transcription, summaries, the bot caller, live monitoring, recording archive and retention are all built.
- `voice.md` proposes calling hours of 09:00 to 20:00. The default in code is 07:00 to 21:00, editable per vertical.
- `voice.md` leaves the transcription vendor open. Deepgram is the default for human calls, ElevenLabs Scribe is selectable, and ElevenLabs is the only agent provider (the Deepgram agent was removed on 2026-08-08).
- `voice.md` describes a bot-or-human router and an inbound safety net. Neither is in the code.
- The README names Clay for enrichment. Clay is off by default and replaced by the provider waterfall.
- `.env.example` does not list `LLM_API_KEY`, `LLM_MODEL`, `AGENT_MODEL` or `GTM_WEBHOOK_SECRET`, which the code reads.

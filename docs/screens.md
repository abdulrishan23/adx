# ADX screens

Where every screen lives across the seven phases in `docs/build/ADX_AUTONOMOUS_BUILD.md`, and what each phase 0 and 1 screen has to handle. The December release is phases 0 and 1. Later phases have a reserved place in the navigation and no screens yet.

## What the screens are about

- **Workspace.** Reportage. Phase 6 adds other clients.
- **Project.** A development, such as Marlin Icon. Performance, leads and creatives all hang off it.
- **Concept.** One creative idea for a project, such as C1 or O1. It holds the copy in English and Arabic and a default render.
- **Ad.** One concept in one size, language and CTA variant, such as `C1_CASH_EN_9x16_CTA`. Editing it makes a new version. A review applies to the version it was given on.
- **Render.** A project's CGI image, used behind the copy.
- **Brand kit.** A project's logos, fonts and colours. Every generated ad is built from it.
- **Review link.** A project's public board. Reviewers open it without signing in, comment, and approve or ask for changes.

## Navigation

Adx is three sub-products behind one Home. **Create** takes a project from brand to live campaign. **Perform** keeps money well spent. **Convert** takes leads through the funnel and talks to them. Home is a desk around the chat bar: a greeting with one short line on how much needs a person (with a "Sample data" chip while the figures are sample), then the cards that deserve attention today as a carousel, from any door and in any shape (a figure, bars, a donut, the rejected ads, the leads waiting), each with "Talk about this" that puts a ready question to Adx; under the bar, four ready-to-fire prompts as pills, with "Show more templates" opening the full catalog grouped by door. The server picks the cards; one that stops mattering stops appearing. **Inbox** (`/inbox`) holds every card: what needs a person (approvals, blocks, every alert check) first, worst first, then updates such as research Adx refreshed, newest first. It filters by needs-you or updates and by door, shows as cards or a list, and alert cards can be snoozed or dismissed from the list. Home's carousel shows the top of the same order. Adx's running jobs are a count you can open, not a feed; their results arrive as cards. Alerts used to have their own screen under Perform; `/alerts` now opens the Inbox. **Present** (`/present`) plays the same cards on a wall, one per slide, advancing on its own.

In the screens a project is called a brand: a development such as The Wadi, marketed with its own facts and kit. Since BD-57 a brand is Reportage or a competitor and a development such as The Wadi is a project under it. Brands (`/brands`) lists ours and the competitors Adx watches, from the engine. A brand page (`/brands/$brandId`) holds its projects, the facts Adx may state (grouped by key, with source and class; two sources that disagree are a conflict an admin decides with a reason), its offers, and the research briefs run for it. Start a brand (`/brands/new`) creates the brand with the brief as its notes, its first project, and a research brief that reads the website, the pages and the competitors named, audits the account when there is history, and starts running at once. Brand knowledge folded into the brand page; `/create/brand` redirects. Research (`/create/research`) shows a brief from the engine: its items as they run (pages, competitor pages, keyword ideas, the account audit) with their state, competitors Adx can't place with a picker to resolve them, the pages read (ours and competitors' kept apart), keyword ideas marked as Google's estimates, and the account audit. Schedules, market events and comparative reports stay on the engine's research screen for now. The brand page follows a development from brand to live campaign to leads, with the steps that arrive in later phases shown as not started rather than hidden. The sidebar shows Home, the three doors, Workspace and Settings. A door opens at its first live screen (Brands, Analytics, Leads) and its other screens appear under it only while it's open. Composed overviews per door come with phases 2 and 3, when Create and Convert have something to compose; Analytics leads with a headline band in the meantime. Items past the release phase are hidden. `VITE_PHASE` sets the release phase for a build and defaults to 1; `web/.env.development` sets it to 6, so `make dev` shows every screen that exists. Screens whose system hasn't shipped carry a "Sample · phase N" chip and run on `web/src/api/mock/create.ts`, so the whole Create loop can be walked: a fact in conflict is decided, keywords are ticked into the plan, a plan is approved, a rejected ad is fixed and resubmitted.

| Door | Item | Route | Phase |
|---|---|---|---|
| Home | Ask Adx | `/` | 1 |
| | Conversation | `/ask` | 1 |
| | Present | `/present` | 1 |
| Create | Brands | `/brands` | 1 |
| | Start a brand | `/brands/new` | 1 |
| | Brand | `/brands/$brandId` | 1 |
| | Brand knowledge | `/create/brand` | 2 |
| | Research | `/create/research` | 2 |
| | Media plans | `/create/plans` | 2 |
| | Creatives | `/creative` | 1 |
| | Landing pages | `/create/pages` | 3 |
| | Launch | `/create/launch` | 3 |
| Perform | Analytics | `/analytics` | 1 |
| | Campaigns | `/campaigns` | 1 |
| | Alerts | `/alerts` | 1 |
| | By project | `/perform/projects` | 1 |
| | Revenue | `/revenue` | 1 |
| | Portals | `/portals` | 1 |
| | Marketers | `/marketers` | 1 |
| | Reports | `/reports` | 1 |
| | Data quality | `/data-quality` | 1 |
| | Proposals | `/proposals` | 3 |
| | Audiences | `/audiences` | 3 |
| | Outcome sync | `/outcomes` | 3 |
| | Website | `/website` | 6 |
| | Organic | `/organic` | 6 |
| Convert | Leads | `/leads` | 1 |
| | Enrichment | `/enrichment` | 4 |
| | Calls | `/convert/calls` | 4 |
| | Inbox | `/convert/inbox` | 5 |
| | CRM sync | `/convert/crm` | 5 |
| Workspace | Agents | `/workflows` | 5 |
| Settings | Connections | `/settings/connections` | 0 |
| | People | `/settings/people` | 0 |
| | Stages and lists | `/settings/lists` | 1 |
| | Guardrails | `/settings/guardrails` | 0 |
| | Scoring | `/settings/scoring` | 4 |
| | Workspaces | `/settings/workspaces` | 6 |

Public pages, with no sign-in: `/privacy`, `/review/$key` (phase 1) and `/approve/$token` (phase 3). `/focus`, `/projects`, `/settings/projects`, `/brief` and `/funnel` redirect to where those screens went.

Website and Organic sit in phase 6 because phase 1 reads only Google Ads, Meta Ads and GoHighLevel. Guardrails is phase 0 because the policy screen comes first, but in phase 0 and 1 it only shows rules as not set up, since nothing writes to an outside system yet.

### Fronts

Home's cards are data, not layout: the server returns them in order, each with a status, a title, one line on why it's there, an optional figure and picture (pace, bars, donut, ads, leads, list), a question for "Talk about this" and one action. Figures come from code; the model only writes the sentences. In phase 1 the cards are mocked in `web/src/api/mock/fronts.ts`, computed from the same fixtures as Campaigns and Leads so figures agree across screens, and ad previews come from the live creative service. The Go endpoint arrives with the metrics engine and the model adapter (B1.3, B1.5).

## Creative

Ported from Creative Studio into ADX's design. Images are files on the server next to the database and are backed up with it.

| Screen | Route | What it does |
|---|---|---|
| Creatives | `/creative` | Projects with their concepts, ads and review state: approved, needs changes, waiting |
| Project | `/creative/$projectId` | Concepts, renders and brand kit tabs. Upload, new concept, copy review link, download approved |
| Upload | `/creative/$projectId/upload` | Drop many images. Size comes from the pixels, and concept, language and CTA from the file name. A check table before saving |
| Concept | `/creative/$projectId/concepts/$conceptId` | Copy in English and Arabic, default render, generate all sizes, each ad with its house-rule checks and review state |
| Editor | `/creative/$projectId/ads/$adId/edit` | Move and resize text and logos, edit copy, crop the render, switch size, save as a new version |
| Review board | `/review/$key` | Phone frames per size, approve or ask for changes, comments with pins, render picks |

Rules carried over from Creative Studio: four sizes, each with and without CTA; no discount above 15% on screen, and every discount shows its condition; logo top centre; Arabic copy is always flagged for native review; any "needs changes" outranks approvals.

One rule changes: a review belongs to the version it was given on. Saving an edited ad clears its approval, and the board shows the new version as waiting.

## States every phase 0 and 1 screen handles

| State | What the screen shows |
|---|---|
| Loading | The layout with placeholders, not a spinner over a blank page |
| Empty | Why it's empty and the one action that fills it |
| Not available | "Not available" for a missing figure, never 0 |
| Partial | The figure, plus what's missing, such as "2 of 3 accounts synced" |
| Stale | When the data was last updated, and that it's older than expected |
| Blocked | What's missing, such as access or a business rule, and who can supply it |
| No permission | That the person's role can't see or do this |
| Error | What failed in plain words, and a retry |

## Phase 0 and 1 screens

| Screen | Phase | Data today | Needs |
|---|---|---|---|
| Sign-in, no access, signed out | 0 | authentik | Session wiring (B0.3) |
| Connections | 0 | GHL live, Google and Meta mock | Read adapters (B1.1), health per account |
| People | 0 | Mock | Users API |
| Guardrails | 0 | Mock | Policy store (B0.4), shown as not set up |
| Analytics | 1 | Mock | Metrics engine (B1.3) |
| Alerts | 1 | Mock | Anomaly checks (B1.4) |
| Home, Ask Adx, Present | 1 | Mock | Fronts endpoint and model adapter (B1.3, B1.5) |
| Projects, Campaigns, Revenue, Portals, Marketers | 1 | Mock, projects live | Metrics engine (B1.3) |
| Reports | 1 | Mock | Reports and schedules (B1.4) |
| Data quality | 1 | Mock | Coverage from the lead copy (B1.2) |
| Leads, lead detail | 1 | GHL live | Stage mapping (B1.2) |
| Stages and lists | 1 | Mock | Mapping store (B1.2) |
| Creative screens and review board | 1 | Live | Built with the port |

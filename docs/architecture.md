# ADX architecture and decisions

The decisions that span features and phases, the tradeoffs behind them, what's deferred and what's still open. Definitions and the feature list are in `reference.md`.

Many of these decisions come from answers Hisham bhayya (WokeWorks CTO) gave in October 2026 (`research/sources/hisham-answers-*.md`). They're the current direction. Points in those answers that still need checking are listed under Open questions.

## Principles

These guide decisions when nothing below covers a case.

- **Track cost per qualified lead daily, revenue monthly.** Done-Deals in AED are what Reportage's CEO cares about, but there are too few of them to act on every day.
- **The platforms are the source of truth.** ADX stores history and its own results, such as scores and temperatures, on top. It never keeps its own copy of spend, conversions or statuses.
- **Read widely, write narrowly.** ADX reads everything it's connected to. It writes only its own fields in GoHighLevel, and only approved changes to the ad platforms.
- **Writes are logged, capped and reversible.** Any change that adds spend or reaches new people needs approval. Nothing is deleted.
- **Show the gaps.** ADX puts sync gaps, missing statuses and weak attribution on screen as numbers. Fixing Reportage's own systems is up to them.
- **Honest numbers.** ADX shows "not available" instead of a zero, keeps counted and estimated figures apart, and shows coverage next to any figure that depends on it.
- **Careful with personal data.** ADX hashes phone and email before upload and doesn't guess nationality (see D-10).
- **Every feature needs a reason to be in ADX.** On the ads side, ADX has to do at least what Ryze does. Ryze (get-ryze.ai) is an AI tool that audits Google and Meta ad accounts, finds wasted spend and applies approved changes. Nothing from Marketing OS comes across just because it already exists.
- **One name per concept.** Marketing OS had two different systems both called "insights", and that confused people.

## Decisions

Each decision has an ID and a date. *Cost* is what we give up by choosing it.

### Scope

- **D-01. ADX is Marketing OS plus lead intelligence** (2026-10-09). It scores leads and traces spend to Done-Deals, and it optimises for deals instead of form fills. *Why:* reporting on clicks and form fills already exists. What Reportage can't see is which campaigns bring buyers. *Cost:* ADX's most useful numbers depend on sales data it doesn't control, like statuses and Akarati deals.
- **D-02. Only the marketing team uses ADX** (2026-10-09). That's about 30 people. Sales OS's rep desk, inbox and assignment don't come across. *Cost:* what happens on the sales side only reaches us as GoHighLevel statuses, and half of those are blank. *Revisit if:* Reportage wants sales on the same tool.
- **D-03. ADX sends no outreach** (2026-10-09). No calls, WhatsApp or email to leads. Messages to staff, like approval links, are fine. Leads that went cold are re-engaged through ad audiences instead. *Why:* outreach is out of scope for Reportage, and they're not open to automated calls. *Cost:* the direct re-engagement Sales OS was built for is gone, and tying website visits to leads gets harder, because ADX never sends a link.
- **D-04. Reportage first, one client** (2026-10-02). No multi-tenancy. We avoid Reportage-only shortcuts when a general version costs about the same, so ADX could become a product later. *Revisit if:* a second client signs on.
- **D-05. Ryze is the benchmark for the ads side, not the spec** (2026-10-09). ADX has to match it at minimum. Ryze has no CRM, no lead scoring and no link from spend to deals, which is where ADX goes further.
- **D-06. Paid media is Google and Meta** (2026-10-09). Portal leads count like any other lead and are reported with spend typed in by hand. *Why:* portals brought 0 deals in September, and their APIs need partner agreements. *Cost:* portal cost per lead is only as good as the typed-in spend, and there's no sync gap report for portals. *Revisit if:* portals start bringing deals.
- **D-07. ADX reads organic social, but doesn't post** (2026-10-09). It reads Instagram, Facebook, LinkedIn and Search Console. Posting and scheduling come later, behind approval.

### Data

- **D-08. The platforms are the source of truth** (2026-10-09). ADX stores history and its own results (scores, temperatures, alerts, logs), never its own copy of spend, conversions or statuses. *Cost:* ADX depends on each platform's API limits, and the platforms revise past numbers, so figures can change after the fact.
- **D-09. GoHighLevel is the only CRM ADX talks to** (2026-10-09). ADX reads leads and statuses from it.
  - ADX writes only its own fields (`adx_temperature`, `adx_score`, `adx_reason`, `adx_updated_at`) and tags starting with `adx:`. It never writes statuses, pipeline stages or GoHighLevel's own fields.
  - Akarati data reaches ADX only through GoHighLevel.
  - Reportage's monthly lead sheet (one Google Sheet per month, from January 2025) is only for backfilling history.

  *Why:* GoHighLevel is Reportage's marketing CRM, and Akarati already syncs into it. Their GoHighLevel admin builds workflows on ADX's own fields, so ADX writes only those. *Cost:* revenue reporting depends on Akarati sending deal values into GoHighLevel, which we don't control.
- **D-10. Personal data** (2026-10-09).
  - Phone and email are hashed before any upload.
  - Opted-out leads never go into audiences.
  - Nationality is never guessed from names, and the country code is used only to tell where someone lives.
  - ADX doesn't exclude any country's leads. That's a targeting decision for the CMO.
- **D-11. Metrics** (2026-10-09). Marketers track cost per qualified lead daily. The CMO and CEO track revenue and return on ad spend monthly. *Why:* about 58 deals a month is too few to act on daily, and deals close slowly. *Cost:* cost per qualified lead depends on sales updating statuses.
- **D-12. Hosting** (2026-10-09). Likely Frankfurt, because UAE data centres aren't an option today. Reportage has to approve it in writing before real lead data moves.

### Writes

- **D-13. Every write goes through a proposal** (2026-10-09). A proposal is approved, executed, then logged.
  - Google changes run in validation mode first. New Meta objects are created paused and launched as a separate step.
  - Nothing is deleted. ADX pauses or archives instead.
  - There are hard caps on daily spend increases and on changes per day. The admin sets them, and nothing in ADX can go past them.
  - Each account has a kill switch.
  - Anything ADX creates gets an `adx_` prefix.
  - Proposals expire after 48 hours.
  - No change is ever applied twice.

  *Why:* changing ad accounts is where mistakes cost money. *Cost:* every change is slower than editing in Google Ads directly, and every write needs a dry run and an undo built for it.

#### Default permissions (D-13)

These are the defaults for marketers. In general, anything that lowers spend or only prepares something is allowed, and anything that raises spend or reaches new people needs approval. Pausing a whole ad set or campaign also needs approval, because it can cut off a large share of leads at once.

| Action | Default |
|---|---|
| Create or update audiences and Customer Match lists | Allowed |
| Upload offline conversions and Conversions API events | Allowed, automatic |
| Exclusion lists, labels, negative keywords | Allowed |
| Set lead temperature and score in GoHighLevel | Allowed |
| Pause an ad | Allowed |
| Lower a budget | Allowed |
| Create a campaign or ad (paused) | Allowed |
| Pause an ad set or campaign | Needs approval |
| Raise a budget within the cap | Needs approval |
| Change bids or target CPA or ROAS | Needs approval |
| Launch anything | Needs approval |
| Change targeting, geography or GCC-only rules (targeting that limits a project like Oceana to GCC nationals) | Needs approval |
| Organic posting | Off (later) |
| Delete | Not available |

- **D-14. Three roles, with approvals by link** (2026-10-09). The roles are Viewer, Marketer and Admin, and the CMO is the admin. Approvals go out as secure links in email or WhatsApp. *Why:* the CMO won't use a new dashboard. The roles are described in `reference.md`.

### Look and build

- **D-15. ADX looks like Marketing OS** (2026-10-01). It uses the Reportage brand, black and white and minimal, instead of the glass look Sales OS used.
- **D-16. No frontend-only prototype** (2026-10-09). We'll scaffold the real app once the tech stack is decided. *Why:* we'd rather put the time into the real app than build the screens twice.

## Reusing the old products

What carries over is settled. Whether the code carries over depends on the tech stack.

- **From Marketing OS:** Google and Meta reporting (61 and about 66 read tools), the background sync and data warehouse, findings and analyses, the proposal model, the copilot, generated reports, the GoHighLevel adapter, GA4 and Search Console reads, and its rules for honest numbers.
- **Marketing OS limits today:**
  - Meta access is `ads_read` only.
  - The Google sign-in app isn't approved for production, so connections expire every 7 days.
  - Writes to ad platforms were never built.
  - It reads no organic social accounts.
  - It has never run on Reportage's real ad accounts.
- **From Sales OS:** the signal and scoring engine, its readiness score (what ADX calls the intent score), which decays over time and has per-signal caps, the logic that ties a browser to a lead, and tracked links that carry a signed per-lead token. Reusing the engine needs three changes:
  - Rewrite its detectors, which were written for course pages, for Reportage's project pages.
  - Change how it combines the scores. Today it multiplies readiness by fit, so a lead with no fit data scores zero, and every portal lead would end up at the bottom.
  - Match visitors to leads by phone and other fields, not only by email.

## Not decided yet

- **Tech stack.** A tech options draft by Twaha (who built Marketing OS), dated 2026-10-07 and shared separately, compares Go and TypeScript for one stack across ADX. It recommends a short Google Ads test build in both before deciding. It doesn't evaluate Python, which Marketing OS uses. Once made, it's added here as the next decision.
- **The `marketing-platform` spec.** Twaha's draft refers to a spec by this name, with requirement IDs like MP-ACT-14. We haven't seen it yet. It needs reading and reconciling with these docs.

## Deferred

These are wanted, but not in the first version.

- Organic posting and scheduling, behind approval.
- Automating approved actions, once proposals have proven reliable.
- Call tracking for telesales.
- Connecting to portal accounts, managing portal spend, and portal sync gap reports.
- An MCP server, so other AI clients can use ADX's tools.
- Multi-tenancy, when a second client signs on (see D-04).
- If the Head of Marketing & AI hires a team, letting them approve that team's proposals.

## Set aside

| Idea | Why | What would bring it back |
|---|---|---|
| Merging both products in full, including the sales desk | Sales staff don't use ADX | Reportage wanting sales on the same tool |
| Voice calling (an AI caller or a softphone) | Reportage isn't open to automated calls | A clear yes from Reportage |
| Finding and scraping leads' personal social profiles | It says little about property buyers, finds only some profiles, and breaks Meta's terms | Nothing likely |
| LinkedIn enrichment and course eligibility | Built for Stafford, where eligibility depended on job history | Doesn't apply |
| Zoho | Only ever pointed at another client's data | A client on Zoho |
| Connecting to Akarati directly | No public API, and its data reaches GoHighLevel anyway | Akarati offering an API, and GoHighLevel falling short |
| Ecommerce and app tools (Shopping, product groups, App campaigns, Meta catalogs) | Reportage doesn't sell online | Doesn't apply |
| Events and roadshows | They run through brokers, outside the marketing CRM | Reportage wanting event results in ADX |
| Creative generation | Out of scope. Creative analysis stays in | A clear ask, or a tool we buy |
| Attribution by channel only | ADX aims for campaign level, using match levels | Doesn't apply |
| A frontend-only prototype | See D-16 | Needing to test flows with Reportage before building |
| The Sales OS glass look | See D-15 | Doesn't apply |

## Open questions

These move to GitHub issues labelled `question`.

- Website visits are meant to be tied to leads through a per-lead token on links marketing sends. ADX sends nothing, and sales don't use ADX. So who sends those links?
- Which status column does GoHighLevel hold: "Lead Status" (MQL, SQL, Qualified and so on) or "Status" (No Answer, Follow Up, Done-Deal and so on)?
- Should "qualified" include a hot score? Probably not, because it mixes a CRM fact with ADX's own estimate.
- What should scoring weights learn from? Probably qualified statuses, with deals as a check, since 58 deals a month is too few.
- Does organic include TikTok or YouTube?
- What format should leadership reports take: PDF, slides or a link?
- How do marketers move leads into the WhatsApp bucket?
- Is "Guru" in Reportage's data PropertyGuru?
- Will Reportage agree that we propose funnel changes (hidden fields, an original-source field, form questions) and the Head of Marketing & AI applies them?
- Is the Head of Marketing & AI Rishan, the WokeWorks CEO who also works inside Reportage's marketing?
- How do Reportage's marketers work day to day: on a laptop or a phone, and in which views?
- How is the 800 number set up? Calls aren't tracked in the first version, but telesales closes the most deals per lead.
- Who on our team owns which areas?

## Waiting on others

These move to GitHub issues labelled `external`.

| What | Why it matters | Owner | Status |
|---|---|---|---|
| Akarati sends deal value (AED) and date into GoHighLevel as real fields | No revenue or return on ad spend without it | Reportage, Akarati admin | Needs an owner and a date |
| The website form to GoHighLevel sync keeps UTMs and click IDs | Google and website attribution | Reportage web and GoHighLevel admin | Known issue |
| Hidden click ID and UTM fields on every form | Campaign-level attribution | Reportage web admin | Not started |
| An original-source field in GoHighLevel, set once | Stops relabelling from erasing attribution | Reportage GoHighLevel admin | Not started |
| PostHog on every site, through GTM | Web intent signals | Reportage web team | Not started |
| A daily availability snapshot from Akarati | Stock-aware ads | Reportage | Not started |
| GoHighLevel access, or an export of their real setup | We need their real fields, statuses and workflows. A trial account won't have them | Reportage | To ask |
| A walkthrough of Akarati | To see what reaches GoHighLevel | Reportage's Akarati admin | To ask |
| GA4 Viewer access | Website reporting | Head of Marketing & AI | To grant |
| Meta permissions beyond `ads_read` (writes, the Leads API, page and Instagram insights) | Writes, lead pulls and organic reporting | WokeWorks, through Meta's app review and business verification | Not started |
| The Google sign-in app approved for production | So connections stop expiring every 7 days | WokeWorks | Not approved |
| LinkedIn Community Management API access | LinkedIn organic | WokeWorks | Apply early. It can take weeks |
| Written approval for Frankfurt hosting | Data protection | Reportage | To request |
| Status updates on every lead | Accurate scoring and return on ad spend | Reportage sales | 53% blank in September |
| Checking reportageoffer.com's final URLs | Ad spend might be going to a password page or a 404 | Head of Marketing & AI | To check |

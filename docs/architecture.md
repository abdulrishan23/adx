# ADX architecture and decisions

The decisions that span features and phases, what each one costs us, and what's deferred or set aside. Definitions and the feature list are in `reference.md`. Open questions and outside dependencies are tracked in two pinned GitHub issues (see the end of this doc).

Many of these decisions come from answers Hisham bhayya (WokeWorks CTO) gave in October 2026 (`research/sources/hisham-answers-*.md`). They're the current direction. Points in those answers that still need checking are open issues.

## How ADX fits

- **Reads.** Google Ads, Meta (ads, lead forms and pages), GA4, Search Console, Instagram, Facebook and LinkedIn pages, PostHog, and GoHighLevel. GoHighLevel holds the leads, their statuses, and the Done-Deals that Akarati syncs into it. ADX never talks to Akarati directly.
- **Stores.** History and its own results: scores, temperatures, alerts, proposals and the log of every change it made. Never its own copy of spend, conversions or statuses.
- **Writes.** Its own fields and `adx:` tags in GoHighLevel. Through proposals, changes to Google and Meta campaigns, audiences, and outcome events.

## Principles

These guide decisions when nothing below covers a case.

- **Show the gaps.** ADX puts sync gaps, missing statuses and weak attribution on screen as numbers. Fixing Reportage's own systems is up to them.
- **Honest numbers.** ADX shows "not available" instead of a zero, keeps counted and estimated figures apart, and shows coverage next to any figure that depends on it.
- **Every feature needs a reason to be in ADX.** On the ads side, ADX has to do at least what Ryze does. Ryze (get-ryze.ai) is an AI tool that audits Google and Meta ad accounts, finds wasted spend and applies approved changes. Nothing from Marketing OS comes across just because it already exists.
- **One name per concept.** Marketing OS had two different systems both called "insights", and that confused people.

## Decisions

Each decision says what we chose, why, what it costs us, and what would make us revisit it. When a decision changes, the new one gets the next number and the old one is marked as replaced.

### Scope

#### D-01. ADX is Marketing OS plus lead intelligence
2026-10-09. ADX scores leads and traces spend to Done-Deals. It optimises for deals instead of form fills.

- **Why.** Reporting on clicks and form fills already exists. What Reportage can't see is which campaigns bring buyers.
- **Cost.** ADX's most useful numbers depend on sales data it doesn't control, like statuses and Akarati deals.

#### D-02. Only the marketing team uses ADX
2026-10-09. About 30 people. Sales, telesales and brokers never log in, so Sales OS's rep desk, inbox and assignment don't come across.

- **Why.** Reportage wants a marketing tool. Sales already work in GoHighLevel and Akarati.
- **Cost.** What happens on the sales side only reaches us as GoHighLevel statuses, and half of those are blank (#35).
- **Revisit if.** Reportage wants sales on the same tool.

#### D-03. ADX sends no outreach
2026-10-09. No calls, WhatsApp or email to leads. Messages to staff, like approval links, are fine. Leads that went cold are re-engaged through ad audiences instead.

- **Why.** Outreach is out of scope for Reportage, and they're not open to automated calls.
- **Cost.** The direct re-engagement Sales OS was built for is gone. Tying website visits to leads also gets harder, because ADX never sends a link (#34).
- **Revisit if.** Reportage asks ADX to message leads.

#### D-04. Reportage first, one client
2026-10-02. No multi-tenancy. We avoid Reportage-only shortcuts when a general version costs about the same, so ADX could become a product later.

- **Why.** There's one client today, and multi-tenancy is expensive to build.
- **Revisit if.** A second client signs on.

#### D-05. Ryze is the benchmark for the ads side, not the spec
2026-10-09. ADX has to match Ryze at minimum on the ads side.

- **Why.** Ryze has no CRM, no lead scoring and no link from spend to deals, which is where ADX goes further.

#### D-06. Paid media is Google and Meta
2026-10-09. Portal leads (Bayut, Property Finder, Property Guru) count like any other lead and are reported with spend typed in by hand. ADX doesn't connect to portal accounts.

- **Why.** Portals brought 0 deals in September, and their APIs need partner agreements.
- **Cost.** Portal cost per lead is only as good as the typed-in spend, and there's no sync gap report for portals.
- **Revisit if.** Portals start bringing deals.

#### D-07. ADX reads organic social but doesn't post
2026-10-09. It reads Instagram, Facebook, LinkedIn and Search Console. Posting and scheduling come later, behind approval.

- **Why.** Organic is part of the marketing team's job, and reading it carries no risk. Posting changes public accounts, so it waits until approvals have proven reliable.
- **Open.** Whether TikTok or YouTube count (#34).

### Data

#### D-08. The platforms are the source of truth
2026-10-09. ADX stores history and its own results, never its own copy of spend, conversions or statuses.

- **Why.** A second copy drifts from what the platforms report, and the platforms revise past numbers for weeks.
- **Cost.** ADX depends on each platform's API limits, and figures can change after the fact.

#### D-09. GoHighLevel is the only CRM ADX talks to
2026-10-09. ADX reads leads and statuses from GoHighLevel. It writes only its own fields (`adx_temperature`, `adx_score`, `adx_reason`, `adx_updated_at`) and tags starting with `adx:`, never statuses, pipeline stages or GoHighLevel's own fields. Akarati data reaches ADX only through GoHighLevel. Reportage's monthly lead sheet (one Google Sheet per month, from January 2025) is only for backfilling history.

- **Why.** GoHighLevel is Reportage's marketing CRM, and Akarati already syncs into it. Their GoHighLevel admin builds workflows on ADX's own fields, so ADX writes only those.
- **Cost.** Revenue reporting depends on Akarati sending deal values into GoHighLevel, which we don't control (#35).
- **Revisit if.** Akarati offers an API and GoHighLevel falls short.

#### D-10. Personal data
2026-10-09. Phone and email are hashed before any upload. Opted-out leads never go into audiences. Nationality is never guessed from names, and the phone country code is used only to tell where someone lives. ADX doesn't exclude any country's leads. That's a targeting decision for the CMO.

- **Why.** Guessing nationality is unreliable and touches sensitive personal data, and audiences have to respect opt-outs.

#### D-11. Metrics
2026-10-09. Marketers track cost per qualified lead daily. The CMO and CEO track revenue and return on ad spend monthly.

- **Why.** About 58 deals a month is too few to act on daily, and deals close slowly.
- **Cost.** Cost per qualified lead depends on sales updating statuses (#35).

#### D-12. Hosting in Frankfurt
2026-10-09. Likely Frankfurt.

- **Why.** UAE data centres aren't an option today.
- **Open.** Reportage has to approve it in writing before real lead data moves (#35).

### Writes

#### D-13. Every write goes through a proposal
2026-10-09. A proposal is approved, executed, then logged.

- **Why.** Changing ad accounts is where mistakes cost money.
- **Cost.** Every change is slower than editing in Google Ads directly, and every write needs a dry run and an undo built for it.
- **Open.** Who runs a proposal after approval, whether Allowed actions still create a proposal, and whether the CMO can change caps inside ADX (#34).

Every write follows these rules.

- Google changes run in validation mode first. New Meta objects are created paused and launched as a separate step.
- Nothing is deleted. ADX pauses or archives instead.
- There are hard caps on daily spend increases and on changes per day. The admin sets them, and nothing in ADX can go past them.
- Each account has a kill switch.
- Anything ADX creates gets an `adx_` prefix.
- Proposals expire after 48 hours.
- No change is ever applied twice.

Default permissions for marketers are below. In general, anything that lowers spend or only prepares something is allowed, and anything that raises spend or reaches new people needs approval. Pausing a whole ad set or campaign also needs approval, because it can cut off a large share of leads at once.

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

#### D-14. Three roles, with approvals by link
2026-10-09. The roles are Viewer, Marketer and Admin, and the CMO is the admin. Approvals go out as secure links in email or WhatsApp. The roles are described in `reference.md`.

- **Why.** The CMO won't use a new dashboard.

### Build

#### D-15. ADX looks like Marketing OS
2026-10-01. It uses the Reportage brand, black and white and minimal, instead of the glass look Sales OS used.

- **Why.** Marketing OS already uses the Reportage brand, and we preferred its plain look.

#### D-16. No frontend-only prototype
2026-10-09. We'll scaffold the real app once the tech stack is decided (#34).

- **Why.** We'd rather put the time into the real app than build the screens twice.
- **Revisit if.** We need to test flows with Reportage before building.

#### D-17. ADX reuses the logic of both old products
2026-10-09. Whether it reuses their code depends on the tech stack (#34).

- **Why.** Both products hold logic that took a long time to get right, like the correctness rules, the scoring engine and the proposal model.

From Marketing OS: Google and Meta reporting (61 and about 66 read tools), the background sync and data warehouse, the analyses behind alerts, the proposal model, the copilot, generated reports, the GoHighLevel adapter, GA4 and Search Console reads, and its rules for honest numbers. Marketing OS never built writes to the ad platforms, reads no organic social accounts, and has never run on Reportage's real ad accounts.

From Sales OS: the signal and scoring engine and its readiness score (what ADX calls the intent score), which decays over time and has per-signal caps. Also the logic that ties a browser to a lead, and tracked links that carry a signed per-lead token. Reusing the engine needs three changes.

- Rewrite its detectors, which were written for course pages, for Reportage's project pages.
- Change how it combines the scores. Today it multiplies readiness by fit, so a lead with no fit data scores zero, and every portal lead would end up at the bottom.
- Match visitors to leads by phone and other fields, not only by email (#34).

## Deferred

Wanted, but not in the first version.

- **Automating approved actions.** See D-13. Revisit once proposals have proven reliable.
- **Connecting to portal accounts,** managing portal spend, and portal sync gap reports. See D-06.
- **Organic posting and scheduling.** See D-07.
- **Call tracking for telesales.** Telesales closed the most deals per lead in September, but it needs call tracking on the 800 number (#34).
- **An MCP server,** so other AI clients can use ADX's tools. The copilot comes first.
- **Multi-tenancy.** See D-04.
- **Approval rights for a team under the Head of Marketing & AI,** if they hire one.

## Set aside

| Idea | Why | What would bring it back |
|---|---|---|
| Merging both products in full, including the sales desk | See D-02 | Reportage wanting sales on the same tool |
| Voice calling (an AI caller or a softphone) | See D-03 | A clear yes from Reportage |
| Finding and scraping leads' personal social profiles | It says little about property buyers, finds only some profiles, and breaks Meta's terms | Nothing likely |
| LinkedIn enrichment and course eligibility | Built for Stafford, where eligibility depended on job history | Doesn't apply |
| Zoho | Only ever pointed at another client's data | A client on Zoho |
| Connecting to Akarati directly | See D-09 | Akarati offering an API, and GoHighLevel falling short |
| Ecommerce and app tools (Shopping, product groups, App campaigns, Meta catalogs) | Reportage doesn't sell online | Doesn't apply |
| Events and roadshows | They run through brokers, outside the marketing CRM | Reportage wanting event results in ADX |
| Creative generation | Out of scope. Creative analysis stays in | A clear ask, or a tool we buy |
| Attribution by channel only | ADX aims for campaign level, using match levels | Doesn't apply |
| A frontend-only prototype | See D-16 | Needing to test flows with Reportage before building |
| The Sales OS glass look | See D-15 | Doesn't apply |

## Open questions and outside dependencies

Both are pinned GitHub issues, each a checklist with one line per item.

- **[Open questions (#34)](https://github.com/WokeWorks/adx/issues/34).** The biggest are the tech stack, the `marketing-platform` spec Twaha's tech draft refers to, and how website visits get tied to leads.
- **[Waiting on others (#35)](https://github.com/WokeWorks/adx/issues/35).** Revenue reporting is blocked until Akarati sends deal values into GoHighLevel.

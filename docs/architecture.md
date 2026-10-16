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

#### D-23. GHL through private integration tokens
2026-10-15. A GHL account admin issues a scoped Private Integration Token for ADX. Each sub-account has its own connection. A reconnect replaces only that account's credentials. ADX encrypts tokens in SQLite with a key outside the database.

- **Why.** This avoids Marketplace installation for the first client.
- **Cost.** An admin must replace tokens manually. OAuth support exists in code, but a live installation is unverified.
- **Revisit if.** Customers need to connect their accounts themselves.

#### D-10. Personal data
2026-10-09. Phone and email are hashed before any upload. Opted-out leads never go into audiences. Nationality is never guessed from names, and the phone country code is used only to tell where someone lives. ADX doesn't exclude any country's leads. That's a targeting decision for the CMO.

- **Why.** Guessing nationality is unreliable and touches sensitive personal data, and audiences have to respect opt-outs.

#### D-11. Metrics
2026-10-09. Marketers track cost per qualified lead daily. The CMO and CEO track revenue and return on ad spend monthly.

- **Why.** About 58 deals a month is too few to act on daily, and deals close slowly.
- **Cost.** Cost per qualified lead depends on sales updating statuses (#35).

#### D-12. Hosting in Frankfurt
2026-10-09. Likely Frankfurt. Replaced by D-21.

- **Why.** UAE data centres aren't an option today.
- **Open.** Reportage has to approve it in writing before real lead data moves (#35).

#### D-21. Hosting on our Hostinger server
2026-10-12. ADX runs on WokeWorks' existing Hostinger server in Mumbai, with Caddy in front. This replaces D-12.

- **Why.** The server is already set up, so deploying is a matter of getting the code onto it.
- **Cost.** Reportage's written approval has to name this location before real lead data moves (#35). Hisham bhayya's note on UAE data law only covered transfers to the EU.
- **Revisit if.** Reportage won't approve the location.

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
- **Open.** Whether approvals go by email, Slack or WhatsApp. Email comes first (#34).

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

#### D-18. The stack
2026-10-12. A Go backend and a React frontend in this repo, deployed separately behind one domain. The backend uses chi, SQLite with sqlc and goose, and River for background jobs. The API's TypeScript types are generated from Go structs with guts. The frontend uses Vite, TanStack Router and Query, and shadcn with Tailwind. `CLAUDE.md` has the layout.

- **Why.** About 30 internal users, so one server and one SQLite file are enough, with no database server to run. River commits a job in the same transaction as the change that needs it, so an approved proposal can't lose its execution job. Plain handlers keep every rule visible in code we review.
- **Cost.** No code carries over from the old products, since Marketing OS is Python and Sales OS is TypeScript. SQLite means one machine and one writer at a time, and backups need Litestream. River's SQLite support is still experimental. If ADX serves several clients later, moving to Postgres means rewriting the migrations and some queries.
- **Open.** Login and MCP sign-in are D-24. The copilot's model SDK is also open. Hosting is D-21.
- **Revisit if.** SQLite's single writer becomes a bottleneck, or a second client signs on.

#### D-24. Sign-in through authentik, with people managed in ADX
2026-10-15. People sign in with WokeWorks' authentik on the web and in Claude. ADX keeps the people, their roles and their status. Only accounts in the authentik group `adx-users` can sign in to ADX. Web sessions, MCP tokens and jobs all become the same `core.Actor`, and core checks the role on every call.

- **Why.** authentik already runs and handles passwords and email. Claude connects with one URL only if the server lets Claude register itself. authentik can't do that without a secret, so without a thin OAuth server in ADX every marketer would paste a client ID and secret into Claude. Roles kept in ADX apply on the next click. Creating accounts from ADX means admins don't wait for WokeWorks.
- **Cost.** ADX holds an authentik API token that can create users and change `adx-users` membership, but can't change or delete users. Every new sign-in depends on authentik being up. The OAuth server is security-sensitive code we own. Someone switched off in authentik directly, not through ADX, keeps an open web session until it ends.
- **Open.** Emails come from Twaha's address. A shared sender such as `noreply@wokeworks.ai` is needed before launch (#35).
- **Revisit if.** Reportage wants Google or Microsoft sign-in (#34), a client that can't register from a URL, such as ChatGPT, needs supporting, or ADX gets a second client.

Sign-in follows these rules.

- There's no self sign-up. An admin adds an email and a role. ADX creates the authentik account in the `users/adx` path, adds it to `adx-users`, and has authentik email a link to set a password. The person shows as Invited until their first sign-in, then Active. An admin can resend the link, and anyone can use "Forgot password?" on the login page.
- Sign-in matches only on the authentik account's UUID, which the provider sends as the subject, so nobody gets in by creating an authentik account with someone else's email. The email stays the one the admin entered.
- The admin page reuses only authentik accounts ADX created, which sit in the `users/adx` path. Reusing any account by email would hand someone's access to whoever made an account with their email first. Someone who already has an authentik account that ADX didn't create is refused for now. The only exception is the first admin from `ADX_BOOTSTRAP_ADMIN`, which links by email on an empty install.
- Disabling someone blocks them at once, ends their web sessions and takes them out of `adx-users`. ADX never switches an authentik account off, so its API key can't change accounts and a Reportage admin can't lock a WokeWorks person out of authentik. ADX also refuses an account an authentik admin switched off, instead of switching it back on.
- Adding writes to authentik first and ADX second. Disabling writes to ADX first and authentik second. A failure halfway leaves the person locked out, not half in. Each step skips work already done, so an admin can retry.
- Web sessions live in SQLite. The cookie holds a random ID, and the table stores only its hash, so a copied database or backup holds nothing anyone can sign in with. ADX looks the session up on every request rather than trusting a JWT, a signed token that stays valid until it expires. Disabling someone or changing their role therefore applies on their next click. A session ends after 7 days idle or 30 days at most. Logout ends the ADX session only.
- For Claude, ADX's OAuth server lets Claude register itself from the ADX URL (CIMD), so a marketer adds ADX with one URL. It sends the login to authentik and issues tokens that last 10 minutes and work only for ADX's MCP endpoint. An issued token can't be taken back, so the short life limits how long a removed person keeps access. Every refresh checks the person again in ADX and in `adx-users`.
- Sign-ins, refused sign-ins, changes to people and MCP calls go to one audit log.
- For whoever runs authentik, this setup has to stay as it is: the `adx-users` group bound to the ADX application; the `adx-auth` provider (confidential, subject mode "user's UUID", RS256); the recovery flow without its identification step, with its blueprint off; a separate `forgot-password` flow with "Pretend user exists" on; the `default-recovery-email` stage with the `adx-set-password` template; the `adx-provisioner` service account with only the rights above; and "Allow users to change email" off.

### Order

#### D-19. MCP comes before the web app
2026-10-12. Each feature reaches Claude over MCP before it gets a web screen. The web app gets screens people need daily, and screens for jobs MCP can't do: signing in, connecting accounts, approving changes and settings. This replaces the MCP server's place under Deferred.

- **Why.** Hisham bhayya set MCP and the read layer as the first priority on 2026-10-12. Every feature is a core function first (D-18), so an MCP tool is a thin wrapper and the web screen can follow.
- **Cost.** People who don't use Claude see less at first. Sign-in has to cover MCP from the start, which makes auth a bigger first piece of work.
- **Revisit if.** Reportage's marketers don't use Claude, or Reportage's data approval doesn't cover ADX data reaching Claude.

#### D-20. Google before Meta
2026-10-12. When a feature covers both platforms, Google lands first and Meta follows as its permissions arrive. Meta's Business Verification and App Review are submitted in the first week.

- **Why.** Meta needs App Review for anything beyond `ads_read`, which takes weeks. Google only needs its OAuth app moved to production. This was Hisham bhayya's call on 2026-10-12.
- **Cost.** Meta brought 61% of September's leads and 28 of its 58 deals. Google brought 2 deals. Until Meta access lands, Meta leads are only seen through GoHighLevel, without their form answers or the real source of relabelled leads.
- **Revisit if.** Meta's approval comes through early.
- **Since D-22,** Meta waits on a Standard Access test instead of App Review. Google still goes first in Phase 1, and Meta starts as soon as the test passes.

#### D-22. Meta through Standard Access
2026-10-12. ADX reaches Reportage's Meta accounts through Standard Access. The person at Reportage who manages their ad accounts gets a tester role on our Meta app and connects it once.

- **Why.** Meta gives Business apps Standard Access to their permissions automatically, but only for people with a role on the app. ADX has one client, and one or two people connect Meta, so App Review and Business Verification aren't needed. Meta brought 28 of September's 58 deals, so waiting weeks for review was expensive.
- **Cost.** Anyone who connects Meta needs a role on our app. Standard Access may come with lower rate limits.
- **Revisit if.** The test fails for ads or lead forms, or ADX takes a second client.

## Deferred

Wanted, but not in the first version.

- **Automating approved actions.** See D-13. Revisit once proposals have proven reliable.
- **Connecting to portal accounts,** managing portal spend, and portal sync gap reports. See D-06.
- **Organic posting and scheduling.** See D-07.
- **Call tracking for telesales.** Telesales closed the most deals per lead in September, but it needs call tracking on the 800 number (#34).
- ~~**An MCP server,** so other AI clients can use ADX's tools. The copilot comes first.~~ Replaced by D-19.
- **Multi-tenancy.** See D-04.
- **Approval rights for a team under the Head of Marketing & AI,** if they hire one.
- **The in-app assistant.** With MCP first (D-19), marketers use ADX's tools from Claude, and an in-app chat would rebuild Claude inside ADX. Revisit if people without Claude need it.
- **Website signals and the page catalogue.** Tying visits to leads is still open (#34), and 61% of leads never visit the site. PostHog still goes on Reportage's sites early, so history builds up.
- **Creating campaigns and ads through ADX.** Writes start with budgets, pauses, negative keywords, bids and targets.
- **Editing the permission checklist inside ADX,** with per-person overrides. D-13's defaults are set in code first. Revisit if the CMO asks.
- **Currency conversion.** Reportage's accounts probably bill in AED, and money already carries a currency code. Revisit if an account bills in another currency.

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
| Postgres on a managed host | See D-18. SQLite is enough for about 30 users | Write contention, or a second client |
| Huma and orval for the API contract | Struct tags hide rules that are easy to miss in review. Plain handlers plus guts keep the shared types | Many more endpoints, or a need for a published API spec |
| Embedding the React build in the Go binary | We'd rather deploy the frontend on its own | Needing a single artifact to ship |
| Atlas for generating migrations | Hand-written goose files are enough for now | Table rebuilds in SQLite migrations getting painful |
| Broker monitor | Meta's Ad Library API only returns all ads for the EU. In the UAE it returns political ads only, so there's no source we can use | A data source for UAE ads |
| In-app support access for WokeWorks | We run the server, so it would guard nothing. WokeWorks people get normal accounts and show in the user list | Reportage asking for it |

## Open questions and outside dependencies

Both are pinned GitHub issues, each a checklist with one line per item.

- **[Open questions (#34)](https://github.com/WokeWorks/adx/issues/34).** The biggest are the `marketing-platform` spec Twaha's tech draft refers to, and how website visits get tied to leads.
- **[Waiting on others (#35)](https://github.com/WokeWorks/adx/issues/35).** Revenue reporting is blocked until Akarati sends deal values into GoHighLevel.

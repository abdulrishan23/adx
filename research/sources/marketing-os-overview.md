# Marketing OS: Feature Overview

Marketing OS (repo `WokeWorks/ads-mcp`) connects a company's ad accounts, analytics and CRM, and gives the marketing team dashboards, automated checks and an AI assistant over that data. The first client is Reportage, a UAE real estate company that runs lead generation campaigns on Google and Meta. Most product choices lean toward that use case: leads matter more than online sales, and ecommerce is out of scope.

This document lists every feature in the code, what it is for, and roughly how it works. It is written from the code on `main` at commit `4c641fc` (2026-09-16). "Live" means the feature is merged and switched on in the committed `docker-compose.yml`. The production environment file is not visible, so anything that depends on secrets (email credentials, for example) is marked as such.

## Stack

| Layer | Choice |
|---|---|
| Backend | Python 3.12 monolith. Starlette routes mounted on the MCP SDK's FastMCP, served by uvicorn |
| Frontend | React 19 single page app (Vite, TanStack Query, React Router, Tailwind, ECharts), served from the same origin |
| Database | One Postgres instance for everything: identity, ad data, CRM data, agent runs, schedules, reports. Schema is replayed on boot, no migration tool |
| Background jobs | Two polling Python processes, `sync-worker` and `schedule-worker`. No Redis or Celery. They coordinate through Postgres locks |
| AI | Anthropic by default. Claude Sonnet 5 for analysis, Claude Haiku 4.5 for small tasks (titles, summaries, explanations). An OpenAI compatible provider can be switched in |
| Auth | Self-hosted Authentik behind a custom sign-in page |
| Hosting | Docker Compose on a VPS behind Caddy or Nginx |

`ARCHITECTURE.md` in the repo still describes SQLite. The project has since moved to Postgres.

## Ad platform primer

This section covers enough of Google Ads and Meta Ads to follow the rest of the document. The sources are the official Google Ads and Meta help and API docs.

### Account structure

| Level | Google Ads | Meta Ads |
|---|---|---|
| Container for many accounts | Manager account (MCC). Manages several ad accounts, billing and access from one login | Business portfolio (formerly Business Manager). Holds ad accounts, Pages, Pixels |
| Account | Ad account, with billing | Ad account, owned by the portfolio |
| Campaign | Budget, bidding strategy, locations, networks | Objective. Budget too, but only if campaign budget is switched on |
| Middle level | Ad group: keywords, audiences, default bids | Ad set: budget (by default), audience, placements, optimization goal, schedule |
| Ad | Text, image or video ad, plus keywords under the ad group | The creative |

The practical difference is where the budget sits. On Google it is always on the campaign. On Meta it sits on the ad set unless campaign budget is turned on. Any feature that compares budgets across the two has to handle this.

Google's automated campaign type, Performance Max, has no ad groups or keywords. It uses asset groups (a pool of headlines, images and videos) and Google decides where to show them. Meta's equivalent, Advantage+, is an ordinary campaign with automatic budget, audience and placements (where the ad appears) all switched on.

### Campaign types and objectives

On Google you pick a campaign type, which decides where ads run: Search, Performance Max, Shopping, Display, Video (YouTube), Demand Gen, App. On Meta you pick a business goal, and ads always run on Meta's own apps: Awareness, Traffic, Engagement, Leads, App promotion, Sales.

### Targeting

Google Search is driven by keywords. A keyword is what the advertiser bids on. A search term is what the person actually typed, and the search terms report shows which real searches triggered ads. Match types (broad, phrase, exact) control how loosely a keyword matches searches. Negative keywords block searches you do not want to pay for, and a lot of Google optimization is finding wasteful search terms and adding them as negatives.

Meta has no keywords. It targets people: custom audiences (uploaded customer lists, website visitors tracked by the Meta Pixel, people who engaged), lookalike audiences (people similar to a seed list), and Advantage+ audience, which treats your targeting as a suggestion and expands beyond it. Placements are where the ad appears (Facebook feed, Instagram stories, and so on).

Google also has audience segments. In targeting mode they narrow who sees ads. In observation mode they only add reporting.

### Conversions and attribution

A conversion is an action you count as a result, such as a lead form submission. Each platform credits a conversion to itself if its own ad was clicked or seen within its attribution window, which is the number of days after a click or view during which it still counts the conversion. Neither platform can see the other's ads. One lead who saw a Meta ad and then clicked a Google ad is counted once by each. That is why Google and Meta conversions are never added together in this product.

Default windows differ. Google counts up to 30 days after a click. Meta counts 1 or 7 days after a click and 1 day after a view. Since Apple's 2021 tracking restrictions on iOS, both platforms fill gaps with modeled (estimated) conversions, and Meta's figures lean on modeling more heavily.

Both platforms accept conversions sent from your own server. Google calls it offline conversion import, and it needs the `gclid` click ID captured from the landing page URL. Meta calls it the Conversions API, and it matches using `fbc` (built from the `fbclid` click ID) and `fbp` (a browser ID cookie). This is how CRM outcomes (a lead became a customer) can be reported back to the ad platforms.

### Metrics that exist on one platform only

| Metric | Platform | Meaning |
|---|---|---|
| Impression share | Google | Share of the impressions you were eligible for that you actually got. "Lost to budget" means the budget ran out, "lost to rank" means bids or ad quality were too low |
| Quality Score | Google | 1 to 10 per keyword, from expected click rate, ad relevance and landing page. A diagnostic only |
| Reach | Meta | Number of people who saw the ad at least once |
| Frequency | Meta | Impressions divided by reach. Rising frequency with falling results usually means people are tired of the ad |
| Learning phase | Meta | An ad set is "learning" until it gets about 50 results in a week. Big edits restart it, and results are unstable during it |

### API access for a multi-tenant product

Google retired developer tokens on 2026-09-09. Access level now belongs to the Google Cloud project behind the OAuth app. Production accounts need at least Explorer or Basic access, and unlimited use needs Standard access after a manual review. An OAuth app left in Testing mode gets refresh tokens that expire after 7 days.

Meta only lets an app read other businesses' ad accounts with Advanced Access. That needs Business Verification and an App Review for each permission. Meta's Ad Library API returns every ad only for ads shown in the EU. Elsewhere it returns only political and issue ads, which limits competitor ad tracking outside the EU.

## Core concepts

### Workspaces and connections

An organization holds workspaces, and each workspace has members with the role owner, admin or member. Access is granted per workspace. For an agency, the organization is the agency and each client is a workspace.

A user connects a provider through OAuth. A connection holds encrypted tokens and belongs to the user who made it. One connection can expose several ad accounts, GA4 properties or Search Console sites. Each ad account has two switches: sync (copy its data into the warehouse) and reporting (count it in all-accounts totals).

### Where data comes from

| Source | What it is | Freshness | Cost |
|---|---|---|---|
| Warehouse | Our own copy of Google and Meta performance, one row per campaign per day, in Postgres | Up to 6 hours old | Free to read |
| Live read | A call to the Google, Meta, GA4 or Search Console API at request time | Current | Slower, uses API quota |
| CRM snapshot | Leads copied from Zoho or HighLevel | As of the last connect or "Sync now" | Free to read |
| Saved runs | Stored assistant conversations, tool results, reports | Fixed when created | Free to read |

On the dashboard, data is read live when one account is selected. When all accounts are selected, totals come from the warehouse. GA4 and Search Console are always read live and never stored.

### Tools

A tool is a Python function the AI model can call, for example "campaign performance for this account and date range". It calls a provider API or reads the warehouse and returns rows. The assistant, scheduled runs and the MCP connectors all use the same tool code. The MCP connectors are Model Context Protocol servers that let Claude and other AI clients call these tools from outside the product.

### Where AI is used

Most features are deterministic Python: the analyses, findings, recommendations, pipeline metrics and the executive summary. The AI model is used only in these places: the assistant, scheduled template runs, report and deck writing, the "Explain this insight" button, filling gaps when a schedule is typed in plain English, and chat titles, summaries and follow-up suggestions. Opening a dashboard page never calls the model.

## Feature summary

| Feature | Providers | How it runs | Uses AI | Status |
|---|---|---|---|---|
| Connections and health checks | All six | OAuth, plus a periodic check | No | Live |
| Metric warehouse and sync | Google, Meta | Background job every 6 hours per account | No | Live |
| Currency conversion | Google, Meta | Daily rates fetched by the sync job | No | Live (AED) |
| Performance overview | Google, Meta, CRM | Warehouse or live read | No | Live |
| Campaigns table | Google, Meta | Warehouse or live read | No | Live |
| Source reports | Google, Meta | Live read | No | Live |
| Warehouse findings | Google, Meta | Written by the sync job | Only "Explain" | Live |
| Analyses (Insights and Tables pages) | Google, Meta | Live read plus rules | No | Live |
| Recommendations | Google | Written by the sync job | No | Live |
| Change proposals and Approvals | Google | Filed by the assistant, reviewed by a person | Yes | Live, nothing is applied |
| CRM snapshot and pipeline | Zoho, HighLevel | Copied on connect or "Sync now" | No | Live |
| Live CRM tools and CRM writes | Zoho, HighLevel | Live API | Yes | Live |
| GA4 and Search Console data | GA4, GSC | Live read, assistant and MCP only | Yes | Live, no dashboard page |
| Assistant and templates | All | Model with tool calls | Yes | Live |
| Generated reports and decks | All | Model writes, server fills numbers | Yes | Live |
| Schedules, tasks, email | All | Background job every 60 seconds | Yes | Live, email depends on credentials |
| Home | All | One database read | No | Live |
| MCP connectors | All | Remote MCP servers | In the client | Live |
| Workspaces, auth, audit log | n/a | n/a | No | Live |

## Features

### Connections and health checks

Clients connect Google Ads, Meta Ads, GA4, Search Console, Zoho CRM and HighLevel themselves through OAuth. Tokens are stored encrypted with a master key that can be rotated.

Google and Meta connections are checked as a side effect of syncing. If the provider's API reports that the token is revoked or expired, the connection is marked revoked and Home shows it with a Reconnect button. After three failed syncs in a row the account is marked as failing. GA4, Search Console and the CRMs are not synced, so the sync job runs a cheap test read against each of them at most every 6 hours. Only errors that clearly mean the token is revoked or expired mark them revoked. Quota or permission errors show a softer warning.

Meta connections request only the `ads_read` permission until Meta approves the app. Tools that need more fail with a clear "missing permission" error. Google manager (MCC) accounts appear in the list but cannot be selected, and reaching client accounts through a manager account is not built.

### Metric warehouse and sync

The warehouse is a copy of Google and Meta performance, one row per campaign per day. It exists so that all-accounts views and portfolio totals are fast and free instead of calling every account's API on each page load.

The sync job wakes every 5 minutes and syncs any account that has not synced in 6 hours. Each sync re-downloads the last 7 days and overwrites them, because platforms keep revising conversion numbers for a few days. The first sync backfills 90 days. Failures retry with backoff up to 6 hours. Each run of the sync job also refreshes exchange rates, checks connection health, writes warehouse findings and generates recommendations. There is a manual "Sync now" per account.

Conversions are stored per provider and a check in the code prevents summing them across Google and Meta.

### Currency conversion

With a reporting currency set (AED in the committed config), all money is converted at read time using daily exchange rates. The sync job fetches the day's rates from a free public currency API. A day with no rate is left out of converted totals and a note says so. With no reporting currency set, totals are shown as a subtotal per currency.

### Performance overview

The Overview page shows how the ads are performing. It has three tabs: Acquisition, Pipeline and Business.

Acquisition with all accounts selected shows portfolio totals with change against the previous period, a spend trend, spend by channel, top campaigns, and an account contribution table that compares each account's share of spend with its share of conversions. It reads the warehouse. It also shows the warehouse findings and a card with the status of the latest weekly digest per account.

Acquisition with one account selected reads live. It shows a spend chart with spikes marked, spend, conversions and cost per conversion, a strip of CRM lead counts, a list of wasted spend from the budget waste analysis, an executive summary (Google only, text assembled by code from fixed templates), and pass/fail health checks.

Pipeline shows CRM lead data (see CRM snapshot and pipeline). Business is a placeholder for revenue data that is not connected yet.

An "Investigate" button on most panels opens the assistant with the current account and date range filled in.

### Campaigns table

A table of campaigns with spend, impressions, clicks, conversions and return on ad spend, filterable by status and searchable by name. Clicking a row opens a side panel with the change against the previous period and the top ad groups (Google) or creatives (Meta). With one account selected the table reads live data. With all accounts selected it reads the warehouse.

### Source reports

Seven ready-made live reports for the selected account: search terms and keywords (Google only), devices, geography, placements and conversion tracking (both), and creative performance (Meta only).

### Warehouse findings

A finding is a short, deterministic alert about the portfolio. The sync job writes them to the `insight` table after syncing, comparing the last 7 days with the 7 days before. The types are:

- An account's share of spend is 20 or more points above its share of conversions, or the reverse.
- One account takes 50% or more of spend.
- Spend, conversions, cost per conversion or return on ad spend moved 25% or more.
- Cost per conversion or return on ad spend moved 15% or more while spend stayed within 10%.

Findings are ranked when read, mainly by severity and money at stake. They appear at the top of the all-accounts Overview and on Home, and the assistant can read them. Each has an "Explain this insight" button that sends the stored numbers to Claude Haiku and caches the answer. The explanation text is not checked against the data.

### Analyses (Insights and Tables pages)

The product has 24 analyses. Each is a Python function that reads live data for one account and applies statistical rules. None uses AI. Thresholds can be tuned per analysis.

The Insights page runs the analyses that produce problems to act on and shows them as cards grouped by severity. They run every time the page opens and nothing is stored. Note that this page does not show the warehouse findings above, even though both are called insights in the product.

| Analysis | Google | Meta |
|---|---|---|
| Budget waste | Spend with zero conversions | Spend with zero clicks, or unusually high cost per click (Meta conversion figures are not trusted) |
| Scaling opportunities | Good return, but losing impression share to budget | Above average click rate and most of the daily budget spent |
| Budget reallocation | Yes | No |
| Anomaly detection | Days outside the normal range | Same |
| Budget pacing | Month to date spend projected against budget | Same |
| Creative fatigue | Click rate falling over time on an ad | Same |

There are two pass/fail health checks: conversion drop (both platforms, with link clicks standing in for conversions on Meta) and tracking health (Google only).

The Tables page runs analyses that produce long ranked lists to work through. For Google: negative keyword candidates, wasteful search terms, keyword gaps (search terms that convert but are not yet exact keywords), location and device bid adjustments (raising or lowering bids by a percentage for a location or device type) and landing pages. For both: audience expansion. For Meta: placements.

### Recommendations

A recommendation is a suggested change found by code in stored data. The only generator today finds wasteful search terms per Google account over 30 days and suggests adding them as negative keywords, with the money at stake. It runs in the sync job. Marking one "done" means a person made the change in Google Ads by hand.

### Change proposals and Approvals

The assistant can propose three Google Ads changes: pause a campaign, set a budget, or add negative keywords. The server builds the "before" state from data the assistant actually fetched. Budget changes are limited to plus or minus 50%. A proposal expires after 14 days. Once a proposal is rejected, the same campaign or keyword list cannot be proposed again for 30 days, and each can have only one open proposal at a time. Proposals are visible only to the user who created them.

The Approvals page merges proposals and recommendations into one queue with pending, approved and rejected tabs. Each item shows the problem, the change, the evidence and the expected impact. Approving only records the decision. Nothing is applied to the ad account. Applying approved changes to Google Ads with an undo is planned but not built, and no branch for it exists.

### CRM snapshot and pipeline

Leads are copied from Zoho or HighLevel into Postgres when the client connects and when they press "Sync now". There is no periodic sync. Lead stages are mapped to new, working, engaged, customer, dead and other, and lead sources to channels such as Google Ads, Meta, website and referral. A daily count per stage is kept because CRMs do not keep stage history.

The Pipeline view and the assistant use this snapshot for lead totals, leads by channel, channel quality (active against dead leads), reasons leads are lost, daily lead volume, and cost per lead. Cost per lead divides warehouse spend by CRM leads per channel. It is labeled directional, meaning a rough indicator rather than an exact figure. It shows no value if the dates or currencies do not line up, or if less than 80% of the spend data is present. Attribution is by channel only, because leads do not carry a click ID.

A Zoho sync fails above 8,000 leads (40 pages of 200). HighLevel reads up to about 20,000 contacts.

### Live CRM tools and CRM writes

Five tools let the assistant query the client's CRM directly using their own modules and fields: list connections, describe the schema, read records, query, and write. A per-provider policy limits which paths are allowed and blocks writes to users, roles and settings.

A write is stored as a proposal and shown as a card in the assistant conversation. It does not appear in the Approvals queue. Approving it sends exactly the stored request to the CRM and shows what the CRM returned. Through MCP, the MCP client's own approval counts and the write runs straight away. CRM writes are the only place the product changes a customer system today. Scheduled runs cannot write.

### GA4 and Search Console data

Fourteen GA4 tools cover traffic, channels, landing pages, key events (GA4 conversions) and traffic from AI assistants (ChatGPT, Claude, Perplexity, Gemini, Copilot, DeepSeek, Grok). AI referral traffic is a minimum, because these assistants often strip the referrer. Seven Search Console tools cover search queries, pages, easy wins (queries with 100 or more impressions, 2% click rate or less, position 20 or better), sitemap health and URL inspection. All are read live. They are available to the assistant and over MCP. There is no dashboard page for them.

### Assistant and templates

The assistant is a chat page where the model answers questions by calling tools. Each run is limited to 15 tool calls, 300 seconds and a token budget, with 3 runs at once per user. Answers stream in, show each tool step, and can include charts and tables that the server builds from stored rows. Only 50 rows of any tool result are sent to the model, and the full result is stored. Before each tool call the server checks that the template allows the tool and the user can see the account. The model cannot choose the account or date range.

There are three modes: no account, one account, and portfolio. Portfolio mode answers across every account the user can see, using warehouse data. A question such as "why did leads drop" is routed to a fixed investigation workflow that runs statistical comparisons and returns a finding and a next step.

Numbers in chat text are written by the model with citation markers. A checker compares them with the stored tool results and records a note, but does not block the answer.

Templates are 16 ready-made jobs defined in TOML files, each with its allowed tools and required connections:

| Group | Templates |
|---|---|
| Monitor | Account summary, Anomaly detector, Budget pacing monitor, Check conversion tracking |
| Optimize | Account health audit, Creative fatigue audit, Device optimizer, Geo optimizer, Scale what is working, Search-term cleanup, Find wasted spend |
| Create | Plan a campaign (produces a plan, creates nothing) |
| Report | Pipeline review, Leadership performance deck, Organic traffic overview, Leadership weekly recap |

### Generated reports and decks

Report templates produce an HTML document or slide deck. The model writes the layout and commentary, but every figure is a tag pointing at a stored tool result, such as a metric, a chart or a table. The server fills in the numbers, draws the charts and adds a sources page. If the text contains a number the model was never shown, the server rejects the deck and the model gets up to three tries.

Decks are sanitized, frozen and versioned (up to 20 versions), and listed under Reports. There are three themes plus a logo, and line, bar and change-against-previous-period charts. PDF comes from the browser's print dialog. There is no PowerPoint, Word or spreadsheet export, and a finished deck cannot be edited.

### Schedules, tasks and email

Any template can run once, daily, weekly or monthly at a local time, with daylight saving handled. A schedule can be typed in plain English. Rules parse the common patterns and Claude Haiku fills any fields the rules missed, marked "suggested". A schedule is either report-only or proposal-only. A proposal-only schedule lets an unattended run file proposals into Approvals. Scheduled runs can never make changes. Limits per user are 20 active schedules, 24 runs a day and 2 million tokens a day.

The schedule job checks every 60 seconds for due runs, and each period can run only once. It is switched off by default in code and switched on in the committed compose file.

The Tasks page lists every run, started by a person or a schedule, with its status (running, completed, failed, cancelled, or capped when it hit a limit) and its steps.

When a run finishes, an email with a link (never figures) is queued in the same database transaction. Delivery uses SMTP or Resend if credentials are set, otherwise emails stay queued. With SMTP, a crash partway through a batch can send some emails twice.

### Home

Home shows what needs attention (revoked connections, failing syncs, pending proposals, failed schedules, new findings), running schedules and recent runs, recent reports, and shortcuts to start a template. It is a single database read with no provider calls, and a test enforces that it never calls the model.

A command palette (Ctrl+K) on every page jumps to any other page.

### MCP connectors

The same tools are exposed to Claude and other MCP clients as four remote MCP servers on one domain, sharing one OAuth sign-in through Authentik:

| Server | Tools |
|---|---|
| Google Ads | 61: 41 reporting, 16 analysis, 4 reference. Also 10 prompts and interactive visual panels on 4 tools |
| Meta Ads | About 66 read tools |
| CRM | 5 live CRM tools, including writes |
| Analytics | 21: 14 GA4, 7 Search Console |

The Google Ads server also serves 22 reference documents on reporting standards and audit methods. The in-app assistant does not use them. The connectors have been tested from Claude on the web only.

### Workspaces, auth and audit log

Invites go by email. Self sign-up is off by default and requires a verified email when on. Owners and admins manage members and workspace settings. Organization settings list workspaces and users. Usage and billing are placeholders, and every integration is available to every workspace until billing exists. Tests fail the build if a data query has no workspace filter, or if a new table is not marked as either per-workspace or shared.

The sign-in page drives Authentik's login flow from the server. Browser sessions and MCP tokens are held in memory, so every deploy signs everyone out and MCP clients have to reconnect.

An audit log records sign-ins, connections, invites and role changes. It can only be written. There is no page or API to read it.

## Known limits

- Meta access is `ads_read` only until Business Verification and App Review are done. Lead forms and any Meta write wait on that.
- Accounts reachable only through a Google manager account do not appear.
- Google OAuth is in Testing mode, so connections expire every 7 days. This comes from the original feature doc and cannot be checked from code.
- Attribution is by channel only. Campaign level attribution needs the click ID saved on each lead, which needs a hidden field on the client's web forms.
- CRM data covers leads only, refreshes only on connect or "Sync now", and has the lead caps above.
- Warehouse and finding rows store the syncing user's id in the `workspace_id` column. Other members of the same workspace may not see findings. This is inferred from the queries and not tested.
- Numbers in chat and in "Explain this insight" text are not blocked when unsourced. Only decks and proposals are strictly checked.
- The evidence panel that showed the rows behind an answer is not shown in the current assistant UI.
- There is no billing, no Slack or webhook delivery, no creative library, no competitor ad tracking and no brand profile.

## Corrections to the original feature doc

The original doc (`marketing-os-features.md`) differs from the code in these places:

- Apply and Undo for Google Ads changes is not built. There is no branch for it and no "Changes to ad accounts" setting.
- Schedules are switched on in the committed compose file. Email depends only on credentials.
- CRM writes are live, so the product is not fully read-only. They are approved in the assistant, not in Approvals.
- "Insights" is two systems: warehouse findings written after sync (Overview, Home) and live analyses (Insights page).
- The 8,000 lead limit applies to Zoho. HighLevel's limit is about 20,000.
- Decks already have a change-against-previous-period chart and three themes.
- The Google Ads MCP server has 61 tools, not 59.
- With a reporting currency set, days without an exchange rate are dropped from totals with a note, rather than shown as per-currency subtotals.
- The doc does not mention the investigation workflow, the per-account reporting switch, report-only and proposal-only schedules, or the command palette.

## Roadmap

The original doc's "Future plans" and "Where it should go" sections cover planned work: report exports to PowerPoint, Google Slides, Word and Sheets, editable decks, per-workspace branding, a creative gallery and fatigue alerts, a brand profile and memory, and more integrations (LinkedIn, Microsoft Ads, HubSpot, Slack, property portals, first-party tracking). None of it is in the code yet.

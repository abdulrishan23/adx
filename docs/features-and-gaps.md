# ADX: what is built, what is next, and where the gaps are

Written 2026-10-19 from the code, `docs/reference.md`, `docs/architecture.md`, `docs/screens.md` and the build ledger in `docs/build/`. The market section comes from vendor pages and comparison sites read the same day; every figure a vendor gives about itself is a claim, not a result we checked.

One sentence on state: every capability below runs locally against real core, temporary SQLite databases and fake providers, with its pass conditions as tests, and none of it is live. Nothing has touched a real ad account, a real lead, a phone or a customer, because the credentials, policies and approvals that would let it have not been supplied. The "What live use needs" table in `docs/build/capability-matrix.md` lists them.

## 1. What ADX is

A marketing tool for Reportage, built as three sub-products behind one Home. **Create** takes a project from brand facts to a live campaign. **Perform** keeps money well spent. **Convert** takes leads through the funnel and talks to them. Underneath sit a supervisor, a workflow engine and a policy layer, so that any change ADX wants to make outside itself goes through a proposal a person approves, under caps and a kill switch.

The thing nobody else in the market does, and the reason ADX exists: spend on Google and Meta, by campaign, tied to leads in GoHighLevel, to qualified leads, to Done-Deals in AED, with the match level and coverage shown next to every figure.

## 2. Built

Grouped by door. "Local" means proven by tests in `make test`. "Live needs" is the exact input that would make it real.

### Home and the assistant

| Capability | Local | Live needs |
|---|---|---|
| Home desk: greeting, summary line, attention cards picked by the server, "Talk about this" questions, prompt pills | Yes, cards still from sample fixtures | Metrics engine and model wired to the fronts endpoint |
| Ask ADX: read-only grounded assistant over allowlisted tools; every number from a calculation, a write-shaped request refused; model outage returns a retryable state | Yes | Anthropic API key and the data-use approval for sending aggregated figures to a model |
| Spoken turns: browser speech to transcript, previewed before sending, actions named but never executed | Yes | Nothing |
| Inbox: everything that needs a person, worst first, then updates; snooze and dismiss; Present mode plays the same cards on a wall | Yes, on sample data | Same as Home |

### Create

| Capability | Local | Live needs |
|---|---|---|
| Brands, projects, versioned offers, facts classified approved or inferred, price conflicts held for an admin | Yes | A precedence rule for price and inventory facts |
| Brand kit per project: logos, fonts, colours | Yes | Nothing |
| Research briefs: market, competitor, SEO and customer, from public pages and Google keyword estimates, private addresses refused, every finding sourced | Yes | Google Cloud OAuth client with Google Ads API access |
| Recurring research: refresh runs, monthly comparison reports, hypotheses that stay hypotheses until reviewed | Yes | Nothing |
| Media plans: typed, immutable versions; allocations must total the budget; model-drafted versions pinned to the frame and stripped of uncited figures; diff and revision impact; acceptance grants no spend | Yes | Model key; Reportage's Excel template for the exact-template export |
| Creative production: four sizes, English and Arabic, with and without CTA; house rules (no discount above 15%, every discount shows its condition, logo top centre, Arabic flagged for native review); render as a durable run with provenance | Yes | Nothing |
| Review board: public link, approve or ask for changes, pinned comments; a review belongs to the version it was given on, so an edit clears approval | Yes | Nothing |
| Claims check: creative copy checked against approved facts before readiness | Yes | Nothing |
| Landing pages: server-rendered, scoped forms, test submissions that never count, rollback that names the old version, readiness | Yes | Which host each page family lives on and an external publishing adapter |
| Measurement: events, tracked URLs, controlled checks, launch readiness that joins plan, assets, pages, accounts and checks | Yes | A connected Google Ads or Meta account for the account gate |
| Launch: each plan line resolved to create, update or pick an existing campaign, netting this month's spend off the line; proposals created for the result | Yes | The write gate below |

### Perform

| Capability | Local | Live needs |
|---|---|---|
| Connections: GoHighLevel by private token (OAuth path exists), Google Ads and Meta by OAuth, one account stored once with currency and time zone, health per account, a failing account does not stop the others | Yes, GoHighLevel reads proven against a test location | Google Cloud OAuth client; Meta Business app id and secret |
| Capability matrix: every connector shows what it reads, what it writes under policy and what it never does, with the reason | Yes | Nothing |
| Metrics engine: minor-unit money, Dubai reporting day, source grain, match coverage, formula version, comparisons against previous, custom and year-over-year periods; missing figures are null, never zero | Yes | Live reads |
| Analytics, Campaigns, By project, Revenue, Portals, Marketers screens | Yes, wired to typed results | Live reads; Done-Deal value in GoHighLevel for Revenue |
| Reports and schedules: daily, weekly, monthly, quarterly, with coverage and delivery state, no duplicate occurrence | Yes | SMTP host, sender, recipients |
| Alerts: cost spike, zero-lead day, stale ingestion, pacing, with eligibility rules so stale data is never reported as zero leads | Yes | Thresholds from Abdul and the CMO |
| Proposals: old value, new value, reason; policy check; approval bound to the target version; atomic budget reservation from envelopes; execution outbox | Yes | Caps and write privileges per account in the policy document |
| Execution: Google Ads and Meta write adapters for budget raise or lower and pause; outbox dispatcher with marker reconciliation so a timeout never creates a duplicate; recovery after restart; compensation that never claims spend back; launch timeline and milestones | Yes, against fakes of the documented responses | An account enabled for writes, a test account, `ADX_LIVE_WRITES=1` and written approval |
| Guardrails: policy per actor, workspace, account and action; daily caps; kill switch that proposals, sends, field writes, workflows and calls all obey | Yes | Nothing |
| Data quality: lead coverage, status coverage, attribution coverage | Yes | Live lead import |

### Convert

| Capability | Local | Live needs |
|---|---|---|
| Lead import from GoHighLevel, versioned status-to-stage and project and source mappings, unknown statuses kept and flagged, campaign match confidence recorded | Yes | Private token for the ADX Test location; which GHL field holds the status; the stage table |
| People and enquiries: conservative matching, shared phones never auto-merge, reversible links and merges, enrichment kept apart by class | Yes | An enrichment provider with a contract |
| Signals and scoring: fit and intent under versioned rules, half-life and damping, unknown identity excluded, corrections with history, temperature thresholds; GoHighLevel webhook for stage and message events | Yes | `ADX_WEBHOOK_SECRET` and a GoHighLevel webhook; rule values confirmed by an admin |
| Sales brief with a rule-based next step; objections; drafts quoting approved facts only | Yes | Nothing |
| CRM field outbox: ADX writes its score into fields it owns, tells an echo from an outside edit, conflicts held | Yes | `ghl.lead_fields.set` allowed and `ADX_LIVE_WRITES=1` |
| Outreach adapters: WhatsApp Cloud (plain text in the 24-hour window) and SMTP, sends gated by suppression, channel mode, quiet hours and approval | Yes | Reportage's messaging policy and the credentials |
| Calls: versioned calling policy, contactability checks, atomic reservation so simultaneous requests ring once, unknown results reconciled not retried, signed callbacks, recording retention; Twilio reference transport | Yes | A chosen UAE route authorized in writing, and Reportage's written reversal of "not open to automated calls" |
| Qualification conversations: ElevenLabs agent over the call stream, barge-in, transcript as evidence, reading applied once per version and only when trusted, uncertain figures held, corrections | Yes | ElevenLabs key and an agent set to `ulaw_8000`, plus the route above |
| Native pipeline: one opportunity per enquiry, owned stages, versioned edits that collide instead of overwrite, follow-up tasks, interactions, previewed resumable import from the lead copy | Yes | Nothing |
| Evaluation ledger: one agreed label per call, agreement and correction rates, predictive claims off until 50 labelled calls at 80% agreement | Yes | Labelled calls |

### Workspace and agents

| Capability | Local | Live needs |
|---|---|---|
| Registry of skills, agents and workflows: immutable versions, schema-checked outputs, bounded graphs, admin needed for anything that widens access or was model-written | Yes | Nothing |
| Workflow engine: leases, once-only external steps under an operation key, downstream invalidation on corrected input, bounded loops, token budgets, the only outside effect is a proposal | Yes | Model key for the runner |
| Supervisor: one inbox over every domain, autonomy modes per capability with daily caps (recommend is the default), arbitration by configured precedence or escalation, non-retroactive policy | Yes | Reportage's objective names |
| Learning: advice, decision, action and delayed outcome kept apart; outcomes attach to the intervention whose window covers them; insights promoted only on observed outcomes and a controlled comparison; model agreement counts for nothing | Yes | Nothing |

### Platform and settings

| Capability | Local | Live needs |
|---|---|---|
| Sign-in through authentik OIDC, memberships per workspace, sessions end on disable, dev admin only in dev mode | Yes | `adx-auth` client from WokeWorks |
| Workspaces: full client isolation, same-named projects and provider ids across clients, workspace switching, versioned industry configuration | Yes | authentik for two memberships in a browser |
| Deployment, readiness, encrypted credentials with the key outside the database, backup and linked-state restore, retention pruning, release script that rolls back if readiness fails | Yes | Server access, hosting approval, a retention period |
| Metered model cap server-wide, cost view against budgets, model release evaluation with regression-blocked promotion, load harness | Yes | A model key to have anything to meter |
| Design language: ink on paper tokens, square corners, Outfit headings, collapsible sidebar, status chips, theme toggle | Yes | Nothing |

## 3. In the pipeline

Three kinds of work are queued, in this order of nearness.

### Finishing the app on the engine

The designed screens are being rewired from sample fixtures to the engine's endpoints and types, one screen at a time, with the engine twin removed after each. Screens still carrying a Sample chip today: Home, Inbox, Project, Start a brand, Brand knowledge, Research, Media plans, Landing pages, Launch. Perform and Convert screens are wired. Also open from the pilot gate: pixel export (PNG or JPG) instead of SVG, the canvas drag editor, and a browser pass over the five phase 2 pages.

### Going live

Not a feature, but the largest item. Each row needs a person outside the codebase.

| Input | Who | Unblocks |
|---|---|---|
| Hosting approval naming the server location; server access | Reportage, WokeWorks | Deployment |
| authentik client, redirect URI, groups scope | WokeWorks | Sign-in |
| GoHighLevel private token for the test location; the status field and stage table | Reportage GHL admin, Abdul | Lead import, stages, qualified leads |
| Google Cloud OAuth client with Google Ads API access; Meta Business app | Reportage, WokeWorks | Live reads, keyword ideas, conversion-action checks |
| Caps and write privileges per account; a test account; written approval; `ADX_LIVE_WRITES=1` | Abdul, CMO | Any external write, the launch journey |
| Model API key and data-use approval | Abdul | Assistant, plan drafting, call readings, workflows |
| SMTP host and sender; recipients; anomaly thresholds | WokeWorks, Abdul | Reports, alerts, email outreach |
| Messaging policy and WhatsApp Cloud credentials | Reportage | Sending approved drafts |
| A UAE telecom route and the written reversal on automated calls; ElevenLabs key | Reportage | Qualification calls |
| Excel media-plan template | Reportage | Exact-template export |
| Done-Deal value and date as real fields in GoHighLevel | Akarati | Revenue and return on ad spend |

### Deferred features

Wanted and recorded in `architecture.md`, not started.

- Automated actions: running approved kinds of change without asking each time. Waits on proposals proving reliable.
- Portal connections: pulling Bayut, Property Finder and Property Guru spend and listings, and portal sync-gap reports. Spend is typed in by hand today.
- Organic reporting and posting: Instagram, Facebook, LinkedIn reach and engagement, Search Console. Posting behind approval.
- Website: GA4 traffic, landing page performance, broken or gated pages. PostHog on Reportage's sites so history builds.
- Website signals and the page catalogue: tying visits to leads is still open. 61% of leads never visit the site.
- Call tracking on the 800 number, so telesales leads get a source. Telesales closed the most deals per lead in September.
- Creating campaigns and ads through ADX. Writes today are budgets and pauses.
- Audiences and outcome sync: hashed lists to Meta and Google, qualified leads and Done-Deals back as offline conversions. The actions are named in policy so they can be denied; the adapters are not built because Reportage has not supplied a data-processing basis.
- Stock-aware ads: flagging ads for sold-out unit types from an Akarati availability snapshot.
- Leadership reports packaging the monthly figures for the CMO and CEO.
- Marketer performance by source owner is wired; approval rights for a team under the Head of Marketing & AI are not.
- MCP OAuth server so Claude can use ADX's tools from outside.
- Editing the permission checklist in-app with per-person overrides; currency conversion; a richer pipeline board than stage and owner.

## 4. Who else is solving this

No product on the list does what section 1 describes. Each takes a slice. Grouped by the slice, with what each does and does not do. Vendor figures are the vendor's.

### Autonomous ad optimisation

The ads side ADX has to match at minimum is Ryze (D-05).

| Product | Does | Does not |
|---|---|---|
| Ryze AI | Builds campaigns, writes copy, shifts budgets and pauses waste on Google, Meta, TikTok, LinkedIn and Pinterest; competitor ad library; AI-visibility tracking; SEO fixes deployed and re-measured; Claude and Grok connectors. $89 a month. | CRM, lead scoring, any link from spend to a deal. Claims a 3.2x ROAS lift we could not verify. |
| Madgicx | Meta-first AI audit, audiences, creative analytics, some creative generation. | Google depth, CRM, approval governance. |
| Birch (was Revealbot) | Transparent rules engine on 15-minute cycles across Meta, Google, TikTok, Snapchat. | Creative, CRM, outcomes. |
| Albert.ai | Fully autonomous bids, budgets, targeting across search, social, programmatic, no per-action approval. Enterprise minimums. | Auditability; reviewers say decisions are hard to override granularly. |
| Smartly.io | Enterprise creative volume plus cross-channel buying, CTV and open web. $5k a month and up. | Lead-side anything. |
| Optmyzr, Marin | Single-channel optimisation with recommendations queued for a person. | Cross-channel, CRM. |

### Creative generation

AdCreative.ai, Pencil, Omneky, Creatify, Arcads. They generate static and video variants at volume, score or predict performance, and some publish straight to Meta and Google. None carries house rules such as a discount cap, bilingual layout rules or a version-bound review. None is tied to approved facts.

### Attribution and outcome sync

| Product | Does | Fit for Reportage |
|---|---|---|
| Hyros | Long attribution windows, 30+ CRM integrations, built for high-ticket call-and-CRM funnels. Quote-driven pricing, aimed at $50k a month and up. | Closest in shape, but generic; no GoHighLevel stage logic, no match levels. |
| Cometly, LeadJourney, Ruler Analytics | Offline conversion tracking for lead-gen, deal-stage events pushed back to ad platforms, CRM sync by webhook. | The feature ADX has deferred under Outcome sync. |
| SourceLoop | Real-estate specific: ties leads to the producing channel and sends qualified and closed leads back to Google, Meta and LinkedIn as offline conversions. | Direct overlap with ADX's Outcome sync and Revenue attribution. US residential market. |
| Triple Whale, Northbeam | Ecommerce and media-mix modelling. | Not a fit. |

### Lead follow-up and qualification

| Product | Does | Does not |
|---|---|---|
| GoHighLevel itself | Conversation AI for first-touch on SMS and chat, Voice AI agent for inbound and outbound (beta outside the US), AI decision nodes in workflows, an Ad Manager. | Scoring that explains itself, spend-to-deal, version-bound approval. Reviewers warn Conversation AI "gets creative" with knowledge it should not have. |
| Lofty (was Chime) | All-in-one: IDX site, CRM, ad lead generation, AI agents for engagement and transaction coordination (Lofty AOS, February 2026). From $449 a month. | UAE off-plan, developers rather than agents. |
| Ylopo, Follow Up Boss, CINC | Remarketing and nurture for US agents; case studies on closed deals attributed to remarketing. | Developer marketing, Arabic, portals. |
| Structurely | Text-first qualification within minutes of lead capture, hands warm leads on. | Voice, ads, attribution. |
| Conversica | Conversational sales assistant. No current detail found. | |

### UAE and real estate specific

| Who | What | Why it matters |
|---|---|---|
| Brightcall, SleekFlow, Gallabox, Verloop | WhatsApp-first qualification bots for Dubai brokers: budget, unit type, community, timeline, payment method; hot leads to an agent on WhatsApp within minutes; Arabic and English. DAMAC's Verloop case study claims 2x conversion against lead-gen without messaging. | WhatsApp is the buyer's channel here. ADX has outbound WhatsApp only, no inbound, no templates. |
| Mira Developments | A WhatsApp assistant holding inventory, pricing and floor plans across all projects. | A developer already doing what ADX's approved-facts drafts would do, live. |
| Property Finder, Bayut | Their own AI assistants for buyer enquiries, property matching and market data; TruBroker and Expert programmes. | They own the portal lead and the portal data ADX cannot connect to yet. |
| Dubai Land Department | DLD Investor AI Assistant with Google Cloud: transaction trends, price appreciation, developer track records. | Market data ADX's research briefs do not read. |
| Google Ask Advisor, Meta Advantage+ and Andromeda | The platforms' own agents now do bids, budgets, placements and audience expansion. Ask Advisor reasons across Ads, Analytics and Merchant Center. | Tools that only adjust bids inside one platform are the most exposed. ADX's bets are on cross-platform truth and the lead side, which the platforms cannot see. |

### Not similar products, same problem

- Agencies and in-house teams with Looker Studio, Supermetrics or spreadsheets, stitching platform exports to CRM exports monthly. This is what Reportage does today, and what every competing developer most likely does.
- Marketing automation suites (HubSpot, Klaviyo) that assume the suite owns the CRM and the inbox. Reportage's CRM is GoHighLevel and the lead inbox is WhatsApp.
- "Agentic marketing platforms" (Auxia Agent Studio, Blueshift, Netcore) selling autonomous campaign agents to consumer brands. They answer the autonomy question the other way: fewer approvals, not safer ones.

## 5. The gaps

Three lists: what we have that nobody else does, what others have that we do not, and where nobody is.

### Where ADX is ahead

1. Spend to lead to qualified lead to Done-Deal at campaign level, with match levels, coverage, and null instead of zero. Hyros and SourceLoop come nearest and are generic or US-residential.
2. Governance as the product, not a setting: every outside write is a proposal with old value, new value and reason; approval is bound to the version it was given on; policy is non-retroactive; a kill switch every entry point obeys; a supervisor inbox over all of it. Albert and Ryze act and let you audit later. Optmyzr queues recommendations but has no policy model.
3. Numbers that come from calculations, never from the model, and a predictive-claims gate that stays shut until the evidence exists. No competitor states this rule.
4. Arabic and English creative under house rules with claims checked against approved facts and a review that dies with the version.
5. A native pipeline with explainable fit and intent scoring, corrections with history, and a brief per enquiry. GoHighLevel's scoring is a black box; Lofty's is agent-side.
6. Evidence-gated learning: an outcome attaches to the intervention that preceded it, and model agreement counts for nothing.
7. Multi-client isolation from day one, so a second developer or an agency account costs no re-keying.

### Where others are ahead, or ADX is missing something

Ordered by how much it would matter to Reportage in the first quarter of use.

| # | Gap | Who has it | Why it matters | Where it sits with us |
|---|---|---|---|---|
| 1 | Nothing is live | Everyone | A product proven only against fakes has no users | Section 3, Going live |
| 2 | Inbound WhatsApp: two-way conversation, templates outside the 24-hour window, bilingual first-touch within minutes | SleekFlow, Gallabox, Verloop, Brightcall, GoHighLevel Conversation AI, Mira Developments | WhatsApp is how Dubai buyers want brochures and answers; Reportage runs WhatsApp campaigns by hand; Reportage said no to automated calls, so text-first qualification fits where voice does not | Not built; needs the messaging policy and a template catalogue |
| 3 | Speed to lead | Structurely, GoHighLevel, every UAE bot vendor | The deals are lost between lead arrival and first contact, not for lack of leads | ADX's next-step rule waits a day for a hot lead; no instant first touch |
| 4 | Outcome sync: qualified leads and Done-Deals back to Google offline conversions and Meta Conversions API, with click ids for match quality | SourceLoop, Cometly, Ruler, LeadJourney, any CAPI integrator | It is the only way the platforms learn what a good lead is; without it Advantage+ optimises for form fills | Deferred; blocked on a data-processing basis from Reportage |
| 5 | Audiences from CRM state | Madgicx, Ylopo, Meta and Google natively | Cold-lead remarketing is what Ylopo's case studies attribute most closed deals to | Deferred, same blocker as 4 |
| 6 | Portal connections: spend, listings, lead sync gap | Property Finder and Bayut tools, Lofty for its market | Portal leads are a large share and their spend is typed by hand | Deferred |
| 7 | Revenue is null | Any CRM with a deal value field | ROAS stays "Not available" until Akarati sends value and date | Blocked on Akarati (#35) |
| 8 | Call tracking on the 800 number | Call-tracking vendors, GoHighLevel phone numbers | Telesales closes the most deals per lead and has no source | Deferred |
| 9 | Website and organic: GA4, Search Console, PostHog, Instagram, Facebook, LinkedIn | Ryze, Supermetrics, the platforms | 61% of leads never visit the site, so this is lower; but organic is a marketing team function with no view | Deferred to phase 6 screens |
| 10 | Channel breadth: TikTok, Snapchat, LinkedIn, Pinterest, programmatic | Ryze, Birch, Albert, Smartly | Reportage buys Google and Meta; Snapchat and TikTok are real in the UAE off-plan market | By decision (D-06); revisit when the team spends there |
| 11 | Creative generation: AI image and video, variants at volume, predicted performance | AdCreative.ai, Pencil, Omneky, Ryze, Madgicx, Smartly | Volume of variants is what Andromeda rewards | Not built by decision; template layout only, never labelled as AI. Pixel export and canvas drag editor also open |
| 12 | Publishing ads and posts from the tool | Ryze, Omneky, AdCreative.ai, every suite | Saves the hand-off step | Never, by decision; a reviewed asset is handed to a person |
| 13 | Campaign creation through ADX | Ryze, Albert | Launch today resolves plan lines to proposals; the create write exists for a paused campaign only | Deferred; writes start with budgets and pauses |
| 14 | Automated actions for approved kinds of change | Ryze, Albert, Birch rules | A daily budget nudge should not need a click every morning | Deferred until proposals prove reliable; the autonomy modes already exist |
| 15 | Competitor ad intelligence | Ryze ad library, Madgicx | Knowing what Emaar and Binghatti run | Set aside: Meta's Ad Library returns political ads only for the UAE. No source |
| 16 | Market data: DLD transactions, price trends, developer track records | DLD Investor AI, Property Finder, Bayut | Research briefs read public pages and keyword estimates only | Not planned; a DLD or portal data source would add it |
| 17 | Forecasting and scenarios: ROAS forecast, budget scenarios, media-mix | Northbeam, Madgicx, Ryze | Leadership asks "what if we moved 20% to Meta" | Pacing projection only; predictive claims gated by design until labelled evidence exists |
| 18 | SEO execution and AI-visibility tracking | Ryze | Reportage's site ranks for its own projects; GEO is new | Research only |
| 19 | Keyword research database | Semrush, Ahrefs | Keyword ideas come from Google's planner only | By decision; read the platform, name the source |
| 20 | Buyer-facing tools on landing pages: payment plan and mortgage calculators, inventory and floor plans | Mira's assistant, portal pages, Dubai proptech | Expected on UAE developer pages | Pages have forms only |
| 21 | Enrichment | Clearbit-style vendors, Lofty | Who a +971 lead actually is | Contract exists; no provider chosen |
| 22 | Multi-currency, scheduler UI, drag-and-drop board, per-person permission overrides | Everyone | Convenience | Deferred by decision; cheap when asked |
| 23 | External access for other AI clients (MCP OAuth) | Ryze ships Claude, Grok and Cursor connectors | D-19 said MCP comes first; the OAuth server is not started | Remaining item under B0.3 |

### Where nobody is

White space that ADX could own, ranked by how little extra building each needs.

1. Spend to Done-Deal for off-plan developers, whose cycle runs 30 to 180 days, through GoHighLevel and the portals, in AED and Dubai time. Every attribution tool is built for US residential agents or ecommerce.
2. Governed autonomy: recommend by default, approval per capability with caps, a kill switch, non-retroactive policy, and a learning ledger that only promotes on observed outcomes. The market sells either dashboards or autopilots. Nothing in between with an audit trail.
3. Bilingual creative compliance tied to approved facts, for a market where a wrong price or an unconditional discount in Arabic is a regulatory problem, not a typo.
4. A qualification agent whose reading is evidence first and fact second: uncertain figures held, corrections re-applied with history, quality measured against agreed labels before any claim. Every voice vendor claims qualification; none shows its agreement rate.
5. One supervisor inbox over ads, pages, CRM fields, calls, workflows and conflicts. GoHighLevel has a conversations inbox; nobody has an operations inbox for a marketing team.

## 6. What this suggests

Not decisions. Questions for Abdul and the CMO, in the order the gaps suggest.

1. Push the live inputs in section 3 before any new feature. A connected pilot on one test location and one ad account each on Google and Meta turns the whole of section 2 from "local" to "verified".
2. Decide WhatsApp inbound before voice. Reportage runs WhatsApp by hand and has said no to automated calls. A text-first qualification agent reuses the brief, the approved facts, the scoring and the supervisor, and needs only the messaging policy and a template catalogue. It closes gaps 2 and 3 and is where every UAE vendor is winning.
3. Get the data-processing basis for hashed uploads. It unblocks outcome sync and audiences (gaps 4 and 5), which is the only route to the platforms optimising for qualified leads rather than form fills.
4. Chase Akarati for Done-Deal value and date (gap 7). Without it Revenue and Leadership reports stay "Not available", which is the headline figure the CEO wants.
5. Portal connections and 800-number call tracking are the two deferred items with the largest share of leads behind them (gaps 6 and 8). Decide whether phase 7 starts with them.
6. Leave creative generation, publishing and channel breadth where they are until the team asks. Andromeda rewards creative volume, so expect the ask within a year.

## Sources

Read 2026-10-19. Vendor pages state the vendor's view.

- Ryze: [review](https://www.get-ryze.ai/blog/ryze-ai-review-2026), [changelog](https://www.get-ryze.ai/changelog), [Capterra](https://www.capterra.com/p/10040383/Ryze/), [Adspirer buyer's guide](https://www.adspirer.com/blog/best-ai-ad-management-platform-2026)
- Madgicx, Birch, Smartly: [Superscale on Revealbot](https://superscale.ai/alternatives/revealbot), [Superscale on Madgicx](https://superscale.ai/alternatives/madgicx), [AdManage on Smartly vs Madgicx](https://admanage.ai/blog/smartly-vs-madgicx), [GetApp](https://www.getapp.sg/compare/113221/129791/smartly/vs/madgicx)
- Albert, Optmyzr: [Omniconvert on Albert vs Madgicx](https://www.omniconvert.com/nexus/compare/albert-ai-vs-madgicx/), [Albert vs Smartly](https://www.omniconvert.com/nexus/compare/albert-ai-vs-smartly/), [GTM directory](https://thegtmdirectory.com/tools/albert/md)
- Creative tools: [Superscale on Omneky](https://superscale.ai/alternatives/omneky), [Omniconvert on AdCreative vs Pencil](https://www.omniconvert.com/nexus/compare/adcreative-ai-vs-pencil/), [Superscale on Pencil](https://superscale.ai/alternatives/pencil)
- Agentic platforms: [Blueshift buyer's guide](https://blueshift.com/blog/best-ai-marketing-agent-platform/), [Auxia Agent Studio](https://www.destinationcrm.com/Articles/CRM-News/CRM-Across-the-Wire/Auxia-Launches-Agent-Studio-176289.aspx), [JoinBrands guide](https://joinbrands.com/blog/ai-agents-for-marketing/)
- Attribution: [Cometly on real estate attribution](https://www.cometly.com/post/real-estate-lead-attribution-tracking), [SourceLoop real estate](https://sourceloop.ai/solutions/real-estate/), [Sierra Interactive on ROI](https://www.sierrainteractive.com/glossary-term/real-estate-marketing-roi/), [Hyros vs Triple Whale](https://hyros.com/updates/hyros-vs-triple-whale), [LeadJourney](https://leadjourney.io/comparison/triple-whale), [Eightx on Polar, Triple Whale, Northbeam](https://eightx.co/blog/polar-vs-triple-whale-vs-northbeam)
- Real estate CRM and follow-up: [Ylopo case study](https://www.ylopo.com/success-stories/2020/5/29/45-closed-deals-in-his-first-8-months-with-ylopo-jason-okeefe-canada), [Ylopo Austin ROI](https://ylopo.com/success-stories/email-1-how-one-austin-team-achieved-a-304-roi-with-ylopo), [Perspective on nurture tools](https://getperspective.ai/blog/best-real-estate-lead-nurturing-software-2026-8-platforms-ranked-by-conversation-depth/markdown), [Stork on AI tools](https://www.stork.ai/blog/best-ai-tools-for-real-estate-agents-2026), [Lofty listing](https://thegtmdirectory.com/tools/lofty-inc)
- GoHighLevel: [RSLA updates 2026](https://rsla.io/blog/gohighlevel-updates-2026), [Ecosire AI guide](https://ecosire.com/blog/ghl-ai-automation-guide), [Marc Andrews review](https://marcandrews.com/gohighlevel-review-2026-best-all-in-one-platform-for-uk/), [Callin on GHL AI](https://callin.io/gohighlevel-ai/)
- UAE: [Brightcall on Dubai voice agents](https://brightcall.ai/blog/top-ai-voice-agents-in-dubai-2026-the-best-ai-calling-solutions-for-sales-support), [SleekFlow on Dubai qualification](https://sleekflow.io/blog/ai-lead-qualification-dubai-real-estate), [Gallabox on Dubai WhatsApp](https://gallabox.com/blog/whatsapp-automation-real-estate-dubai), [Gallabox on lead validation](https://gallabox.com/blog/ai-agents-real-estate-lead-validation-dubai), [Mhub on developers using AI](https://www.mhub.my/post/how-property-developers-worldwide-are-using-ai-to-win-leads-faster), [Aigents on Dubai AI](https://aigentsrealty.com/blog/dubai-real-estate-ai-revolution-2026), [Red Spider on 2026 strategies](https://www.redspider.ae/blog/ai-driven-digital-marketing-strategies-for-real-estate-brands-in-2026)
- Platforms: [Miraflow on Ask Advisor](https://miraflow.ai/blog/google-ads-ask-advisor-ai-agent-2026), [eMarketer on agentic buying](https://www.emarketer.com/content/google-agentic-ai-ad-buying-part-of-new-industry-norm), [Tenten on Andromeda](https://medium.com/@tentenco/meta-ads-strategy-2026-why-andromeda-gem-and-ios-26-broke-the-old-playbook-78cba1ad4820), [Bravery on AI optimisation](https://braverytechnology.com/ai-ad-optimisation/)
- Offline conversions: [Converteo on Meta offline CAPI](https://converteo.com/en/blog/lookup-meta-offline-capi-offline-sales-digital-investments/), [AdAdvisor docs](https://adadvisor.ai/docs/learn/offline-conversions), [AiSensy on CAPI for click-to-WhatsApp](https://m.aisensy.com/blog/meta-conversion-api-click-to-whatsapp-ads/), [Adlibrary on offline conversions](https://adlibrary.com/posts/meta-offline-conversions-guide)

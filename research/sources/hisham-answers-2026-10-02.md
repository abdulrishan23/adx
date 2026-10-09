# Hisham bhayya's answers to the ADX questions

Sent on 2026-10-02 in answer to the question list for Rishan. Hisham bhayya generated it with an LLM, so it mixes facts about Reportage with product proposals, and some claims need checking. Those are tracked as open questions. The text below is kept as he sent it, with the tables restored.

---

Adx for Reportage is an AI marketer on top of GoHighLevel, used only by the 30-person marketing team. The one number that matters is revenue from marketing: Done-Deals in AED against ad spend. Sales workflow is out of scope, so questions 15–18 and 21 are dropped.

## Scope and users

Adx is a Reportage-only build for now; multi-tenancy is out of scope. It is still called Adx because we expect Reportage's needs to match the wider market.

| Question | Answer |
|---|---|
| Q1 Who uses it | Marketing only, about 30 people. Sales, telesales and brokers never log in. |
| Q6 Scope beyond paid ads | Organic social is in (all of it). Events run through the broker pipeline, so they are out. Walk-ins are direct leads with no paid source. Creative analysis is in; creative generation is out. Portals (Bayut, Property Finder, Property Guru, Dubizzle) are parked. |
| Q8 Does Adx message leads | No. Outreach is out for Reportage. No calls, no WhatsApp, no email from Adx. |
| Q9 SEO and organic | v1 reads and reports organic Instagram, Facebook, LinkedIn and Search Console, and suggests content. Posting and scheduling are v2, behind approval. |
| Q11 Ryze | Ryze is the benchmark for the ads side, not the spec. Our spec adds CRM outcomes, scoring and real-estate rules on top. |
| Q12 What Reportage has seen | They used the current Marketing OS, a Ryze clone, and gave no useful feedback. Ryze-level parity is the floor. |
| Q35 The one number | Revenue from marketing. The CEO's brief: "you bring me money, you can do whatever with AI." |
| Q15–18, Q21 | Dropped. They are about the sales process. |

## Data flow

Adx never talks to Akarati; outcomes reach it through GoHighLevel. Every write goes back as an approved change, and in GHL only to Adx-owned fields.

## Data sources and system of record

The connected platforms are the source of truth. Adx stores history and its own results (scores, temperatures, insights, logs), never a competing copy of spend or conversions.

| Source | What Adx does with it | Notes |
|---|---|---|
| GoHighLevel | Reads all leads, statuses and outcomes. Writes only Adx-owned fields (see Writes). | The marketing CRM. Akarati syncs into it; Adx never talks to Akarati. GHL is incomplete: its syncs from Meta, Google and the portals drop leads. |
| Meta Ads | Reads campaigns, spend and every lead via the Leads API. Writes audiences, conversions and approved changes. | About 78 active ads. Meta lead pulls also reconcile against GHL. |
| Google Ads | Same as Meta. | One live account (Reportage Felix Properties). A second is closed. Build for several accounts anyway. |
| GA4 | Reads traffic and landing-page performance. | Set up, barely used. Adx becomes the thing that reads it. |
| Search Console, organic social | Reads. | See Scope. |
| PostHog | Ingests all web events via batch export. | To be installed on every site. See scoring. |
| Akarati (via GHL) | Outcomes only, synced into GHL. | Done-Deal with value in AED, plus a daily availability snapshot (project, unit type, units left). |

Q2: the monthly lead sheet is a report, not a system. It is only useful as historical backfill for scoring and reporting.

Q3: paid media is Google and Meta only. Bayut, Property Finder, Property Guru and Dubizzle are parked: zero deals in September, with 180,000 AED budgeted each for the Bayut and Property Finder campaigns.

Q10: GA4 needs Viewer access granted.

Q22: prices and unit types come from the campaigns and Akarati. Availability comes from the Akarati snapshot.

Sync gap report: because GHL is incomplete, Adx shows platform leads versus GHL leads per form, for example "Meta delivered 1,240 and GHL has 1,010." We do not fix their syncs; we make the gap visible.

## How leads arrive and attribution

Most paid leads are Meta instant forms, so attribution is solid for them and weak everywhere else. Each lead records the best match level it reached: click ID, then platform lead ID, then hashed phone/email, then UTM (campaign level only).

| Route | Share (Sept) | ID Adx relies on | Status |
|---|---|---|---|
| Meta instant forms | 2,109 leads (61%) | Meta lead ID + ad/ad set/campaign IDs, pulled from the Leads API and matched to GHL by phone/email | Works without GHL |
| Google ads to website | 465 leads (13%) | gclid/gbraid/wbraid + UTMs | Broken: the website form to GHL sync drops UTMs (known issue, confirmed) |
| Website forms (organic, other) | 151 leads | Same as above | Same issue |
| WhatsApp (website button, ad buttons, form thank-you screen) | 162 leads | Phone match | Partial |
| Telesales (inbound calls to 800 77 552) | 37 leads, 16% close rate | None | Unattributed in v1; call tracking is v2 |

Q4: they run every Google campaign type. Search ads carry a WhatsApp message button. Features depend on type: search-term cleanup only on Search; asset-level only on Performance Max; fatigue and placements on Demand Gen, YouTube and Display.

Q5: Meta ads point to instant forms. Google ads land on reportageoffer.com (Wix) and reportagegroup.com (WordPress). reportageoffer.com's homepage is currently password-gated and a recently advertised path returns 404; check the live final URLs.

Q7: "WhatsApp Camp." is not a source. It is the WhatsApp lead groups: website-button leads plus hot Meta leads reassigned by hand. All 201 September rows belong to the Meta owner. Adx treats it as a handling channel and recovers the true source from the Meta Leads API, so Meta stops being under-credited.

Q14: Telesales means inbound callers from the website number. Generator is the marketer who owns the source (balsam: Meta and WhatsApp groups; Reem: Meta; Abdul: Google, Meta, WhatsApp; Jawad: portals). Brokers are excluded from cost per lead and ROAS but counted as waste. Team and PC are sales-side and ignored.

Q25: web is tracked through PostHog, including WhatsApp and phone clicks. Calls and DMs are not tracked in v1.

Q27: Google Ads tags (AW-10796109704, AW-16730115006), a Meta Pixel (348508161311994) and GTM (GTM-NZN766Z) are installed. The Conversions API exists but is badly set up. Auditing and fixing it is a week-one deliverable.

### Tracking fix list for Reportage's web/GHL admin

- Hidden form fields on every website and landing form: gclid, gbraid, wbraid, fbclid, _fbc, _fbp, utm_source, utm_medium, utm_campaign, utm_content, landing page URL
- Map them into GHL custom fields; fix the sync that drops them
- An original-source field in GHL, set once on arrival and never edited
- Install PostHog on every site through GTM
- One project name list, used in campaign names and the Project field (today: "Wadi" vs "The Wadi", two R.V spellings)
- Fill the Google campaign-type field (T) for every Google lead, or let Adx derive it

## Lead statuses and the revenue event

Done-Deal is the sale and the only revenue event. Its value is always AED; anything else is an import error. Today the value sits in free text under "Telesales Feedback", so the Akarati sync must carry value and currency as real fields.

| Status | Sept count | Adx stage |
|---|---|---|
| (blank) | 1,813 (53%) | Open, never updated |
| No Answer | 596 | Attempted |
| Follow Up | 565 | Engaged |
| Not Interested | 168 | Disqualified |
| Unreachable / wrong number | 96 | Dead (junk) |
| Done-Deal | 58 | Won: revenue |
| Duplicated | 41 | Excluded |
| Meeting | 35 | Meeting |
| Potential | 27 | Engaged, promising |
| Broker | 25 | Excluded, counted as waste |
| Ready To Move | 16 | Disqualified: wants a finished unit; Reportage sells off-plan |
| Sales offer | 4 | Offer sent |
| Low Budget | 4 | Disqualified (fit) |
| No availability | 3 | Disqualified: wanted stock that is sold out (usually studios) |

Half the leads never get a status, so Adx reports status coverage as its own metric. Scores and ROAS are only as good as the updates.

### September 2026 baseline

3,445 leads; directional, since some deals come from earlier months' leads.

| Source | Leads | Done-Deals | Deals per 100 leads |
|---|---|---|---|
| Meta | 2,109 | 28 | 1.3 |
| Google | 465 | 2 | 0.4 |
| WhatsApp | 162 | 6 | 3.7 |
| Website | 151 | 5 | 3.3 |
| Telesales | 37 | 6 | 16 |
| WhatsApp Camp. | 201 | 0 | 0 |
| Property Guru | 113 | 0 | 0 |
| Bayut | 98 | 0 | 0 |
| Property Finder | 80 | 0 | 0 |

The Wadi produced 23 of the 58 deals. By phone code, +971 is 85% of leads (2,936, 43 deals); +91 had 124 leads and 0 deals; +966 had 24 leads and 9 deals.

## Fit and intent scoring

Every lead gets a fit score on arrival from its form answers, then an intent score that moves with behaviour. Together they set its temperature (hot, warm, cold), which Adx writes to GHL. Starting weights are guesses; Adx recalibrates them against Done-Deals.

Fit (Q23), from the Meta instant form (The Wadi example asks all four):

| Input | Options | Weight |
|---|---|---|
| When to buy | This month (launch offer), Within 3 months, Just exploring | Strongest |
| How to pay | Cash (up to 40% off), Payment plan (1% monthly), Mortgage, Not decided | Strong |
| Unit type | 1–2 BR, 3–4 BR, Duplex/Townhouse, Not sure | Checked against availability |
| Purpose | Live, Invest, Both | Segment, light weight |
| Phone country code | Not asked; read from the number | Residency only (+971 vs +91 vs GCC) |

The options lead ("Launch Offer", "Up to 40% Off"), so self-reported urgency is inflated. The outcome data decides how much to trust it.

Budget is not asked. Unit type is the proxy.

Never infer nationality from names. It is unreliable and touches nationality as personal data. Today a marketer does this by hand and drops +91 numbers. Country code gives residency only: Indian buyers living in the UAE have +971 numbers. Where nationality matters (Oceana is GCC nationals only), add a form question: UAE / GCC / Other.

Audience is about 70% Emirati and 30% Indian. Report and compare Arabic/GCC campaigns separately from English/Indian ones.

Intent (Q24): signals that move a lead hotter

- Taps "Message us on WhatsApp" on the form's thank-you screen (match by phone; one lead, not two)
- Submits a second form, or returns to a landing page
- Engages with retargeting ads
- Web behaviour from PostHog, weighted by a page catalogue: each URL maps to project, page type (pricing, payment plan, floor plans, brochure, location, amenities, contact), intent weight and market. Adx drafts the catalogue by crawling and classifying pages; a marketer confirms it
- Brochure downloads, phone or WhatsApp clicks, abandoned forms, three or more projects compared in one session
- Status moving from a nurture state back to Follow Up in GHL

Identity is the hard part. About 61% of leads (Meta instant forms) never visit the site, so web signals only attach when they later click a link that identifies them. Every link marketing sends carries an opaque per-lead token, never phone or email, and never in ad URLs. Adx shows coverage, for example "web signals available for 31% of active leads."

## Writes, guardrails and the permission checklist

Adx writes in v1, to GHL and to the ad platforms, always as proposal → approval → execute → log. Automation comes in v2, once proposals have proven reliable.

Writing to GHL. Adx writes only its own fields and never touches native fields, pipeline stages or anything sales owns: adx_temperature (hot/warm/cold), adx_score, adx_reason, adx_updated_at, plus adx:-prefixed tags. Reportage's GHL admin wires workflows to these fields on purpose, and Adx keeps a list of which workflows listen to them. Writes are batched and rate-limited.

Writing to the ad platforms. Every write follows these rules:

- Logged with who, why, when, old value and new value, and a revert button
- Dry-run first: Google's validation mode; on Meta, anything new is created paused and launched as a separate step
- Never delete; pause or archive only
- Hard caps no one can override from inside Adx: maximum daily spend increase per account, maximum changes per day
- Adx-created objects carry an adx_ prefix
- A kill switch per account stops all Adx writes immediately
- Customer data is hashed before upload; opted-out leads never enter audiences

The permission checklist. Admins set each write action to Allowed, Needs approval or Off, per role, with per-person overrides. The rule behind the defaults: anything that lowers spend or only prepares is allowed; anything that raises spend or reaches new people needs approval.

| Action | Default for marketers |
|---|---|
| Create or update audiences and Customer Match lists | Allowed |
| Upload offline conversions and Conversions API events | Allowed (automatic) |
| Exclusion lists, labels, negative keywords | Allowed |
| Set lead temperature and score in GHL | Allowed (the click is the approval) |
| Pause an ad | Allowed |
| Lower a budget | Allowed |
| Create a campaign or ad (created paused) | Allowed |
| Pause an ad set or campaign | Needs approval |
| Raise a budget within the cap | Needs approval |
| Change bids or target CPA/ROAS | Needs approval |
| Launch anything | Needs approval |
| Change targeting, geography or GCC-only rules | Needs approval |
| Organic posting | Off (v2) |
| Delete | Not available in Adx |

## Roles and approvals

Three roles; the CMO is the admin and approves everything that needs approval (Q34).

| Role | Who | Can do |
|---|---|---|
| Viewer | CEO and leadership | Reports and dashboards only |
| Marketer | The marketing team, including the Head of Marketing & AI | Use the copilot, create proposals, do whatever the checklist allows |
| Admin | The CMO | Approve proposals, manage users, connections, caps, the kill switch and the checklist |

- Approver flag: an admin can give approval rights to a specific marketer without a fourth role. It is off at launch but built from day one, so a slow CMO does not become a bottleneck later.
- No self-approval below the top: a marketer's proposal goes to the CMO. The CMO's own proposals go through, labelled "self-approved" in the log.
- Proposals expire after 48 hours with no action. Spend never changes by default.
- Approvals reach the approver where they are: email or WhatsApp with a secure approve/reject link showing what changes and why. The CMO will not live in a new dashboard.
- WokeWorks support access sits outside the roles: logged, time-limited and visible to the CMO.
- After January: if the Head of Marketing & AI builds a team, he gets the approver flag for them; his own proposals still go to the CMO.

## Performance-manager features

The daily job of a performance manager is the spec (Q29–Q32): Adx opens each morning with what changed and what it would do, each suggestion a ready-made proposal under the checklist.

| Feature | What it does | Source question |
|---|---|---|
| Morning brief | Spend vs pacing, cost per lead and per qualified lead vs 7-day average, yesterday's leads, delivery problems, frequency and CTR drops | Q29 |
| Revenue attribution | Campaign → lead → qualified → Done-Deal, daily and automatic, by project. Most campaigns are single-project; read the project from the campaign | Q28, Q30 |
| Alerts | Pacing, wasted-spend list, search-term cleanup, creative fatigue, anomalies (cost spikes, zero-lead days, disapproved ads), weekly report | Q31 |
| Dead-lead re-engagement | Meta custom audiences and Google Customer Match from No Answer, Not Available and Not Interested leads; re-engagement flips temperature to hot. Excludes wrong numbers, brokers, duplicates and active deals from prospecting | Q19, Q33 |
| Outcomes back to the platforms | Qualified-lead and Done-Deal events to Meta (Conversions API) and Google (offline conversions), so bidding optimises for buyers | Q33 |
| Conversions API audit | Match quality, duplication against the Pixel, which events are sent. Week-one deliverable | Q27 |
| Stock-aware advertising | Flags ads and forms promoting sold-out unit types; reports demand outrunning stock from "No availability" leads | Q22 |
| Sync gap report | Leads in Meta or Google versus leads in GHL, per form | Q2 |
| Broker ad monitor | Brokers (We Care Properties, F A J Properties, Very Intelligent Professionals) run ads on Reportage projects such as Verdana. Adx watches Meta's public Ad Library and reports them | New |
| Per-marketer performance | Generator gives each marketer's sources, so results split by owner | Q14 |

Q32: no feedback on the current Marketing OS, so Ryze-level parity is the floor. Q20 (where leads get lost) is answered by the diagnostics above: sync gaps, dropped UTMs, a possibly broken landing site, broker leads and reassigned Meta leads.

## Access, hosting and data rules

Production connects to Reportage's own accounts, granted by the Head of Marketing & AI, who has full access (Q26).

- Accounts: the live Google Ads account (Reportage Felix Properties), the Meta Business and Page (lead forms need read access for the Leads API), GA4 (Viewer), Search Console, organic social, PostHog and GHL.
- Hosting (Q36): Frankfurt for now; UAE data centres are not an option today. UAE data protection law generally allows transfers to the EU, but get Reportage's written approval before real lead data moves.
- Personal data: phone and email are hashed before any platform upload. No nationality guessing from names. PostHog masks form inputs if session recording is on.
- Who sees which leads: every role sees all marketing leads for now; no rules were given. Revisit if Reportage asks.

## Open dependencies

Revenue reporting is blocked until the Akarati outcome sync lands; everything else can ship around the gaps.

| Dependency | Why it matters | Owner | Status |
|---|---|---|---|
| Akarati → GHL outcome sync: Done-Deal with value (AED) and date | Without it there is no ROAS and nothing to show the CEO | Reportage (Akarati admin) | Needs a named owner and date |
| Daily availability snapshot (project, unit type, units left) | Stock-aware advertising | Reportage | Not started |
| Website form → GHL sync keeps UTMs and click IDs | Google and website attribution | Reportage web/GHL admin | Known issue |
| Original-source field in GHL | Stops manual relabelling from erasing attribution | Reportage GHL admin | Not started |
| PostHog on every site | Web intent signals | Reportage web team (via GTM) | Not started |
| Check reportageoffer.com final URLs | Possible spend on a password gate or 404 | Head of Marketing & AI | To check |
| Written approval for Frankfurt hosting | Data protection | Reportage | To request |
| Nationality question on GCC-only forms | Oceana eligibility without guessing | Head of Marketing & AI | Proposed |
| Status updates on every lead (53% blank in Sept) | Scoring and ROAS accuracy | Reportage sales | Adx reports coverage |

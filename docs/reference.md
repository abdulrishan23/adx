# ADX reference

Who uses ADX, what the shared terms mean, and which features we've considered. The reasons behind these choices are in `architecture.md`. Scope labels (likely, possible, later, set aside) are explained in the root README.

## Users and roles

ADX is used by Reportage's marketing team, about 30 people. Sales, telesales and brokers never log in. There is one client and no multi-tenancy.

| Role | Who | Can do |
|---|---|---|
| Viewer | The CEO and leadership | See reports and dashboards |
| Marketer | The marketing team, including the Head of Marketing & AI | Use the copilot, create proposals, and do whatever the permission checklist allows |
| Admin | The CMO | Approve proposals, and manage users, connections, caps, the kill switch and the checklist |

- An admin can give one marketer approval rights without a new role. This is off at first.
- Nobody below the CMO approves their own proposal. The CMO's own proposals go through, marked "self-approved".
- Every role sees all marketing leads for now. Reportage gave no rules on this.
- WokeWorks people get normal accounts, so they show in the user list with everyone else.

## Concepts

If a feature uses one of these words, it means what's written here.

### Connections

- **Connection.** A credential plus the accounts ADX reads through it (D-26).
- **Credential.** What grants ADX access to a provider: a GoHighLevel private token, an OAuth sign-in or a service account. One Google or Meta sign-in reaches many accounts. A GoHighLevel private token reaches one location.
- **Account.** Something ADX reads through a credential: a GoHighLevel location, a Google Ads account, a GA4 property, a Search Console site, or a Meta ad account or Page. ADX stores each one once, even when several credentials reach it, with its currency and time zone. An ad account a sign-in discovers starts **off**: an admin switches on the ones that belong to the workspace, and only those are read (D-26).
- **GHL connection.** A GoHighLevel sub-account connected with its own private token (D-23). GoHighLevel's API calls a sub-account a location. The token is the credential and the location is the account. Several can coexist. Connecting verifies access. Lead imports and ongoing health checks are separate features.

### Leads and sources

- **Lead.** A person who enquired. Leads live in GoHighLevel, and ADX reads them. There's no single reliable key, because phone numbers repeat and some emails are placeholders, so matching uses several fields.
- **Source.** Where a lead came from: Meta, Google, WhatsApp, the website, telesales (calls to the 800 number on the website), a property portal (a listing site such as Bayut or Property Finder), a walk-in or a referral. The original source should be set once when the lead arrives and never edited.
- **Handling channel.** A bucket marketers move leads into by hand, like "WhatsApp Camp.". It isn't a source. For leads that came from Meta forms, ADX recovers the real source from the Meta Leads API.
- **Source owner.** The marketer responsible for a source. Reportage's data calls this "Generator".
- **Match level.** How sure we are of where a lead came from. From strongest to weakest: click ID, platform lead ID, hashed phone or email, UTM (campaign level only). Each lead records the best level it reached.
- **Identity.** The keys ADX holds for one person: hashed phone, hashed email, click IDs, PostHog visitor ID, GoHighLevel contact ID and later the Akarati ID, each with its match level. Attribution, stage inference and the CRM cutover all read the same table (D-30).
- **Lead reference.** A lead's record in an outside system, stored as the system plus its ID there. A lead has an ADX ID of its own and one reference per system that holds it (D-28).

### Campaigns and money

- **Campaign, ad set, ad.** The Google and Meta objects. Most campaigns are for one project.
- **Project.** A Reportage development, such as The Wadi, Verdana or Oceana. ADX reads it from the campaign and keeps one list of project names, so the same project isn't spelled two ways.
- **Spend.** Comes from the ad platforms. Portal spend is typed in by hand each month.
- **Done-Deal.** A sale, and the only revenue event. Its date is the booking date and its value is the gross unit price, usually in AED and converted at the day's rate when it isn't. A lead can have several. Deals come from Akarati through GoHighLevel, as contact custom fields from 14 November 2026, and from Akarati directly once GoHighLevel is dropped (D-28). Cancellations never reach GoHighLevel, so revenue is reported as booked and says so.

### Pipeline

- **Status.** A lead's status in GoHighLevel. There are about 14. ADX never writes them (D-09). Which GoHighLevel column this is, "Status" or "Lead Status", is open (#34). Telesales fills the ones that matter.
- **Stage.** ADX's own grouping of statuses, used for funnel maths. ADX never writes it back to GoHighLevel, and it's separate from GoHighLevel's pipeline stages. Admins edit the status-to-stage table in Settings, keyed by source system, and ADX flags any status that isn't in it yet.
- **Inferred stage.** The stage ADX works out itself when a lead's status is blank, with a confidence and a reason. Only Follow Up and Potential can be inferred. It comes from what GoHighLevel recorded anyway, the lead form's answers and the lead's behaviour. It counts in qualified-lead figures once the reviewer accepts it or after 14 days of telesales silence, and figures always split "by status" from "inferred" (D-29).
- **Reviewer.** The marketer who accepts, rejects or corrects inferred stages in the Review queue. Today the Head of Marketing & AI.
- **Qualified.** A lead with the status Follow Up, Meeting, Potential or Done-Deal, or an accepted inferred stage of Follow Up or Potential. Whether a hot score should also count is open (#34).

Google and Meta have no lead stages. A conversion there is just an event someone defines, so a qualified lead or a Done-Deal only reaches a platform if ADX sends it.

### Metrics

- **Pacing.** Month-to-date spend projected to the end of the month, against the monthly budget. On Google, the monthly budget is worked out from the campaign's daily budget.
- **Cost per qualified lead.** Spend divided by qualified leads, by campaign, compared with its 7-day average. Marketers check it daily (D-11).
- **Revenue and return on ad spend.** Done-Deal value, and that value divided by spend, by campaign and project. The CMO and CEO check them monthly (D-11).
- **Coverage.** The share of leads that have what a figure depends on, such as a status, a match level or web signals. It's shown next to any figure that depends on it.

Money is in AED, converted at the rate on the day. Google and Meta conversions are never added together, because both claim the same lead. Revenue is as booked, because cancelled deals never reach ADX (D-28).

### Scoring

- **Fit score.** How closely a lead matches what sells. It comes from the form answers (when they want to buy, how they'll pay, unit type, purpose) and from the phone country code. Portal leads have almost none of these.
- **Intent score.** How engaged a lead is now. It goes up or down as signals come in (see Signal below).
- **Temperature.** Hot, warm or cold, set from fit and intent. ADX writes it to GoHighLevel in its own fields.
- **Signal.** A behaviour that changes the intent score, such as a second form, a WhatsApp tap on the thank-you screen, a pricing or floor plan page view, or a status moving back to Follow Up. Website signals only count once a visit is tied to a lead (D-30). Each source has its own signal set, so a lead who never visits the site isn't marked down for it: WhatsApp replies, call outcomes, email opens and repeat enquiries stand in.
- **Tracked link.** A link to the website carrying a signed token for one lead. ADX writes it to the lead's `adx_link` field, GoHighLevel's message carries it, and a tap ties that visitor to the lead. Off until Reportage approves it (D-30).
- **Page catalogue.** A table that maps each website URL to its project, page type, intent weight and market. ADX drafts it by crawling the site, and a marketer confirms it.

A phone country code shows where someone lives, not their nationality. Buyers from India who live in the UAE have +971 numbers (see D-10).

### In the app

- **Alert.** An automated observation about an account, such as a cost spike, wasted spend or a fatigued ad. It can come with a draft proposal.
- **Copilot.** The AI assistant inside ADX. It answers questions from real data and drafts proposals. Every number it states comes from a calculation, never from the model.

### Changes to outside systems

- **Proposal.** A change ADX or a marketer wants to make in Google, Meta or GoHighLevel. It shows the old value, the new value and the reason. The rules are in D-13.
- **Permission checklist.** For each write action and role, one of Allowed, Needs approval or Off.
- **Guardrails.** Caps on spend increases and on changes per day, and a kill switch per account (D-13).
- **Audience.** A hashed list of leads sent to Meta or Google.
- **Outcome event.** A qualified lead or a Done-Deal sent back to Google as an offline conversion, or to Meta through the Conversions API.
- **Sync gap.** Leads a platform delivered that never reached GoHighLevel, counted per form.

## Features

Each row is one feature. The Area column is the Area field on GitHub, and it runs across phases. Ideas we decided against are under Set aside in `architecture.md`.

Phase 1 delivers the read layer through Claude over MCP. Later work includes the ADX web app and its required in-app Copilot (D-25). GA4 and Copilot are in Phase 5. Organic social and Search Console reporting follow in Phase 6.

| Area | Feature | What it does | Scope |
|---|---|---|---|
| Connections | Connections | Connects GoHighLevel, Google Ads, Meta, GA4, Search Console, organic social accounts and PostHog, and shows each one's health and last sync | Likely |
| Morning brief | Morning brief | The home screen. Shows spend against pacing, cost per lead and per qualified lead against the 7-day average, yesterday's leads, and the day's alerts, each with a suggested change where there is one | Likely |
| Revenue attribution | Revenue attribution | Campaign to lead to qualified lead to Done-Deal, by project, updated daily. Computes the monthly revenue and return on ad spend, as booked, from 14 November 2026 (D-28) | Likely |
| Alerts | Alerts | Runs the checks: pacing, wasted spend, search-term cleanup, creative fatigue, frequency and CTR drops, delivery problems, and anomalies such as cost spikes, zero-lead days and disapproved ads. Which checks run depends on campaign type. The Morning brief shows them | Likely |
| Campaigns | Campaigns | Campaign, ad set and ad views with spend, leads, qualified leads, deals and the temperature mix | Likely |
| Proposals | Proposals | Every write to Google, Meta or GoHighLevel goes through proposal, approval, execution and the log. Approvals arrive as links, by email first (#34) | Likely |
| Lead scoring | Lead scoring | Fit and intent scores, a temperature written to GoHighLevel, and weights tuned by hand against outcomes | Likely |
| Audiences | Audiences | Meta custom audiences and Google Customer Match built from leads that went cold. A lead that re-engages turns hot | Likely |
| Outcome sync | Outcome sync | Sends qualified leads and Done-Deals to Google and Meta, and audits Reportage's current Conversions API setup | Likely |
| Data quality | Data quality | Platform leads against GoHighLevel leads per form, status coverage and attribution coverage, and leads flagged as uncontacted | Likely |
| Data quality | Review queue | Leads with a blank status, ADX's inferred stage with its reason and confidence, and bulk accept, reject or correct. Reports precision against the reviewer's corrections and auto-accepts once it holds (D-29) | Likely |
| Portal reporting | Portal reporting | Leads, qualified leads, deals and cost per lead for each portal, with spend typed in by hand | Likely |
| Organic | Organic reporting | Reach, engagement, follower growth and top posts on Instagram, Facebook and LinkedIn, and Search Console data. Claude and Copilot can use the results to suggest content | Likely |
| Website | Website | GA4 traffic and landing page performance, and flags for broken or password-gated pages | Likely |
| Website | Visitor identity | The site snippet spec, hidden fields that carry click IDs into GoHighLevel, tracked links and the nightly match of PostHog visitors to leads (D-30). Snippet and links wait on the agency and on Reportage's approval | Likely |
| Marketer performance | Marketer performance | Results split by source owner | Likely |
| Copilot | Copilot | Answers questions from tool results and drafts proposals inside the web app, in Phase 5 | Likely |
| Copilot | MCP server | Lets Claude use ADX's tools, with the first read layer in Phase 1 (D-25) | Likely |
| Leadership reports | Leadership reports | Packages Revenue attribution's monthly figures into a report for the CMO and CEO | Likely |
| Settings | Settings | Roles, the permission checklist, caps and kill switch, the stage table, the inference points and precision bar, the page catalogue and the project list | Likely |
| Platform | Platform | Deploys, backups, email, monitoring and the web app shell. The work every feature needs that belongs to none of them | Likely |
| Stock-aware ads | Stock-aware ads | Flags ads promoting sold-out unit types, using a daily availability snapshot from Akarati (#35) | Possible |
| Proposals | Automated actions | Runs approved kinds of change without asking each time (see Deferred) | Later |
| Portal reporting | Portal connections | Connects to portal accounts to pull spend and manage listings (see Deferred) | Later |
| Organic | Organic posting | Posts and schedules organic content, behind approval (see Deferred) | Later |
| Data quality | Call tracking | Tracks calls to the 800 number so telesales leads get a source (see Deferred) | Later |

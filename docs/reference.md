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
- WokeWorks support access isn't one of these roles. It's logged, time-limited and visible to the CMO.

## Concepts

If a feature uses one of these words, it means what's written here.

### Leads and sources

- **Lead.** A person who enquired. Leads live in GoHighLevel, and ADX reads them. There's no single reliable key, because phone numbers repeat and some emails are placeholders, so matching uses several fields.
- **Source.** Where a lead came from: Meta, Google, WhatsApp, the website, telesales (calls to the 800 number on the website), a property portal (a listing site such as Bayut or Property Finder), a walk-in or a referral. The original source should be set once when the lead arrives and never edited.
- **Handling channel.** A bucket marketers move leads into by hand, like "WhatsApp Camp.". It isn't a source. For leads that came from Meta forms, ADX recovers the real source from the Meta Leads API.
- **Source owner.** The marketer responsible for a source. Reportage's data calls this "Generator".
- **Match level.** How sure we are of where a lead came from. From strongest to weakest: click ID, platform lead ID, hashed phone or email, UTM (campaign level only). Each lead records the best level it reached.

### Campaigns and money

- **Campaign, ad set, ad.** The Google and Meta objects. Most campaigns are for one project.
- **Project.** A Reportage development, such as The Wadi, Verdana or Oceana. ADX reads it from the campaign and keeps one list of project names, so the same project isn't spelled two ways.
- **Spend.** Comes from the ad platforms. Portal spend is typed in by hand each month.
- **Done-Deal.** A sale, and the only revenue event. Its value is in AED. It should come from Akarati through GoHighLevel, but today the value and date don't arrive as real fields. See Waiting on others in `architecture.md`.

### Pipeline

- **Status.** A lead's status in GoHighLevel. There are about 14. ADX never writes them. Which GoHighLevel column this is ("Status" or "Lead Status") is still open.
- **Stage.** ADX's own grouping of statuses, used for funnel maths. ADX never writes it back to GoHighLevel, and it's separate from GoHighLevel's pipeline stages. Admins edit the status-to-stage table in Settings, and ADX flags any status that isn't in it yet.
- **Qualified.** A lead with the status Follow Up, Meeting, Potential or Done-Deal. Whether a hot score should also count is still open.

Google and Meta have no lead stages. A conversion there is just an event someone defines, so a qualified lead or a Done-Deal only reaches a platform if ADX sends it.

### Metrics

- **Pacing.** Month-to-date spend projected to the end of the month, against the monthly budget. On Google, the monthly budget is worked out from the campaign's daily budget.
- **Cost per qualified lead** by campaign, against its 7-day average. Marketers check this daily.
- **Revenue and return on ad spend** by campaign and project. The CMO and CEO check this monthly.
- **Coverage.** The share of leads that have what a figure depends on, such as a status, a match level or web signals. It's shown next to any figure that depends on it.

Money is in AED, converted at the rate on the day. Google and Meta conversions are never added together, because both claim the same lead.

### Scoring

- **Fit score.** How closely a lead matches what sells. It comes from the form answers (when they want to buy, how they'll pay, unit type, purpose) and from the phone country code. Portal leads have almost none of these.
- **Intent score.** How engaged a lead is now. It goes up or down as signals come in (see Signal below).
- **Temperature.** Hot, warm or cold, set from fit and intent. ADX writes it to GoHighLevel in its own fields.
- **Signal.** A behaviour that changes the intent score, such as a second form, a WhatsApp tap on the thank-you screen, a pricing or floor plan page view, or a status moving back to Follow Up.
- **Page catalogue.** A table that maps each website URL to its project, page type, intent weight and market. ADX drafts it by crawling the site, and a marketer confirms it.

A phone country code shows where someone lives, not their nationality. Buyers from India who live in the UAE have +971 numbers. ADX never guesses nationality from names.

### In the app

- **Alert.** An automated observation about an account, such as a cost spike, wasted spend or a fatigued ad. It can come with a draft proposal.
- **Copilot.** The AI assistant inside ADX. It answers questions from real data and drafts proposals. Every number it states comes from a calculation, never from the model.

### Changes to outside systems

- **Proposal.** A change ADX or a marketer wants to make in Google, Meta or GoHighLevel. It shows the old value, the new value and the reason. It expires after 48 hours.
- **Permission checklist.** For each write action and role, one of Allowed, Needs approval or Off.
- **Guardrails.** Caps on spend increases and on changes per day, and a kill switch per account.
- **Audience.** A hashed list of leads sent to Meta or Google.
- **Outcome event.** A qualified lead or a Done-Deal sent back to Google as an offline conversion, or to Meta through the Conversions API.
- **Sync gap.** Leads a platform delivered that never reached GoHighLevel, counted per form.

## Features

Each feature name is an Area on GitHub.

| Area | What it does | Scope |
|---|---|---|
| Connections | Connects GoHighLevel, Google Ads, Meta, GA4, Search Console, organic social accounts and PostHog, and shows each one's health and last sync | Likely |
| Morning brief | The home screen. Spend against pacing, cost per lead and per qualified lead against the 7-day average, yesterday's leads, delivery problems, frequency and CTR drops, each with a suggested change where there is one | Likely |
| Revenue attribution | Campaign to lead to qualified lead to Done-Deal, by project, updated daily. It computes the monthly revenue and return on ad spend that Leadership reports packages | Likely |
| Alerts | Pacing, wasted spend, search-term cleanup, creative fatigue and anomalies such as cost spikes, zero-lead days and disapproved ads, plus a weekly report. Which checks run depends on campaign type | Likely |
| Campaigns | Campaign, ad set and ad views with spend, leads, qualified leads, deals and the temperature mix | Likely |
| Proposals | Every write to Google, Meta or GoHighLevel goes through proposal, approval, execution and the log. Approvals arrive as links in email or WhatsApp | Likely. Automating approved actions is later |
| Lead scoring | Fit and intent scores, a temperature written to GoHighLevel, and weights recalibrated against outcomes | Likely |
| Audiences | Meta custom audiences and Google Customer Match built from leads that went cold. A lead that re-engages turns hot | Likely |
| Outcome sync | Qualified leads and Done-Deals sent to Google and Meta, plus an audit of Reportage's current Conversions API setup | Likely |
| Data quality | Platform leads against GoHighLevel leads per form, status coverage and attribution coverage | Likely |
| Portal reporting | Leads, qualified leads, deals and cost per lead for each portal, with spend typed in by hand | Likely. Connecting to portal accounts is later |
| Organic | Reach, engagement, follower growth and top posts on Instagram, Facebook and LinkedIn, Search Console data, and content suggestions | Likely for reading. Posting is later |
| Website | GA4 traffic and landing page performance, and flags for broken or password-gated pages | Likely |
| Stock-aware ads | Flags ads promoting sold-out unit types, using a daily availability snapshot from Akarati | Possible |
| Broker monitor | Watches brokers running ads on Reportage projects | Possible |
| Marketer performance | Results split by source owner | Likely |
| Copilot | Answers questions from real data and drafts proposals. Every number comes from a calculation, never from the model | Likely |
| Leadership reports | Packages Revenue attribution's monthly figures into a report for the CMO and CEO | Likely |
| Settings | Roles, connections, the permission checklist, caps and kill switch, the stage table, the page catalogue and the project list | Likely |

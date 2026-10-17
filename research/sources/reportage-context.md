# What we already know about Reportage

Most of this comes from the Marketing OS planning docs in `ads-mcp/.planning/`, which were written while building that product for Reportage. Each section names its source. Where sources disagree, this file says so instead of picking one.

**Updated 2026-10-09.** Hisham bhayya's answers (`hisham-answers-2026-10-02.md` and `hisham-answers-2026-10-09.md`) corrected several points below, and the corrections are made in place. The agreed, current picture is in `docs/`.

## The company

- A UAE real estate developer that sells off-plan property and runs lead generation on Google Ads and Meta Ads. Source: `marketing-os-overview.md`.
- The Marketing OS research calls them an Abu Dhabi off-plan developer. We have been describing them as Dubai based. They probably operate in both. Source: `ads-mcp/.planning/fable_future_deep_reasearch.md` D2.
- Reporting currency is AED. Their site is `reportage.ae`. Source: `marketing-os-overview.md`, Build Plan v4 §1.
- Google ads land on reportageoffer.com (Wix) and reportagegroup.com (WordPress). Source: Hisham bhayya.
- Their leadership includes a CEO, a CMO and a Head of Marketing & AI. The marketing team is about 30 people. Source: Hisham bhayya.
- Build Plan v4 §6 says Rishan confirmed that Reportage's CRM team would add click tracking to lead records. We do not know whether that work happened.

## People and org

What the team knows first-hand:

- Rishan is the WokeWorks CEO. He also contracts at Reportage as their head of AI, works with their upper management, runs at least one of their campaigns, and is a big part of their marketing. He is our only contact there. We talk to Rishan, and Rishan talks to Reportage.
- Twaha, who built Marketing OS, has access to Reportage's lead sheet. It is kept as one sheet per month, and his phase 04 notes profile a 22,099-row export up to July 2026. Source: `ads-mcp/.planning/phases/04-waved-integrations-lead-outcomes/04-CONTEXT.md`.
- As far as we know, Reportage is not open to automated calls.
- GoHighLevel is Reportage's marketing CRM (Hisham bhayya). Marketing OS integrated HighLevel because the Ryze parity brief of 2026-08-26 made Zoho and GoHighLevel P0, but only ever tested it on an empty sandbox sub-account. Source: `ads-mcp/.planning/initiatives/ryze-parity/RYZE-PARITY-STRATEGY.md`, `DEMO-LOG.md`.
- GA4 and Search Console were added to Marketing OS in the same Ryze parity work. No Reportage property was ever connected. GA4 is set up at Reportage but barely used (Hisham bhayya).
- Hisham bhayya is the WokeWorks CTO. He answered the question list on 2026-10-02 and 2026-10-09.

What the docs add:

- Build Plan v4 is written in Reportage's own voice. It says "our enquiry forms", and says Meta development mode covers "any advertising account we administer, which covers all of ours".
- The Marketing OS research says "the boss" named Ryze as the target.
- Build Plan v4 mentions a separate "CRM team" at Reportage.

What we infer from that. These are not confirmed:

- WokeWorks sits inside Reportage's marketing through Rishan. The people who built Marketing OS also run Reportage's ads. Most marketing-side answers come straight from Rishan.
- The Build Plan suggested we administer all of Reportage's ad accounts. Hisham bhayya's answer is that production connects to Reportage's own accounts, granted by the Head of Marketing & AI, who has full access.
- Rishan is probably "the boss" who asked for Ryze.
- Sales and the CRM sit with other teams that Rishan does not run. The sales side is where we have no insider, so Rishan may need to find those answers inside Reportage.

## Where their leads come from

The channel picklist from their lead management system has 14 values. Source: `ads-mcp/.planning/FABLE-ROADMAP-RESEARCH-PROMPT.md:66`.

```
Bayut · Direct Brand · Direct message · Google · Guru · Meta · Property finder ·
Referrel · Telesales · Walk-in · Website · Whats app · whats app Camp. · xploon
```

Grouped by type:

| Group | Sources | Visible to Marketing OS today |
|---|---|---|
| Paid ads | Google, Meta | Yes |
| Property portals | Bayut, Property finder, Guru, xploon | No. "Guru" and "xploon" are unconfirmed, possibly portals or aggregators |
| Messaging | Whats app, whats app Camp., Direct message | No. Click-to-WhatsApp ads carry a referral id that can tie some of these to Meta campaigns |
| Direct and organic | Website, Direct Brand | No |
| Offline | Telesales, Walk-in, Referrel | No |

Hisham bhayya's answers add meaning to some of these. "whats app Camp." is a bucket leads are moved into by hand, not a source. "Telesales" means calls to the 800 number on the website. "Guru" is probably Property Guru (to confirm). Dubizzle is not a separate source. The September 2026 numbers per source are in `hisham-answers-2026-10-02.md`.

Only 2 of the 14 sources are visible to the current Marketing OS. The research doc calls this the biggest strategic gap, because portals are usually the main lead engine for a UAE developer.

## Lead data and the CRM

- Each lead is one row with channel, project, status and contact details. There are two status fields. No row carries a click id (gclid, fbclid) or UTM parameters. Source: Build Plan v4 §6, `ads-mcp/.planning/PROJECT.md`.
- Leads land in GoHighLevel, the marketing CRM, and sales work them there. Akarati is the sales CRM where deals close. It also takes portal leads, and it syncs into GoHighLevel. The monthly Google Sheet is a report, useful only to backfill history. Source: Hisham bhayya.
- The Done-Deal value currently sits in free text under "Telesales Feedback". Source: Hisham bhayya.
- Historically the leads lived in Google Sheets. Source: research prompt.
- The Marketing OS research says Reportage was in the early stages of adopting a CRM called something like "akarathi", most likely Akarati (akarati.com, by Eveorex Technologies in Dubai). Akarati is an off-plan sales and ERP system focused on DLD, Oqood and escrow. No public API or webhooks were found. Its documented data movement is Excel upload. Source: `fable_future_deep_reasearch.md` finding 5.
- As of 2026-10-01 we believed Reportage bought Akarati but hadn't migrated. Hisham bhayya's answers settle it: Akarati is in use as the sales CRM and syncs into GoHighLevel.
- A Google Sheets lead ingestion was built in Marketing OS phase 04 and later rolled back. Source: `fable_future_deep_reasearch.md` D5.

## Attribution

Marketing OS reports in three tiers. Channel exact is available now. Campaign estimated is used only when one campaign was running on a channel when the lead arrived. Campaign exact needs the click id saved on the lead, which needs two hidden fields on their web forms. Source: Build Plan v4 §6.

The click id only exists at the moment of the click. Every lead captured without it can never be tied to a campaign later. Build Plan v4 calls this the only item that gets permanently worse while it waits.

## State of the ad connections

- Meta requests only `ads_read` until Meta approves the app. Source: `marketing-os-overview.md` Connections.
- The only Meta account Marketing OS has been tested against is a zero-spend test account named `reportage_test`. Source: `ads-mcp/.planning/STATE.md:265`.
- On 2026-09-04 the Google Ads credential was revoked and the warehouse was nearly empty. Source: `ads-mcp/.planning/PRODUCT-ISSUES.md` D-3.
- As of 2026-10-02 our Google OAuth app is not approved for production. In Testing mode, connections expire every 7 days.

- Twaha's Google connection held Stafford Global and Inspect Motors under a manager account, not Reportage. Source: `ads-mcp/.planning/debug/google-list-accounts-empty.md`.
- In production, ADX connects Reportage's own accounts: one live Google Ads account (Reportage Felix Properties, a second one is closed), the Meta Business and Page (about 78 active ads), GA4, Search Console, organic social, PostHog and GoHighLevel. Source: Hisham bhayya.

Put together, Marketing OS has not yet run on Reportage's real ad data for any length of time.

## Marketing OS decisions that do not carry over

Marketing OS Build Plan v4 §5 ruled out sending WhatsApp, email and SMS, and writing into the CRM. Those were decisions for Marketing OS. ADX is a different, merged product and does not inherit them.

## Users named so far

ADX's users are now Reportage's marketing team only, about 30 people. Sales users are out of scope. The roles are in `docs/reference.md`. The table below is from the earlier Marketing OS research, and its sales rows no longer apply.

The Marketing OS research describes four users. Source: `fable_future_deep_reasearch.md` D2.

| User | Job | Served today |
|---|---|---|
| Marketing manager | Decides where budget goes. Wants cost per qualified lead by channel and campaign | Partly. No portals, no qualified CPL |
| Performance buyer | Runs Google and Meta day to day. Possibly WokeWorks itself | Best served |
| Sales manager | Lead quality, speed to lead, which sources produce sales | Barely |
| Executive or owner | "For every dirham, how many booked or sold units, and from which channel?" | Not yet |

Sales OS adds a fifth user, the sales rep who works leads every day.

## Industry ranges (not Reportage's numbers)

From secondary sources in the research doc. Useful for realistic prototype data, not as targets. Meta real estate cost per lead runs roughly AED 50 to 300, AED 100 to 500 for off-plan investor products. Lead to sale conversion is commonly 0.5% to 3%. Sales cycles run from weeks to many months.

## Designs we have not seen

Build Plan v4 mentions two earlier designs filled with example data: a "media report" and an "AI Super Agent" workspace. The second listed 59 named specialists, including a WhatsApp qualifier, a 21 day nurture sequence and a form blueprint. Neither design is in either repo. They are worth finding, because they show what was already pitched to Reportage.

# What we know and what we need to ask

**Superseded on 2026-10-09.** Hisham bhayya's answers (`docs/sources/hisham-answers-*.md`) answered almost all of this, and the remaining ones are tracked as open questions. Kept for history.

For each topic, this file separates what the code and the existing docs already answer from what we still need to ask. We ask Rishan, our CEO. Rishan answers what WokeWorks decides and takes the rest to Reportage. Each question is tagged with who most likely answers it.

Sources are the two product repos. `docs/sources/` has the full inventories.

## Ask first

We need ADX out soon. These answers unblock the most work, roughly in this order.

1. **What date are we working to, and what has to exist by then?** A demo for Reportage, a pilot their staff use, or a live product. *Rishan*
2. **Which records does ADX own?** Does ADX replace the Google Sheet as the place leads live, or sit beside the sheet and Akarati and read from them? Everything on the sales side depends on this. *Rishan, then Reportage*
3. **Who at Reportage uses ADX, and how many of each?** Marketing people, sales agents, sales managers, brokers, their leadership. *Reportage*
4. **Do we administer Reportage's Google and Meta ad accounts, or do they?** This decides whether Meta works in development mode or needs App Review first. Also, are their Google accounts under a manager account? Marketing OS cannot reach those yet. *Rishan*
5. **What does a lead go through, in their words, from first contact to sale?** The values in both status fields of their sheet, and their reasons for a dead lead. *Reportage*
6. **Can we sit with two or three of their sales agents and a sales manager for an hour?** At Stafford the signals and scoring worked because they were tuned with the staff. We need the same input here, and no document replaces it. *Rishan to arrange*
7. **Can we get a copy of the lead sheet with personal details removed?** *Reportage*
8. **Is Ryze still the reference, and how closely?** See Product scope. *Rishan*

## Product scope

### Known

- When Marketing OS was planned, "the boss" asked for "what ryze.ai has, as a product". The research prompt calls Ryze "the boss's stated target". We assume the boss is Rishan. Source: `ads-mcp/.planning/FABLE-ROADMAP-RESEARCH-PROMPT.md`.
- Ryze covers marketing only. It has no CRM, no sales desk and no messaging to leads. Its home screen is a queue of proposed changes with money attached, and it applies those changes to the ad accounts. Source: `ads-mcp/.planning/RYZE-SCREENS-2026-09-04.md`.
- ADX is broader than Ryze because it merges in Sales OS. So Ryze can only be the reference for the marketing half.

### Ask

- Is Ryze a checklist for the marketing half, or the shape of the whole product? Do we copy its features one for one, or build our own take with changes? *Rishan*
- What should ADX do that Ryze cannot? Our docs say the answer is joining ad spend to bookings and sales. Does Rishan agree that this is the pitch? *Rishan*
- Is ADX Reportage only for good, or Reportage first and then a product? This decides how generic names and structure should be. *Rishan*
- What has Reportage already been shown or promised, and how does ADX relate to it? *Rishan*

## Ad platforms and integrations

### Known

- Marketing OS connects six things. Two are ad platforms: Google Ads (61 read tools) and Meta Ads (about 66 read tools, which also covers Instagram ads). Two are analytics: GA4 and Search Console, which are read live and have no dashboard page. Two are CRMs: Zoho and HighLevel. Source: `ads-mcp/src/platform_core/connections/catalog.py`, `marketing-os-overview.md`.
- When Marketing OS was planned, the operator stated that Reportage's paid media is Google and Meta only. Source: `FABLE-ROADMAP-RESEARCH-PROMPT.md`.
- Meta requests only `ads_read` until Meta approves the app. Development mode only reads accounts we administer. Reading accounts we do not administer needs App Review, and Business Verification is a separate process. When the review pack was written, the privacy policy and data deletion pages that Meta requires did not exist. Source: `ads-mcp/.planning/META-APP-REVIEW-PACK.md`.
- Google accounts that can only be reached through a manager account do not show up. Source: `marketing-os-overview.md`.
- No property portal is integrated. Bayut and Property Finder have lead APIs behind partner agreements, and both offer manual exports. Source: `ads-mcp/.planning/fable_future_deep_reasearch.md`.

### Ask

- Is it really only Google and Meta for paid ads? Any TikTok, Snapchat, LinkedIn or programmatic? *Reportage*
- Do they pay for Bayut and Property Finder listings, and roughly how much? If yes, do they want that spend in ADX next to Google and Meta? *Reportage*
- Who runs their ads day to day, them, an agency or us? Who would approve changes made from ADX? *Reportage*
- Do they use GA4 and Search Console, and do they care about organic traffic? If not, those integrations stay out. *Reportage*
- Do they use Meta instant forms and click-to-WhatsApp ads? Reading instant form leads needs another Meta permission and another review. *Reportage*
- Can their web team add hidden fields for click ids to the enquiry forms? Without them, cost per lead stays at the channel level. *Reportage*
- Who completes Meta Business Verification and App Review for our app, and when? *Rishan*

## The CRM and what it does in our product

### Known

- An ad platform knows spend, impressions, clicks and the form fills it can see. It does not know what happened to the person afterwards. A CRM holds the leads themselves and what happened to them: contacted, qualified, visited, bought. Leads land in a CRM from many places, not only ads.
- In Marketing OS the CRM is the second half of a join. Leads are copied into `crm_lead` with a canonical stage (new, working, engaged, customer, dead, other) and a channel (Google Ads, Meta, website, referral and others). Cost per lead is ad spend for a channel divided by the CRM leads from that channel. The join is by channel only, because leads carry no click id. Source: `ads-mcp/src/platform_core/crm/model.py`, `crm/services/crm_cost_per_lead.py`.
- The canonical lead has no phone number and no project field. For Reportage, project is central and many leads arrive by phone or WhatsApp. Source: `crm/model.py`.
- The assistant can also query a connected CRM live, and can propose writes to it. A write runs only after a person approves it. Source: `crm/services/crm_write.py`.
- Zoho and HighLevel were added for a Ryze parity demo on 2026-08-31. They are not Reportage's CRM. The Zoho data used before that was Stafford's sandbox. Source: `ads-mcp/.planning/initiatives/ryze-parity/RYZE-PARITY-STRATEGY.md`.
- Nothing reads Reportage's actual leads today. A Google Sheets import was built and later rolled back, and Akarati has no known API.
- Retargeting is a separate thing from the CRM. It means showing ads again to people who visited the site or are on an uploaded list, and it is set up inside Google and Meta as audiences. Marketing OS can read audiences but cannot create or upload them. Sending qualified leads back to Google and Meta so their bidding learns is also not built. It works best with click ids, though both platforms can also match on hashed email or phone.

### Ask

- Have they started using Akarati? For which part: leads, inventory, bookings, after-sale? When will leads move into it? *Reportage*
- If leads move to Akarati, can Akarati give us an API or a scheduled export? *Reportage, possibly via Akarati's vendor*
- Should ADX ever write back into Akarati or the sheet, or only read? *Rishan, then Reportage*
- Do they want ADX to push audiences or qualified leads back to Google and Meta? *Reportage*

## Writes

### Known

- Marketing OS applies nothing to the ad platforms. The assistant can propose three Google changes: pause a campaign, set a budget within 50%, add negative keywords. Approving one only records the decision. A person then makes the change in Google Ads. Meta is read only. Source: `marketing-os-overview.md`, `ads-mcp/src/platform_core/proposals/model.py`.
- Sales OS writes freely to its own records: stage, assignment, notes, tasks, interactions. Reps do this from buttons on the lead page. The assistant can do the same, but each write pauses for a click. Sending a message goes into an approval queue. Sales OS never writes to Zoho. Source: `sales-os-overview.md`.
- So both products already follow one pattern. Records we own get normal buttons. Changes to outside systems and outgoing messages go through a person's approval.

### Ask

- Which changes to Google and Meta should ADX actually make? For example pause, budgets, bids, negative keywords, audiences, creatives. The coming list of missing write features will feed this. *Rishan, then Reportage*
- Who at Reportage is allowed to approve a change that spends money? *Reportage*
- Should writes live in the assistant only, or also have buttons in the UI? This is ours to decide, but Rishan may have a view. The options are below. *Rishan*

### Options for where writes live

1. Assistant only. One guarded path and quick to build. But a sales agent assigns leads and changes stages dozens of times a day, and typing a sentence for each is slow.
2. Buttons for everything. Familiar, but it means rebuilding large parts of the Google and Meta interfaces, which nobody wants.
3. Split by who owns the record. Leads, stages, tasks and messages get normal buttons. Changes to Google, Meta or Akarati become proposals, created by the assistant, by a finding, or by an "Apply" button, and they all land in one approvals queue. Both products already work this way.

Option 3 is my recommendation. The prototype is a good place to test it.

## Sales side: what changes from Stafford

### Known

- The parts of Sales OS that are not specific to Stafford carry over. These are the signal engine, readiness scoring, the sweep, assignment, tasks, notes, interactions, suppression, re-entry, the action centre, approvals, the assistant, WhatsApp and email sending and the inbox. Source: `sales-os-overview.md`, "Stafford-specific vs generic".
- These parts are Stafford-shaped and must be rebuilt or remapped:
  - The course catalog, which becomes projects and possibly units.
  - Eligibility, which becomes a property fit model.
  - The 26 pipeline stages and the dead reasons.
  - The interaction outcomes.
  - The 23 WhatsApp templates, which need new Meta approvals.
  - Every prompt.
  - The website events.
  - The lead fields.
- The score multiplies readiness by fit. A lead with no fit data scores 0. Portal and walk-in leads arrive with little data. Source: `dead-leads-stafford/lib/scoring/readiness.ts`.
- Leads are matched by email, and so are website visitors. Reportage leads often have only a phone number. Source: `dead-leads-stafford/lib/signals/identity-bridge.ts`.
- Voice outreach is out.

### Ask

- Their projects: names, status (launching, selling, handed over), price ranges, unit types, payment plans. Where does this live today? Do they track individual units and availability? *Reportage*
- What makes a lead a good fit in their eyes? Budget, cash or mortgage, investor or end user, timeline, where the buyer lives, nationality, interest in the Golden Visa. This becomes the fit model. *Reportage sales*
- Their pipeline stages and dead reasons, exactly as the sheet has them. *Reportage*
- How sales is organised: team size, by project, language or market, in house agents or brokers, team leads. *Reportage*
- How leads get assigned today, and how fast agents respond. Is there a target? *Reportage*
- How agents message leads: personal WhatsApp, a WhatsApp Business number, email. Which languages do they write in? Would they accept messages sent from a shared business number through ADX? *Reportage*
- What an agent records after talking to a lead. These become the interaction outcomes. *Reportage sales*
- How many dead leads sit in the sheet, and do they want them worked again? That was the original job of Sales OS. *Reportage*
- What a sales manager wants to see about the team each morning. *Reportage*
- Any rules on WhatsApp consent, data storage or who may see which leads? *Reportage*

## Signals

### Known

- A signal does two jobs. It ranks who to contact now, and it brings a cold lead back when they show new intent. Both only pay off when an agent has more leads than they can work by hand.
- Website signals need Google Tag Manager on the client's site, sending events we define, and a way to tie a visitor to a lead. That happens through a form submit, or a tracked link we sent them. Source: `sales-os-overview.md`, Signals.
- The proposed catalog has two kinds of social signal. Personal-profile enrichment (finding and scraping a lead's own Instagram or Facebook) helped Stafford only through LinkedIn and job history. For property buyers it adds little, hit rates are partial, and scraping goes against Meta's terms. Owned-social signals (DMs and comments on Reportage's own pages) arrive by Meta webhook. They start as a platform id, and get tied to a lead later, the same way a website cookie does today. Reading DMs needs Meta messaging permissions. Source: `docs/sources/proposed-reportage-signal-catalog.pdf`.

### Ask

- How many leads does one agent handle at a time? If it is a few dozen, ranking matters less than speed of response. *Reportage*
- Is Google Tag Manager on reportage.ae, and who can add events to it? *Reportage*
- What does someone do on the website before buying? For example price list, payment plan, floor plans, brochure download, comparing projects, clicking WhatsApp. We confirm these with the sales staff. *Reportage sales*
- How much enquiry traffic comes through their Instagram and Facebook DMs and comments? Who answers them today? *Reportage*
- From their experience, what tells them a lead is getting serious? This is the core of the session with sales staff. *Reportage sales*

## Internal to WokeWorks

- Who owns ADX after the prototype, and which backend does the real build start from? *Rishan*
- Will Reportage get a link to the prototype, or will we only show it to them? *Rishan*
- Branding: ADX, Reportage, or both? *Rishan*
- Where are the "media report" and "AI Super Agent" designs mentioned in Build Plan v4? They show what was pitched before. *Rishan*

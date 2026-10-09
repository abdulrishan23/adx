# Unification draft

**Superseded on 2026-10-09.** Hisham bhayya's answers narrowed ADX to the marketing team, which replaces most of this draft. The current picture is in `docs/`. Kept for history.

**Parked on 2026-10-01.** We wait for the list of missing write features from the products ADX wraps around, then pick this up again. Voice outreach has since been ruled out, and the calling rows were removed.

First pass at how Marketing OS and Sales OS become one app for Reportage. This is a hypothesis to argue with. Nothing here is decided. Several parts depend on answers in `questions.md`, and those are marked.

## The loop ADX serves

Neither product covers the whole journey of a lead. Marketing OS sees the money and the first click. Sales OS sees the person after they become a lead. ADX is the first product that sees both ends.

1. Money is spent on Google, Meta, the portals and events.
2. A lead arrives from one of 14 sources.
3. The lead lands in one place. It is de-duplicated and assigned.
4. It is scored on fit and readiness.
5. An agent works it over WhatsApp and email.
6. It moves through Reportage's stages: qualified, site visit, booking, sale.
7. Marketing sees cost per stage by source, project and campaign.
8. Leads that went cold come back when they show new intent.
9. Later, qualified leads and sales are sent back to Google and Meta so their bidding learns.

Marketing OS stops at step 2 and reads a snapshot of step 6. Sales OS starts at step 3 and has no idea what step 1 cost. The demo story should walk this loop end to end, because that is the argument for merging.

## Primitives

These are the nouns both halves of the app share. Getting them right is most of the design work.

| Primitive | Comes from | What changes for ADX |
|---|---|---|
| Project | New | Reportage's developments. Replaces Stafford's course catalog. Ties together campaigns (by a mapping rule), leads (by their project field) and website pages. Unit inventory under a project is an open question |
| Lead | Sales OS `leads`, Marketing OS `crm_lead` | One record per person, owned by ADX if ADX becomes the system of record. Sales OS keys leads on email. In a market where WhatsApp, walk-ins and portals bring leads without an email, the key probably has to be the phone number, with merging |
| Source and channel | Both | Two levels. A channel group (paid ads, portal, messaging, direct, offline) and the source itself (the 14 values). Marketing OS normalises to about 11 channels today. Reportage's own list replaces it |
| Attribution tier | Marketing OS | Every lead carries how sure we are of where it came from: channel exact, campaign estimated, or campaign exact. Shown wherever lead counts meet spend |
| Spend | Marketing OS warehouse | Google and Meta spend from the APIs. New: manual spend entry for portals, events and anything without an API, so cost per lead covers more than 2 of 14 sources |
| Campaign | Marketing OS | Unchanged, plus a link to a project |
| Pipeline stage | Both | One list in Reportage's words, grouped into phases. Sales works the stages. Marketing reports cost per stage. Replaces Stafford's 26 values and Marketing OS's six canonical stages |
| Score | Sales OS | Readiness times fit. Fit was course eligibility. For property it becomes something like budget against project price, purpose and timeline. The multiplication means a lead with no fit data scores 0, which matters because portal and walk-in leads arrive with little data |
| Signal | Sales OS | A conclusion about a person. "Viewed the payment plan twice this week". Rewritten for project pages, price lists, floor plans and payment plans |
| Finding | Marketing OS | A conclusion about money. "Meta spend up 30% while qualified leads fell". Kept separate from signals but shown in the same attention feed |
| Activity | Sales OS | One timeline per lead: website visits, messages, calls, logged interactions, stage changes |
| Proposal | Both | Something the system or the assistant wants to do that needs a person's click. Sales OS: send a message, spend on enrichment. Marketing OS: pause a campaign, change a budget, add negative keywords. One queue, one policy |
| Task | Sales OS | A to-do for a person. Marketing OS uses "Tasks" for agent runs. ADX gives the word to people and calls agent runs "runs" |
| Assistant thread and template | Both | One assistant. Sales OS tools and Marketing OS tools in one registry, under Sales OS's rule that the assistant never reaches more than the person using it |
| Report | Marketing OS | Generated decks and recaps. Adds sales views: team performance, speed to lead, pipeline by project |
| Role | Both | Sales agent, sales manager, marketing, leadership, admin. The role decides the home screen and the nav. No workspaces or organisations, because ADX serves one client |

## Feature map

### Merge into one

| ADX feature | Built from | Notes |
|---|---|---|
| Home | Sales OS dashboard and action centre, Marketing OS Home | One role-aware home. Sales OS already bands its feed into decide, answer, chase and watch. That also settles Marketing OS's open question about connection errors sitting next to spend findings in one list |
| Approvals | Sales OS `pending_actions`, Marketing OS proposals and recommendations, assistant CRM writes | One queue. Today Sales OS proposals go to the lead's rep and expire in 24 hours, and Marketing OS proposals are private to whoever made them and expire in 14 days. ADX needs one rule for who sees what and when it expires |
| Assistant | Both assistants | Templates and decks from Marketing OS. Tool cards, inline write approvals and role-bounded actors from Sales OS |
| Attention feed | Sales OS signals, Marketing OS findings and insights | Two kinds of item in one feed, each with its own detail view |
| Leads and pipeline | Sales OS leads list and lead page, Marketing OS Pipeline tab | The lead list and lead page are where sales works. The pipeline report reads the same records |
| Automations | Marketing OS schedules, Sales OS sweep, signal chains and automation canvas | Recurring reports and digests sit beside the rules that react to signals. Possibly one page, possibly two |
| Settings and integrations | Marketing OS Integrations and Settings, Sales OS Inflow and admin pages | One place for ad accounts, lead sources, messaging channels, team, projects, templates and AI settings |

### Keep from one side

| Feature | From | Reframed for Reportage |
|---|---|---|
| Acquisition overview | Marketing OS | Spend and results across Google and Meta, plus manual spend for other sources |
| Campaigns | Marketing OS | Unchanged, with the project column |
| Insights and Tables | Marketing OS | The research doc suggests folding Tables into the assistant. Worth testing in the prototype |
| Generated reports | Marketing OS | Add a weekly leadership digest that joins spend to bookings |
| Lead page | Sales OS | Brief, score, timeline, notes, tasks, interactions, outreach dock. Fit replaces eligibility |
| Outreach inbox | Sales OS | WhatsApp first. Email only if Reportage uses it. No calling |

### New for ADX

| Feature | Why |
|---|---|
| Projects | Every view needs to filter and group by project |
| Full funnel by source and project | Spend to lead to qualified to visit to booking to sale. The one number Reportage's owner is said to want |
| Lead capture from every source | Sheet import, portal exports, Meta lead forms, click-to-WhatsApp, a quick-add form for walk-ins and telesales |
| Manual spend entry | So portals and events get a cost per lead |
| Speed to lead | Real estate buyers usually go with whoever answers first. Needs confirming with Reportage |
| Phone-first identity and merge | See the Lead primitive |

### Leave out of the prototype

- Workspaces, organisations and the org settings pages. One client.
- MCP connectors. Real, but not something a Reportage user sees.
- LinkedIn enrichment and course eligibility. Property buyers are not judged on job history. Enrichment might come back in a different form.
- The demo console and `/inflow-next`.

## Navigation options

Three ways to organise the app. Picking one is the first prototype decision after the stack.

1. **Two halves.** A Marketing section and a Sales section, with Home, Assistant and Approvals shared. Easiest to build from the existing products. The risk is that it feels like two apps under one logo, which is the thing we are trying to avoid.
2. **Along the funnel.** Spend, Leads, Conversations, Outcomes. Marketing and sales views are lenses on the same records. The most coherent and the hardest to get right.
3. **By role.** Each role gets its own nav, and the same screens appear in different places. Matches how people work, but every screen has to make sense from several angles.

My lean is a funnel-shaped structure with role-aware homes, so a mix of 2 and 3. It is worth sketching all three as clickable nav shells before choosing.

## What depends on leadership

| If they say | Then |
|---|---|
| ADX holds the leads | We design a CRM for agents. Lead capture, dedupe and stages are core screens |
| ADX sits beside the sheet or Akarati | Leads are read mostly. Outreach and signals still work, but stage changes happen elsewhere |
| WhatsApp only | The inbox becomes a WhatsApp inbox and email drops out |
| They track units | Projects get an inventory, and fit can match budget to available units |
| Brokers matter | We need a broker entity and probably a broker view |

## A demo story to aim for

One Monday at Reportage, told through four people.

- The marketing manager opens Home. Meta's cost per lead on one project looks fine, but its qualified rate fell by half. Bayut leads for the same project convert to site visits three times better.
- The sales manager sees 40 new leads from six sources overnight. Three are unassigned and two have waited past the response target.
- An agent sees that a lead who went cold last year came back to a project page and viewed the payment plan twice. ADX has drafted a WhatsApp. The agent approves it, gets a reply and books a site visit.
- The CEO gets the weekly digest. For every dirham, how many leads, visits and bookings came from each source.

All numbers in the story are made up for the prototype.

# Hisham bhayya's answers to the follow-up questions

Sent on 2026-10-09 in answer to the follow-ups on the first round (`hisham-answers-2026-10-02.md`). Generated with an LLM, like the first round, so some points need checking. Those are tracked as open questions. The text below is kept as he sent it, with the tables restored.

---

The second round of dev questions, answered. "v1" throughout this doc means the Adx we are building for Reportage, not the current Marketing OS.

| Question | Answer |
|---|---|
| Does v1 read organic metrics? | Yes: Instagram, Facebook, LinkedIn and Search Console (reach, engagement, follower growth, top posts), plus content suggestions. Posting is v2. The current Marketing OS does not read organic, so this is new build. Apply for LinkedIn's Community Management API early; approval can take weeks. |
| What are CRM outcomes, and what is scored? | Outcomes are what happened to a lead in GHL: its status and, for a Done-Deal, the value. Scoring is lead scoring: fit (form answers) plus intent (behaviour) gives hot, warm or cold, written to GHL. It is a marketing OS that optimises for Done-Deals instead of form fills, not a sales tool. |
| Which metric matters most in the app? | Daily, for marketers: cost per qualified lead by campaign against the 7-day average (qualified = Follow Up, Meeting, Potential, Done-Deal, or a hot score). Monthly, for the CMO and CEO: revenue and ROAS by campaign and project. Deals are too few (58 in Sept) and too slow to steer by day to day. |
| GoHighLevel vs Akarati? | GHL is the marketing CRM: leads land there from Meta, Google and web forms, and its workflows run there. Akarati is the sales CRM where deals close; it also takes portal leads and syncs into GHL. Adx only needs the Akarati fields that arrive in GHL. Demos: a GHL trial account to build against, and a 30-minute walkthrough from Reportage's Akarati admin. |
| Are the reportageoffer.com ads broken? | Unknown, probably not. The homepage is password-gated and one path returns 404, but Google normally disapproves ads whose landing page fails. Check the landing-pages report in Google Ads once we have access. The dropped UTMs are a separate, confirmed problem. |
| What are the WhatsApp lead groups? | "WhatsApp Camp." is a label in the lead sheet, not an ad campaign. It holds website WhatsApp-button leads plus hot Meta leads marketers move there by hand, which hides their real source. Adx recovers it from the Meta Leads API. |
| Telesales, and only 4 marketers? | Telesales means phone calls to the 800 number on the website; WhatsApp taps are a separate source. The team is about 30; the four names are the source owners in the September sheet, so per-marketer performance covers them. |
| Is PostHog set up? | No. Today there are Google Ads tags, the Meta Pixel and GTM. PostHog is installed through GTM as part of the fix list (Q25 corrected). |
| What is the Adx stage column? | A new read-only mapping for funnel maths, not taken from the current Marketing OS. Several of the 14 GHL statuses mean the same thing for the funnel; the reason is kept. No drift: Adx never writes status, GHL stays the source of truth. Build it as an editable config table that flags unmapped statuses. |
| Are +91 numbers blacklisted? | No. One marketer drops them by hand; Adx does not. September had 124 +91 leads and 0 deals, too few to act on. Country code is one fit input whose weight is learnt from deals. Excluding India from targeting would be a CMO decision. |
| Can we change the funnel to identify leads better? | We propose; the Head of Marketing & AI applies. Low risk, already in the fix list: hidden click-ID and UTM fields, an original-source field, PostHog, per-lead link tokens. Form-question changes (a UAE / GCC / Other question, a budget question, less leading options) cut volume, so they need CMO approval and a test on one campaign first. |

## Portal leads

Portal leads count in Adx like any other lead; only managing portal spend is parked. "Parked" means Adx does not connect to the portal accounts in v1, not that their leads are ignored.

| Portal | Sept leads | Done-Deals | How it reaches GHL |
|---|---|---|---|
| Property Guru | 113 | 0 | To confirm |
| Bayut | 98 | 0 | Akarati, then GHL |
| Property Finder | 80 | 0 | To confirm |
| Dubizzle | Not a separate source in the Sept sheet | — | To confirm |

What Adx does with them in v1:

- Reads them from GHL, labelled by portal, with Jawad as generator
- Reports leads, qualified leads, Done-Deals and cost per lead per portal. Portals have no ads API we connect to, so the monthly cost is entered by hand (for example 180,000 AED each for Bayut and Property Finder)
- Scores them on intent and phone code only: portal enquiries carry no form answers, so fit is thin
- Adds them to re-engagement audiences (hashed) once they go cold, like any other lead

Not in v1: connecting to portal accounts, managing listings or spend, and sync gap reports for portals (that needs each portal's own lead export).

To confirm: how each portal reaches GHL (all through Akarati, or some direct), and whether Dubizzle leads are labelled on their own or folded into Bayut.

Still to confirm with Reportage: whether they agree to the propose-then-apply rule for funnel changes, and how marketers move leads into the WhatsApp bucket.

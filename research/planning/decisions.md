# Decisions

**Superseded on 2026-10-09.** The decision log moved to `docs/architecture.md`. This file is kept for history.

What we have settled, newest first. Each entry says what was decided and why. When a decision changes, add a new entry instead of editing the old one.

## 2026-10-09

These mostly come from Hisham bhayya's answers (`docs/sources/hisham-answers-2026-10-02.md` and `-2026-10-09.md`). They are the current direction, not final. Points that need checking are tracked as open questions.

- **ADX is Marketing OS plus lead intelligence.** It means scoring leads and tracing ad spend to Done-Deals. It optimises for deals instead of form fills.
- **Users are the marketing team only, about 30 people.** Sales, telesales and brokers never log in. The sales desk, inbox and assignment from Sales OS do not come across. This replaces the earlier idea of merging both products in full.
- **No outreach from ADX.** No calls, WhatsApp or email. Cold leads are re-engaged through ad audiences instead.
- **GoHighLevel is the CRM ADX reads.** ADX writes only its own fields and tags there, never statuses or native fields. Akarati data arrives through GoHighLevel. The monthly sheet is only for backfill.
- **The platforms are the source of truth.** ADX stores history and its own results, never a competing copy.
- **Paid media is Google and Meta.** Portal leads are reported like any other lead, with spend typed in by hand. Connecting to portal accounts is for later.
- **v1 writes to Google, Meta and GoHighLevel.** Every write goes proposal, approval, execution, log, with caps, a kill switch and a permission checklist. Automating approved actions is for later.
- **Three roles: Viewer, Marketer, Admin.** The CMO is the admin and approver. Approvals go out as links in email or WhatsApp.
- **Organic social is read in v1.** Instagram, Facebook, LinkedIn and Search Console. Posting is for later.
- **Ryze is the benchmark for the ads side, not the spec.** Matching it is the floor.
- **Hosting is likely Frankfurt,** pending Reportage's written approval.
- **No frontend-only prototype.** We scaffold the real app once the tech decisions are made. This replaces the 2026-10-01 prototype decisions.

## 2026-10-02

- **ADX goes to Reportage as a pilot their team uses.** This repo plans the product, even if it only ever holds the prototype.
- **Reportage first, maybe a product later.** ADX starts as a fast-iteration build for Reportage. It could later become a product, or a service for other companies. For now we still build for Reportage only, with no multi-tenancy.
- **The point of the questions is deciding what to leave out.** ADX wraps ad platforms, analytics and CRMs, plus our two products. We ask how Reportage's marketing and sales teams actually work, so we can drop the features they will not use. That keeps development fast and the product clear.
- **The question list goes to Rishan.** He is our CEO and Reportage's head of AI, and he answers or finds out. It lives in a shared doc linked from the README. `questions.md` keeps the code-backed answers behind it.

## 2026-10-01

- **ADX is a new, merged product.** Decisions written for Marketing OS, such as Build Plan v4's ban on sending WhatsApp and email, do not carry over. ADX makes its own calls.
- **ADX serves Reportage only.** No multi-tenancy, workspaces or organisations.
- **The prototype is frontend only, with no backend.** It exists to explore user flows and UX quickly, and to brainstorm the product.
- **Prototype base is Vite, React and TypeScript.** No serious alternative came up. The libraries on top are still to be confirmed. The production stack is a separate, open decision.
- **The visual language follows Marketing OS.** Black and white, minimal, generic. Not the Sales OS liquid glass look.
- **No voice outreach for Reportage.** As far as we know it is out of the question. The Sales OS softphone and AI caller do not come across.
- **Unification is deferred.** We wait for the list of write features we are missing from the products ADX wraps around (the CRM, the ad platforms and others), then map features. `unification-draft.md` is parked until then.
- **Communication runs through Rishan.** We talk to Rishan, our CEO. Rishan talks to Reportage.

## Open

- The tech stack. Twaha's `Tech Options.md` (2026-10-07) compares Go and TypeScript for a fresh build.
- Twaha's `marketing-platform` spec, which his tech doc cites and which we haven't seen.
- How team docs, decisions and tasks are represented on GitHub.
- The open questions, tracked separately.

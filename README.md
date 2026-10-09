# ADX

ADX is a marketing tool for Reportage, a UAE property developer. It's Marketing OS plus lead intelligence: it scores leads and traces ad spend through to Done-Deals (sales). The users are Reportage's marketing team, about 30 people.

## Why it exists

Reportage spends on Google and Meta ads. Leads land in GoHighLevel, the marketing CRM, and deals close in Akarati, the sales CRM, which syncs back into GoHighLevel. The marketing team can see clicks, form fills and cost per lead, but nobody can say which campaigns bring buyers, and the ad platforms optimise for cheap form fills.

In September 2026 there were 3,445 leads and 58 Done-Deals. Half the leads never got a status, UTMs get lost between the website and GoHighLevel, and when a marketer relabels a lead by hand, its original source is lost. ADX is meant to connect ad spend to sales. It shows revenue by campaign and project, scores every lead, helps marketers make changes in the ad accounts safely, and sends outcomes back to Google and Meta so their bidding optimises for buyers instead of form fills.

It's built for Reportage first. It could become a product, or a service for other property developers, later.

## Where it comes from

ADX draws on two products WokeWorks already built:

- **Marketing OS** (`WokeWorks/ads-mcp`). Google and Meta reporting and analysis. Reportage never used it properly.
- **Sales OS** (`WokeWorks/dead-leads-stafford`, built for Stafford Global). Lead scoring and a sales desk.

What ADX takes from each is decision D-17 in `docs/architecture.md`. Whether it reuses their code depends on the tech stack, which isn't decided yet (#34). There is no app code in this repo yet.

## Docs

| Path | What it holds |
|---|---|
| `docs/reference.md` | Roles, shared definitions and the features we've considered, with their scope labels |
| `docs/architecture.md` | How ADX fits together, the decisions and what each costs us, and what's deferred or set aside |
| `research/` | Raw inputs: what the two old products do, notes on Reportage, answers from Hisham bhayya (WokeWorks CTO), and early drafts |

## Scope labels

Features, deferred items and set-aside ideas carry one of four scope labels. Decisions are the current direction unless marked as replaced.

- **Likely.** The current direction. It can still change.
- **Possible.** Discussed, not agreed.
- **Later.** Wanted, but not in the first version.
- **Set aside.** Decided against for now, with the reason recorded.

## How work is tracked

Work lives in the ADX GitHub Project.

- **Area** is a field on every issue. It's the Area column of the features table in `docs/reference.md`, and it runs across phases.
- **Phase** is a field on every issue. It says when the work happens. The phases aren't set yet.
- **A parent issue** is one area within one phase. It finishes inside that phase.
- **Sub-issues** are the tasks inside a parent.
- **Blocked by** links one issue to another it can't start without. Most cross-feature dependencies are these.
- **Labels.** `deferred` marks work moved to a later phase, with the reason in a comment. `question` and `external` mark the two pinned checklist issues.

Open questions and outside dependencies are two pinned issues with checklists: Open questions (#34) and Waiting on others (#35). They stay off the board.

Change the docs through a pull request, so everyone sees when a definition or decision moves. When a decision changes, add the new one and mark the old one as replaced instead of deleting it.

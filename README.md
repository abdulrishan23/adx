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

What ADX takes from each is decision D-17 in `docs/architecture.md`. ADX reuses their logic, not their code, because the stack is new (D-18).

## Code

`server/` is the Go backend and `web/` is the React frontend. They deploy separately behind one domain. `CLAUDE.md` has the layout and conventions in short.

To run it you need Go 1.26, Node 22, pnpm and [air](https://github.com/air-verse/air). Then:

- `make dev` runs both, at http://localhost:5173.
- `make gen` regenerates sqlc code and the TypeScript API types.
- `make test` and `make check` run the tests and linters. `make check` also needs [golangci-lint](https://golangci-lint.run).

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

Work lives in the [ADX GitHub Project](https://github.com/orgs/WokeWorks/projects/2).

- **Area** is a field on every issue. It's the Area column of the features table in `docs/reference.md`, and it runs across phases.
- **Phase** is a field on every issue. It says when the work happens. The whole team works through the phases in order.
- **A parent issue** is one area within one phase. It finishes inside that phase.
- **Sub-issues** are the tasks inside a parent.
- **Blocked by** links one issue to another it can't start without. Most cross-feature dependencies are these.
- **Labels.** `deferred` marks work moved to a later phase, with the reason in a comment. `question` and `external` mark the two pinned checklist issues.

Open questions and outside dependencies are two pinned issues with checklists: [Open questions (#34)](https://github.com/WokeWorks/adx/issues/34) and [Waiting on others (#35)](https://github.com/WokeWorks/adx/issues/35). They stay off the board.

Change the docs through a pull request, so everyone sees when a definition or decision moves. When a decision changes, add the new one and mark the old one as replaced instead of deleting it.

## How we work

1. **Pick up.** Run `/adx-next` in Claude Code. It shows what's free in the current phase, assigns the ticket you pick, and goes through what the feature does, what it touches and the decisions it needs. Whoever picks a ticket owns those decisions, and they go on the ticket before code starts.
2. **Build.** One branch per ticket, named `{number}-{short-name}`, merged through a PR once CI passes.
3. **Wrap up.** Run `/adx-update`. It closes finished sub-issues, records what was done differently or deferred, files new tickets, and proposes doc changes for decisions that reach beyond the ticket.

Both commands need `gh` signed in with the project scope: `gh auth refresh -s project`.

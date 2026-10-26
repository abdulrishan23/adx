# G3: Next step on every project card

From "Projects section: gaps and next moves", item 2. Approved 2026-10-26.

Deliverable: `ProjectProgress.next` names the one action that moves the project, who can take it (role) and the section it's in; the Projects page shows it on each card. Docs: `docs/screens.md`.
Allowed changes: `server/core/campaigns/progress.go`, `progress_test.go`, `web/src/routes/projects.tsx`, `web/src/api/types.gen.ts` (generated), `docs/screens.md`.
Run limit: 3 attempts per node, 3 parallel subagents.

## Rules

The next step is the first section, in order, that isn't done, read with what it holds:

| Section state | Next step | Who |
|---|---|---|
| brand, no facts | Record the first fact | marketer |
| brand, facts need review | Approve N facts | admin |
| brand, approved facts, no brief | Start research | marketer |
| brand, brief running | Wait for research (not an action; shows as "Research running") | nobody |
| media, no plan | Draft a plan | marketer |
| media, plan ready to accept | Accept the plan | admin |
| media, plan draft or needs input | Finish the plan | marketer |
| studio, nothing | Upload ads or ask Adx for concepts | marketer |
| studio, ads waiting review | Review N ads | marketer |
| studio, ads need changes | Fix N ads | marketer |
| studio, approved ads, no page | Write the page | marketer |
| measurement, blocked | Fix: first blocker's message | admin |
| campaigns, none | Start a campaign | marketer |
| campaigns, in progress | Open the campaign furthest along | marketer |
| everything done | Nothing waiting | nobody |

## Nodes

| id | reads | produces | check |
|---|---|---|---|
| core | progress.go and the table above | `Next{Section, Label, Role}` on `ProjectProgress`, computed in `Progress` from the same counts the states use | `go test ./core/campaigns` with a case per row that the existing fixture can reach |
| screen | G2's `ProgressAll` on the Projects page | each card shows the next step and its role under the name | `pnpm typecheck`, `pnpm test`, screenshot |
| docs | screens.md | the table above, shortened | resolves |

## Edges

- core -> screen (and G2.api -> screen)
- docs parallel

## Code nodes

- `make gen` (shared with G2)

## Rules proved

- gentypes keeps one TypeScript namespace across core packages, so a new exported struct needs a name no other package uses (`ProjectNext`, not `NextStep`).
- A section that is pending because there is nothing to check yet is not a next step; the rule table falls through to the push.

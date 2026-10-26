# G4: Approval queue per project

From "Projects section: gaps and next moves", item 1. Approved 2026-10-26.

Deliverable: `ProjectProgress.needs` lists what waits on a person in this project, each with a kind, a count, who can act and where; a "Needs you" strip at the top of every subsection page shows the items the signed-in role can act on first, the rest after. Docs: `docs/screens.md`.
Allowed changes: `server/core/campaigns/progress.go`, `progress_test.go`, `web/src/routes/project-sections.tsx`, `web/src/api/types.gen.ts` (generated), `docs/screens.md`.
Out of scope: feeding the Inbox. Home and Inbox cards are still sample data in phase 1 (`web/src/api/mock/fronts.ts`); the strip is the real list until the Go desk endpoint exists.
Run limit: 3 attempts per node, 3 parallel subagents.

## Rules

| Waiting on | Who acts | Where |
|---|---|---|
| Facts needing review | admin | Brand section (brand page records the decision) |
| Offers in draft | admin | Brand section |
| Ads awaiting review | marketer | Studio |
| Ads needing changes | marketer | Studio |
| Plans ready to accept | admin | Media |
| Pages drafted, never published | marketer | Studio |
| Measurement blockers | admin | Measurement |
| Accepted plans not yet authorized to launch | admin | the engine's launch screen |

## Nodes

| id | reads | produces | check |
|---|---|---|---|
| core | progress.go; brands facts and offers; studio ads; plans; pages; execution launch state through `LaunchReader` | `Needs []Need{Kind, Count, Role, Section, Label}` on `ProjectProgress`, empty when nothing waits | `go test ./core/campaigns` with a case that fills three kinds and checks another project shows none |
| screen | core output | a strip under the section row: items for the signed-in role first, each a link to its section; hidden when empty | `pnpm typecheck`, `pnpm test`, screenshot |
| docs | screens.md | the rules table | resolves |

## Edges

- core -> screen
- docs parallel

## Code nodes

- `make gen` (shared with G2)

## Rules proved

- (filled after the run)

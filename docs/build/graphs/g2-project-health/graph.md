# G2: Project health in one number

From "Projects section: gaps and next moves", item 7. Approved 2026-10-26.

Deliverable: `ProjectProgress` carries `done`, `total` and `blocker`; the Projects page cards and the sidebar switcher show "3 of 5" (or the blocker) per project. Docs: `docs/screens.md`.
Allowed changes: `server/core/campaigns/progress.go`, `progress_test.go`, `server/api/campaigns.go`, `server/api/router.go`, `web/src/api/campaigns.ts`, `web/src/api/types.gen.ts` (generated), `web/src/routes/projects.tsx`, `web/src/app/app-shell.tsx`, `docs/screens.md`.
Run limit: 3 attempts per node, 3 parallel subagents.

## Nodes

| id | reads | produces | check |
|---|---|---|---|
| core | progress.go, the section rules in screens.md | `Done`, `Total` (released sections only, so a phase-2 section doesn't count against a phase-1 release) and `Blocker` (the first error note, or "") on `ProjectProgress`; `ProgressAll` listing every project's progress in one call | `go test ./core/campaigns` with a case for done/total/blocker |
| api | core output | `GET /api/projects/progress` returning `{ projects: [ProjectProgress] }`; `make gen` | `go test ./api`, `types.gen.ts` has `ProjectsProgressResponse` |
| screen | api output; projects.tsx card; app-shell.tsx switcher | each card shows "n of 5" with a mark, or the blocker in red; the switcher's option labels carry "· n of 5" | `pnpm typecheck`, `pnpm test`, a screenshot of the Projects page |
| docs | screens.md | one sentence on the health number | resolves |

## Edges

- core -> api -> screen
- docs parallel

## Code nodes

- `make gen` after api
- `gofmt`, `biome check --write`

## Rules proved

- The server doesn't know the web release phase, so anything that counts released sections is computed on the screen from `PROJECT_SECTIONS` and `released()`; the server supplies facts (states, blocker), not the summary.
- One request for every project's progress keeps the Projects page at one round trip, at the cost of running every project's readiness checks on each poll.

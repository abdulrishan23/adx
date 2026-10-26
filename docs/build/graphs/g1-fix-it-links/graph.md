# G1: Fix-it links land on the thing

From "Projects section: gaps and next moves", item 3. Approved 2026-10-26.

Deliverable: Measurement's Fix link and the Campaigns gate open the exact account, page or plan the line is about, not a list. Docs: `docs/screens.md` one line.
Allowed changes: `web/src/routes/project-sections.tsx`, `docs/screens.md`.
Run limit: 3 attempts per node, 3 parallel subagents.

## Nodes

| id | reads | produces | check |
|---|---|---|---|
| screen | `fixFor` in project-sections.tsx; `LaunchItem.refKind`/`refId` in `core/measurement`; the connections screen's account rows | `fixFor` returns a page route for `page`, a plan route for `plan`, the connections screen with `?account=<id>` for `account`; the gate's rows link to Measurement with `?item=<plan-code-ref>` and Measurement highlights that row | `pnpm typecheck`, `pnpm test`, a screenshot of a Fix link's target |
| docs | screens.md Projects rules | one sentence under Measurement | the sentence resolves to what the screen does |

## Edges

- none between screen and docs (parallel)

## Code nodes

- `biome check --write` after screen

## Rules proved

- (filled after the run)

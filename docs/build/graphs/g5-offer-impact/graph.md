# G5: Offer-change impact in the project

From "Projects section: gaps and next moves", item 9. Approved 2026-10-26.

Deliverable: the Brand section lists, under each offer, the creatives built on an older version of it, with their review state and a link to each; Studio shows the same count on the Creatives heading. Redraft is a link to the concept's generate action, not a new batch action: Studio's generation already redoes an ad from the current offer, and a batch that regenerates many ads unasked would spend model budget without a person choosing. Docs: `docs/screens.md`.
Allowed changes: `web/src/api/brands.ts`, `web/src/routes/project-sections.tsx`, `docs/screens.md`.
Core and API already exist: `brands.OfferImpact` and `GET /api/offers/{id}/impact`.
Run limit: 3 attempts per node, 3 parallel subagents.

## Nodes

| id | reads | produces | check |
|---|---|---|---|
| screen | `useOfferImpact` (add if missing) in brands.ts; Overview's Offers panel in project.tsx; the project's offers | under each offer in the Brand section, "N creatives on an older version" expanding to the list with review state and a link to the ad; nothing shown when none | `pnpm typecheck`, `pnpm test`, screenshot with a revised offer |
| docs | screens.md | one sentence | resolves |

## Edges

- none (parallel)

## Code nodes

- `biome check --write`

## Rules proved

- `AffectedCreative.id` is an engine creative id, not a studio ad id; a link from the Brand section must go where the id resolves. Two creative stores still exist (`core/creative`, `core/studio`), which BD-59 folds later.

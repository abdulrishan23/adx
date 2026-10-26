| node | requirement | verdict | evidence |
|---|---|---|---|
| screen | a page line's Fix opens that page inside the project | pass (code), unresolved (browser) | `fixFor` in web/src/routes/project-sections.tsx routes `page` to `/projects/$projectId/pages/$pageId` with the line's refId; `pnpm typecheck` clean. The dev database has no plan that produces a page line, so no screenshot |
| screen | a plan line's Fix opens that plan inside the project | pass (code), unresolved (browser) | `fixFor` routes `plan` to `/projects/$projectId/plans/$planId` with the line's planId; no dev data to show it |
| screen | an account line's Fix opens the connections screen | pass | `/settings/connections` has no search parameter, so Fix opens the screen; recorded in the component comment |
| screen | a gate row lands on its line in Measurement | pass (code), unresolved (browser) | the gate links with `hash={checkId(it)}` to rows carrying `id={checkId(it)}` and `target:bg-accent`; no dev blocker to click |
| docs | screens.md says where Fix lands | pass | docs/screens.md, Projects rules |

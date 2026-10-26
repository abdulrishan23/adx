| node | requirement | verdict | evidence |
|---|---|---|---|
| core | `Blocker` names the first blocked measurement line, "" when none | pass | TestProgressFollowsTheProjectThroughItsSections ("blocker names the first blocked line") |
| core | `ProgressAll` returns every project in the workspace in project order; an outsider is refused | pass | TestProgressFollowsTheProjectThroughItsSections ("progress for every project", "outsider on all") |
| api | `GET /api/projects/progress` returns `{projects: [...]}` and the type reaches `types.gen.ts` | pass | `go test ./api` ok; `ProjectsProgressResponse` in web/src/api/types.gen.ts |
| screen | each project card shows "n of m" with a mark, or the blocker in red; m counts released sections only | pass | `healthOf` in web/src/api/campaigns.ts; dev app on 2026-10-26: Bianca "0 of 5", Wadi "1 of 5" with the mark; `pnpm typecheck`, `pnpm test` 86 passed |
| screen | the switcher's options carry "· n of m" or "· blocked" | pass | dev app on 2026-10-26: the switcher reads "Wadi Al Safa Villas · 1 of 5" (accessibility tree) |
| docs | screens.md describes the health number | pass | docs/screens.md, Projects rules |

Graph: done core, api, screen, docs; running none; blocked none
Outputs: server/core/campaigns/progress.go (Blocker, ProgressAll), server/api/campaigns.go (projectsProgress), server/api/router.go, web/src/api/campaigns.ts (useProjectsProgress, healthOf), web/src/routes/projects.tsx, web/src/app/app-shell.tsx, docs/screens.md
Decisions: the health number is computed on the screen from the sections and the released phases, not on the server, because the server doesn't know the web release phase (VITE_PHASE). The server supplies the blocker. The helper takes the released section ids so the id-to-segment match lives in one place.
Open issues: healthOf has no unit test of its own; the screen node's scope was three files. Add one with the next change to campaigns.ts.
Next action: none; the graph is complete.

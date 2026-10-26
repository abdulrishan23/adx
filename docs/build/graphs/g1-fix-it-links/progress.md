Graph: done screen, docs; running none; blocked none
Outputs: web/src/routes/project-sections.tsx (fixFor, checkId, MeasurementGate links), docs/screens.md
Decisions: the section routes have no search parameters, so the gate uses a URL hash to land on a Measurement row rather than a `?item=` query. The connections screen has no account parameter, so an account line opens the screen, not the row.
Open issues: the page, plan and gate links are verified by type and code review only. The dev database holds no plan that produces a measurement line, so none was clicked in a browser. First live plan with a page will show it.
Next action: none; the graph is complete.

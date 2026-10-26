Graph: done core, screen, docs; running none; blocked none
Outputs: server/core/campaigns/progress.go (ProjectNext, nextStep), server/core/campaigns/progress_test.go, web/src/routes/projects.tsx, docs/screens.md
Decisions: the measurement row only produces a next step while a blocker exists; with no plan yet it falls through to Campaigns, since "nothing to check" isn't an action. Type names are ProjectNext and ProjectNeed because gentypes keeps one namespace and outreach already has NextStep.
Open issues: four rows of the rule table (ads waiting, ads needing changes, open the furthest campaign, nothing waiting) have no fixture in campaigns_test; they are code-reviewed, not tested. A studio fixture with reviewed ads would close them.
Next action: none; the graph is complete.

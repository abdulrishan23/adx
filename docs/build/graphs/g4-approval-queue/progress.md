Graph: done core, screen, docs; running none; blocked none
Outputs: server/core/campaigns/progress.go (ProjectNeed, waiting), server/core/campaigns/progress_test.go, web/src/routes/project-sections.tsx (NeedsStrip, needLink), docs/screens.md
Decisions: the strip orders the signed-in role's items first and keeps the rest visible, so a marketer sees what the admin still owes the project. Launch authorization links to the engine's launch screen because the designed screens don't authorize a launch. The Inbox is not fed: its cards are sample data in phase 1.
Open issues: three kinds (offers in draft, ads awaiting review, ads needing changes) have no fixture in campaigns_test and are code-reviewed only.
Next action: none; the graph is complete.

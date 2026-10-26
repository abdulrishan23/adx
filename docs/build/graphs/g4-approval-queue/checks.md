| node | requirement | verdict | evidence |
|---|---|---|---|
| core | nothing waits on an empty project | pass | TestProgressFollowsTheProjectThroughItsSections ("empty project's next step and needs") |
| core | a fact needing review is one need of kind facts_review in brand | pass | same test ("needs:") |
| core | a ready plan and a drafted page are plans_accept and pages_publish | pass | same test ("needs with a ready plan and a drafted page") |
| core | a measurement blocker is a need | pass | same test ("a blocker is a need") |
| core | an accepted plan waits for launch authorization, not acceptance | pass | same test ("an accepted plan waits for authorization") |
| core | another project shows none of this | pass | same test ("the other project borrowed") |
| core | offers in draft, ads awaiting review, ads needing changes | unresolved | not reachable from the fixture; code review only |
| screen | a Needs you strip under the section row, the signed-in role's items first, each linking to its section; hidden when empty | pass | dev app on 2026-10-26: "Needs you · 1 fact needs review · admin" on every section of Wadi Al Safa Villas, none on Bianca Townhouses; `pnpm typecheck`, `pnpm test` 86 passed |
| docs | screens.md lists the kinds and who acts | pass | docs/screens.md, Projects rules |

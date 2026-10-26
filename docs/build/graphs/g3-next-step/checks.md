| node | requirement | verdict | evidence |
|---|---|---|---|
| core | empty project → "Record the first fact", marketer | pass | TestProgressFollowsTheProjectThroughItsSections ("empty project's next step") |
| core | a fact needing review → "Approve 1 fact", admin | pass | same test ("a fact needing review is the admin's next step") |
| core | brand done → "Draft a plan" | pass | same test ("brand done, media next") |
| core | a plan ready to accept → "Accept the plan", admin | pass | same test ("a ready plan is the admin's next step") |
| core | media done, no ad approved → studio is next | pass | same test ("studio next once media is done") |
| core | a measurement blocker → "Fix: <message>", admin | pass | nextStep's measurement branch; blocker asserted in the same test |
| core | rows the fixture can't reach (ads waiting, ads needing changes, open the furthest campaign, nothing waiting) | unresolved | no fixture with reviewed ads or a live launch; covered by code review only |
| screen | each card shows the next step and its role | pass | dev app on 2026-10-26: "Record the first fact · marketer", "Approve 1 fact · admin"; `pnpm typecheck`, `pnpm test` 86 passed |
| docs | screens.md lists the rule | pass | docs/screens.md, Projects rules |

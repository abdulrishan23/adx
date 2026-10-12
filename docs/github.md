# GitHub setup and commands

How to set up `gh` for ADX, and the commands `/adx-next` and `/adx-update` use. How tickets are structured is in the root README.

## Setup

1. Install the GitHub CLI (`gh`).
2. Sign in: `gh auth login`.
3. Add the project scope. Editing the Project needs it: `gh auth refresh -s project`.
4. Check that it works: `gh project view 2 --owner WokeWorks`.

## The board

- Project "ADX", number 2 under WokeWorks. Its ID is `PVT_kwDODnjbLs4BmV57`.
- Fields: Status (Todo, In Progress, Done), Area, Phase.
- Labels: `deferred`, `question`, `external`.
- Pinned checklists: Open questions (#34) and Waiting on others (#35).

## Reading

| What | Command |
|---|---|
| Who am I | `gh api user --jq .login` |
| All tickets with their fields | `gh project item-list 2 --owner WokeWorks --format json --limit 500` |
| One ticket with comments | `gh issue view N --comments` |
| Its sub-issues | `gh api repos/WokeWorks/adx/issues/N/sub_issues --jq '.[] \| {number, title, state}'` |
| Its parent | `gh api repos/WokeWorks/adx/issues/N/parent --jq '{number, title}'` |
| What blocks it | `gh api repos/WokeWorks/adx/issues/N/dependencies/blocked_by --jq '.[] \| {number, title, state}'` |
| Field and option IDs | `gh project field-list 2 --owner WokeWorks --format json` |
| The PR for this branch | `gh pr view --json number,state,url` |

In `item-list` output, each field's value sits under its name in camelCase: `status`, `area`, `phase`.

## Writing

| What | Command |
|---|---|
| Create a ticket on the board | `gh issue create --repo WokeWorks/adx --title "..." --body "..." --project ADX` |
| Get its board item ID | `gh project item-add 2 --owner WokeWorks --url ISSUE_URL --format json --jq .id` |
| Set Status, Area or Phase | `gh project item-edit --id ITEM_ID --project-id PVT_kwDODnjbLs4BmV57 --field-id FIELD_ID --single-select-option-id OPTION_ID` |
| Assign yourself | `gh issue edit N --add-assignee @me` |
| Add a label | `gh issue edit N --add-label deferred` |
| Comment | `gh issue comment N --body "..."` |
| Make N a sub-issue of P | `gh api -X POST repos/WokeWorks/adx/issues/P/sub_issues -F sub_issue_id=$(gh api repos/WokeWorks/adx/issues/N --jq .id)` |
| Mark N blocked by B | `gh api -X POST repos/WokeWorks/adx/issues/N/dependencies/blocked_by -F issue_id=$(gh api repos/WokeWorks/adx/issues/B --jq .id)` |
| Close as done | `gh issue close N --reason completed` |
| Edit a checklist (#34, #35) | `gh issue view 34 --json body --jq .body > body.md`, edit it, then `gh issue edit 34 --body-file body.md` |

`item-add` returns the existing item when the issue is already on the board, so it's a safe way to look up an item ID. Sub-issue and Blocked by links take the issue's internal ID, not its number. The `$(...)` part looks it up.

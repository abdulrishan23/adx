---
name: adx-next
description: Pick the next ADX ticket from the GitHub Project, then shape it with the person picking it up before any code is written. Use at the start of a work session, or when someone asks what to work on next.
argument-hint: "[issue number]"
disable-model-invocation: true
---

# /adx-next: pick up a ticket

Start-of-work command. Find where the team is, offer what to pick up, and once someone picks, shape the ticket with them before writing code.

Whoever picks a ticket owns it: what the feature does, how it touches other features, how it gets built, and the decisions made on the way. Your job is to do the reading and digging, and bring them clear choices. They make the calls.

Setup: if `gh auth status` fails or lacks the `project` scope, have them run `gh auth login`, then `gh auth refresh -s project`. The board is Project 2 under WokeWorks, with the fields Status, Area and Phase. Sub-issues and Blocked by links go through `gh api` (`repos/WokeWorks/adx/issues/N/sub_issues` and `.../dependencies/blocked_by`), and they take the issue's internal `id`, not its number.

Checklists: a checklist in a parent ticket holds tasks nobody has scoped yet. Once a task is scoped, it becomes a sub-issue and its checklist line goes away. Nobody builds straight from a checklist line.

## 1. Read the board

Read in parallel:
- The Project's tickets with Status, Area, Phase, assignees and labels
- Who is calling (`gh api user --jq .login`)
- #34 (open questions) and #35 (waiting on others), only to know what's blocked
- `git log --oneline -10` and the current branch

Work out:
- The current phase: the earliest Phase that still has open tickets
- What's In Progress, the caller's own first
- Free tickets in the current phase: unassigned, and not blocked by an open ticket or an unticked line in #35
- Tickets labelled `deferred` whose reason no longer holds

## 2. Offer 2 to 5 options

Priority:
1. The caller's own In Progress work
2. Deferred tickets that are now unblocked
3. Free tickets in the current phase, following what other tickets depend on
4. Unblocked tickets from later phases, when the current phase has nothing free for this person, so nobody sits idle

```
Phase N: X of Y tickets done

1. #123 Phase 1 · Connections. {Why now, in one line}
2. #124 ...
```

If an issue number was passed, skip straight to step 3 with it. If the board looks stale, for example merged PRs for tickets still open, say so and suggest `/adx-update` first. Start nothing until they pick.

## 3. Take the ticket

Once they pick, show what you're about to do, then:
- Assign it to them and set Status to In Progress
- Suggest a branch named `{number}-{short-name}`

## 4. Shape it before code

Read:
- The ticket, its sub-issues, comments, parent, and what blocks it
- The feature in `docs/reference.md`, and the decisions in `docs/architecture.md` that touch it
- Tickets in other Areas that feed this one or depend on it
- The code it will touch (`CLAUDE.md` has the layout)

Bring back, kept short:
- **What it does.** A few sentences in product terms, so you both agree on the goal.
- **What it touches.** What it needs from other features, what they will need from it, and any shared shape both sides have to agree on, such as data, a core function or an MCP tool.
- **Decisions to make.** Each with the options, the tradeoff and your recommendation. List the obvious ones too, one line each, so nothing gets decided silently.
- **Open questions.** Anything that needs someone outside the team, and whether it blocks the work or can be worked around.

Ask the owner to decide. Go one decision at a time if they prefer. If you fill a gap with your own pick, say so.

## 5. Write it down, then build

Once decided:
- If the ticket holds a checklist, turn the lines this work covers into sub-issues, with the parent's Area and Phase and any Blocked by links, and remove those lines. Lines nobody is taking stay as they are.
- Comment on the ticket with the goal, each decision with a one-line reason, and anything left for later. This is what the next person reads.
- If a decision reaches beyond this ticket, say it belongs in `docs/architecture.md`. `/adx-update` proposes the change at the end.
- Propose the build steps, files and tests. Wait for a go.

## Rules

- Keep the options tight. Group small sub-issues under their ticket.
- Show every assignment, field change and comment before making it.
- MCP first (D-19): a feature's MCP tools come before its web screen.
- One clear question beats a list of five.

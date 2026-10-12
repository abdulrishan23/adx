---
name: adx-update
description: Wrap up ADX work. Bring the ticket, the GitHub Project and the docs in line with what was built, and check the decisions made along the way are clear and owned. Use at the end of a task or before opening a PR.
argument-hint: "[issue number]"
disable-model-invocation: true
---

# /adx-update: record what happened

End-of-work command. Bring the ticket, the Project and the docs in line with what was built. Nothing is written until the person confirms.

Setup: if `gh auth status` fails or lacks the `project` scope, have them run `gh auth login`, then `gh auth refresh -s project`. The board is Project 2 under WokeWorks, with the fields Status, Area and Phase. Sub-issues and Blocked by links go through `gh api` (`repos/WokeWorks/adx/issues/N/sub_issues` and `.../dependencies/blocked_by`), and they take the issue's internal `id`, not its number.

## 1. Gather

Read in parallel:
- The ticket being worked on. Take it from the argument, the branch name, or the caller's In Progress tickets. Ask if it's unclear.
- Its sub-issues, its comments (including the decisions comment from `/adx-next`), and what blocks it
- What changed: commits on this branch against main (or recent commits on main), `git diff --stat`, and uncommitted work
- The open PR for this branch, if any

In a fresh session, read the changed files to understand what was built.

## 2. Compare

Check what happened against the ticket and its decisions comment, and sort it into:
- **Done as planned.** Sub-issues or checklist items that are finished.
- **Done differently.** What changed from the ticket or its decisions, and why.
- **Deferred.** Planned work that was pushed out, with the reason and where it fits now.
- **New.** Work or tickets discovered along the way.
- **Decisions.** Every choice made during the work, including small ones made in code.
- **Docs.** Decisions that reach beyond this ticket, new or changed definitions, answered or new open questions, and new outside dependencies.

## 3. Check the decisions are owned

Show the decisions as a short list, one line each with its reason. Then ask: "Anything here you'd decide differently, or that's still open?"

Keep it a quick review. If a decision was made by the AI without the person weighing in, say so plainly and let them confirm or change it.

## 4. Propose the updates

Show one summary:

```
#123 Phase 1 · Connections

Close: #130, #131
Status: In Progress → Done
Comment: {what was done differently, and decisions not yet on the ticket}
Deferred: #132, labelled deferred, {reason}
New tickets: {title} · {Area} · {Phase} · blocked by #…
Docs: architecture.md {new decision}, reference.md {definition}
#34 / #35: tick {line}, add {line}
```

Ask: "Does this look right?"

## 5. Write it

On approval:
- Close finished sub-issues and tick finished checklist items.
- Set Status. Done only when every sub-issue is closed and the work is merged or in an open PR.
- Comment on the ticket for anything done differently: what was planned, what happened instead, why, and what it changes for other tickets. Be thorough here. It's how the next person learns the code differs from the ticket.
- For deferred work, add the `deferred` label, comment with the reason and the phase it fits, and move its Phase field if it moves.
- Create new tickets with their Area and Phase, attach them as sub-issues where they belong, and add Blocked by links.
- Docs: a decision that reaches beyond one ticket or one phase goes in `docs/architecture.md`, and definitions go in `docs/reference.md`. A changed decision gets a new number and the old one is marked replaced, never deleted. Follow the README on how doc changes land.
- #34 and #35: tick answered lines with the answer underneath, and add new lines. Keep them as checklists.

## Rules

- Finished work is a close or a tick, with no extra prose.
- Differences get full detail. Deferred work always says why.
- Don't rewrite a ticket to match what was built. The ticket keeps the original intent, and comments record what changed.
- Show every write before making it.

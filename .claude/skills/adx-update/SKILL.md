---
name: adx-update
description: Wrap up ADX work. Bring the ticket, the GitHub Project and the docs in line with what was built, and check the decisions made along the way are clear and owned. Use at the end of a task or before opening a PR.
argument-hint: "[issue number]"
disable-model-invocation: true
---

# /adx-update: record what happened

End-of-work command. Bring the ticket, the Project and the docs in line with what was built. Nothing is written until the person confirms.

Setup: if `gh auth status` fails or lacks the `project` scope, have them run `gh auth login`, then `gh auth refresh -s project`. The board is Project 2 under WokeWorks, with the fields Status and Area. Each phase is a milestone, from `1. Foundations` to `6. Breadth`, and unplanned work has none. Sub-issues and Blocked by links go through `gh api` (`repos/WokeWorks/adx/issues/N/sub_issues` and `.../dependencies/blocked_by`), and they take the issue's internal `id`, not its number.

Checklists: a checklist in a parent ticket holds tasks nobody has scoped yet. Once a task is scoped, it becomes a sub-issue and its checklist line goes away.

## 1. Gather

Read in parallel:
- The ticket being worked on. Take it from the argument, the branch name, or the caller's In Progress tickets. Ask if it's unclear.
- Its sub-issues, its comments (including the plan comment from `/adx-next`), and what blocks it
- What changed: commits on this branch against main (or recent commits on main), `git diff --stat`, and uncommitted work
- The open PR for this branch, if any

In a fresh session, read the changed files to understand what was built.

## 2. Compare

Check what happened against the ticket and its plan comment, and sort it into:
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
Follow-up comment: {the draft}
Deferred: #132, labelled deferred, {reason}
New tickets: {title} · {Area} · {milestone} · blocked by #…
Docs: architecture.md {new decision}, reference.md {definition}
#34 / #35: tick {line}, add {line}
```

Ask: "Does this look right?"

## 5. Write it

On approval:
- Close finished sub-issues and tick finished checklist items.
- Set Status. Done only when every sub-issue is closed and the work is merged or in an open PR.
- Post the follow-up comment on the ticket. It's how the next person learns where the code differs from the plan.
- For deferred work, add the `deferred` label, comment with the reason and the phase it fits, and move its milestone if it moves.
- New work that's scoped becomes a ticket with its Area and milestone, attached as a sub-issue where it belongs, with Blocked by links. New work that isn't scoped yet becomes a checklist line on the parent it belongs to.
- Docs: a decision that reaches beyond one ticket or one phase goes in `docs/architecture.md`, and definitions go in `docs/reference.md`. A changed decision gets a new number and the old one is marked replaced, never deleted. Follow the README on how doc changes land.
- #34 and #35: tick answered lines with the answer underneath, and add new lines. Keep them as checklists.

The follow-up comment is plain prose with no headings. It covers what changed from the plan and why, what that changes for other tickets, anything the build turned up that the next person needs, and new tickets or checklist lines by number. If the work matched the plan, it's one line saying so. Write it like the plan comment: for a teammate who opens the ticket cold, in short, plain sentences, saying each thing once.

Include significant edge cases discovered during work and relevant tests added or considered, in one or two sentences per test. Distinguish added tests from proposed tests and state relevant test results. Skip routine test lists.

## Rules

- Finished work is a close or a tick, with no extra prose.
- Differences and deferred work always say why.
- Don't rewrite a ticket to match what was built. The ticket keeps the original intent, and comments record what changed.
- Show every write before making it.

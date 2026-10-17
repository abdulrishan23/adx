---
name: adx-update
description: Wrap up ADX work when finished or set aside because it is paused or blocked. Bring the ticket, Project and docs in line with the outcome. Also use before opening a PR.
argument-hint: "[issue number]"
disable-model-invocation: true
---

# /adx-update: record what happened

Bring the ticket, Project and docs in line with finished work, or work set aside because it is paused or blocked. Active troubleshooting stays in chat. Nothing is written until the person confirms.

Setup: if `gh auth status` fails or lacks the `project` scope, have them run `gh auth login`, then `gh auth refresh -s project`. The board is Project 2 under WokeWorks, with the fields Status and Area. Each phase is a milestone, from `1. Foundations` to `6. Breadth`, and unplanned work has none. Sub-issues and Blocked by links go through `gh api` (`repos/WokeWorks/adx/issues/N/sub_issues` and `.../dependencies/blocked_by`), and they take the issue's internal `id`, not its number.

Checklists: a checklist in a parent ticket holds tasks nobody has scoped yet. Once a task is scoped, it becomes a sub-issue and its checklist line goes away.

## 1. Gather

Read in parallel:
- The ticket being worked on. Take it from the argument, the branch name, or the caller's In Progress tickets. Ask if it's unclear.
- Its sub-issues, its comments (including the proposal and any existing outcome), and what blocks it
- What changed: commits on this branch against main (or recent commits on main), `git diff --stat`, and uncommitted work
- The open PR for this branch, if any

In a fresh session, read the changed files to understand what was built.

## 2. Compare

Check what happened against the ticket and its plan comment, and sort it into:
- **Done as planned.** Sub-issues or checklist items that are finished.
- **Done differently.** What changed from the ticket or its decisions, and why.
- **Deferred.** Planned work that was pushed out, with the reason and where it fits now.
- **New.** Work or tickets discovered along the way.
- **Decisions.** Material choices that differ from the agreed proposal or remain unresolved.
- **Docs.** Decisions that reach beyond this ticket, new or changed definitions, answered or new open questions, and new outside dependencies.

## 3. Check the decisions are owned

Review only unresolved or changed decisions with the owner. Skip decisions already agreed.

Keep it a quick review. If a decision was made by the AI without the person weighing in, say so plainly and let them confirm or change it.

## 4. Propose the updates

Show one summary:

```
#123 Phase 1 · Connections

Close: #130, #131
Status: In Progress → Done
Outcome comment: {the draft, editing the existing outcome if there is one}
Deferred: #132, labelled deferred, {reason}
New tickets: {title} · {Area} · {milestone} · blocked by #…
Docs: architecture.md {new decision}, reference.md {definition}
#34 / #35: tick {line}, add {line}
```

Ask: "Does this look right?"

Follow the writing rules in `CLAUDE.md`, including `/deslop` before publishing ticket updates, docs or PR titles/descriptions.

## 5. Write it

On approval:
- Close finished sub-issues and tick finished checklist items.
- Set Status. Done only when every sub-issue is closed and the work is merged or in an open PR.
- Post one short outcome when the work is done or set aside. Edit the existing outcome for the same work, including after resuming, instead of adding another.
- For deferred work, add the `deferred` label, record the reason and phase in that ticket's outcome rather than a separate progress comment, and move its milestone if it moves.
- New work that's scoped becomes a ticket with its Area and milestone, attached as a sub-issue where it belongs, with Blocked by links. New work that isn't scoped yet becomes a checklist line on the parent it belongs to.
- Docs: a decision that reaches beyond one ticket or one phase goes in `docs/architecture.md`, and definitions go in `docs/reference.md`. A changed decision gets a new number and the old one is marked replaced, never deleted. Follow the README on how doc changes land.
- #34 and #35: tick answered lines with the answer underneath, and add new lines. Keep them as checklists.

The outcome is one short paragraph with no headings. Focus on deviations and their reasons, remaining work, and a brief validation result. If work is paused or blocked and put aside, say what stopped it and what would unblock it. Include another ticket's number only when the outcome affects it. If the work matched the proposal, one sentence is enough.

Mention an edge case or test only when it explains a deviation or remaining concern. Skip routine test lists, setup history and chronological progress reports. Keep interim questions and necessary updates brief and in chat.

## Rules

- Finished work is a close or a tick, with no extra prose.
- Differences and deferred work always say why.
- Don't rewrite a ticket to match what was built. The ticket keeps the original intent, and comments record what changed.
- Show every write before making it.

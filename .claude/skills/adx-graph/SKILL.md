---
name: adx-graph
description: Run a feature that spans several areas (store, core, API, screens, docs) as a graph of bounded nodes with a check on every edge, resumable from docs/build/graphs. Use after /adx-next has shaped the ticket, for anything that touches more than one area or more than one ticket. Small fixes and doc edits don't need it.
argument-hint: "[issue number or short name]"
disable-model-invocation: true
---

# /adx-graph: build a feature as a graph

A feature is a graph of bounded nodes, not one long session. Each node is one job with one input, one output and one check. Edges carry gates that open on evidence. Three files under `docs/build/graphs/<name>/` let another session rerun or extend the work: `graph.md` (the plan and the rules it proved), `checks.md` (what was checked and the evidence) and `progress.md` (where it stands). Stop when every node's check passes or the agreed limit is reached.

This is the ADX adaptation of the autoresearch idea: establish a baseline, give every node one scope and one evaluator, change one thing and measure it, keep what works and record what failed, prefer fewer nodes when results are comparable.

Everything in `CLAUDE.md` still holds. In particular: core first, every function takes a `core.Actor`, tests beside the implementation in one `<domain>_test.go`, `make gen` after schema or query changes, `make check` and `make test` before committing, Conventional Commit messages, and the steering questions about permissions, data meaning and failed or repeated actions before coding. A graph is how the ticket's plan gets executed, not a replacement for shaping it.

## 0. Inputs

Fill these before anything else. If a missing detail changes the outcome, ask one focused question; otherwise continue.

- Task: the ticket (number and title) or the agreed description.
- Deliverable: the branch, the screens or endpoints, and the docs that change.
- Acceptance checks: from the ticket's proposal comment and the tests the user agreed each rule becomes.
- Sources: the docs and code the work reads (`docs/reference.md`, `docs/architecture.md`, `docs/screens.md`, the domain's core package).
- Allowed changes: which packages and folders may change. Anything outside is a blocker to report, not a thing to edit.
- Run limit: how many correction attempts per node (default 3) and how many parallel subagents (default 3).

Name the graph folder after the ticket: `docs/build/graphs/<number>-<short-name>/`.

## 1. Split

Break the task into nodes before any work starts. Split by what each node must read, not by folder. The usual ADX shape:

| Node | Produces | Check |
|---|---|---|
| store | migration, queries, `make gen` output | `go build ./...` and the generated types diff |
| core | the domain function and its `<domain>_test.go` | `go test ./core/<domain>` |
| api | the handler, route and types | `go test ./api` and the generated `types.gen.ts` |
| screen | the designed screen or engine screen | `pnpm typecheck`, `pnpm test`, a screenshot of the state that matters |
| docs | `reference.md`, `architecture.md`, `screens.md` | the decision numbers resolve and `reference.md` agrees |

Draw an edge only when a node consumes another node's output (api needs core; screen needs api's types). Nodes with no edge between them are parallel. Write nodes, edges and checks into `graph.md` and pause for approval before running.

## 2. Nodes

Run each node as its own subagent with only the context it needs: the node spec, its inputs (file paths, the decision it implements), and its check. A subagent builds the artifact for its node only, runs the check against the saved artifact, and returns the output path, a verdict and the evidence (the command and its output, or the screenshot path). Keep node specs reusable: a core node for one domain reads the same as for another.

Two nodes never edit the same file. If two would, merge them or sequence them.

## 3. Code, not model calls

If a step has exactly one correct answer, it is a command, not a model call: `make gen`, `gofmt`, `biome check --write`, `make migration name=...`, a sort or a dedupe. List every such step in `graph.md` as a code node and never route it through a subagent's judgment. If a required tool is unavailable (no `gh` scope, no provider credentials, no dev database), name the blocker in `progress.md` and keep the nodes that don't need it.

## 4. Permissions

Keep edits inside the allowed changes. Writes to outside systems (GitHub comments, pushes, anything that reaches a provider) are not part of a node; they happen after the graph completes and only when the user asks. Commit each accepted node's work on the ticket's branch so a failed later node never takes a good earlier one with it.

## 5. Gates

Every edge that carries risky work has a gate. Open it on evidence in this order: deterministic checks first (tests, `make check`, typecheck), then the node's report, then the model's confidence last. Never weaken a gate to get a pass.

Record each gate in `checks.md`, one row per requirement:

```
| node | requirement | verdict | evidence |
```

Verdicts are pass, fail or unresolved. Evidence is a file path, a test name, or the actual command output. Leave missing evidence visible as unresolved; never fill it in from memory.

## 6. Return paths

Correction edge: a failed gate sends the unit back to the node that made it, with the reason, the evidence and the scope "fix this node only". Cap it at the run limit (default 3). A node still failing after the cap becomes a blocker in `progress.md`, and the graph continues with what doesn't depend on it.

Learning edge: after an accepted result, write the rule it proved into `graph.md` under Rules ("the sidebar query must poll because mutations on other screens don't invalidate it"). If the rule holds beyond this ticket, it goes to `CLAUDE.md` or the docs through `/adx-update`, not silently.

## 7. Completion

Before running, write the completion checklist from the acceptance checks. Each item points at observable evidence.

- Every node's output exists and can be opened.
- Every gate ran against the saved artifact, not a description of it.
- Failures are fixed or reported in `progress.md`.
- `graph.md` matches the nodes that actually ran.
- `make check` and `make test` pass on the branch.

If a limit or blocker stops the run, return a partial status with the exact nodes left.

## Execution loop

1. Read `graph.md` and pick the next ready nodes.
2. Run independent nodes in parallel, up to the subagent limit.
3. Pass every output through its gate.
4. Return failed units, never the whole batch.
5. Merge accepted outputs and commit them.
6. Continue until completion, a blocker, or the run limit.

## Handoff

After each stage, update `progress.md`:

- Graph: nodes done, running and blocked.
- Outputs: exact paths to the current saved files.
- Decisions: what changed in `graph.md` and why.
- Open issues: failures, uncertainties, blockers.
- Next action: the next ready node.

On resumption, read `graph.md` and `progress.md` first, then continue from the recorded next node.

## Delivery

Return the deliverable with `graph.md`, `checks.md` and `progress.md`. Say what was checked, what passed and what is unresolved. Then run `/adx-update` so the ticket and docs record the outcome. Report measured usage (tokens, time) only when the tools report it; never infer savings from the method.

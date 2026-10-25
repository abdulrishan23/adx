# Build graphs

One folder per feature run with `/adx-graph` (`.claude/skills/adx-graph/SKILL.md`): `docs/build/graphs/<ticket>-<short-name>/`. Each holds the three files below, so a later session can rerun or extend the work from git. They're committed with the feature and reviewed with its PR. Small fixes don't get a folder.

## graph.md

```markdown
# <ticket>: <title>

Deliverable: <branch, screens or endpoints, docs>
Allowed changes: <packages and folders>
Run limit: <attempts per node> attempts, <n> parallel subagents

## Nodes

| id | reads | produces | check |
|---|---|---|---|
| store | docs/architecture.md D-xx, server/store/queries/x.sql | migration 000xx, queries, generated code | go build ./... |
| core | store output, the decision | core/<domain>/<fn>.go and <domain>_test.go | go test ./core/<domain> |

## Edges

- store -> core
- core -> api
- api -> screen
- (docs is parallel to everything)

## Code nodes

- make gen after store
- biome check --write after screen

## Rules proved

- <the rule an accepted result proved, one line each>
```

## checks.md

```markdown
| node | requirement | verdict | evidence |
|---|---|---|---|
| core | a viewer may read, an outsider is refused | pass | TestProgressFollowsTheProjectThroughItsSections |
| screen | the section row scrolls at phone width | unresolved | no screenshot taken |
```

Verdicts are pass, fail or unresolved. Evidence is a path, a test name or actual output. Missing evidence stays unresolved.

## progress.md

```markdown
Graph: done <ids>; running <ids>; blocked <ids>
Outputs: <exact paths>
Decisions: <what changed in graph.md and why>
Open issues: <failures, uncertainties, blockers>
Next action: <the next ready node>
```

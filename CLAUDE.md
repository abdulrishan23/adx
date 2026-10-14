# ADX

Marketing tool for Reportage. What it does and why: `docs/reference.md` (concepts, features) and `docs/architecture.md` (decisions).

## Layout

- `server/` is the Go module. `web/` is the React app. They deploy separately behind one domain, and `/api` goes to Go.
- `server/core` holds business logic, one package per domain. Every function takes a `core.Actor`.
- Build features in core first. `server/api`, MCP and `server/jobs` are thin wrappers that call the same core functions.
- `server/store` is SQLite: migrations, queries, sqlc output. Core may use it directly.
- `server/providers` holds outside API clients. Core never imports them. It declares the interface it needs, and `cmd/adx` passes the client in.

## Patterns

- A handler parses and checks input, calls one core function, then `writeJSON` or `h.fail`. `api/projects.go` is the example.
- Errors meant for users are `core.Error` (`core.Invalid`, `core.Conflict`, `core.NotFound`, `core.ErrForbidden`). Anything else is logged and returned as a 500.
- Money is int64 minor units plus a currency code. Times are stored in UTC, and the reporting day is Dubai time.
- A missing figure is null and shows as "Not available", never 0.
- Writes that must happen together go through `store.DB.Tx`. Keep it short and never call an outside API inside it.
- Auth isn't designed yet. `api/actor.go` gives every request a dev admin, in dev only.

## Business rules

- For every applicable ticket, use known context and ask focused steering questions before coding. Establish permissions, data meaning, valid changes and expected outcomes.
- Use concrete examples of missing, duplicate or conflicting data and failed or repeated actions to expose unclear rules. Ask the user what the result must be, rather than silently choosing a rule.
- If context is missing, help the user identify who can supply it or which source to consult. Keep dependent decisions open and continue work that does not need those answers.
- Revisit these questions as new cases emerge during implementation. Turn agreed rules into tests.

## Tests

- Ask which wrong behavior each test catches, without an exhaustive bug list.
- Test core functions directly with real temporary SQLite databases and fake outside providers. Assert results and stored state.
- Keep domain tests in one `<domain>_test.go` file beside the core code. Split only when navigation needs it.
- HTTP/MCP integration tests include real core and check parsing, identity, responses and error mapping. Job tests check wiring and failure handling. Provider tests check outgoing requests and incoming responses. Browser tests cover a few critical user journeys.
- Write tests alongside implementation and add regression tests for bugs. Run focused tests during work. Review changed tests with the feature. CI runs the full suite.

## Ticket communication

- Post one concise proposal before implementation and one short completion comment when the task is done.
- Focus the completion comment on deviations and their reasons, remaining work, and a brief validation result. If work matched the proposal, one sentence is enough.
- Keep questions and necessary progress updates brief and in chat. Do not post interim setup, progress, or test reports to tickets.
- Update the existing proposal or completion comment instead of adding another for the same task.

## Changing things

- Schema: `make migration name=...`, write the SQL, then `make gen`.
- Queries: edit `server/store/queries/*.sql`, then `make gen`.
- API types: Go structs with json tags reach `web/src/api/types.gen.ts` through `make gen`. Commit generated files.
- Run `make check` and `make test` before committing.
- Commit messages are one-line Conventional Commits: `type: message`, with no scope and no body.

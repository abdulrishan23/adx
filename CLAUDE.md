# ADX

Marketing tool for Reportage. What it does and why: `docs/reference.md` (concepts, features) and `docs/architecture.md` (decisions).

## Layout

- `server/` is the Go module. `web/` is the React app. They deploy separately behind one domain, and `/api` goes to Go.
- `server/core` holds business logic, one package per domain. Every function takes a `core.Actor`.
- `server/api` and `server/jobs` are thin wrappers that call core.
- `server/store` is SQLite: migrations, queries, sqlc output. Core may use it directly.
- `server/providers` holds outside API clients. Core never imports them. It declares the interface it needs, and `cmd/adx` passes the client in.

## Patterns

- A handler parses and checks input, calls one core function, then `writeJSON` or `h.fail`. `api/projects.go` is the example.
- Errors meant for users are `core.Error` (`core.Invalid`, `core.Conflict`, `core.NotFound`, `core.ErrForbidden`). Anything else is logged and returned as a 500.
- Money is int64 minor units plus a currency code. Times are stored in UTC, and the reporting day is Dubai time.
- A missing figure is null and shows as "Not available", never 0.
- Writes that must happen together go through `store.DB.Tx`. Keep it short and never call an outside API inside it.
- Auth isn't designed yet. `api/actor.go` gives every request a dev admin, in dev only.

## Changing things

- Schema: `make migration name=...`, write the SQL, then `make gen`.
- Queries: edit `server/store/queries/*.sql`, then `make gen`.
- API types: Go structs with json tags reach `web/src/api/types.gen.ts` through `make gen`. Commit generated files.
- Run `make check` and `make test` before committing.

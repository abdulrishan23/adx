# ADX build inventory (B0.1)

Taken on 2026-10-17 from the git baseline `23b8843` (pristine ADX `main` import plus `docs/build/`). This records what exists before the seven-phase build starts, so later tasks extend rather than duplicate it. No secret values appear here.

## Working tree state

- One commit, no remote, clean tree. Nothing in progress to preserve.
- The spec (section 3) mentions "existing lead ingestion" and `web/src/api/mock/index.ts` with partial mocks. Neither exists in this checkout. There is no mock layer, no leads table and no import. GoHighLevel contacts are only probed (`ReadAccess`) to prove the token works. Lead import is new work under B1.2.
- There is no MCP server, no sessions table and no sign-in flow. `api/actor.go` hands every request a dev admin when `ADX_ENV=dev` and refuses everything otherwise.

## Server (`server/`, Go module `github.com/WokeWorks/adx/server`, go 1.26, toolchain 1.27.2 installed)

| Package | What it holds | Tests |
|---|---|---|
| `core` | `Actor{UserID, Role, Via}`, roles viewer/marketer/admin, `core.Error` kinds invalid/forbidden/not_found/conflict/unavailable | none |
| `core/audit` | `Record` writes `audit_log` inside the caller's transaction | covered via users |
| `core/projects` | List/Create projects (unique name, case-insensitive) | `projects_test.go` |
| `core/users` | People, roles, status; authentik account lifecycle; `SignIn(Identity)`; `Bootstrap` | `users_test.go`, fake in `userstest` |
| `core/connections` | AES-GCM sealed credentials bound to `provider:subject`; GoHighLevel OAuth (start/finish with state + browser binding) and private token connect; `Check` refreshes and re-verifies | `connections_test.go` |
| `api` | chi router, `withActor(dev)`, `/api/health`, `/api/projects`, `/api/connections/crm/*`, error mapping in `respond.go`, cross-origin protection | `projects_test.go`, `connections_test.go` |
| `jobs` | River on SQLite, one periodic `store_optimize` job, `Migrate` for River tables | none |
| `providers/gohighlevel` | OAuth token/location-token, `/locations/{id}`, read-access probes; never logs bodies | `gohighlevel_test.go` |
| `providers/authentik` | Users API: find/create/group add-remove/recovery email | `accounts_test.go` |
| `store` | `DB{Read, Write}` two pools, goose migrations embedded, sqlc output, `Tx`, `IsUniqueViolation` | `storetest.Open` helper, no own tests |
| `cmd/adx` | env config (`ADX_ENV`, `ADX_ADDR`, `ADX_DB_PATH`, `ADX_GHL_CREDENTIALS_FILE`, `ADX_CONNECTION_KEY_FILE`, `ADX_BOOTSTRAP_ADMIN`, `ADX_AUTHENTIK_*`), startup order, dev-only localhost rule | `main_test.go` |
| `cmd/gentypes` | guts: Go json structs in `./api` and `./core/...` to `web/src/api/types.gen.ts` | none |

Migrations (goose, `store/migrations`):

| Version | Tables |
|---|---|
| 00001 | `projects` |
| 00002 | `connection_oauth_attempts`, `connections` |
| 00003 | `connections` rebuilt with `auth_method` |
| 00004 | `users`, `audit_log` |
| 00005 | `credentials`, `accounts` (data copied from `connections`, which is dropped) |

Queries: `audit.sql`, `connections.sql`, `projects.sql`, `users.sql`.

Baseline test run (2026-10-17): `go test ./...` all packages pass. `go vet` and `golangci-lint run` report 0 issues. No pre-existing failures.

## Web (`web/`)

- Vite 8, React 19, TanStack Router and Query, Tailwind 4, shadcn (base-nova), biome, vitest. `pnpm@10.33.4` pinned.
- Routes: `/` redirects to `/projects`; `/projects` (list and add); `/privacy` (also pre-rendered to `privacy/index.html` at build).
- API client: `getJSON`/`postJSON` with `ApiError`; `useProjects`, `useCreateProject`.
- `lib/format.ts`: `formatMoney(minor, currency)` and `NOT_AVAILABLE`.
- Baseline: `pnpm check`, `pnpm typecheck`, `pnpm test` (4 tests) pass.

## Tooling installed for this build

| Tool | Version | How |
|---|---|---|
| Go | 1.27.2 (module says 1.26.0) | present |
| Node | 22.23.1 (`~/.local/bin`); nvm also has 20.20.2 | present |
| pnpm | 10.33.4 | `npm install -g pnpm@10.33.4` under node 22, and corepack under nvm 20 |
| sqlc | 1.31.1 | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1` (the Makefile also runs it through `tools.mod`) |
| golangci-lint | 2.14.0 | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0` |
| air | not installed | optional, only for `make dev` |

The `/deslop` skill the repo asks for before publishing docs is not present on this machine. Docs in this build were not run through it.

## Reuse sources

| Spec name | Spec path | Actual path here | State |
|---|---|---|---|
| Voice source (Stafford Sales OS) | `/Users/hishamahamad/Documents/WokeWorks/stafford-dead-leads/` | `/Users/abdulrishan/Desktop/stafford-dead-leads/` | present: `voice.md`, `lib/twilio/`, `lib/interactions.ts`, `lib/signals/*`. Inventory in `docs/build/reuse-voice.md` (B0.6) |
| Creative source | `/Users/hishamahamad/Downloads/CODE_SNAPSHOT.md` | not on this machine. Likely equivalent: `/Users/abdulrishan/Desktop/creative-studio/` (Next.js app) | inventoried in `docs/build/reuse-creative.md` (B0.6); units missing from it are marked blocked |
| Marketing OS (`WokeWorks/ads-mcp`) | not referenced by the spec | not on this machine | only `research/sources/marketing-os-overview.md` |

## Routes that exist versus routes the spec adds

Existing: `/api/health`, `/api/projects` (GET, POST), `/api/connections/crm` (GET), `/api/connections/crm/connect` (GET, HTML form), `/api/connections/crm/start` (POST), `/api/connections/crm/private` (POST form), `/api/connections/crm/callback` (GET), `/api/connections/crm/{locationID}/check` (POST).

Everything in the spec's route families (`/api/session`, `/api/accounts`, `/api/analytics`, …) is new.

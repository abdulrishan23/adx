.PHONY: dev dev-server dev-web gen migration test check build

# Runs the Go server (rebuilt on save by air) and the Vite dev server together.
# Open http://localhost:5173.
dev:
	$(MAKE) -j2 dev-server dev-web

dev-server:
	cd server && ADX_ENV=dev air

dev-web:
	cd web && pnpm dev

# Regenerates sqlc code from store/queries and the TypeScript API types from
# Go structs. Commit what it produces.
gen:
	cd server && go tool -modfile=tools.mod sqlc generate
	cd server && go run ./cmd/gentypes

# Creates the next numbered migration file: make migration name=add_leads
migration:
	cd server && go tool -modfile=tools.mod goose -dir store/migrations -s create $(name) sql

test:
	cd server && go test ./...
	cd web && pnpm test

check:
	cd server && go vet ./... && golangci-lint run
	cd web && pnpm check && pnpm typecheck

build:
	cd server && go build -o bin/adx ./cmd/adx
	cd web && pnpm build

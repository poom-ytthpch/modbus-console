.PHONY: preflight install typecheck lint test build security release-check full dev core tunnel
preflight:
	./scripts/preflight.sh
install:
	pnpm install
	cd core && go mod download
typecheck:
	pnpm typecheck
lint:
	pnpm lint
test:
	pnpm test
build:
	pnpm build
security:
	cd core && staticcheck ./... && "$$(go env GOPATH)/bin/govulncheck" ./...
release-check:
	./scripts/release-check.sh
full:
	./scripts/full-check.sh
dev:
	pnpm dev
core:
	pnpm core:run
tunnel:
	./scripts/dev-tunnel.sh

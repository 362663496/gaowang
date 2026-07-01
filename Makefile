-include .env
export

.PHONY: api-test api-run web-install web-dev compose-up compose-down

api-test:
	cd apps/api && go test ./...

api-run:
	cd apps/api && go run ./cmd/api

web-install:
	@if [ -f apps/web/package.json ]; then cd apps/web && npm install; else echo "web app not scaffolded yet"; fi

web-dev:
	@if [ -f apps/web/package.json ]; then cd apps/web && npm run dev; else echo "web app not scaffolded yet"; fi

compose-up:
	docker compose up --build

compose-down:
	docker compose down

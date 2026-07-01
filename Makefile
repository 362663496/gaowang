.PHONY: api-test api-run web-install web-dev compose-up compose-down

api-test:
	cd apps/api && go test ./...

api-run:
	cd apps/api && go run ./cmd/api

web-install:
	cd apps/web && npm install

web-dev:
	cd apps/web && npm run dev

compose-up:
	docker compose up --build

compose-down:
	docker compose down

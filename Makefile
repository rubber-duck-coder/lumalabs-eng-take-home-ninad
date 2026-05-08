.PHONY: unit integration e2e verify run frontend-install frontend-dev frontend-build compose-up compose-down

unit:
	go test ./...

integration:
	go test -tags=integration ./integration/...

e2e:
	@if [ -z "$(BASE_URL)" ]; then echo "BASE_URL is required (for example BASE_URL=http://localhost:8080 make e2e)"; exit 1; fi
	BASE_URL=$(BASE_URL) go test -tags=e2e ./e2e/...

verify: unit integration frontend-build

run:
	go run ./cmd/control-plane

frontend-install:
	cd frontend && npm install

frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm install && npm run build

compose-up:
	docker compose up --build

compose-down:
	docker compose down -v

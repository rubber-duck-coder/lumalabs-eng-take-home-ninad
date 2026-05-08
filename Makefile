.PHONY: unit integration e2e verify run frontend-install frontend-dev frontend-build compose-up compose-down

unit:
	go test ./...

integration:
	go test -tags=integration ./integration/...

e2e:
	@echo "e2e not implemented yet; set BASE_URL when available"

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

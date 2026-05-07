.PHONY: unit integration e2e verify run

unit:
	go test ./...

integration:
	go test ./...

e2e:
	@echo "e2e not implemented yet; set BASE_URL when available"

verify: unit

run:
	go run ./cmd/control-plane

.PHONY: unit integration e2e verify run frontend-install frontend-dev frontend-build compose-up compose-down vm-deploy gcp-check-env gcp-vm-create gcp-vm-ssh gcp-vm-deploy gcp-vm-url gcp-vm-reviewer

GCP_VM_NAME ?= luma-take-home-review
GCP_ZONE ?= us-west1-b
GCP_MACHINE_TYPE ?= e2-standard-2
GCP_IMAGE_FAMILY ?= ubuntu-2204-lts
GCP_IMAGE_PROJECT ?= ubuntu-os-cloud
GCP_REPO_URL ?= https://github.com/rubber-duck-coder/lumalabs-eng-take-home-ninad.git
GCP_CREDENTIALS_FILE ?=

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

vm-deploy:
	git pull --ff-only
	docker compose up --build -d --remove-orphans
	docker compose ps

gcp-check-env:
	@if [ -z "$(GOOGLE_CLOUD_PROJECT)" ]; then echo "GOOGLE_CLOUD_PROJECT is required"; exit 1; fi
	@if [ -z "$(GCP_CREDENTIALS_FILE)" ]; then echo "GCP_CREDENTIALS_FILE is required"; exit 1; fi
	@gcloud auth activate-service-account --key-file="$(GCP_CREDENTIALS_FILE)" >/dev/null
	@gcloud config set project "$(GOOGLE_CLOUD_PROJECT)" >/dev/null
	@echo "gcloud auth/project configured for $(GOOGLE_CLOUD_PROJECT)"

gcp-vm-create: gcp-check-env
	@if gcloud compute instances describe "$(GCP_VM_NAME)" --zone "$(GCP_ZONE)" >/dev/null 2>&1; then \
		echo "VM $(GCP_VM_NAME) already exists in $(GCP_ZONE)"; \
	else \
		gcloud compute instances create "$(GCP_VM_NAME)" \
			--zone "$(GCP_ZONE)" \
			--machine-type "$(GCP_MACHINE_TYPE)" \
			--image-family "$(GCP_IMAGE_FAMILY)" \
			--image-project "$(GCP_IMAGE_PROJECT)" \
			--boot-disk-size "30GB" \
			--tags http-server,https-server; \
	fi

gcp-vm-ssh: gcp-check-env
	@gcloud compute ssh "$(GCP_VM_NAME)" --zone "$(GCP_ZONE)"

gcp-vm-deploy: gcp-check-env
	@gcloud compute ssh "$(GCP_VM_NAME)" --zone "$(GCP_ZONE)" --command "\
		sudo apt-get update -y && \
		sudo apt-get install -y git ca-certificates curl make && \
		if ! command -v docker >/dev/null 2>&1; then \
			curl -fsSL https://get.docker.com | sudo sh; \
			sudo usermod -aG docker $$USER; \
		fi && \
		if [ ! -d lumalabs-eng-take-home-ninad/.git ]; then \
			git clone $(GCP_REPO_URL); \
		fi && \
		cd lumalabs-eng-take-home-ninad && \
		git pull --ff-only && \
		docker compose up --build -d --remove-orphans && \
		docker compose ps \
	"

gcp-vm-url: gcp-check-env
	@IP=$$(gcloud compute instances describe "$(GCP_VM_NAME)" --zone "$(GCP_ZONE)" --format='value(networkInterfaces[0].accessConfigs[0].natIP)'); \
	if [ -z "$$IP" ]; then echo "No external IP found for $(GCP_VM_NAME)"; exit 1; fi; \
	echo "Share this URL: http://$$IP"; \
	echo "Health check: curl -sf http://$$IP/api/health"

gcp-vm-reviewer: gcp-vm-create gcp-vm-deploy gcp-vm-url
	@echo "Reviewer flow complete."

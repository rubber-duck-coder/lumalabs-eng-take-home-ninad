# Infra RFC

## Goal

Provide a fresh-checkout local setup, cheap deployment path, and repeatable verification against both local and deployed environments.

## Non-Goals

- Kubernetes.
- Real GPU infrastructure.
- Dedicated domain.
- Paid observability vendor.
- Complex IaC.

## Architecture Assumption

- Prefer one public origin serving API and UI.
- Split containers only if frontend/backend tooling makes one container awkward.
- Local command: `docker compose up --build`.
- Primary deploy target: Google Cloud VM running Docker Compose.
- Target spend: `$0`; hard cap: `$100`.

## Local Development

Expected setup:

```bash
cp .env.example .env
docker compose up --build
```

Expected URLs:

- App: `http://localhost:8080`
- Health: `http://localhost:8080/health`

Seed data should include mixed GPU types, capacity classes, providers/zones, and demo workloads.

## Config

Use `.env.example` for all config.

Suggested values:

```bash
PORT=8080
APP_ENV=local
DATABASE_URL=
SEED_DEMO_DATA=true
LOG_LEVEL=info
PUBLIC_BASE_URL=http://localhost:8080
```

No secrets should be committed.

## Deployment

- Use a single VM with Docker Compose and a public Nginx frontend.
- Prefer Dockerfile deploy if Docker is used locally.
- Use `/health` for health checks.
- Document VM startup and container boot timing.
- Use managed Postgres for the demo/runtime path; reserve CockroachDB for production multi-region deployments.

Alternatives:

- Railway if the VM approach is blocked.
- Fly.io if Docker deployment needs more control.

## Verification Commands

Preferred commands:

```bash
make unit
make integration
make e2e
make verify
```

`make e2e` should accept `BASE_URL`:

```bash
BASE_URL=http://localhost:8080 make e2e
BASE_URL=http://<vm-public-ip> make e2e
```

## E2E Scope

- Submit workload.
- Observe placement or queue reason.
- View fleet summary and utilization.
- Trigger node failure or spot preemption.
- Verify affected workload state, events, and utilization update.

## Observability

- Structured logs to stdout.
- Request logs.
- Scheduler decision events.
- Disruption events.
- `/health`.
- Recent events visible through API/UI.

## Phases

- Phase 1: Docker Compose, health endpoint, seed data, verify command.
- Phase 2: scheduler test commands and local demo flow.
- Phase 3: disruption demo and integration tests.
- Phase 4: Google Cloud VM deploy and local/deployed E2E.

## Open Questions

- Should the deployed demo use managed Postgres by default, with CockroachDB reserved for production multi-region deployments?
- Should the public origin be a single reverse-proxied frontend/API host, or should the frontend call the API directly?

## Technology Decisions

### Local Runtime: Docker Compose

Decision: use Docker Compose for local development.

Pros:
- Fresh-checkout path is predictable.
- Mirrors container deployment.
- Easy reviewer command.

Cons:
- Requires Docker installed.
- Slightly slower than native dev commands.

Alternatives:
- Native Python/Node commands: faster for developers, less consistent for reviewers.
- Dev containers: strong isolation, more setup than needed.

NFR fit:
- Reliability: consistent environment reduces setup failures.
- Maintainability: one local command keeps docs simple.

### Deployment Target: Google Cloud VM First

Decision: use a Google Cloud VM as the primary deployment target.

Pros:
- Full control over networking and runtime.
- Simple Docker Compose deploy model matches local development.
- One public URL can serve the frontend and proxy API calls.
- No dependency on a hosted PaaS-specific deploy pipeline.

Cons:
- More VM maintenance than a hosted PaaS.
- Firewall and OS hardening are the operator's responsibility.

Alternatives:
- Render: simpler hosted deploy, but not the chosen path for this repo.
- Railway: simple deploy, but free trial/credits are limited and verification can restrict network access.
- Fly.io: strong Docker support, but current pricing is pay-as-you-go and has more operational knobs.

NFR fit:
- Cost: within budget for a single small VM.
- Availability: enough for demo, not production.
- Latency: no hosted cold start, just VM startup and container boot.
- Operability: logs and health checks are sufficient for review.

### Service Shape: One App First

Decision: prefer one public origin serving UI and API.

Pros:
- Avoids CORS and multi-service public deploy coordination.
- Lower cost and simpler health checks.
- Easier E2E target with one `BASE_URL`.

Cons:
- Less representative of production separation.
- Frontend/backend build pipeline must be coordinated.

Alternatives:
- Split static frontend plus API on separate public origins: cleaner separation, more deploy config.
- API-only with FastAPI docs: fastest, but weaker user/admin experience.

NFR fit:
- Availability and reliability improve by reducing moving parts.
- Maintainability is acceptable for take-home scale.

### Database Choice: Postgres for Demo, CockroachDB for Production

Decision: use managed Postgres for the demo/runtime path, and prefer CockroachDB if the product needs native multi-region production durability.

Pros:
- Postgres is easy to run locally and widely understood by reviewers.
- CockroachDB preserves the same SQL shape while adding multi-region primitives.
- The decision keeps the demo simple without locking out a production upgrade path.

Cons:
- CockroachDB is more operationally complex than Postgres.
- The extra production preference adds another step to the platform story.

Alternatives:
- In-memory state: too fragile for the demo runtime.
- SQLite: easy locally, but not the right operational model for a fleet control plane.

NFR fit:
- Demo operability improves with a real database.
- Production resilience improves with a multi-region option.

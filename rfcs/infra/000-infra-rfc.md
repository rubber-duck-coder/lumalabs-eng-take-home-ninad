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

- Prefer one containerized app serving API and UI.
- Split services only if frontend/backend tooling makes one container awkward.
- Local command: `docker compose up --build`.
- Primary deploy target: Render.
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

- Use Render Web Service with repo-based deploy.
- Prefer Dockerfile deploy if Docker is used locally.
- Use `/health` for health checks.
- Document cold-start behavior if using free tier.
- Add managed Postgres only if in-memory or SQLite state blocks the demo.

Alternatives:

- Railway if Render blocks deployment.
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
BASE_URL=https://<render-app>.onrender.com make e2e
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
- Phase 4: Render deploy and local/deployed E2E.

## Open Questions

- Will Render free-tier cold starts affect E2E reliability?
- Is in-memory state acceptable for deployed review?
- Should deploy be one service or split static frontend plus API?

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

### Deployment Target: Render First

Decision: use Render as primary deployment target.

Pros:
- Official docs support free web services/static sites for preview use.
- Simple Git-based deploy.
- Built-in logs, health checks, env vars, and generated URL.
- No dedicated domain required.

Cons:
- Free instances can cold start.
- Free-tier limits are not production-grade.

Alternatives:
- Railway: simple deploy, but free trial/credits are limited and verification can restrict network access.
- Fly.io: strong Docker support, but current pricing is pay-as-you-go and has more operational knobs.

NFR fit:
- Cost: best fit for `$0` target under `$100` cap.
- Availability: enough for demo, not production.
- Latency: cold starts are acceptable if documented and E2E uses health polling.
- Operability: logs and health checks are sufficient for review.

### Service Shape: One App First

Decision: prefer one deployed service serving UI and API.

Pros:
- Avoids CORS and multi-service deploy coordination.
- Lower cost and simpler health checks.
- Easier E2E target with one `BASE_URL`.

Cons:
- Less representative of production separation.
- Frontend/backend build pipeline must be coordinated.

Alternatives:
- Split static frontend plus API: cleaner separation, more deploy config.
- API-only with FastAPI docs: fastest, but weaker user/admin experience.

NFR fit:
- Availability and reliability improve by reducing moving parts.
- Maintainability is acceptable for take-home scale.

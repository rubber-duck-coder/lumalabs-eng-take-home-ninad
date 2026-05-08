# Task Checkpoints

## Purpose

Track granular progress so work can resume after rate limits, context loss, battery loss, or agent interruption.

## Status Legend

- `todo`: not started.
- `doing`: active.
- `blocked`: needs decision or dependency.
- `review`: implementation done, needs coordinator review.
- `done`: accepted and checkpointed.

## Checkpoint Protocol

At every logical checkpoint, update:

- Task status.
- Owner or agent.
- Files changed.
- Tests run.
- Decisions made.
- Blockers or follow-ups.
- Resume note.

## Current State

- Phase: Phase 5 local infra.
- Last completed checkpoint: Phase 4 frontend shell + admin dashboard.
- Active implementation: dockerization scaffolding and Postgres-backed persistence next.
- Next recommended task: implement the Postgres store and validate the compose stack.

## Decision Log Index

- Backend runtime: Go modular monolith, logged in `rfcs/be/000-backend-rfc.md` and `rfcs/be/001-scalable-backend-runtime-and-state-rfc.md`.
- External API: REST first, gRPC deferred until service split.
- State: in-memory for skeleton only; Postgres for the demo/runtime path, CockroachDB for production multi-region durability.
- Deployment target: Render first, Railway/Fly fallback.

## Task Board

| ID | Phase | Task | Owner | Status | Dependencies | Resume Note |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | 0 | Create Go module layout | coordinator | done | none | Go module created. |
| T002 | 0 | Add `Makefile` with unit/verify placeholders | coordinator | done | T001 | `make unit` and `make verify` available. |
| T003 | 0 | Add health endpoint | coordinator | done | T001 | `GET /health` implemented. |
| T004 | 0 | Add first unit test | coordinator | done | T001 | Gateway health test added. |
| T005 | 1 | Define domain models | backend agent | done | T001 | Workload, node, event types added. |
| T006 | 1 | Implement scheduler package | backend agent | done | T005 | Policy implemented in `internal/scheduler`. |
| T007 | 1 | Add scheduler unit tests | backend agent | done | T006 | Resource fit, priority, queueing, spot tests added. |
| T008 | 1 | Add in-memory repositories and seed data | backend agent | done | T005 | Deterministic seed fleet added. |
| T009 | 1 | Add workload submit/list APIs | coordinator | done | T006,T008 | Return placement or queue reason. |
| T010 | 2 | Add fleet summary/utilization API | coordinator | done | T008 | Admin read path added early. |
| T011 | 2 | Add event log API | coordinator | done | T008 | Scheduler/admin visibility added early. |
| T012 | 2 | Add node failure endpoint | backend agent | done | T006,T008 | Implemented and adversarial-review fixes applied. |
| T013 | 2 | Add spot preemption endpoint | backend agent | done | T006,T008 | Implemented and adversarial-review fixes applied. |
| T014 | 2 | Add node recovery endpoint | backend agent | done | T006,T008 | Implemented and adversarial-review fixes applied. |
| T015 | 3 | Add API integration tests | test agent | done | T009-T014 | Implemented, reviewed, and validated. |
| T016 | 4 | Add frontend dashboard shell | frontend agent | done | stable APIs | Implemented as React+Vite shell. |
| T017 | 4 | Add workload submission UI | frontend agent | done | T009,T016 | Implemented against `/workloads`. |
| T018 | 4 | Add admin dashboard sections | frontend agent | done | T010-T014,T016 | Fleet, utilization, events, disruptions. |
| T019 | 5 | Add Dockerfile | infra agent | done | T003,T016 | One app container preferred. |
| T020 | 5 | Add Docker Compose | infra agent | done | T019,T025 | Local full stack. |
| T021 | 6 | Add parameterized E2E suite | test agent | todo | T016-T020 | Uses `BASE_URL`. |
| T022 | 7 | Add Render deploy docs/config | infra agent | todo | T020 | Budget-safe deploy. |
| T023 | 8 | Write `APPROACH.md` | coordinator | todo | core demo stable | Capture tradeoffs. |
| T024 | 8 | Update README and video notes | coordinator | todo | T023 | Submission polish. |
| T025 | 5 | Add Postgres-backed store | backend + infra | todo | T009-T018 | Replace in-memory persistence for the demo/runtime path and wire `DATABASE_URL`. |

## Checkpoint Entries

### 000: Planning Baseline

Status: done

Files:
- `prd/000-gpu-control-plane-prd.md`
- `plans/000-master-rfc-plan.md`
- `plans/001-execution-plan.md`
- `rfcs/fe/000-frontend-rfc.md`
- `rfcs/be/000-backend-rfc.md`
- `rfcs/be/001-scalable-backend-runtime-and-state-rfc.md`
- `rfcs/infra/000-infra-rfc.md`

Tests run:
- None; planning only.

Decisions:
- Go modular monolith for backend v1.
- REST external API.
- In-memory first, Postgres target.
- Render primary deployment target.

Resume note:
- Start at T001 with repo skeleton. Do not implement beyond Phase 0 until skeleton is reviewed or tests pass.

### 001: Backend MVP Checkpoint

Status: done

Owner: coordinator plus backend coding agents

Tasks:
- T001 through T011 complete.

Files:
- `go.mod`
- `Makefile`
- `cmd/control-plane/main.go`
- `internal/domain/*`
- `internal/store/*`
- `internal/scheduler/*`
- `internal/gateway/*`

Tests run:
- `make unit`
- `make unit` after adversarial-review fixes

Decisions:
- Scheduler package currently uses local scheduler DTOs with gateway adapters to domain models.
- Fleet summary and event APIs were pulled into Phase 1 because they were low-cost and useful for verification.
- Store now owns atomic schedule-and-allocate for in-memory state to prevent API-level split-brain updates.

Blockers:
- Admin disruption APIs remain Phase 2 work.
- Scheduler tick/global pending queue remains Phase 2 work.

Resume note:
- Initial checkpoint is ready to commit and push.

Review notes:
- Two adversarial review agents flagged non-atomic scheduling and possible GPU over-allocation as commit-blocking.
- Fixed with `MemoryStore.ScheduleWorkload`, which runs scheduler decision and workload/node mutation under one lock.
- Added tests for atomic scheduling and concurrent API submissions.
- Also fixed timestamp-only ID collision by adding per-process atomic sequence suffix.

### 002: Local Secret Protection

Status: done

Owner: coordinator

Tasks:
- Expanded `.gitignore` for local env, credentials, keys, logs, and build artifacts.
- Added local pre-commit hook to block obvious secret paths and staged secret-looking diffs.
- Added hook installer script.
- Removed tracked `excalidraw.log` from git while keeping local ignored log behavior.

Files:
- `.gitignore`
- `scripts/git-hooks/pre-commit`
- `scripts/install-git-hooks.sh`
- `excalidraw.log` removed from git tracking

Tests run:
- `make unit`
- Installed local hook with `sh scripts/install-git-hooks.sh`
- Verified hook blocks staged fake API-key content
- Verified `.env`, `.take-home-token`, and `excalidraw.log` are ignored

Review notes:
- Two adversarial agents reviewed the secret-protection changes.
- We accepted local-only protection as sufficient for this private solo repo.
- Fixed the high-value hook issue: staged file iteration is now null-delimited and whitespace-safe.
- Deferred CI/server-side secret scanning by user decision.

Resume note:
- Commit and push this guardrail checkpoint before Phase 2.

### 003: Phase 2 Disruption APIs

Status: done

Owner: coordinator plus backend coding agent

Tasks:
- Added atomic pending queue scheduling.
- Added scheduler tick endpoint.
- Added node failure, recovery, and spot preemption endpoints.
- Added store and gateway tests for disruption flows.

Files:
- `internal/store/memory.go`
- `internal/store/memory_test.go`
- `internal/gateway/router.go`
- `internal/gateway/router_test.go`
- `plans/002-task-checkpoints.md`

Tests run:
- `make unit`
- `make unit` after adversarial-review fixes

Decisions:
- Disrupted workloads are requeued and immediately rescheduled when capacity exists.
- Spot preemption marks the spot node failed, frees allocation, records preemption, and requeues affected workloads.
- Scheduler tick is explicit and deterministic; no background timer yet.

Review notes:
- Two adversarial review agents flagged silent scheduler error swallowing and stale `RunningWorkloadIDs` eviction as commit-blocking.
- Fixed scheduler pending pass to propagate internal scheduling errors.
- Fixed node eviction to derive affected workloads from running placement state instead of only node bookkeeping.
- Added regression tests for stale running-list eviction, invalid disruption requests, and disruption event emission.

Blockers:
- None currently known.

Resume note:
- Commit and push Phase 2 disruption checkpoint.

### 004: Phase 3 API Integration Tests

Status: done

Owner: coordinator plus test coding agent

Tasks:
- Added integration-only API test suite under `integration/`.
- Added flow coverage for submit/inspect/events and disruption lifecycle.
- Scoped `make integration` to run integration-tagged package tests only.

Files:
- `integration/api_integration_test.go`
- `Makefile`
- `plans/002-task-checkpoints.md`

Tests run:
- `make unit`
- `make integration`

Decisions:
- Integration suite uses HTTP-level assertions via `httptest.NewServer` with real router/store wiring.
- Integration tests are tag-gated with `//go:build integration` and isolated from default unit target.

Blockers:
- None currently known.

Resume note:
- Commit and push this Phase 3 checkpoint.

Review notes:
- Adversarial review 1 found no blockers; noted minor residual risk around event payload depth.
- Adversarial review 2 flagged medium gaps in node-state and event-linkage assertions.
- Added stronger integration assertions for node health/allocation transitions and event attribution counts.
- Re-ran `make unit` and `make integration`; both pass.

### 005: Phase 4 Frontend Shell + Submit Flow

Status: done

Owner: coordinator

Tasks:
- Added standalone `frontend/` React+Vite+TypeScript app.
- Implemented workload submit form and result panel.
- Implemented workload list, fleet summary, node inventory, event log, and admin disruption controls.
- Added backend CORS support for local cross-origin frontend dev.
- Added Make targets for frontend install/dev/build and wired frontend build into `make verify`.

Files:
- `frontend/*`
- `frontend/package-lock.json`
- `frontend/src/vite-env.d.ts`
- `internal/gateway/router.go`
- `internal/gateway/router_test.go`
- `Makefile`
- `plans/002-task-checkpoints.md`

Tests run:
- `make unit`
- `npm run build`
- `make verify`

Decisions:
- Keep backend API-only; frontend runs as separate app.
- Use dependency-light frontend stack aligned with RFC (`React + Vite + TypeScript`).
- `make verify` now includes the frontend production build.
- Frontend stays local-first and consumes only the backend API surface.

Blockers:
- None currently known.

Resume note:
- Commit and push the Phase 4 frontend checkpoint.

### 006: Phase 5 Dockerized Local Stack

Status: done

Owner: coordinator

Tasks:
- Added backend `Dockerfile`.
- Added frontend `Dockerfile` and `nginx.conf`.
- Added `docker-compose.yml` wiring the API, frontend, and Postgres services.
- Added `.dockerignore` files for root and frontend build contexts.
- Added `make compose-up` and `make compose-down` helpers.

Files:
- `Dockerfile`
- `frontend/Dockerfile`
- `frontend/nginx.conf`
- `docker-compose.yml`
- `.dockerignore`
- `frontend/.dockerignore`
- `Makefile`
- `plans/002-task-checkpoints.md`

Tests run:
- `docker compose config`
- `make verify`
- `go test ./...` with repo-local `GOCACHE`/`GOMODCACHE`

Decisions:
- Keep the app split into API and frontend containers for now.
- Wire Postgres into compose so the runtime can switch over cleanly when the store implementation lands.

Blockers:
- Postgres-backed store implementation still needs to land.

Resume note:
- Implement the Postgres store next, then switch the backend runtime to it.

### 007: Demo Data Seed/Clear Controls

Status: done

Owner: coordinator

Tasks:
- Added one-click admin dashboard buttons to seed and clear demo data.
- Added in-memory demo dataset reset support on the backend.
- Exposed `POST /admin/demo/seed` and `POST /admin/demo/clear` for the dashboard.
- Seeded deterministic nodes, workloads, and event log entries so the UI starts with useful demo state.

Files:
- `frontend/src/App.tsx`
- `internal/gateway/router.go`
- `internal/gateway/router_test.go`
- `internal/store/memory.go`
- `internal/store/memory_test.go`
- `internal/store/store.go`
- `plans/002-task-checkpoints.md`

Tests run:
- `make verify`
- `docker compose up --build -d`
- `curl -i -s -X POST http://localhost:8080/admin/demo/seed`
- `curl -i -s -X POST http://localhost:8080/admin/demo/clear`
- `curl -sf http://localhost:8080/health`

Decisions:
- Keep demo reset operations local and deterministic.
- Seed data should include both running and queued workloads so the dashboard has immediate contrast.
- Clear should remove all in-memory state so admin users can return to a clean slate quickly.

Blockers:
- None currently known.

Resume note:
- Next move is the Postgres-backed store migration.

## Template

### NNN: Short Checkpoint Name

Status:

Owner:

Tasks:

Files:

Tests run:

Decisions:

Blockers:

Resume note:

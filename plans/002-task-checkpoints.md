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

- Phase: Phase 1 backend MVP.
- Last completed checkpoint: local secret-protection guardrails added and verified.
- Active implementation: none.
- Next recommended task: pre-commit adversarial review, initial git commit, then Phase 2 disruptions.

## Decision Log Index

- Backend runtime: Go modular monolith, logged in `rfcs/be/000-backend-rfc.md` and `rfcs/be/001-scalable-backend-runtime-and-state-rfc.md`.
- External API: REST first, gRPC deferred until service split.
- State: in-memory for skeleton, Postgres target when durable state is needed.
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
| T012 | 2 | Add node failure endpoint | backend agent | todo | T006,T008 | Disruption path. |
| T013 | 2 | Add spot preemption endpoint | backend agent | todo | T006,T008 | Spot disruption path. |
| T014 | 2 | Add node recovery endpoint | backend agent | todo | T006,T008 | Recovery path. |
| T015 | 3 | Add API integration tests | test agent | todo | T009-T014 | Real app wiring. |
| T016 | 4 | Add frontend dashboard shell | frontend agent | todo | stable APIs | Single page first. |
| T017 | 4 | Add workload submission UI | frontend agent | todo | T009,T016 | Enterprise flow. |
| T018 | 4 | Add admin dashboard sections | frontend agent | todo | T010-T014,T016 | Fleet, utilization, events, disruptions. |
| T019 | 5 | Add Dockerfile | infra agent | todo | T003,T016 | One app container preferred. |
| T020 | 5 | Add Docker Compose | infra agent | todo | T019 | Local full stack. |
| T021 | 6 | Add parameterized E2E suite | test agent | todo | T016-T020 | Uses `BASE_URL`. |
| T022 | 7 | Add Render deploy docs/config | infra agent | todo | T020 | Budget-safe deploy. |
| T023 | 8 | Write `APPROACH.md` | coordinator | todo | core demo stable | Capture tradeoffs. |
| T024 | 8 | Update README and video notes | coordinator | todo | T023 | Submission polish. |

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

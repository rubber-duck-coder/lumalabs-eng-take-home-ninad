# Execution Plan

## Goal

Ship the smallest working GPU control plane first, then iterate in tested layers until local, deployed, and E2E flows are complete.

## Operating Principles

- Always keep `main` runnable.
- Build MVP first; add complexity only after verification.
- Prefer parallel workstreams with clear file ownership.
- Use unit tests for every core behavior before integration/E2E.
- Use smaller/faster coding agents for bounded implementation tasks.
- Keep coordinator/planner on the strongest available model for cross-workstream context.
- Record major tradeoff decisions in the relevant RFC `Decision Log`.

## MVP Definition

MVP is complete when:

- One Go backend process starts locally.
- Seeded fleet is available through API.
- Workload can be submitted.
- Scheduler places or queues workload with explanation.
- Admin can view fleet, workloads, utilization, and events through API.
- Unit tests cover scheduler resource fit, priority, queueing, and spot tolerance.
- `make unit` and `make verify` exist.

No frontend or deployment is required for MVP.

## Phase 0: Repo Skeleton

Owner: coordinator or small coding agent.

Deliver:
- Go module layout.
- `Makefile`.
- Basic app entrypoint.
- Health endpoint.
- Test command wiring.

Acceptance:
- `make unit` passes.
- Local app starts.
- `GET /health` works.

Parallelism:
- Low; establish shared structure first.

## Phase 1: Backend Core MVP

Owner: backend coding agents with disjoint modules.

Parallel tasks:
- Domain models: workloads, nodes, events.
- Scheduler package: placement and queueing logic.
- In-memory repositories and seed data.
- Gateway/API handlers for health, fleet, workloads.
- Unit tests for scheduler behavior.

Acceptance:
- Submit workload through API.
- Workload becomes `running` or `pending`.
- Response includes placement or queue reason.
- Scheduler tests pass.

Coordination rule:
- Scheduler owns policy only.
- API handlers do not duplicate scheduling logic.

## Phase 2: Admin And Disruptions

Owner: backend coding agents.

Parallel tasks:
- Fleet summary and utilization API.
- Event log API.
- Node failure endpoint.
- Spot preemption endpoint.
- Node recovery endpoint.
- Disruption unit/API tests.

Acceptance:
- Admin can trigger disruption.
- Affected workloads are rescheduled, queued, preempted, or failed.
- Events explain what happened.

## Phase 3: Integration Tests

Owner: test-focused coding agent.

Deliver:
- API integration tests using real app wiring.
- Deterministic seed/reset path.
- Submit -> schedule -> inspect flow.
- Disruption -> reschedule/queue flow.

Acceptance:
- `make integration` passes locally.
- Tests do not require external services.

## Phase 4: Minimal Frontend

Owner: frontend coding agents.

Parallel tasks:
- Single-page dashboard shell.
- Workload submission form.
- Workload/fleet/event tables.
- Utilization summary.
- Disruption controls.
- Frontend unit/component tests.

Acceptance:
- Reviewer can complete demo flow from UI.
- UI displays backend explanations.
- `make verify` includes frontend checks.

## Phase 5: Local Infra

Owner: infra coding agent.

Deliver:
- Dockerfile.
- `docker-compose.yml`.
- `.env.example` updates.
- Local run docs.
- Health polling helper if needed.

Acceptance:
- Fresh checkout can run with `docker compose up --build`.
- API and UI available at one local URL.

## Missing Control-Plane Behaviors

The current system now runs, schedules, persists state, and reacts to explicit node disruptions. The remaining gaps versus the original problem statement are:

- The demo is not yet exercising the full set of behaviors through end-to-end scenarios and deployment docs.
- The backend could still grow stronger rebalance actions beyond conservative pending-order policy if we decide the demo needs live movement of running workloads.
- The frontend now has a left-side navigation split for user flow, admin dashboard, and admin ops, but the remaining UI work should stay focused on clarity rather than new surface area.

Recommended follow-up sequence:

1. Modularize the control plane into clearer internal responsibilities.
2. Cover the implemented behaviors in E2E scenarios and submission docs.
3. Decide whether the demo needs stronger rebalance actions beyond pending-order policy.

## Scheduling Strategy

This is the optimization policy that will sit under the scheduler once the control-plane boundaries are stable.

Policy shape:
- Hard constraints first.
- Class-aware scoring second.
- Rebalance only on meaningful state changes.
- Prefer stability over churn.

Hard constraints:
- GPU type and GPU count must fit.
- Node health must allow scheduling.
- Capacity class must be allowed for the workload.
- Zone/provider constraints must be respected.
- Spot tolerance must be honored.

Soft objectives:
- Training: reliability and uninterrupted runtime.
- Inference: latency, replica spread, and availability.
- Batch: cost efficiency and throughput.
- Cross-cutting: utilization, fragmentation, fairness, and locality.

Rebalance triggers:
- Node health changes.
- Spot preemption risk.
- Priority inversion.
- Material demand shift across GPU types or zones.
- Fragmentation that strands capacity.

Anti-churn rule:
- Do not move running workloads unless the expected improvement is large enough or the workload is resumable/checkpointable.

Acceptance:
- Scheduler decisions remain explainable.
- Workload-class behavior is explicit in tests.
- Rebalance thresholds are deterministic and easy to tune.

Current implementation:
- Training keeps packing tight on eligible on-demand nodes.
- Inference prefers less-utilized eligible on-demand capacity.
- Batch prefers spot when tolerated and otherwise packs tightly on on-demand nodes.
- Inference workloads now support replica-aware scale-out placement across distinct eligible nodes.
- Priority preemption is now active in the store layer for higher-priority workloads when a fit exists only after eviction of lower-priority work.
- Pending workload ordering is now class-aware within the same priority tier, favoring inference over training over batch and non-spot work before spot-tolerant work.

## Phase 6: Control-Plane Modularization

Owner: coordinator plus backend coding agents.

Goal:
- Keep one binary and one database, but split the control plane into explicit internal modules with single responsibilities.

Current progress:
- `gateway` now stays at the transport boundary.
- `controlplane` owns orchestration.
- `events` and `fleet` are extracted as dedicated internal packages.
- `workloads` is now extracted for submit, queue, and scheduler-tick orchestration.
- `fleet` now also owns node disruption and preemption policy.
- `reconciler` now owns health simulation and automatic recovery flows.
- Workload submission now carries resumability and drain/checkpoint metadata for preemption-aware flows.
- The next boundary to isolate is how reconciliation feeds into future scheduling policy and priority preemption.

Target module boundaries:
- `gateway`: HTTP transport, request validation, response shaping.
- `workloads`: submit, queue, schedule, preempt, and lifecycle transitions.
- `fleet`: node health, capacity, disruption, and reconciliation.
- `reconciler`: simulated health changes and background recovery transitions.
- `scheduler`: pure placement and rebalance policy.
- `events`: append-only audit trail and event fanout.
- `store`: persistence adapters and transactional state loading/saving.
- Preemption contract lives across `workloads`, `fleet`, and `events`; checkpoint execution remains a workload-runtime concern outside the control plane.

Acceptance:
- New behaviors can land without adding more policy to the gateway or persistence layer.
- Scheduler remains a pure decision engine.
- Fleet/Workload orchestration is testable without HTTP.
- Postgres and memory adapters stay thin around shared orchestration logic.

Parallelism:
- Yes, once module seams are clear.

## Phase 6: E2E

Owner: test/infra coding agent.

Deliver:
- E2E test suite parameterized by `BASE_URL`.
- Local E2E target.
- Seed/reset support for stable tests.

Acceptance:
- `BASE_URL=http://localhost:8080 make e2e` passes.
- E2E covers submit, placement/queueing, disruption, events, and utilization.

## Phase 7: Deployment

Owner: infra coding agent with coordinator review.

Deliver:
- Render deployment config/docs.
- Deployed health check.
- Deployed app URL.
- Production env notes.

Acceptance:
- App deploys within budget.
- `BASE_URL=<deployed-url> make e2e` passes.
- Cold-start behavior documented if using free tier.

## Phase 8: Submission Polish

Owner: coordinator.

Deliver:
- `APPROACH.md`.
- README setup/test/deploy updates.
- `video.md` link placeholder or final link.
- Final verification run.

Acceptance:
- Reviewer has local run path, deployed URL, test commands, and architecture/tradeoff explanation.

## Agent Strategy

- Coordinator: owns architecture consistency, RFC decision logs, integration review, and final submission quality.
- Backend agents: implement isolated packages and tests.
- Frontend agents: implement UI sections after API contracts stabilize.
- Infra agents: implement Docker/deploy/test command wiring.
- Test agents: add integration and E2E tests without changing core policy.

Use smaller/faster agents for bounded tasks with clear ownership. Use strongest available model for planning, cross-cutting design, debugging complex integration failures, and final review.

## Parallelization Rules

- Do not parallelize before repo skeleton exists.
- Assign disjoint file/module ownership.
- Stabilize API contracts before frontend expands.
- Keep scheduler policy changes centralized.
- Avoid two agents editing the same file unless coordinator integrates sequentially.
- Every agent must leave tests passing for its scope.

## Stop Conditions

Pause for alignment if:

- A decision changes architecture boundaries.
- Budget or deployment assumptions change.
- A task requires adding external paid services.
- Tests require nondeterministic timing/background behavior.
- The implementation drifts from the RFC decision logs.

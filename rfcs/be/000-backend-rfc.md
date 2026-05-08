# Backend RFC

## Goal

Build a Go backend that models a simulated GPU fleet, accepts workload submissions, schedules deterministically, records explainable events, and handles admin-triggered disruptions.

## Non-Goals

- Real GPU provisioning.
- Kubernetes integration.
- Auth, billing, quotas, tenancy.
- Distributed scheduler.
- Production HA persistence.

## Domain Model

### Workload

- `id`
- `type`: training, inference, batch
- `gpu_type`: H100, A100, L4
- `gpu_count`
- `priority`: low, normal, high
- `duration_seconds`
- `spot_tolerant`
- `state`: pending, running, completed, failed, preempted
- `placement`
- `status_reason`
- `scheduling_explanation`
- timestamps

### Node

- `id`
- `gpu_type`
- `total_gpus`
- `allocated_gpus`
- `zone`
- `provider`
- `capacity_class`: on_demand, spot
- `health`: healthy, failed, recovering
- `running_workload_ids`

### Event

- `id`
- `timestamp`
- `type`
- `actor`
- `workload_id`
- `node_id`
- `message`
- `metadata`

## Scheduler Policy

- Run on submission, recovery, disruption, and explicit tick.
- Order pending work by priority, then submission time, then stable ID.
- Require healthy node, exact GPU type match, enough free GPUs, and spot compatibility.
- Training prefers on-demand.
- Batch prefers spot when spot tolerant.
- Inference prefers stable on-demand, then spot only if tolerated.
- Queue when no valid placement exists.
- Store concise queue reasons and explanations.

## API Surface

- `GET /health`
- `POST /workloads`
- `GET /workloads`
- `GET /workloads/{id}`
- `GET /nodes`
- `GET /fleet/summary`
- `GET /events`
- `POST /scheduler/tick`
- `POST /admin/nodes/{id}/fail`
- `POST /admin/nodes/{id}/recover`
- `POST /admin/nodes/{id}/preempt-spot`

## Disruption Handling

- Node failure marks node failed, frees allocations, disrupts running workloads, emits events, and reruns scheduler.
- Spot preemption applies to spot nodes, marks affected workloads preempted, requeues eligible work, emits events, and reruns scheduler.
- Recovery marks node healthy, emits event, and reruns scheduler.
- Preemption is a protocol, not just a node state change: emit `preempt_notice`, mark `drain_started_at`, and persist checkpoint metadata when a workload declares itself resumable. Stateless inference should drain and retry rather than checkpoint.

## State

- Start in-memory with deterministic seed data.
- Add Postgres when integration/E2E or deployed demo stability requires durable state.
- Keep scheduler independent of storage implementation.

## Testing

- Unit tests: resource fit, priority, FIFO tie-break, spot tolerance, workload preferences, queue reasons, completion, disruption recovery.
- API tests: workload submission, fleet summary, events, disruption endpoints, deterministic tick.
- E2E: submit workloads, observe placement/queueing, trigger disruption, verify events and utilization.

## Phases

- Phase 1: models, seeded fleet, health, workload APIs.
- Phase 2: scheduler, queue reasons, unit tests.
- Phase 3: events, tick, completion.
- Phase 4: disruptions and recovery.
- Phase 5: stable API contract for frontend and E2E.

## Open Questions

- Should high-priority work preempt running lower-priority work?
- Should preempted workloads remain `preempted` or move back to `pending` with history?
- Should checkpoint metadata live in workload state, event history, or both?
- Is in-memory state acceptable for deployed review?

## Technology Decisions

### Backend Runtime: Go

Decision: use Go for Gateway API and core backend modules.

Pros:
- Strong concurrency and low memory overhead.
- Good fit for scheduler, workload, fleet, gateway, and future rate limiting/auth.
- One backend language reduces tooling and operational complexity.
- Static binaries and containers deploy cleanly.

Cons:
- More boilerplate than FastAPI.
- Slightly slower initial setup.

Alternatives:
- Python/FastAPI: faster CRUD/API iteration, weaker long-term fit for concurrency-heavy control plane hot paths.
- Node/Express: viable gateway layer, weaker fit for CPU-heavy scheduler logic.
- Java/Kotlin: strong production backend, too heavy for this take-home.

NFR fit:
- Latency: Go is a better fit for low-latency control-plane APIs.
- Availability: simple binaries and predictable runtime behavior simplify deploy/restart.
- Operability: consistent runtime across gateway and services simplifies logs, metrics, and ownership.

### State: In-Memory First, Postgres When Durable

Decision: start in-memory with deterministic seed data.

Pros:
- Fastest path to scheduler correctness.
- No database migrations or deploy dependency.
- Tests are deterministic and isolated.

Cons:
- State resets on restart or deploy cold start.
- Less realistic than durable persistence.

Alternatives:
- SQLite: simple durability, but poor fit for concurrent writes, HA, and deployed multi-instance services.
- Managed Postgres: realistic and durable, but adds cost/configuration.
- Distributed SQL/NoSQL: better future HA options, too much for v1.

NFR fit:
- Availability: resettable demo state is acceptable if documented.
- Testability: in-memory is best for unit/integration speed.
- Deployment: Postgres is the target durable store for the demo/runtime path; CockroachDB is the production preference when multi-region durability matters.

### Scheduler Execution: Explicit Tick

Decision: use explicit scheduler ticks before background timers.

Pros:
- Deterministic tests.
- Clear demo control.
- Avoids race conditions.

Cons:
- Less realistic than autonomous scheduling.

NFR fit:
- Reliability and test determinism outweigh real-time automation for v1.

## Decision Log

- Use Go for Gateway API and core backend modules.
- Implement v1 as one Go process with clear internal modules: gateway, workloads, scheduler, fleet, events.
- Keep REST as the external API.
- Defer internal gRPC until services split.
- Use in-memory state only for the initial walking skeleton; prefer Postgres for the demo/runtime store and CockroachDB for production multi-region durability.
